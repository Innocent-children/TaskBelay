import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, readFile, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { createInterface } from "node:readline";
import test from "node:test";

const fakeCorePath = fileURLToPath(new URL("./fixtures/fake-core.mjs", import.meta.url));
const currentCoreVersion = (await readFile(new URL("../../../CORE_VERSION", import.meta.url), "utf8")).trim();
const currentProcessDigest = JSON.parse(
  await readFile(new URL("../../../protocol/fixtures/graph-server-info.json", import.meta.url), "utf8"),
).supported_processes[0].definition_digest;
const exactTools = [
  "taskbelay_server_info",
  "taskbelay_open_task",
  "taskbelay_get_task",
  "taskbelay_get_next_action",
  "taskbelay_submit_requirements",
  "taskbelay_submit_design",
  "taskbelay_submit_tasks",
  "taskbelay_submit_implementation",
  "taskbelay_submit_test",
  "taskbelay_submit_comprehension",
  "taskbelay_submit_refactor",
  "taskbelay_submit_delivery",
  "taskbelay_prepare_task_branch_rename",
  "taskbelay_prepare_task_relocation",
  "taskbelay_resolve_blocker",
  "taskbelay_recover_action",
  "taskbelay_cancel_task",
  "taskbelay_abandon_task",
];
const exactActionMembers = [
  "task_id", "revision", "action_id", "action_kind", "submission_tool", "process_id",
  "process_definition_digest", "current_node", "node_purpose", "entry_conditions",
  "completion_conditions", "allowed_effects", "required_evidence", "method_profile",
  "method_steps", "available_transitions", "payload_contract", "guidance",
  "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest",
  "issuance_content_digest", "issued_at",
].sort();

test("fake Core serves the current eighteen-tool catalog and complete structured results", async (t) => {
  const fixture = await makeFixture(t, "catalog");
  const client = await fixture.client();
  const tools = await client.listTools();
  assert.deepEqual(tools.map((tool) => tool.name), exactTools);
  for (const tool of tools) {
    assert.equal(tool.inputSchema.type, "object");
    assert.equal(tool.inputSchema.additionalProperties, false);
    assert.equal(tool.annotations.openWorldHint, false);
  }
  assert.deepEqual(tools[0].inputSchema.required, []);
  assert.deepEqual(tools[1].inputSchema.required, ["host", "repository_path"]);
  assert.equal(tools[1].inputSchema.properties.additional_repositories.maxItems, 7);
  assert.equal(tools[1].inputSchema.properties.additional_repositories.items.additionalProperties, false);
  assert.deepEqual(tools[1].inputSchema.properties.additional_repositories.items.required, ["key", "repository_path", "workspace_origin"]);
  assert.deepEqual(tools[4].inputSchema.required, [
    "host",
    "task_id",
    "action_id",
    "transition_id",
    "summary",
    "reason",
    "artifacts",
    "method_results",
    "node_result",
  ]);
  assert.deepEqual(tools.find((tool) => tool.name === "taskbelay_prepare_task_relocation").inputSchema.required, ["host", "task_id", "revision"]);
  assert.deepEqual(tools.find((tool) => tool.name === "taskbelay_abandon_task").inputSchema.required, ["host", "task_id", "revision", "reason"]);
  const resolveBlocker = tools.find((tool) => tool.name === "taskbelay_resolve_blocker").inputSchema;
  assert.deepEqual(resolveBlocker.properties.relocation_destinations.items.required, ["key", "repository_path"]);
  assert.deepEqual(resolveBlocker.properties.history_resolution.required, ["choice", "reason"]);
  const info = await client.callTool("taskbelay_server_info", {});
  assert.equal(info.result.version, currentCoreVersion);
  assert.equal(info.result.supported_processes[0].definition_digest, currentProcessDigest);
  assert.deepEqual(info.result.method_profiles, ["plain", "spec-kit", "openspec"]);
  assert.deepEqual(info.result.host_preferences, {
    codex: { codebase_memory: false },
    deepseek: { codebase_memory: true },
  });
  assert.deepEqual(info.result.tools, exactTools);
});

