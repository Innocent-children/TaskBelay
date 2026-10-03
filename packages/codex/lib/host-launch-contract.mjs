// Host launch discovery describes the existing coordinator operations. The coordinator owns execution.
const text = (description) => ({ type: "string", minLength: 1, description });
const path = (description) => text(`Normalized absolute path. ${description}`);
const nullablePath = (description) => ({ anyOf: [path(description), { type: "null" }], description });
const array = (items, description) => ({ type: "array", items, description });
const object = (properties, optional = []) => ({
  type: "object", additionalProperties: false, properties,
  required: Object.keys(properties).filter((key) => !optional.includes(key)),
});
const key = { ...text("Stable repository key confirmed by the user; keep the same key throughout this launch."), pattern: "^[a-z0-9][a-z0-9._-]{0,127}$" };
const identity = {
  launch_id: text("Copy receipt.launch_id from prepare; reuse it for every repository of the same Task."),
  repository_key: key,
};
const source = path("Original source checkout, used to verify the saved repository identity.");
const receiptOutput = {
  receipt_path: "Absolute path of the retained Host provisioning record.",
  receipt: "Retained launch identity and operation_status. Read phase before choosing the next operation.",
};
const anchor = object({
  request_digest: text("Copy unchanged from host-launch inspect."),
  repositories: array(object({
    repository_key: key,
    canonical_root: path("Source root returned by inspect."),
    head: text("Source HEAD returned by inspect."),
    status_digest: text("Source status digest returned by inspect."),
    dirty_paths: array({ type: "string" }, "Bounded source dirty paths returned by inspect."),
    dirty_paths_truncated: { type: "boolean" },
  }), "Copy the complete repositories array from inspect, including all confirmed repositories."),
});
const assessment = object({
 change_level: { enum: ["small", "standard", "large", "uncertain"] },
 observed_repositories: array(text("Canonical assessed root.")),
 candidate_components: array(text("Affected responsibility.")), candidate_paths: array(text("Candidate file.")),
 public_contract_flags: array(text("Public contract impact.")), persistence_or_state_flags: array(text("State impact.")),
 host_or_platform_flags: array(text("Host or platform impact.")), verification_shape: array(text("Planned check.")),
 unknowns: array(text("Unresolved question.")), recommendation: { enum: ["direct", "taskbelay", "clarify"] },
 reasons: array(text("Reason shown to the user.")), anchor,
});
const userChoice = object({source: {const:"user"}, mode: {const:"taskbelay"}, summary: text("Actual user choice after the assessment was displayed; reuse a still-valid answer.")});
const hostResult = { description: "Complete original Host tool result, including its wrapper if present. Use null when the result was lost; never fabricate an ID." };
const cleanup = object({ core_task: {type:"object",description:"Complete actual terminal Task from Core; its effective repository binding selects the branch. Never supply receipt.target_branch as a substitute."}, ...identity, source_repository_path: source, terminal: { const: true }, authorized: { const: true } });

