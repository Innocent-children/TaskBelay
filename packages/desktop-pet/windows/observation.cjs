const { execFile } = require("node:child_process");
const { promisify } = require("node:util");
const run = promisify(execFile);
function origin(value) {
  return (
    typeof value === "string" &&
    /^http:\/\/127\.0\.0\.1:(\d{1,5})$/.test(value) &&
    +value.split(":").at(-1) > 0 &&
    +value.split(":").at(-1) <= 65535
  );
}
class Observer {
  constructor(request) {
    this.request = request;
    this.controller = null;
    this.url = null;
  }
  cancel() {
    this.controller?.abort();
    this.controller = null;
  }
  async connect() {
    const { stdout } = await run(
      this.request.corePath,
      ["webui", "status", "--json"],
      {
        windowsHide: true,
        encoding: "utf8",
        timeout: 3000,
        maxBuffer: 65536,
        env: { ...process.env, TASKBELAY_DATA_DIR: this.request.dataDirectory },
        signal: this.controller?.signal,
      },
    );
    const state = JSON.parse(stdout);
    if (
      !["ready", "read_only"].includes(state.readiness) ||
      !origin(state.url) ||
      state.core_identity !== this.request.coreIdentity ||
      state.data_root_digest !== this.request.dataRootDigest
    )
      throw new Error("Core identity or readiness changed");
    this.url = state.url;
    const live = await this.read("/api/system/status");
    if (
      !["ready", "read_only"].includes(live.readiness) ||
      live.core_identity !== state.core_identity ||
      live.data_root_digest !== state.data_root_digest ||
      live.url !== state.url
    )
      throw new Error("Service identity changed");
    return state;
  }
  async read(path) {
    if (
      !origin(this.url) ||
      !/^\/api\/(system\/status|tasks(?:[/?]|$))/.test(path)
    )
      throw new Error("Invalid local endpoint");
    const signal = this.controller
      ? AbortSignal.any([this.controller.signal, AbortSignal.timeout(3000)])
      : AbortSignal.timeout(3000);
    const response = await fetch(this.url + path, {
      signal,
      redirect: "error",
    });
    if (response.status === 404) return null;
    if (!response.ok) throw new Error("Service read failed");
    let size = 0;
    const chunks = [];
    for await (const chunk of response.body) {
      size += chunk.length;
      if (size > 2 * 1024 * 1024)
        throw new Error("Service response is too large");
      chunks.push(chunk);
    }
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  }
  async list(page = 1, lifecycle = "") {
    const value = await this.read(
      `/api/tasks?page=${page}${lifecycle ? "&lifecycle=" + lifecycle : ""}`,
    );
    if (
      !value ||
      !Array.isArray(value.items) ||
      typeof value.has_next !== "boolean"
    )
      throw new Error("Invalid task list");
    return value;
  }
  async detail(id) {
    const value = await this.read("/api/tasks/" + encodeURIComponent(id));
    if (
      value &&
      (!value.summary ||
        value.summary.task_id !== id ||
        typeof value.summary.status !== "string" ||
        typeof value.summary.lifecycle !== "string")
    )
      throw new Error("Invalid task detail");
    return value;
  }
}
function presentation(
  summary,
  previous,
  connected = true,
  selected = true,
  continuous = true,
) {
  if (!connected)
    return {
      phase: "disconnected",
      clip: "disconnected",
      prompt: previous?.phase !== "disconnected",
      rest: false,
      summary: previous?.summary ?? null,
      stale: true,
    };
  if (!summary)
    return {
      phase: selected ? "missing" : "idle",
      clip: "idle",
      prompt: false,
      rest: selected,
      summary: null,
      stale: false,
    };
  const observed =
    continuous &&
    previous?.summary?.task_id === summary.task_id &&
    !previous.stale;
  let phase = summary.archived ? "archived" : summary.lifecycle;
  const clip =
    phase === "done"
      ? "complete"
      : phase === "blocked"
        ? "blocked"
        : phase === "active" || !["archived", "cancelled"].includes(phase)
          ? "working"
          : "idle";
  const prompt =
    observed &&
    ((phase === "done" &&
      ["active", "blocked"].includes(previous.summary.lifecycle)) ||
      (phase === "blocked" &&
        (previous.summary.lifecycle !== "blocked" ||
          previous.summary.blocker !== summary.blocker)));
  return {
    phase,
    clip,
    prompt: Boolean(prompt),
    rest: ["done", "cancelled", "archived"].includes(phase) && !prompt,
    summary,
    stale: false,
  };
}
module.exports = { Observer, presentation, origin };