test("fake Core projects one multi-repository task, Action, revision, and digest", async (t) => {
  const fixture = await makeFixture(t, "multi-repository");
  const client = await fixture.client();
  const opened = await client.callTool("taskbelay_open_task", {
    ...openArguments(),
    repository_path: "/workspace/core",
    primary_repository_key: "core",
    workspace_origin: workspaceOrigin("core"),
    additional_repositories: [{ key: "docs", repository_path: "/workspace/docs", workspace_origin: workspaceOrigin("docs") }],
  });
  assert.equal(opened.result.task.primary_repository_key, "core");
  assert.equal(opened.result.task.workspace_origin.canonical_worktree_root, "/workspace/core");
  assert.deepEqual(opened.result.task.additional_repositories.map(({ key, workspace_origin }) => ({
    key, root: workspace_origin.canonical_worktree_root,
  })), [{ key: "docs", root: "/workspace/docs" }]);
  assert.equal(opened.result.task.revision, 1);
  const action = opened.result.task.current_action;
  assert.deepEqual(Object.keys(action).sort(), exactActionMembers);
  assert.equal(action.process_definition_digest, currentProcessDigest);
  assert.equal(action.payload_contract, "requirements-result");
  for (const member of ["repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest"]) {
    assert.match(action[member], /^[0-9a-f]{64}$/u, member);
  }
  assert.deepEqual(Object.keys(action.required_evidence[0]).sort(), ["kind", "required"]);
  assert.deepEqual(Object.keys(action.method_steps[0]).sort(), ["purpose", "required", "step_id"]);
  assert.deepEqual(Object.keys(action.available_transitions[0]).sort(), [
    "description", "destination_node", "guard_id", "reason_required", "selection_condition", "transition_id",
  ]);
  assert.equal(Object.hasOwn(action, "repository_scope_digest"), false);
  const next = await client.callTool("taskbelay_get_next_action", { host: "codex", task_id: action.task_id });
  assert.deepEqual(next.result.process, {
    process_id: "standard-development",
    process_definition_digest: currentProcessDigest,
  });

  const calls = await fixture.toolCalls();
  assert.deepEqual(calls.find((call) => call.name === "taskbelay_open_task").arguments.additional_repositories, [
    { key: "docs", repository_path: "/workspace/docs", workspace_origin: workspaceOrigin("docs") },
  ]);
});

test("driver creates and resumes one graph task while surfacing Core conflicts", async (t) => {
  const fixture = await makeFixture(t, "resume");
  const first = await fixture.client({ session: "session-create" });
  const opened = await first.callTool("taskbelay_open_task", openArguments());
  assert.equal(opened.result.created, true);
  assert.equal(opened.result.task.current_cursor, "REQUIREMENTS");
  assert.equal(opened.result.task.primary_repository_key, "primary");
  assert.deepEqual(opened.result.task.additional_repositories, []);
  const taskId = opened.result.task.task_id;
  await first.close();

  const resumedClient = await fixture.client({ session: "session-resume" });
  const resumed = await resumedClient.callTool("taskbelay_open_task", {
    host: "codex",
    repository_path: "/workspace/example",
    workspace_origin: workspaceOrigin("primary"),
  });
  assert.equal(resumed.result.created, false);
  assert.equal(resumed.result.task.task_id, taskId);
  await resumedClient.close();

  const conflictFixture = await makeFixture(t, "conflict");
  const conflictClient = await conflictFixture.client({ selectedCase: "conflict" });
  assert.equal((await conflictClient.callTool("taskbelay_open_task", openArguments())).error.code, "ACTIVE_TASK_CONFLICT");

  const hostFixture = await makeFixture(t, "host-conflict");
  const hostClient = await hostFixture.client({ selectedCase: "host-conflict" });
  assert.equal((await hostClient.callTool("taskbelay_open_task", openArguments())).error.code, "HOST_OWNERSHIP_CONFLICT");
});

test("driver forwards one closed graph action identity and continues from Core result", async (t) => {
  const fixture = await makeFixture(t, "success");
  const client = await fixture.client();
  const opened = await client.callTool("taskbelay_open_task", openArguments());
  const action = opened.result.task.current_action;
  const submitted = submissionArguments(action);
  const applied = await client.callTool(action.submission_tool, submitted);
  assert.equal(applied.result.task.current_cursor, "DESIGN");
  assert.deepEqual(applied.result.task.current_action.available_transitions.map((edge) => edge.transition_id), [
    "design_ready",
    "design_requires_requirements",
  ]);
  const calls = await fixture.toolCalls();
  assert.deepEqual(calls.find((call) => call.name === action.submission_tool).arguments, submitted);
});

