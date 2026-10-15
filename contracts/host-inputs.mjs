import { inputObject as object, field } from '../scripts/interface-schema.mjs';
const text = description => field(description);
const path = description => field('Normalized absolute path. '+description);
const bool = (description, value) => field(description,'boolean',value===undefined?{}:{default:value});
const list = (items,description,extra={}) => field(description,'array',{items,...extra});
const key = field('Stable lowercase repository key; retain it across this workspace operation.','string',{pattern:'^[a-z0-9][a-z0-9._-]{0,127}$'});
const root = object({key,repository_path:path('Existing source worktree.')});
const launch = {launch_id:text('Actual identity returned by prepare; read its receipt after response loss.')};
const selected = object({key,repository_path:path('Existing source root.'),workspace_mode:field('Reuse the current or actually selected workspace mode; ask only for a missing choice.','string',{enum:['current_branch','new_branch','dedicated_worktree']}),source_type:field('Original source type.','string',{enum:['local','remote']}),remote_name:field('Actual remote name, empty for local sources.'),base_branch:text('Original selected source branch.'),target_branch:text('Actual selected target branch; current_branch retains the existing one.'),carry_changes:bool('Actual choice to retain local edits; remote sources cannot carry them.'),worktree_path:path('Current source path for local modes; exact destination for a dedicated worktree.')});
const freshness=object({repositories:list(object({key,repository_path:path('Current root.'),source_type:field('Original workspace source type.','string',{enum:['local','remote']}),remote_name:field('Original remote name, empty for local.'),base_branch:text('Original source branch, not a new task-branch upstream.')},['source_type','remote_name','base_branch']),'One to eight bound repositories.',{minItems:1,maxItems:8}),fetch:bool('Fetch original source refs under actual Git authority.',true),integrate:bool('Separately authorized clean fast-forward only.',false),authorization_ref:text('Actual retained fetch/workspace authorization.'),integration_authorization_ref:text('Actual separate integration authorization, required with integrate=true.'),operation_id:text('Stable integration identity, required with integrate=true.'),recover:bool('Read-only reconciliation of the original stopped operation; no Git replay.',false)},['fetch','integrate','authorization_ref','integration_authorization_ref','operation_id','recover']);
export const workspaceInputs={
 freshness,
 inspect:object({request:text('Exact actual user request.'),repositories:list(root,'All selected repositories.',{minItems:1,maxItems:8})}),
 prepare:object({request:text('Actual retained request, unchanged from inspection.'),authorization_ref:text('Existing real workspace authorization; no repeat plan approval.'),repositories:list(selected,'One to eight exact source/destination choices.',{minItems:1,maxItems:8}),handoff:{description:'Retained original discussion for a dedicated workspace; null is valid for the current session.',anyOf:[{type:'object',description:'Original handoff object retained unchanged. Discussion is required for dedicated workspaces; other original context fields are allowed.',additionalProperties:true,required:['discussion'],properties:{discussion:list({description:'An original message retained unchanged; preserve source IDs and user text.'},'Original discussion in order.',{minItems:1})}},{type:'null'}]}}),
 provision:object(launch),status:object(launch),scope:object(launch),
 'bind-task':object({...launch,task_id:text('Actual Core Task ID from a successful open or recovery; never a locally invented cursor.')}),
 open:object(launch),resume:object(launch),launch:object(launch),
 'retry-launch':object({...launch,previous_caller_stopped:bool('Must be true from actual inspection; a timeout alone is insufficient.'),session_not_started:bool('Must be true only after proving the original Host was never called.'),reason:text('Actual observation establishing no session started.')}),
 'record-session':object({...launch,session_id:text('Actual matching Host session UUID, not a queued or fabricated ID.')}),
 'cleanup-worktree':object({...launch,repository_key:key,task_id:text('Fresh actual terminal Core Task.'),revision:field('Fresh terminal revision.','integer',{minimum:1}),terminal:bool('Must reflect actual terminal state; Core binding is independently read.'),authorized:bool('Actual separate deletion authority, never implied by completion.')}),
 'cleanup-branch':object({...launch,repository_key:key,task_id:text('Fresh actual terminal Core Task.'),revision:field('Fresh terminal revision.','integer',{minimum:1}),terminal:bool('Actual terminal state.'),authorized:bool('Separate actual branch-deletion authority; worktree cleanup must already be complete.')})
};
export const workspaceDescriptions={
 freshness:'Observe the original selected upstream. Fetch/integration require actual existing authority. Same integration identity reads its retained receipt; recovery never repeats fetch/merge or undoes a successful root.',
 inspect:'Read actual repository roots, branches, status and upstream information without creating a Task or workspace. It does not grant Git permissions.',
 prepare:'Persist the actual request, authority and source choices, then freeze the selected base. Remote sources perform authorized fetch. Retain the returned launch_id/receipt_path; it does not create a branch yet.',
 provision:'Create the selected branch/worktree from the frozen source, or keep current_branch. New branches have no implicit upstream. Partial or uncertain operations retain original receipts and are not automatically repeated.',
 status:'Read the saved workspace operation; do not treat saved observations as a fresh remote check or dispatch another operation because its result was lost.',
 scope:'Verify every prepared root and return repositories suitable for Core open_task. Copy the returned origin unchanged. Does not create a Core Task or infer completion.',
 'bind-task':'Associate a successful actual Core Task identity with the original preparation receipt; no second progress state is stored.',
 open:'Return ZCode UI guidance only. action_required does not mean a session was opened. Reuse the saved Core Task and original request.',
 resume:'Continue the original prepared workspace/session using retained identity. ZCode returns UI guidance; Claude returns native argv. Neither return alone proves a Host action executed.',
 launch:'Save a single Claude session launch identity and return native argv. Execute only under actual session authority; read the original record after uncertainty.',
 'retry-launch':'Prepare another call only after actual inspection proves the previous caller stopped without invoking Claude. Missing IDs or timeouts do not prove this.',
 'record-session':'Record an actually observed Claude session ID matching the retained launch identity.',
 'cleanup-worktree':'Delete only the receipt-owned dedicated worktree under actual separate deletion authority, after reading the exact terminal Core Task. Dirty, unpushed or uncertain resources are retained.',
 'cleanup-branch':'Safely delete only the receipt-owned branch after worktree cleanup and separate actual authorization. Preserve ordinary Git deletion refusals.'
};
export function deepseekInput(operation){
 const base={operation:field('Exact workspace_coordinator operation.','string',{const:operation})};
 if(operation==='freshness')return object({...base,...Object.fromEntries(Object.entries(freshness.properties).map(([k,v])=>[k==='repositories'?'freshness_repositories':k,v]))},Object.keys(freshness.properties).filter(k=>!freshness.required.includes(k)));
 if(operation==='provision')return object({...base,authorization_ref:text('Actual workspace authorization retained by the Host guard.'),request:text('Original current request.'),profile:text('Current DSH Profile.'),repositories:list(object({repository_key:key,workspace_mode:selected.properties.workspace_mode,source_repository_path:path('Source root inside current Workspace Root.'),source_type:selected.properties.source_type,carry_changes:selected.properties.carry_changes,remote_name:selected.properties.remote_name,base_branch:selected.properties.base_branch,target_branch:selected.properties.target_branch}),'Complete repository selection.',{minItems:1,maxItems:8})});
 if(operation==='consume')return object({...base,...launch});
 return object({...base,...launch,repository_key:key,task_id:text('Actual terminal Core Task.'),revision:field('Current terminal revision.','integer',{minimum:1}),authorization_ref:text('Actual separate deletion authorization.'),...(operation==='cleanup_worktree'?{}:{source_repository_path:path('Original source checkout used for terminal cleanup/relaunch.')})});
}
