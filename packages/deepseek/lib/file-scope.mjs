import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { statSync } from "node:fs";
import { dirname, isAbsolute, resolve } from "node:path";


const SUPPORTED_TOOLS = new Set(["write", "edit", "str_replace_editor"]);
const MAX_OUTPUT_BYTES = 1024 * 1024;

export function registerFileScopeGate(ctx, {
  runtimePath,
  dataDirectory,
  workspaceRoot = process.cwd(),
  spawnImpl = spawn,
} = {}) {
  if (typeof runtimePath !== "string" || typeof dataDirectory !== "string" || typeof workspaceRoot !== "string") {
    throw new Error("file-scope gate paths are required");
  }
  return ctx.on("tools/pre-execute", async (execution, next) => {
    if (!SUPPORTED_TOOLS.has(execution?.name)) {
      return next();
    }
    if (execution.name === "str_replace_editor" && execution.arguments?.command === "view") {
      return next();
    }
    let result;
    try {
      const request = preparedWrite(execution, workspaceRoot);
      result = await runCoreCheck(request, { runtimePath, dataDirectory, spawnImpl });
    } catch {
      return { kind: "deny", reason: "TaskBelay file-scope check was unavailable; the write was stopped." };
    }
    if (result.decision === "allow") return next();
    if (result.decision === "deny") {
      return {
        kind: "deny",
        reason: typeof result.reason === "string" && result.reason.trim() !== ""
          ? result.reason
          : "TaskBelay stopped this write before execution.",
      };
    }
    return { kind: "deny", reason: "TaskBelay file-scope check returned an unknown decision; the write was stopped." };
  });
}

export function preparedWrite(execution, workspaceRoot) {
  const rawPath = execution?.name === "str_replace_editor"
    ? execution.arguments?.path
    : execution?.arguments?.file_path;
  const complete = typeof rawPath === "string" && rawPath.trim() !== "" && rawPath === rawPath.trim() && !rawPath.includes("\0");
  const absolute = complete
    ? resolve(workspaceRoot, rawPath)
    : resolve(workspaceRoot, ".taskbelay-unresolved-path");
  const normalizedArguments = stableJSON(execution?.arguments ?? {});
  return {
    host: "deepseek",
    repository_path: existingTargetDirectory(dirname(absolute)),
    tool_name: execution?.name ?? "",
    paths: complete ? [absolute] : [],
    intent_digest: createHash("sha256").update(`${execution?.name ?? ""}\0${normalizedArguments}`).digest("hex"),
    path_parse_complete: complete && isAbsolute(absolute),
  };
}

/**
 * New-file parents may not exist; Git must start in an existing directory of
 * the target repository, which may be below a multi-repository Workspace Root.
 */
function existingTargetDirectory(path) {
  for (;;) {
    try {
      if (!statSync(path).isDirectory()) throw new Error("write target parent is not a directory");
      return path;
    } catch (error) {
      const parent = dirname(path);
      if (error?.code !== "ENOENT" || parent === path) throw error;
      path = parent;
    }
  }
}

async function runCoreCheck(request, { runtimePath, dataDirectory, spawnImpl }) {
  const child = spawnImpl(runtimePath, ["host-check", "pre-file-write"], {
    cwd: dirname(runtimePath),
    env: { ...process.env, TASKBELAY_DATA_DIR: dataDirectory },
    stdio: ["pipe", "pipe", "pipe"],
    shell: false,
    windowsHide: true,
  });
  const stdout = [];
  const stderr = [];
  let outputBytes = 0;
  const collect = (target) => (chunk) => {
    outputBytes += chunk.length;
    if (outputBytes > MAX_OUTPUT_BYTES) child.kill("SIGKILL");
    else target.push(Buffer.from(chunk));
  };
  child.stdout.on("data", collect(stdout));
  child.stderr.on("data", collect(stderr));
  child.stdin.end(`${JSON.stringify(request)}\n`);
  const status = await new Promise((resolveStatus, reject) => {
    const timer = setTimeout(() => child.kill("SIGKILL"), 30_000);
    child.once("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.once("exit", (code, signal) => {
      clearTimeout(timer);
      resolveStatus({ code, signal });
    });
  });
  if (status.code !== 0 || status.signal !== null || outputBytes > MAX_OUTPUT_BYTES) {
    throw new Error(Buffer.concat(stderr).toString("utf8") || "Core host check failed");
  }
  return JSON.parse(Buffer.concat(stdout).toString("utf8"));
}

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value !== null && typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}