test("lost and truncated graph mutations force exact reads before retry", async (t) => {
  for (const selectedCase of ["loss", "truncation"]) {
    await t.test(selectedCase, async () => {
      const fixture = await makeFixture(t, selectedCase);
      const mutating = await fixture.client({ selectedCase, session: "session-uncertain" });
      const opened = await mutating.callTool("taskbelay_open_task", openArguments());
      const action = opened.result.task.current_action;
      const submitted = submissionArguments(action);
      await assert.rejects(mutating.callTool(action.submission_tool, submitted), (error) => error.uncertain === true);
      await mutating.close();

      const recovering = await fixture.client({ session: "session-recovery" });
      const task = await recovering.callTool("taskbelay_get_task", { host: "codex", task_id: action.task_id });
      const next = await recovering.callTool("taskbelay_get_next_action", { host: "codex", task_id: action.task_id });
      assert.equal(task.result.recovery_assessment.classification, "completed_and_recorded");
      assert.equal(task.result.recovery_assessment.operation.source_cursor, "REQUIREMENTS");
      assert.equal(next.result.current_cursor, "DESIGN");
      const calls = await fixture.toolCalls();
      assert.equal(calls.filter((call) => call.name === action.submission_tool).length, 1);
    });
  }
});

test("driver preserves graph errors, blocker, cancellation, and terminal outcomes", async (t) => {
  for (const [selectedCase, code] of [["domain-error", "ACTION_STALE"], ["budget", "VERIFICATION_BUDGET_EXCEEDED"]]) {
    const fixture = await makeFixture(t, selectedCase);
    const client = await fixture.client({ selectedCase });
    const opened = await client.callTool("taskbelay_open_task", openArguments());
    const action = opened.result.task.current_action;
    const result = await client.callTool(action.submission_tool, submissionArguments(action));
    assert.equal(result.error.code, code);
  }

  const blockerFixture = await makeFixture(t, "blocker");
  const blockerClient = await blockerFixture.client({ selectedCase: "blocker" });
  const blockerOpen = await blockerClient.callTool("taskbelay_open_task", openArguments());
  const blockedAction = blockerOpen.result.task.current_action;
  const blocked = await blockerClient.callTool(blockedAction.submission_tool, submissionArguments(blockedAction));
  assert.equal(blocked.result.task.current_cursor, "BLOCKED");
  assert.equal(blocked.result.task.resume_cursor, "REQUIREMENTS");

  const terminalFixture = await makeFixture(t, "terminal");
  const terminalClient = await terminalFixture.client({ selectedCase: "terminal" });
  const terminalOpen = await terminalClient.callTool("taskbelay_open_task", openArguments());
  const terminalAction = terminalOpen.result.task.current_action;
  const terminal = await terminalClient.callTool(terminalAction.submission_tool, submissionArguments(terminalAction));
  assert.equal(terminal.result.task.current_cursor, "DONE");
  assert.equal(terminal.result.task.outcome.status, "completed");

  const cancelFixture = await makeFixture(t, "cancel");
  const cancelClient = await cancelFixture.client();
  const cancelled = await cancelClient.callTool("taskbelay_cancel_task", {
    request_id: "request-cancel",
    host: "codex",
    task_id: "task-graph-0001",
    revision: 1,
    reason: "User explicitly requested cancellation.",
  });
  assert.equal(cancelled.result.task.current_cursor, "CANCELLED");
  assert.equal(cancelled.result.task.outcome.status, "cancelled");
});

test("fake Core rejects unknown argument members before dispatch", async (t) => {
  const fixture = await makeFixture(t, "closed");
  const client = await fixture.client();
  await assert.rejects(
    client.callTool("taskbelay_server_info", { alias: "forbidden" }),
    (error) => error.rpcCode === -32602 && /unexpected field alias/.test(error.message),
  );
});

function openArguments() {
  return {
    host: "codex",
    repository_path: "/workspace/example",
    new_task: {
      request: "Define one bounded graph requirement.",
      initial_scope: ["one repository"],
      initial_out_of_scope: ["real host"],
      known_acceptance_criteria: [],
      method_profile: "plain",
    },
  };
}

