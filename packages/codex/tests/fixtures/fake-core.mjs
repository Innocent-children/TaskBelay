#!/usr/bin/env node

import { appendFile, mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { dirname, isAbsolute, resolve } from "node:path";
import { createInterface } from "node:readline";

const statePath = requiredIsolatedPath("FAKE_CORE_STATE");
const tracePath = requiredIsolatedPath("FAKE_CORE_TRACE");
const selectedCase = process.env.FAKE_CORE_CASE ?? "success";
const session = process.env.FAKE_CORE_SESSION ?? "session-1";
const coreVersion = (await readFile(new URL("../../../../CORE_VERSION", import.meta.url), "utf8")).trim();
const processDefinitionDigest = JSON.parse(await readFile(new URL("../../../../protocol/fixtures/graph-server-info.json", import.meta.url), "utf8")).supported_processes[0].definition_digest;
const state = await readState();
const tools = toolDefinitions();
const toolByName = new Map(tools.map((tool) => [tool.name, tool]));

const input = createInterface({ input: process.stdin, crlfDelay: Infinity });
for await (const line of input) {
  if (line.trim() === "") continue;
  let request;
  try {
    request = JSON.parse(line);
  } catch {
    writeMessage({ jsonrpc: "2.0", id: null, error: { code: -32700, message: "Parse error" } });
    continue;
  }
  await handleMessage(request);
}

async function handleMessage(request) {
  if (request?.jsonrpc !== "2.0" || typeof request.method !== "string") {
    writeMessage({ jsonrpc: "2.0", id: request?.id ?? null, error: { code: -32600, message: "Invalid Request" } });
    return;
  }
  await appendTrace({ method: request.method, id: request.id ?? null, params: request.params ?? null });
  if (!("id" in request)) return;
  if (request.method === "initialize") {
    writeMessage({
      jsonrpc: "2.0",
      id: request.id,
      result: {
        protocolVersion: request.params?.protocolVersion ?? "2025-06-18",
        capabilities: { tools: { listChanged: false } },
        serverInfo: { name: "taskbelay-fake-core", version: coreVersion },
        instructions: "Test-only Core current Core contract fixture server.",
      },
    });
    return;
  }
  if (request.method === "tools/list") {
    writeMessage({ jsonrpc: "2.0", id: request.id, result: { tools } });
    return;
  }
  if (request.method !== "tools/call") {
    writeMessage({ jsonrpc: "2.0", id: request.id, error: { code: -32601, message: "Method not found" } });
    return;
  }
  const params = request.params;
  if (!isPlainObject(params) || !hasExactKeys(params, ["name", "arguments"]) || !toolByName.has(params.name)) {
    writeMessage({ jsonrpc: "2.0", id: request.id, error: { code: -32602, message: "Invalid tool call" } });
    return;
  }
  const validationError = validateTopLevelArguments(params.arguments, toolByName.get(params.name).inputSchema);
  if (validationError) {
    writeMessage({ jsonrpc: "2.0", id: request.id, error: { code: -32602, message: validationError } });
    return;
  }
  state.calls.push({ number: state.calls.length + 1, session, name: params.name, arguments: structuredClone(params.arguments) });
  await writeState();
  if (params.name.startsWith("taskbelay_submit_")) {
    await handleApply(request.id, params.arguments);
    return;
  }
  writeToolResult(request.id, await envelopeFor(params.name, params.arguments));
}

async function handleApply(id, arguments_) {
  state.operationId = `operation-${state.calls.length}`;
  state.actionId = arguments_.action_id;
  if (!["domain-error", "budget"].includes(selectedCase)) state.applyCount += 1;
  if (selectedCase === "loss") {
    state.responseLost = true;
    await writeState();
    process.exit(75);
  }
  if (selectedCase === "truncation") {
    state.responseLost = true;
    await writeState();
    writeMessage({
      jsonrpc: "2.0",
      id,
      result: { content: [{ type: "text", text: '{"ok":true' }], isError: false },
    });
    return;
  }
  await writeState();
  writeToolResult(id, await envelopeFor("taskbelay_submit_requirements", arguments_));
}

async function envelopeFor(tool, arguments_) {
  if (tool === "taskbelay_server_info") {
    return success(tool, {
      product: "taskbelay",
      version: coreVersion,
      transport: "stdio",
      health: "ready",
      supported_hosts: ["codex", "deepseek"],
      supported_processes: [{ process_id: "standard-development", definition_digest: processDefinitionDigest, new_task_supported: true }],
      method_profiles: ["plain", "spec-kit", "openspec"],
      host_preferences: {
        codex: { codebase_memory: false },
        deepseek: { codebase_memory: true },
      },
      tools: tools.map((toolDefinition) => toolDefinition.name),
    });
  }
  if (tool === "taskbelay_open_task") {
    if (selectedCase === "conflict") return failure(tool, "ACTIVE_TASK_CONFLICT");
    if (selectedCase === "host-conflict") return failure(tool, "HOST_OWNERSHIP_CONFLICT");
    const created = state.opened !== true;
    if (created && arguments_.new_task !== null && arguments_.new_task !== undefined) {
      state.scope = {
        primary_repository_key: arguments_.primary_repository_key ?? "primary",
        repository_path: arguments_.repository_path,
        workspace_origin: arguments_.workspace_origin,
        additional_repositories: [...(arguments_.additional_repositories ?? [])]
          .sort((left, right) => left.key.localeCompare(right.key)),
      };
    }
    state.opened = true;
    await writeState();
    return success(tool, { created, task: currentTask(), recovery_assessment: null });
  }
  if (tool === "taskbelay_get_task") {
    return success(tool, { task: currentTask(), recovery_assessment: recoveryAssessment() });
  }
  if (tool === "taskbelay_get_next_action") {
    const task = currentTask();
    return success(tool, {
      task_id: task.task_id,
      process: processReference(),
      current_cursor: task.current_cursor,
      revision: task.revision,
      method_profile: task.intent.method_profile,
      blocker: task.blocker,
      action: task.current_action,
      outcome: task.outcome,
      recovery_assessment: recoveryAssessment(),
    });
  }
  if (tool === "taskbelay_cancel_task") {
    state.cancelled = true;
    await writeState();
    return success(tool, { task: currentTask(), claim_released: true });
  }
  if (tool === "taskbelay_prepare_task_branch_rename") { state.blocked=true; await writeState(); return success(tool,{task:currentTask(),rename_id:"rename-0001"}); }
  if (tool === "taskbelay_prepare_task_relocation") {
    state.blocked = true;
    await writeState();
    return success(tool, { task: currentTask(), relocation_id: "relocation-0001" });
  }
  if (tool === "taskbelay_abandon_task") {
    state.cancelled = true;
    await writeState();
    return success(tool, { task: currentTask(), claim_released: true });
  }
  if (tool.startsWith("taskbelay_submit_")) {
    if (selectedCase === "domain-error") return failure(tool, "ACTION_STALE", state.operationId);
    if (selectedCase === "budget") return failure(tool, "VERIFICATION_BUDGET_EXCEEDED", state.operationId);
    if (selectedCase === "blocker") state.blocked = true;
    if (selectedCase === "terminal") state.done = true;
    await writeState();
    return success(tool, { task: currentTask(), recovery_assessment: null }, state.operationId);
  }
  if (tool === "taskbelay_recover_action" || tool === "taskbelay_resolve_blocker") {
    return success(tool, { task: currentTask(), recovery_assessment: recoveryAssessment() }, state.operationId);
  }
  throw new Error(`unsupported tool ${tool}`);
}

function currentTask() {
  const terminal = state.done || state.cancelled;
  const currentCursor = state.cancelled ? "CANCELLED" : state.done ? "DONE" : state.blocked ? "BLOCKED" : state.applyCount > 0 ? "DESIGN" : "REQUIREMENTS";
  const revision = state.cancelled || state.done || state.blocked ? 2 : state.applyCount > 0 ? 2 : 1;
  return {
    task_id: "task-graph-0001",
    origin_host: "codex",
    process_id: "standard-development",
    process_definition_digest: processDefinitionDigest,
    intent: {
      request: "Define one bounded graph requirement.",
      initial_scope: ["one repository"],
      initial_out_of_scope: ["real host"],
      known_acceptance_criteria: [],
      method_profile: "plain",
    },
    verification: { plan: null, current_budget: null, usage: { automatic_commands: 0, full_suite_runs: 0, evidence_items: 0 }, adjustments: [] },
    current_cursor: currentCursor,
    resume_cursor: state.blocked ? "REQUIREMENTS" : null,
    primary_repository_key: state.scope?.primary_repository_key ?? "primary",
    workspace_origin: persistedWorkspaceOrigin(
      state.scope?.workspace_origin ?? workspaceOriginSelection("primary"),
      state.scope?.repository_path ?? "/workspace/example",
      "primary",
    ),
    repository: repositoryProjection(state.scope?.repository_path ?? "/workspace/example", "primary"),
    additional_repositories: (state.scope?.additional_repositories ?? []).map((entry) => ({
      key: entry.key,
      workspace_origin: persistedWorkspaceOrigin(entry.workspace_origin, entry.repository_path, entry.key),
      repository: repositoryProjection(entry.repository_path, entry.key),
    })),
    baselines: { requirements: state.applyCount > 0 ? { revision: 1, digest: digest() } : null, design: null, task_plan: null, history: [] },
    implementation: null,
    test: null,
    comprehension: null,
    current_action: terminal ? null : state.blocked ? blockerAction(revision) : state.applyCount > 0 ? designAction(revision) : requirementsAction(revision),
    blocker: state.blocked ? { blocker_id: "blocker-graph-0001", code: "TASK_BLOCKED", resume_node: "REQUIREMENTS" } : null,
    last_operation: state.operationId ? { operation_id: state.operationId } : null,
    evidence: [],
    current_changed_paths: [],
    relocation: null,
    outcome: state.cancelled ? { status: "cancelled", summary: "Cancelled by the user." } : state.done ? { status: "completed", summary: "Completed by the fixture." } : null,
    revision,
  };
}

function repositoryProjection(canonicalRoot, key) {
  return {
    worktree_instance_digest: digest(),
    identity_digest: digest(),
    history_digest: digest(),
    content_digest: digest(),
    current_branch: `codex/${key}`,
    detached: false,
    current_head: objectID(),
    head_tree: objectID(),
    history_relation: "exact",
    base_commit_ancestor: true,
    changed_entries: [],
    task_surface: [],
    observed_at: "2026-08-23T00:00:00Z",
    binding_digest: digest(),
  };
}

function workspaceOriginSelection(key) {
  return {
    mode: "dedicated_worktree",
    source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    base_commit: objectID(),
    task_branch: `codex/${key}`,
    provisioning_receipt_id: `fixture-receipt-${key}`,
  };
}

function persistedWorkspaceOrigin(selection, canonicalRoot, key) {
  return {
    ...selection,
    source_repository_group_digest: digest(),
    canonical_worktree_root: canonicalRoot,
    worktree_git_dir_digest: digest(),
    provisioning_receipt_id: selection?.provisioning_receipt_id ?? `fixture-receipt-${key}`,
  };
}

function requirementsAction(revision) {
  return action(revision, "COMPLETE_REQUIREMENTS", "REQUIREMENTS", "requirements-result", [
    transition("requirements_ready", "DESIGN", false),
  ]);
}

function designAction(revision) {
  return action(revision, "COMPLETE_DESIGN", "DESIGN", "design-result", [
    transition("design_ready", "TASKS", false),
    transition("design_requires_requirements", "REQUIREMENTS", true),
  ]);
}

function blockerAction(revision) {
  return action(revision, "RESOLVE_BLOCKER", "BLOCKED", "blocker-resolution", []);
}

function action(revision, kind, node, payloadContract, availableTransitions) {
  return {
    action_id: `action-${node.toLowerCase()}-${revision}`,
    action_kind: kind,
    submission_tool: submissionTool(kind),
    task_id: "task-graph-0001",
    revision,
    process_id: "standard-development",
    process_definition_digest: processDefinitionDigest,
    current_node: node,
    node_purpose: `Complete ${node}.`,
    entry_conditions: ["Current Task authority is available."],
    completion_conditions: ["Current node facts are complete."],
    allowed_effects: ["read_repository"],
    required_evidence: [{ kind: "repository_observation", required: true }],
    repository_binding_digest: digest(),
    issuance_identity_digest: digest(),
    issuance_history_digest: digest(),
    issuance_content_digest: digest(),
    payload_contract: payloadContract,
    available_transitions: availableTransitions,
    method_profile: "plain",
    method_steps: [{ step_id: `${node.toLowerCase()}.fixture`, purpose: "Complete fixture work.", required: true }],
    guidance: "Follow the current fixture Action.",
    issued_at: "2026-08-23T00:00:00Z",
  };
}

function submissionTool(kind) {
  return {
    COMPLETE_REQUIREMENTS: "taskbelay_submit_requirements",
    COMPLETE_DESIGN: "taskbelay_submit_design",
    RESOLVE_BLOCKER: "taskbelay_resolve_blocker",
  }[kind];
}

function transition(transitionId, destination, reasonRequired) {
  return { transition_id: transitionId, destination_node: destination, guard_id: `${transitionId}_guard`, description: "Fixture transition.", selection_condition: "Fixture guard is satisfied.", reason_required: reasonRequired };
}

function recoveryAssessment() {
  if (!state.responseLost) return null;
  return {
    classification: "completed_and_recorded",
    operation: {
      operation_id: state.operationId,
      process_id: "standard-development",
      process_definition_digest: processDefinitionDigest,
      source_cursor: "REQUIREMENTS",
      expected_revision: 1,
      action_id: state.actionId,
      action_kind: "COMPLETE_REQUIREMENTS",
    },
    task_revision: 2,
    action_retry_safe: false,
    next_advice: "Read the current action.",
  };
}

function processReference() {
  return { process_id: "standard-development", process_definition_digest: processDefinitionDigest };
}

function success(tool, result, requestId = null) {
  return { ok: true, tool, request_id: requestId, result };
}

function failure(tool, code, requestId = null) {
  return {
    ok: false,
    tool,
    request_id: requestId,
    error: { code, message: code },
    recovery: { retry_safe: false, action: "read_task", message: "Read Core authority." },
  };
}

function writeToolResult(id, envelope) {
  writeMessage({
    jsonrpc: "2.0",
    id,
    result: {
      content: [{ type: "text", text: JSON.stringify(envelope) }],
      structuredContent: envelope,
      isError: envelope.ok !== true,
    },
  });
}

function toolDefinitions() {
	const submissionRequired = ["host", "task_id", "action_id", "transition_id", "summary", "reason", "artifacts", "method_results", "node_result"];
	const metadata = [
    ["taskbelay_server_info", [], [] , true, false, true],
    ["taskbelay_open_task", ["host", "repository_path"], ["host", "repository_path", "primary_repository_key", "additional_repositories", "new_task"], false, false, false],
    ["taskbelay_get_task", ["host", "task_id"], ["host", "task_id", "operation_probe", "baseline_history"], true, false, true],
    ["taskbelay_get_next_action", ["host", "task_id"], ["host", "task_id", "operation_probe"], true, false, true],
    ["taskbelay_submit_requirements", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_design", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_tasks", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_implementation", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_test", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_comprehension", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_refactor", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_submit_delivery", submissionRequired, submissionRequired, false, false, true],
    ["taskbelay_prepare_task_branch_rename", ["host","task_id","revision","repository_key","target_branch","reason"], ["host","task_id","revision","repository_key","target_branch","reason"], false,false,true],
    ["taskbelay_prepare_task_relocation", ["host", "task_id", "revision"], ["host", "task_id", "revision"], false, false, true],
    ["taskbelay_resolve_blocker", ["host", "task_id", "action_id"], ["host", "task_id", "action_id", "choice", "reason", "rename_id", "rename_choice", "relocation_id", "relocation_destinations", "history_resolution"], false, false, true],
    ["taskbelay_recover_action", ["host", "task_id", "action_id"], ["host", "task_id", "action_id"], false, false, true],
    ["taskbelay_cancel_task", ["request_id", "host", "task_id", "revision", "reason"], ["request_id", "host", "task_id", "revision", "reason"], false, true, false],
    ["taskbelay_abandon_task", ["host", "task_id", "revision", "reason"], ["host", "task_id", "revision", "reason"], false, true, false],
  ];
  return metadata.map(([name, required, properties, readOnlyHint, destructiveHint, idempotentHint]) => {
    const inputSchema = closedSchema(required, properties);
    if (name === "taskbelay_open_task") {
      inputSchema.properties.workspace_origin = workspaceOriginSchema();
      inputSchema.properties.primary_repository_key = {
        type: "string",
        pattern: "^[a-z0-9][a-z0-9._-]{0,127}$",
      };
      inputSchema.properties.additional_repositories = {
        type: "array",
        maxItems: 7,
        items: {
          type: "object",
          additionalProperties: false,
          required: ["key", "repository_path", "workspace_origin"],
          properties: { key: { type: "string" }, repository_path: { type: "string" }, workspace_origin: workspaceOriginSchema() },
        },
      };
    }
    if (name === "taskbelay_resolve_blocker") {
      inputSchema.properties.relocation_destinations = {
        type: "array",
        items: {
          type: "object",
          additionalProperties: false,
          required: ["key", "repository_path"],
          properties: { key: { type: "string" }, repository_path: { type: "string" } },
        },
      };
      inputSchema.properties.history_resolution = {
        type: "object",
        additionalProperties: false,
        required: ["choice", "reason"],
        properties: { choice: { const: "accept_current_history" }, reason: { type: "string" } },
      };
    }
    return {
      name,
      title: name,
      description: `${name} current Core fixture definition.`,
      inputSchema,
      annotations: { title: name, readOnlyHint, destructiveHint, idempotentHint, openWorldHint: false },
    };
  });
}

function workspaceOriginSchema() {
  return {
    type: "object",
    additionalProperties: false,
    required: ["mode", "source_type", "carry_changes", "remote_name", "base_branch", "base_commit", "task_branch", "provisioning_receipt_id"],
    properties: {
      mode: { enum: ["new_branch", "current_branch", "dedicated_worktree"] },
      source_type: { enum: ["local", "remote"] },
      carry_changes: { type: "boolean" },
      remote_name: { type: "string" },
      base_branch: { type: "string" },
      base_commit: { type: "string" },
      task_branch: { type: "string" },
      provisioning_receipt_id: { type: "string" },
    },
  };
}

function closedSchema(required, properties) {
  return { type: "object", additionalProperties: false, required, properties: Object.fromEntries(properties.map((name) => [name, {}])) };
}

function validateTopLevelArguments(arguments_, schema) {
  if (!isPlainObject(arguments_)) return "tool arguments must be an object";
  for (const required of schema.required) {
    if (!Object.hasOwn(arguments_, required)) return `tool arguments missing ${required}`;
  }
  const allowed = new Set(Object.keys(schema.properties));
  for (const key of Object.keys(arguments_)) {
    if (!allowed.has(key)) return `tool arguments contain unexpected field ${key}`;
  }
  return null;
}

async function readState() {
  try {
    return JSON.parse(await readFile(statePath, "utf8"));
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
    return { calls: [], opened: false, scope: null, applyCount: 0, responseLost: false, blocked: false, done: false, cancelled: false, operationId: null, actionId: null };
  }
}

async function writeState() {
  await mkdir(dirname(statePath), { recursive: true });
  const temporaryPath = `${statePath}.tmp-${process.pid}`;
  await writeFile(temporaryPath, `${JSON.stringify(state)}\n`, { mode: 0o600 });
  await rename(temporaryPath, statePath);
}

async function appendTrace(value) {
  await mkdir(dirname(tracePath), { recursive: true });
  await appendFile(tracePath, `${JSON.stringify(value)}\n`, { mode: 0o600 });
}

function requiredIsolatedPath(name) {
  const value = process.env[name];
  if (!value || !isAbsolute(value)) throw new Error(`${name} must be an absolute path`);
  return resolve(value);
}

function writeMessage(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

function hasExactKeys(value, keys) {
  return Object.keys(value).length === keys.length && keys.every((key) => Object.hasOwn(value, key));
}

function isPlainObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function digest() {
  return "5265db6c44ce12ea55d9fdb072b4dcb2345f6e2a1e89b016644c2819e320f2c1";
}

function objectID() {
  return "5265db6c44ce12ea55d9fdb072b4dcb2345f6e2a";
}