const contracts = {
  inspect: {
    description: "Read source repositories and return the assessment anchor without writing Git or a receipt.",
    input_schema: object({ request: text("Exact current request; retain the same string for prepare."), repositories: { ...array(object({ key, repository_path: source }), "All proposed repositories."), minItems: 1, maxItems: 8 } }),
    output_fields: { request_digest: "Request identity.", repositories: "Canonical roots, HEADs, status digests and bounded dirty paths. Pass this entire result as prepare.assessment.anchor." },
    next_step: "Complete assessment and obtain the user's execution and workspace choices before prepare.",
  },
  prepare: {
    description: "After user confirmation, save the launch material and receipt, resolve the selected local or remote base and freeze its commit and selected workspace contents.",
    input_schema: object({
      request: text("Same exact request supplied to inspect."), assessment, user_choice: userChoice,
      ...identity, repository_path: source,
      workspace_mode: { enum: ["new_branch", "current_branch", "dedicated_worktree"], default: "new_branch", description: "Default to a new branch in the current directory. Other modes follow the user's explicit choice." },
      source_type: { enum: ["local", "remote"] }, carry_changes: { type: "boolean", description: "Explicit user choice; only true for local sources." },
      remote_name: { type: "string", description: "Confirmed remote, or empty for local." }, base_branch: text("Confirmed branch name on the selected source."),
      target_branch: text("Confirmed new branch, or the observed current branch for current_branch."),
      surface: { enum: ["current_session", "managed_worktree", "cli_worktree"] },
      worktree_path: nullablePath("The current canonical repository_path for current_session; null for managed_worktree; destination for cli_worktree."),
      handoff_file: nullablePath("Null for current_session. Dedicated worktrees require complete JSON material from references/task-handoff.md."),
    }, ["launch_id"]),
    output_fields: { ...receiptOutput, resumed: "Whether a retained preparation was found.", fetch_performed: "Whether this invocation fetched the base." },
    next_step: "Retain receipt.launch_id before preparing another repository. Phase=prepared proceeds to local-provision for current_session, dispatch-start for managed_worktree, or cli-provision. Inspect status after uncertainty.",
  },
  "local-provision": {
    description: "Check Core directory availability, then create the selected local branch or retain the current branch without changing directories.",
    input_schema: object(identity),
    output_fields: { ...receiptOutput, workspace_origin: "Verified local origin. Retain local files, index and ignored content." },
    next_step: "After every selected root is provisioned and authorized, call scope and open one Core Task in the current session. A provisioned retry reads saved data; Core checks the actual workspace. Preserve uncertain branch operations for inspection.",
  },
  status: {
    description: "Read one exact launch/repository record without repeating the operation.",
    input_schema: object(identity), output_fields: { ...receiptOutput, receipt: "Saved record, or null when this identity has no record." },
    next_step: "Use the retained phase and original Host operation/thread marker. An uncertain result never authorizes another dispatch.",
  },
  "dispatch-start": {
    description: "Persist the complete managed-worktree request in dispatch_prepared. Repeated calls return the same saved request.",
    input_schema: object({ ...identity, project_id: text("Codex saved-project ID returned by its project listing.") }),
    output_fields: { ...receiptOutput, should_dispatch: "Always false; dispatch-call grants the single Host call.", host_request: "Exact persisted Codex create_thread arguments; reading them never authorizes dispatch." },
    next_step: "Save stdout to a file and parse that file. Call dispatch-call with the retained dispatch_attempt_id before invoking the Host.",
  },
  "dispatch-call": {
    description: "Atomically claim one Host creation call after the saved request has been read successfully.",
    input_schema: object({ ...identity, dispatch_attempt_id: text("Current receipt.operation_status.dispatch_attempt_id.") }),
    output_fields: { ...receiptOutput, should_dispatch: "True only for the first claim; call the Host once in this execution.", host_request: "Complete retained request." },
    next_step: "Save and parse complete stdout before calling create_thread unchanged. Record its complete response with dispatch-result. If interrupted before the call, use dispatch-recover; if the result is unknown, inspect the Host by saved title and initial prompt, then use dispatch-reconcile.",
  },
  "dispatch-recover": {
    description: "Restore a dispatch that provably never called the Host; empty IDs alone are insufficient. Retains the exact request and rotates the claim ID.",
    input_schema: object({ ...identity, dispatch_attempt_id: text("Current claim ID."), host_call_not_made: { const: true }, previous_caller_stopped: { const: true }, reason: text("Observed call sequence proving no create_thread call and that the previous caller has stopped.") }),
    output_fields: { ...receiptOutput, should_dispatch: "False; use dispatch-call with the new claim ID.", host_request: "Unchanged saved request." },
    next_step: "Read the saved request, then dispatch-call. Never use recovery for a timeout or missing Host response.",
  },
  "dispatch-reconcile": {
    description: "Match actual Host task initial prompts to the retained request after searching by launch/repository title. An empty result never authorizes creation.",
    input_schema: object({ ...identity, candidates: array(object({ thread_id: text("Actual threadId from Host inspection, never clientThreadId."), initial_prompt: text("Complete initial user prompt read from that Host task.") }), "Inspected Host candidates. Include all matches; do not infer absence from a bounded task list.") }),
    output_fields: { ...receiptOutput, matched: "Whether exactly one task matched the complete saved prompt.", should_dispatch: "Always false." },
    next_step: "Continue the matched task. If no match or Host inspection is unavailable, retain uncertainty and inspect again later; never create another task.",
  },
  "dispatch-result": {
    description: "Record the complete Codex creation result, including structuredContent/result or a single JSON text content block. Save clientThreadId as queued and threadId as dispatched; tool errors, missing or malformed results record uncertainty.",
    input_schema: object({ ...identity, host_result: hostResult }), output_fields: { ...receiptOutput, changed: "Whether the record changed." },
    next_step: "Queued/clientThreadId results require Host inspection, never another create_thread. The destination session consumes the receipt with bootstrap.",
  },
  bootstrap: {
    description: "Verify the destination managed worktree and create the confirmed target branch.",
    input_schema: object({ ...identity, worktree_path: path("Actual destination worktree discovered from the Host.") }),
    output_fields: { ...receiptOutput, workspace_origin: "Verified origin for this repository; use scope after all repositories are provisioned." },
    next_step: "After every confirmed repository is provisioned, call scope once with the complete repository key set before opening the Core Task.",
  },
  "cli-provision": {
    description: "Create the confirmed CLI worktree and return the relaunch argv using retained requirements.",
    input_schema: object({ ...identity, source_repository_path: source, additional_worktree_paths: { ...array(path("Verified additional Task worktree."), "Already provisioned additional roots; use [] for one repository."), maxItems: 7 } }),
    output_fields: { ...receiptOutput, workspace_origin: "Verified origin.", relaunch: "{executable, arguments}; arguments include -C, each --add-dir and the saved bootstrap prompt." },
    next_step: "Launch the returned argv after all roots are prepared and authorized. The destination reads scope before Core Task creation.",
  },
  scope: {
    description: "Read and validate the complete set of provisioned records for one Task and build its Core repository arguments.",
    input_schema: object({ launch_id: identity.launch_id, repository_keys: { ...array(key, "Every confirmed repository, including the primary."), minItems: 1, maxItems: 8, uniqueItems: true }, primary_repository_key: key }),
    output_fields: { repository_path: "Primary Task worktree.", workspace_origin: "Primary verified origin.", primary_repository_key: "Present for multiple repositories.", additional_repositories: "Present for multiple repositories; closed {key,repository_path,workspace_origin} entries." },
    next_step: "Use the complete result as the repository fields of taskbelay_open_task; add host and new_task from the admitted request. Core re-observes every worktree.",
  },
  "handoff-start": {
    description: "Record one Host handoff attempt after Core prepares relocation.",
    input_schema: object({ ...identity, relocation_id: text("Copy from taskbelay_prepare_task_relocation.result.relocation_id."), thread_id: text("Other Codex task being moved; the calling task cannot move itself.") }),
    output_fields: { ...receiptOutput, should_dispatch: "Call the Host handoff only when true.", host_request: "Exact handoff_thread arguments when should_dispatch=true." },
    next_step: "Call handoff_thread once and record its complete result using handoff-result.",
  },
  "handoff-result": {
    description: "Retain the Host handoff operation identity or an uncertain result.",
    input_schema: object({ ...identity, host_result: hostResult }), output_fields: { ...receiptOutput, changed: "Whether the record changed." },
    next_step: "Poll the retained host_operation_id with the Host and save each changed result through handoff-status; never redispatch.",
  },
  "handoff-status": {
    description: "Save a polled Host handoff status and the actual destination path.",
    input_schema: object({ ...identity, status: { enum: ["succeeded", "failed", "pending"] }, revision: { type: "integer", minimum: 0, description: "Host operation revision, not Core Task revision." }, worktree_path: nullablePath("Actual destination path, or null while unavailable.") }),
    output_fields: { ...receiptOutput, changed: "Whether the record changed.", relocation_id: "Retained Core relocation ID when updated." },
    next_step: "Only after Host success, resolve the Core relocation blocker with the exact destination paths.",
  },
  "cleanup-decision": {
    description: "Read-only cleanup eligibility from current Core and Git facts; this does not authorize deletion.",
    input_schema: object({ lifecycle: text("Current Core cursor, such as DONE or CANCELLED."), surface: { enum: ["current_session", "managed_worktree", "cli_worktree"] }, clean: { type: "boolean" }, pushed: { type: "boolean" }, stateCertain: { type: "boolean" } }),
    output_fields: { automatic_cleanup: "Always false.", worktree_cleanup: "Keep, Host-owned cleanup or separate authorization requirement.", branch_cleanup: "Keep, Host-owned cleanup or separate authorization requirement." },
    next_step: "Keep uncertain, active, dirty or unpushed resources. Worktree and branch deletion require separate current authorizations.",
  },
  "cleanup-worktree": {
    description: "Remove an exact clean terminal CLI worktree after explicit deletion authorization.",
    input_schema: cleanup, output_fields: { ...receiptOutput, changed: "Whether deletion completed now.", uncertain: "Inspect the original attempt when true; do not repeat deletion." },
    next_step: "Branch deletion requires its own authorization and completed worktree cleanup. Managed worktree cleanup belongs to the Host.",
  },
  "cleanup-branch": {
    description: "Safely delete the confirmed CLI task branch after worktree cleanup and separate authorization.",
    input_schema: cleanup, output_fields: { ...receiptOutput, changed: "Whether deletion completed now.", uncertain: "Inspect the original attempt when true; do not repeat deletion." },
    next_step: "Retain the resulting record; ordinary Git safe-deletion refusals remain effective.",
  },
};