function submissionArguments(action) {
  return {
    host: "codex",
    task_id: action.task_id,
    action_id: action.action_id,
    transition_id: "requirements_ready",
    summary: "Requirements are bounded and testable.",
    reason: "",
    artifacts: { current: [], other_process: [] },
    method_results: {
      "requirements.capture": { capability: "", summary: "Captured requirements." },
      "requirements.clarify": { capability: "", summary: "Resolved material questions." },
      "requirements.validate": { capability: "", summary: "Validated requirements." },
    },
    node_result: {
        problem_class: "none",
        baseline: {
          goal: "Define one bounded graph requirement.",
          scope: ["one repository"],
          out_of_scope: ["real host"],
          acceptance_criteria: ["The graph task reaches DESIGN."],
          constraints: [],
          assumptions: [],
        },
        unresolved_questions: [],
    },
  };
}

function workspaceOrigin(key) {
  return {
    mode: "dedicated_worktree",
    source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    base_commit: "5265db6c44ce12ea55d9fdb072b4dcb2345f6e2a",
    task_branch: `codex/${key}`,
    provisioning_receipt_id: `launch-0001:${key}`,
  };
}

async function makeFixture(t, name) {
  const root = await realpath(await mkdtemp(join(tmpdir(), `taskbelay-fake-core-${name}-`)));
  const statePath = join(root, "state.json");
  const tracePath = join(root, "trace.jsonl");
  const clients = [];
  t.after(async () => {
    await Promise.all(clients.map((client) => client.close()));
    await rm(root, { recursive: true, force: true });
  });
  return {
    async client({ selectedCase = "success", session = "session-1" } = {}) {
      const client = await FakeCoreClient.start({ statePath, tracePath, selectedCase, session });
      clients.push(client);
      return client;
    },
    async toolCalls() {
      return JSON.parse(await readFile(statePath, "utf8")).calls;
    },
  };
}

class FakeCoreClient {
  static async start(options) {
    const client = new FakeCoreClient(options);
    await client.initialize();
    return client;
  }

  constructor({ statePath, tracePath, selectedCase, session }) {
    this.nextId = 1;
    this.pending = new Map();
    this.stderr = "";
    this.closed = false;
    this.child = spawn(process.execPath, [fakeCorePath], {
      env: { ...process.env, FAKE_CORE_STATE: statePath, FAKE_CORE_TRACE: tracePath, FAKE_CORE_CASE: selectedCase, FAKE_CORE_SESSION: session },
      stdio: ["pipe", "pipe", "pipe"],
    });
    this.exitPromise = once(this.child, "exit").then(([code, signal]) => {
      const error = new Error(`fake Core exited before a complete result (${code ?? signal})${this.stderr ? `: ${this.stderr.trim()}` : ""}`);
      error.uncertain = true;
      for (const { reject, timer } of this.pending.values()) {
        clearTimeout(timer);
        reject(error);
      }
      this.pending.clear();
      return { code, signal };
    });
    this.child.stderr.setEncoding("utf8");
    this.child.stderr.on("data", (chunk) => { this.stderr += chunk; });
    createInterface({ input: this.child.stdout, crlfDelay: Infinity }).on("line", (line) => {
      const response = JSON.parse(line);
      const pending = this.pending.get(response.id);
      if (!pending) return;
      clearTimeout(pending.timer);
      this.pending.delete(response.id);
      pending.resolve(response);
    });
  }

  async initialize() {
    const response = await this.request("initialize", { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "fake-driver", version: "0.2.0" } });
    assert.equal(response.result.serverInfo.name, "taskbelay-fake-core");
    assert.equal(response.result.serverInfo.version, currentCoreVersion);
    this.notify("notifications/initialized", {});
  }

  async listTools() {
    return (await this.request("tools/list", {})).result.tools;
  }

  async callTool(name, arguments_) {
    const response = await this.request("tools/call", { name, arguments: arguments_ });
    if (response.error) {
      const error = new Error(response.error.message);
      error.rpcCode = response.error.code;
      throw error;
    }
    const result = response.result;
    let decoded;
    try {
      decoded = JSON.parse(result.content[0].text);
    } catch (cause) {
      const error = new Error(`fake Core returned an incomplete result for ${name}`, { cause });
      error.uncertain = true;
      throw error;
    }
    assert.deepEqual(decoded, result.structuredContent);
    return decoded;
  }

  request(method, params) {
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`fake Core request timed out: ${method}`));
      }, 5_000);
      this.pending.set(id, { resolve, reject, timer });
      this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
    });
  }

  notify(method, params) {
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`);
  }

  async close() {
    if (this.closed) return;
    this.closed = true;
    if (this.child.exitCode === null && this.child.signalCode === null) this.child.stdin.end();
    await this.exitPromise;
  }
}
