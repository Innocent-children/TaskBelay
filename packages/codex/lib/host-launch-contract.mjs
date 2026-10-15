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
const hostResult = { description: "Complete original Host tool result, including its wrapper if present. Use null when the result was lost; never fabricate an ID." };
const cleanup = object({ ...identity, source_repository_path: source, terminal: { type: "boolean", const: true, description: "Actual Core terminal state, read before cleanup; not inferred from finished edits." }, authorized: { type: "boolean", const: true, description: "Actual separate deletion authority. Completion alone does not grant it." } });

const contracts = {
 freshness: {
  description:"Assess the explicitly bound original source; task-branch upstream changes do not change source selection. Integration has a durable receipt and never replays an existing operation.",
  input_schema:object({repositories:array(object({key,repository_path:path("Current canonical worktree being assessed"),source_type:{type:"string",enum:["local","remote"],description:"Original selected source type; preserve it after creating the task branch."},remote_name:{type:"string",description:"workspace_origin.remote_name; empty for local source"},base_branch:text("workspace_origin.base_branch; a remote source need not have a local branch")},["source_type","remote_name","base_branch"]),"All task repositories; bound callers copy all three original source fields"),fetch:{type:"boolean",default:true,description:"Fetch corresponding refs under actual existing authority; default true."},integrate:{type:"boolean",default:false,description:"Separately authorized clean fast-forward; default false."},authorization_ref:text("Actual authority to fetch refs"),integration_authorization_ref:text("Separate actual integration authority"),operation_id:text("Stable integration identity; required with integrate=true"),recover:{type:"boolean",default:false,description:"Reconcile the original stopped operation without replaying Git mutations."}},["fetch","integrate","authorization_ref","integration_authorization_ref","operation_id","recover"]),
  output_fields:{repositories:"Per-root remote_freshness plus integration state, frozen target and any read-only recovery observation",integration_blocked:"True for blocked, partial, unknown or reconciled batches",operation:"Integration operation_id, receipt_path, status and replayed. Same identity reads its record; recover=true only reconciles a stopped original owner and never repeats Git mutation"},
  next_step:"Copy original source_type/remote_name/base_branch for every bound root. Retain operation_id/receipt_path and all per-root results. Partial success stays applied; after read-only reconciliation, remaining work needs a fresh separately authorized assessment/identity. Do not present an incomplete batch as successful."
 },
  inspect: {
    description: "Read source repositories and return the workspace observation without writing Git or a receipt.",
    input_schema: object({ request: text("Exact current request; retain the same string for prepare."), repositories: { ...array(object({ key, repository_path: source }), "All proposed repositories."), minItems: 1, maxItems: 8 } }),
    output_fields: { request_digest: "Request identity.", repositories: "Canonical roots, HEADs, status digests and bounded dirty paths. Pass this entire result as the workspace selection." },
    next_step: "Reuse existing authorization; obtain only missing workspace choices before prepare.",
  },
  prepare: {
    description: "Using existing actual authority, save the launch material and receipt, resolve the selected local or remote base and freeze its commit and selected workspace contents.",
    input_schema: object({
      request: text("Same exact request supplied to inspect."), authorization_ref: text("Reference to actual authorization for this workspace operation; retain an existing valid choice."),
      ...identity, repository_path: source,
      workspace_mode: { enum: ["new_branch", "current_branch", "dedicated_worktree"], description: "Retain the user-selected/current workspace. Ask only for a missing choice; do not repeat an already authorized decision." },
      source_type: { type: "string", enum: ["local", "remote"], description: "Original selected source type; local retains the original local branch." }, carry_changes: { type: "boolean", description: "Explicit user choice; only true for local sources." },
      remote_name: { type: "string", description: "Confirmed remote, or empty for local." }, base_branch: text("Confirmed branch name on the selected source."),
      target_branch: text("Confirmed new branch, or the observed current branch for current_branch."),
      surface: { type: "string", description: "Actual selected Host execution surface; do not infer a new chat request.", enum: ["current_session", "managed_worktree", "cli_worktree"] },
      worktree_path: nullablePath("The current canonical repository_path for current_session; null for managed_worktree; destination for cli_worktree."),
      handoff_file: nullablePath("Null for current_session. Dedicated worktrees require complete retained requirements JSON matching the packaged lib/task-handoff.mjs contract."),
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
    input_schema: object({ ...identity, dispatch_attempt_id: text("Current claim ID."), host_call_not_made: { type: "boolean", const: true, description: "True only after observing that the original Host creation call never occurred." }, previous_caller_stopped: { type: "boolean", const: true, description: "True only after confirming the original dispatcher has stopped. A timeout is insufficient." }, reason: text("Observed call sequence proving no create_thread call and that the previous caller has stopped.") }),
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
    output_fields: { repositories: "Complete verified {key,repository_path,workspace_origin} array for current Core open_task; copy it unchanged." },
    next_step: "Use the complete result as the repository fields of taskbelay_open_task; add host, authorization, plan and checks from the actual request. Reuse the actual user request; no separate plan confirmation is required. Core re-observes every worktree.",
  },



  "cleanup-decision": {
    description: "Read-only cleanup eligibility from current Core and Git facts; this does not authorize deletion.",
    input_schema: object({ lifecycle: text("Current Core status: active, done or cancelled."), surface: { type: "string", description: "Actual selected Host execution surface; do not infer a new chat request.", enum: ["current_session", "managed_worktree", "cli_worktree"] }, clean: { type: "boolean", description: "Actual current worktree has no pending edits." }, pushed: { type: "boolean", description: "Actual current commits are retained at an observed remote." }, stateCertain: { type: "boolean", description: "All original operations have a known effect. Uncertainty retains resources." } }),
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

export const HOST_LAUNCH_OPERATIONS = Object.freeze(Object.keys(contracts));

export function describeHostLaunchOperation(operation) {
  if (!Object.hasOwn(contracts, operation)) throw new Error(`unknown host-launch operation ${operation}`);
  return structuredClone({ operation, transport: "One closed JSON object on stdin; one JSON result on stdout; errors on stderr with nonzero exit.", ...contracts[operation] });
}

export function hostLaunchHelp(operation) {
  if (operation !== undefined) return `${JSON.stringify(describeHostLaunchOperation(operation), null, 2)}\n`;
  return `Usage: taskbelay-codex host-launch <operation>\nRead a complete operation contract: taskbelay-codex host-launch <operation> --help\n\n${HOST_LAUNCH_OPERATIONS.map((name) => `${name}: ${contracts[name].description}`).join("\n")}\n`;
}