contracts.supersede = {
  description: "Permanently supersede an unexecuted launch, or a failed launch with recorded proof of zero target calls, after a new explicit assessment and choice.",
  input_schema: object({...identity, expected_receipt_digest:text("Exact receipt_digest from status."), reason:text("Explicit reason for the changed choice."), replacement:{...structuredClone(contracts.prepare.input_schema), required:Object.keys(contracts.prepare.input_schema.properties)}}),
  output_fields: {...receiptOutput, receipt_digest:"Successor digest.", predecessor:"Permanently superseded record with its complete unique seed.", prepare_input:"Saved successor input for the ordinary prepare operation."},
  next_step:"Call prepare with prepare_input. Never execute the predecessor again. After an interrupted successor write, read the predecessor and call supersede-resume with its exact seed_digest.",
};
contracts["supersede-resume"] = {
  description:"Complete only the successor named by a permanent supersession seed; never choose another successor.",
  input_schema:object({...identity, seed_digest:text("Exact predecessor.supersession.seed_digest from status.")}),
  output_fields:structuredClone(contracts.supersede.output_fields),
  next_step:contracts.supersede.next_step,
};
contracts.status.output_fields.receipt_digest = "Digest for explicit supersession CAS, or null when no receipt exists.";

export const HOST_LAUNCH_OPERATIONS = Object.freeze(Object.keys(contracts));

export function describeHostLaunchOperation(operation) {
  if (!Object.hasOwn(contracts, operation)) throw new Error(`unknown host-launch operation ${operation}`);
  return structuredClone({ operation, transport: "One closed JSON object on stdin; one JSON result on stdout; errors on stderr with nonzero exit.", ...contracts[operation] });
}

export function hostLaunchHelp(operation) {
  if (operation !== undefined) return `${JSON.stringify(describeHostLaunchOperation(operation), null, 2)}\n`;
  return `Usage: taskbelay-codex host-launch <operation>\nRead a complete operation contract: taskbelay-codex host-launch <operation> --help\n\n${HOST_LAUNCH_OPERATIONS.map((name) => `${name}: ${contracts[name].description}`).join("\n")}\n`;
}
