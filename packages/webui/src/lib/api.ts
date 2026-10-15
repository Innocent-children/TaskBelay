export interface RepositorySummary {
  key: string;
  repository_path: string;
  branch: string;
  upstream: string | null;
  changed_paths: string[];
}
export interface Summary {
  task_id: string;
  request_summary: string;
  host: string;
  status: string;
  lifecycle: string;
  repositories: RepositorySummary[];
  revision: number;
  blocker: string | null;
  archived: boolean;
}
export interface Check {
  id: string; kind: string; rationale: string; conditions: string;
  inputs: string[]; argv?: string[]; purpose: string; full_suite?: boolean;
  goal_ids: string[]; compares?: string;
}
export interface Plan {
  approach: string; verification_basis: string; expected_paths?: string[];
  review_after_lines?: number; checks: Check[];
}
export interface Evidence {
  id: string; check_id: string; source: string; status: string;
  summary: string; recorded_at: string;
}
export interface AcceptedFailure {
  check_id: string; failures: string[]; reason: string;
}
export interface CheckSummary {
  id: string; goal_ids: string[]; purpose: string; kind: string;
  status: string; source: string; evidence_id: string | null;
  run_id: string | null; applicable: boolean; summary: string; reason: string;
  accepted_failure?: AcceptedFailure;
}
export interface Gap {
  code: string; path: string; subject: string; message: string;
  action: string; related_ids: string[];
}
export interface View {
  identity: {task_id: string; revision: number};
  host: string; status: string; archived: boolean; summary: string;
  context: {
    repositories: RepositorySummary[];
    plan: Plan;
    authorization: {
      allowed_paths?: string[]; forbidden_paths?: string[]; request: string;
      goals: {id: string; statement: string}[];
      constraints?: {id: string; statement: string}[];
      dependencies?: {path: string; allowed: string[]}[];
      limits: {runs?: number; execution_ms?: number; core_ms?: number; host_ms?: number; tokens?: number};
    };
  };
  details: {
    repositories: {
      key: string;
      remote_baseline?: {state: string; checked_at: string; basis: string};
      remote_latest?: {state: string; checked_at: string; basis: string};
      workspace_origin: {canonical_worktree_root: string; task_branch: string};
    }[];
    changes: {revision: number; kind: string; summary: string; user_ref?: string; at: string}[];
    evidence: Evidence[];
    attempts: {run_id: string; check_id: string; status: string; summary: string}[];
    exceptions: AcceptedFailure[];
  };
  progress: {goal_id: string; status: string; summary: string; next_step?: string}[];
  checks: CheckSummary[];
  concerns: {id: string; summary: string; applicable: boolean; reason: string;
    resolution?: {kind: "verified" | "not_blocking" | "scope_changed"; basis: string}}[];
  limitations: {id: string; summary: string; basis: string}[];
  issue: null | {summary: string; options: string[]};
  issue_digest?: string;
  digests: {authorization: string; content: string; plan: string};
  usage: {
    runs: number; execution_ms: number; reserved_ms: number; core_calls: number;
    core_ms: number; observation_ms: number; uncertain_ms: number;
    host: {coverage: "unreported" | "partial" | "reported"; observed_through: string | null;
      tokens: number | null; active_ms: number | null; wait_ms: number | null};
  };
  refreshed: boolean;
  budget: {
    feasible: boolean; required_runs: number; worst_case_ms: number; retry_reserve_ms: number;
    warnings: string[];
    limits: {name: string; limited: boolean; actual: number | null; reserved: number;
      uncertain: number; limit: number; remaining: number | null; coverage: string}[];
    checks: {check_id: string; required: boolean; timeout_ms: number; startable: boolean; reason: string}[];
  };
  decision: {advice: string; ready: boolean; gaps: Gap[]; missing_checks: string[]; pending_runs: string[]};
}
export interface Run {
  run_id: string; check_id: string; status: string;
  output: {stdout: string; stderr: string; total_bytes: number; truncated: boolean};
  truncated: boolean; retry: string;
}

export class SavedStateRequired extends Error {}

function responseFailure(value: any): Error {
  const details = [
    value.error?.message ?? "Core rejected the operation",
    ...(value.error?.gaps ?? []).map((gap: Gap) => gap.subject + ": " + gap.message),
    ...(value.error?.details ?? []).map((detail: {path: string; message: string}) => detail.path + ": " + detail.message),
    value.recovery?.message,
  ].filter(Boolean);
  const message = [...new Set(details)].join("; ");
  return value.outcome === "applied" || value.outcome === "unknown" || value.recovery?.operation === "read_before_retry"
    ? new SavedStateRequired(message) : new Error(message);
}

async function responseJSON(response: Response): Promise<any> {
  const value = await response.json();
  if (!response.ok || value.ok === false)
    throw responseFailure(value);
  return value;
}
export async function read<T>(path: string): Promise<T> {
  return responseJSON(await fetch(path));
}
export async function call<T>(tool: string, args: unknown): Promise<T> {
  const csrf = document.querySelector<HTMLMetaElement>(
    'meta[name="taskbelay-session"]',
  )?.content;
  let response: Response;
  let value: any;
  try {
    response = await fetch(`/api/core/${tool}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ csrf, arguments: args }),
    });
    value = await response.json();
  } catch {
    throw new SavedStateRequired(
      "The operation response was lost. Refresh the saved task before another action; no automatic retry was made.",
    );
  }
  if (!response.ok || value.ok === false)
    throw responseFailure(value);
  return value.result;
}
export function identity(view: View) {
  return {
    task_id: view.identity.task_id,
    revision: view.identity.revision,
    operation_id: crypto.randomUUID(),
  };
}
export async function callTask(tool: string, args: unknown): Promise<View> {
  type TaskIdentity = { identity: { task_id: string } };
  const result = await call<TaskIdentity | {view: TaskIdentity}>(tool, args);
  const task = "view" in result ? result.view.identity : result.identity;
  try {
    return (
      await read<{ result: View }>(
        "/api/tasks/" + encodeURIComponent(task.task_id) + "?refresh=1",
      )
    ).result;
  } catch (error) {
    throw new SavedStateRequired(
      "Task saved; refreshing its display failed. Refresh the task to read the saved result. " +
        String(error),
    );
  }
}
