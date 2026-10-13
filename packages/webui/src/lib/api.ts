export interface Summary { task_id: string; request_summary: string; lifecycle: string; worktree_path: string; revision: number; blocker: string | null; archived: boolean }
export interface Check { id: string; kind: string; rationale: string; conditions: string; inputs: string[]; argv?:string[]; purpose:string; full_suite?:boolean }
export interface Plan { approach:string; verification_basis:string }
export interface Evidence { id: string; check_id: string; source: string; status: string; summary: string; recorded_at: string }
export interface View {
 task: { repositories:{key:string;remote_baseline?:{state:string;checked_at:string;basis:string};remote_latest?:{state:string;checked_at:string;basis:string};workspace_origin:{canonical_worktree_root:string;task_branch:string}}[]; plan:Plan; archived:boolean;  exceptions:{check_id:string;failures:string[];reason:string}[]; task_id: string; status: string; revision: number; authorization: { allowed_paths?:string[]; forbidden_paths?:string[]; expected_paths?:string[]; request: string; goals: { id: string; statement: string; checks: string[] }[]; constraints: {id:string;statement:string}[]; dependencies?:{path:string;allowed:string[]}[]; limits: {runs?:number;execution_ms?:number;core_ms?:number;host_ms?:number;tokens?:number} }; checks: Check[]; progress: {goal_id:string;status:string;summary:string}[]; evidence: Evidence[]; attempts: {run_id:string;check_id:string;status:string;summary:string}[]; issue: null | {summary:string;options:string[]}; summary: string; risks: string[]; usage: { runs:number; execution_ms:number; reserved_ms:number;core_calls:number;core_ms:number;observation_ms:number;host_reported_ms:number;host_reported_wait_ms:number;host_reported_tokens:number;uncertain_ms:number } };
 plan_digest: string; authorization_digest: string; content_digest: string; refreshed: boolean;
 budget:{feasible:boolean;required_runs:number;worst_case_ms:number;retry_reserve_ms:number;warnings:string[];limits:{name:string;limited:boolean;actual:number;reserved:number;uncertain:number;limit:number;remaining:number}[];checks:{check_id:string;required:boolean;timeout_ms:number;startable:boolean;reason:string}[]};
 control_cost: {calls:number;wall_ms:number;unknown_ms:number;pending:number};
 decision: { accepted_failures:{check_id:string;failures:string[];reason:string}[]; advice:string; ready:boolean; findings:{id:string;kind:string;path:string;detail:string;blocking:boolean}[];missing_checks:string[];pending_runs:string[];evidence:{evidence_id:string;valid:boolean;reason:string}[] };
}
export async function read<T>(path:string):Promise<T> { const response=await fetch(path);const value=await response.json();if(!response.ok || value.ok===false)throw new Error(value.error?.message ?? "Local Core is unavailable");return value; }
export async function call<T>(tool:string,args:unknown):Promise<T> {const csrf=document.querySelector<HTMLMetaElement>('meta[name="taskbelay-session"]')?.content;const response=await fetch(`/api/core/${tool}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({csrf,arguments:args})});const v=await response.json();if(!response.ok||v.ok===false)throw new Error(v.error?.message??'Core rejected the operation');return v.result;}
export function identity(view:View) {return {task_id:view.task.task_id,revision:view.task.revision,operation_id:crypto.randomUUID()};}

export async function callTask(tool:string,args:unknown):Promise<View> {
 const result=await call<{task:{task_id:string}}>(tool,args);
 try { return (await read<{result:View}>('/api/tasks/'+encodeURIComponent(result.task.task_id)+'?refresh=1')).result; }
 catch(e) { throw new Error('Task saved; refreshing its display failed. Refresh the task to read the saved result. '+String(e)); }
}
