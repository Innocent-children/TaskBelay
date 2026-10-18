import { mkdtemp, mkdir, writeFile, readFile, rm, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { execFile as execCallback } from 'node:child_process';
import { promisify } from 'node:util';
import { pathToFileURL } from 'node:url';
import { workspaceInputs, workspaceDescriptions, deepseekInput } from '../contracts/host-inputs.mjs';
import { freshnessOutputSchema } from '../contracts/freshness-output.mjs';
import { exampleSchema, mergeSchema } from './interface-schema.mjs';
const execFile=promisify(execCallback);
const root=resolve(import.meta.dirname,'..');
const core=resolve(process.env.ENGINEERING_CORE ?? join(root,'.evaluation',process.platform==='win32'?'taskbelay.exe':'taskbelay'));
const notes=JSON.parse(await readFile(join(root,'contracts/field-notes.json'),'utf8'));
const parent=await realpath(await mkdtemp(join(tmpdir(),'taskbelay-contracts-')));
const data=join(parent,'data');await mkdir(data);
const emptyConfig=join(parent,'gitconfig');await writeFile(emptyConfig,'');
const environment={...process.env,TASKBELAY_DATA_DIR:data,GIT_CONFIG_GLOBAL:emptyConfig,GIT_CONFIG_SYSTEM:emptyConfig};
const records=[];
const ids=new Map();
let serial=0;
const run=async(program,args,options={})=>execFile(program,args,{encoding:'utf8',maxBuffer:8*1024*1024,env:environment,...options});
const git=async args=>(await run('git',args)).stdout;
function normalize(value,key=''){
 if(value===null)return null;
 if(Array.isArray(value))return value.map(v=>normalize(v,key));
 if(typeof value==='object')return Object.fromEntries(Object.keys(value).sort().map(k=>[k,normalize(value[k],k)]));
 if(typeof value==='string'){
  if(value.includes(parent))value=value.replaceAll(parent,'/example');
  if(value.includes(root))value=value.replaceAll(root,'/package/taskbelay');
  if(/_at$|observed_through/.test(key)&&/^\d{4}-/.test(value))return '2026-10-14T09:00:00Z';
  if(/^[a-f0-9]{40,64}$/.test(value))return 'a'.repeat(value.length);
  if(/^[a-f0-9]{8}-[a-f0-9-]{27}$/.test(value)){
   if(!ids.has(value))ids.set(value,`00000000-0000-4000-8000-${String(ids.size+1).padStart(12,'0')}`);
   return ids.get(value);
  }
 }
 return value;
}
async function coreCall(method,args){
 const command=method==='check.run'?['check','run']:method.startsWith('host-check.')?['host-check',method.slice(11)]:['call',method];
 const child=await new Promise((resolveChild,reject)=>{
  const child=execCallback(core,command,{encoding:'utf8',maxBuffer:8*1024*1024,env:environment},(error,stdout,stderr)=>{
   let value;try{value=JSON.parse(stdout)}catch{reject(error??new Error(stderr||stdout));return}
   if(value.ok!==true){reject(new Error(JSON.stringify(value)));return}resolveChild(value.result);
  });child.stdin.end(JSON.stringify(args));
 });return child;
}
async function terminalTask(scope){
 const n=++serial;
 const opened=await coreCall('taskbelay_open_task',{operation_id:`example-open-${n}`,host:'cli',repositories:scope.repositories,authorization:{user_ref:'fixture-user-request',request:'Verify the prepared clean workspace without publishing',goals:[{id:'goal',statement:'The prepared tracked contents pass the declared check'}]},plan:{approach:'Inspect existing tracked contents',verification_basis:'Actual native Git command in an isolated fixture',checks:[{id:'check',kind:'command',argv:['git','diff','--quiet'],repository_key:scope.repositories[0].key,goal_ids:['goal'],rationale:'Verify the tracked contents',conditions:'Isolated fixture',retry:'repeatable',timeout_ms:3000}]}});
 const identity=(view,op)=>({task_id:view.identity.task_id,revision:view.identity.revision,operation_id:op});
 const run=await coreCall('check.run',{...identity(opened,`example-run-${n}`),check_id:'check',rationale:'Verify current tracked contents'});
 const checkpoint=await coreCall('taskbelay_checkpoint',{...identity(run.view,`example-review-${n}`),progress:[{goal_id:'goal',status:'done',summary:'The actual command passed'}],assessment:{authorization_digest:run.view.digests.authorization,content_digest:run.view.digests.content,summary:'Current fixture review',review:{summary:'Existing repository responsibility and targeted verification fit this fixture'}}});
 const done=await coreCall('taskbelay_finish_task',{...identity(checkpoint,`example-finish-${n}`),summary:'Prepared contents verified',acceptance:[{goal_id:'goal',evidence_ids:[run.run.run_id],basis:'Actual current Git result'}]});
 return coreCall('taskbelay_get_task',{task_id:done.identity.task_id,refresh:false,detail:true});
}
async function repository(label){
 const dir=join(parent,label);await mkdir(dir);await git(['init','-b','main',dir]);
 await git(['-C',dir,'config','user.name','Example']);await git(['-C',dir,'config','user.email','example@localhost']);
 await git(['-C',dir,'commit','--allow-empty','-m','fixture']);return dir;
}
async function record(host,op,input,action,description=workspaceDescriptions[op],inputSchema=workspaceInputs[op]){
 const output=await action();
 const name=`${host}.host-launch.${op}`;
 let entry=records.find(x=>x.name===name);
 if(!entry){entry={name,description,input_schema:inputSchema,output_schema:null,transport:'One closed JSON object on stdin; success stdout is the complete JSON result. Failure exits nonzero with stderr. Preserve any returned launch/receipt identity before another action.',examples:[]};records.push(entry)}
 entry.output_schema=op==='freshness'?freshnessOutputSchema:mergeSchema(entry.output_schema,exampleSchema(output,notes));
 entry.examples.push({name:'success',preconditions:'Actual helper and Git/Core fixture. Paths, IDs and timestamps are illustrative; Host session observations are simulated and do not establish a real Host session.',input:normalize(input),output:normalize(output)});
 return output;
}
async function recordInvalid(host,op){
 const entry=records.find(x=>x.name===`${host}.host-launch.${op}`);
 if(!entry)return;
 const args=[join(root,'packages',host,'bin',`taskbelay-${host}.mjs`),'host-launch',op];
 const result=await new Promise(resolveResult=>{const child=execCallback(process.execPath,args,{env:{...environment,HOME:parent,USERPROFILE:parent,LOCALAPPDATA:join(parent,'appdata')},encoding:'utf8'},(error,stdout,stderr)=>resolveResult({exit_code:error?.code??0,stdout,stderr}));child.stdin.end('{"unexpected":true}');});
 if(result.exit_code===0)throw new Error('invalid helper input accepted: '+entry.name);
 entry.examples.push({name:'unknown_member',preconditions:'Unknown top-level request field; actual CLI rejects this before a workspace operation.',input:{unexpected:true},output:normalize(result)});
}
async function workspaceHost(host){
 const workspace=await import(pathToFileURL(join(root,'packages',host,'lib/workspace.mjs')));
 const repo=await repository(host);
 const options={environment:{...environment,HOME:parent,USERPROFILE:parent,LOCALAPPDATA:join(parent,'appdata')},runGit:git,checkWorkspaceAvailable:async repository_path=>coreCall('host-check.workspace-available',{repository_path}),readTask:async task_id=>({ok:true,result:await coreCall('taskbelay_get_task',{task_id,refresh:false,detail:true})})};
 // Preflight uses the native Core command, not the remote MCP catalog.
 options.checkWorkspaceAvailable=async repository_path=>{
  const value=await new Promise((resolveValue,reject)=>{const child=execCallback(core,['host-check','workspace-available'],{env:environment,encoding:'utf8'},(error,stdout)=>{if(error)return reject(error);resolveValue(JSON.parse(stdout).result)});child.stdin.end(JSON.stringify({repository_path}));});return value;
 };
 const selection={key:'primary',repository_path:repo,workspace_mode:'new_branch',source_type:'local',remote_name:'',base_branch:'main',target_branch:'feature/example',carry_changes:false,worktree_path:repo};
 const request='Prepare the selected branch without publishing';
 const fresh={repositories:[{key:'primary',repository_path:repo,source_type:'local',remote_name:'',base_branch:'main'}],fetch:false,integrate:false};
 await record(host,'freshness',fresh,()=>workspace.freshness(fresh,options));
 await record(host,'inspect',{request,repositories:[{key:'primary',repository_path:repo}]},()=>workspace.inspect({request,repositories:[{key:'primary',repository_path:repo}]},options));
 const prepared=await record(host,'prepare',{request,authorization_ref:'fixture-workspace-request',repositories:[selection],handoff:null},()=>workspace.prepare({request,authorization_ref:'fixture-workspace-request',repositories:[selection],handoff:null},options));
 const launch={launch_id:prepared.launch_id};
 await record(host,'status',launch,()=>workspace.status(prepared.launch_id,options));
 await record(host,'provision',launch,()=>workspace.provision(prepared.launch_id,options));
 const scope=await record(host,'scope',launch,()=>workspace.scope(prepared.launch_id,options));
 const done=await terminalTask(scope);
 await record(host,'bind-task',{...launch,task_id:done.identity.task_id},()=>workspace.bindTask(prepared.launch_id,{task_id:done.identity.task_id},options));
 if(host==='zcode')for(const op of ['open','resume'])await record(host,op,launch,()=>workspace.session(prepared.launch_id,op,options));
 else {
  const started=await record(host,'launch',launch,()=>workspace.session(prepared.launch_id,'launch',{},options));
  const proof={previous_caller_stopped:true,session_not_started:true,reason:'The completed fixture call returned argv and did not invoke Claude'};
  await record(host,'retry-launch',{...launch,...proof},()=>workspace.session(prepared.launch_id,'retry-launch',proof,options));
  await record(host,'record-session',{...launch,session_id:started.session_id},()=>workspace.session(prepared.launch_id,'record-session',{session_id:started.session_id},options));
  await record(host,'resume',launch,()=>workspace.session(prepared.launch_id,'resume',{},options));
 }
 await record(host,'status',launch,()=>workspace.status(prepared.launch_id,options));
 const cleanupSelection={...selection,workspace_mode:'dedicated_worktree',base_branch:'feature/example',target_branch:'feature/cleanup',worktree_path:join(parent,host+'-worktree')};
 const cleanupReceipt=await workspace.prepare({request,authorization_ref:'fixture-workspace-request',repositories:[cleanupSelection],handoff:{discussion:[{role:'user',text:request}]}},options);
 await workspace.provision(cleanupReceipt.launch_id,options);
 const terminal=await terminalTask(await workspace.scope(cleanupReceipt.launch_id,options));
 const cleanup={repository_key:'primary',task_id:terminal.identity.task_id,revision:terminal.identity.revision,terminal:true,authorized:true};
 for(const op of ['cleanup-worktree','cleanup-branch'])await record(host,op,{launch_id:cleanupReceipt.launch_id,...cleanup},()=>workspace.cleanup(cleanupReceipt.launch_id,op,cleanup,options));
 for(const op of new Set(records.filter(r=>r.name.startsWith(host+'.')).map(r=>r.name.split('.').at(-1))))await recordInvalid(host,op);
}
async function codexHost(){
 const {runCLI}=await import(pathToFileURL(join(root,'packages/codex/bin/taskbelay-codex.mjs')));
 const {describeHostLaunchOperation}=await import(pathToFileURL(join(root,'packages/codex/lib/host-launch-contract.mjs')));
 const repo=await repository('codex');
 const paths={productSupportRoot:join(parent,'codex-support'),dataDirectory:data,usesDefaultDataDirectory:false,packageRoot:join(root,'packages/codex'),enforcePrivateModes:true};
 const invoke=async(op,input)=>{
  let stdout='',stderr='';
  const reply=await runCLI(['host-launch',op],{resolvePaths:async()=>paths,environment,runGit:git,readInput:async()=>JSON.stringify(input),stdout:{write:x=>{stdout+=x}},stderr:{write:x=>{stderr+=x}},checkWorkspaceAvailable:async repository_path=>coreCall('host-check.workspace-available',{repository_path})});
  if(reply.code!==0)throw new Error(op+': '+stderr);
  return JSON.parse(stdout);
 };
 const save=(op,input)=>{const c=describeHostLaunchOperation(op);return record('codex',op,input,()=>invoke(op,input),c.description+' '+c.next_step,c.input_schema)};
 const request='Prepare the selected branch without publishing';
 await save('inspect',{request,repositories:[{key:'primary',repository_path:repo}]});
 await save('freshness',{repositories:[{key:'primary',repository_path:repo,source_type:'local',remote_name:'',base_branch:'main'}],fetch:false,integrate:false});
 const base={request,authorization_ref:'fixture-workspace-request',repository_key:'primary',repository_path:repo,workspace_mode:'new_branch',source_type:'local',carry_changes:false,remote_name:'',base_branch:'main',target_branch:'feature/example',surface:'current_session',worktree_path:repo,handoff_file:null};
 const prepared=await save('prepare',base);
 const identity={launch_id:prepared.receipt.launch_id,repository_key:'primary'};
 await save('status',identity);await save('local-provision',identity);
 await save('scope',{launch_id:identity.launch_id,repository_keys:['primary'],primary_repository_key:'primary'});
 const handoff={request,goal:'Verify the prepared tracked contents',confirmed_requirements:[{text:request,source_ids:['user-1']}],terminology:[],scope_and_constraints:['No publication'],investigation:[],work_requirements:[],unconfirmed_suggestions:[],assumptions:[],open_questions:[],discussion:[{id:'user-1',role:'user',text:request}]};
 const handoffFile=join(parent,'handoff.json');await writeFile(handoffFile,JSON.stringify(handoff));
 const managed=await invoke('prepare',{...base,workspace_mode:'dedicated_worktree',base_branch:'feature/example',target_branch:'feature/managed',surface:'managed_worktree',worktree_path:null,handoff_file:handoffFile});
 const managedID={launch_id:managed.receipt.launch_id,repository_key:'primary'};
 const start=await save('dispatch-start',{...managedID,project_id:'fixture-project'});
 const claimed=await save('dispatch-call',{...managedID,dispatch_attempt_id:start.receipt.operation_status.dispatch_attempt_id});
 const recovered=await save('dispatch-recover',{...managedID,dispatch_attempt_id:claimed.receipt.operation_status.dispatch_attempt_id,host_call_not_made:true,previous_caller_stopped:true,reason:'The fixture completed the helper call and never invoked a Host creation tool'});
 await invoke('dispatch-call',{...managedID,dispatch_attempt_id:recovered.receipt.operation_status.dispatch_attempt_id});
 await save('dispatch-result',{...managedID,host_result:{clientThreadId:'fixture-queued-thread'}});
 await save('dispatch-reconcile',{...managedID,candidates:[{thread_id:'fixture-final-thread',initial_prompt:start.host_request.prompt}]});
 const managedPath=join(parent,'codex-managed');await git(['-C',repo,'worktree','add','--detach',managedPath,managed.receipt.base_commit]);
 await save('bootstrap',{...managedID,worktree_path:managedPath});
 const cliPath=join(parent,'codex-cli');
 const cli=await invoke('prepare',{...base,workspace_mode:'dedicated_worktree',base_branch:'feature/example',target_branch:'feature/cli',surface:'cli_worktree',worktree_path:cliPath,handoff_file:handoffFile});
 const cliID={launch_id:cli.receipt.launch_id,repository_key:'primary'};
 await save('cli-provision',{...cliID,source_repository_path:repo,additional_worktree_paths:[]});
 const scope=await invoke('scope',{launch_id:cliID.launch_id,repository_keys:['primary'],primary_repository_key:'primary'});
 await terminalTask(scope);
 await save('cleanup-decision',{lifecycle:'done',surface:'cli_worktree',clean:true,pushed:true,stateCertain:true});
 for(const op of ['cleanup-worktree','cleanup-branch'])await save(op,{...cliID,source_repository_path:repo,terminal:true,authorized:true});
 for(const op of new Set(records.filter(r=>r.name.startsWith('codex.')).map(r=>r.name.split('.').at(-1))))await recordInvalid('codex',op);
}

async function deepseekHost(){
 const {createWorkspaceCoordinator}=await import(pathToFileURL(join(root,'packages/deepseek/lib/workspace-coordinator.mjs')));
 const {assessRepositoryFreshness}=await import(pathToFileURL(join(root,'packages/deepseek/lib/repository-freshness.mjs')));
 const repo=await repository('deepseek');
 const remote=join(parent,'deepseek-remote.git');await git(['init','--bare',remote]);await git(['-C',repo,'remote','add','origin',remote]);await git(['-C',repo,'push','-u','origin','main']);
 const command=async(program,args,options={})=>{
  try{const r=await run(program,args,{cwd:options.cwd,env:{...environment,...options.env},signal:options.signal});return {...r,code:0,signal:null};}
  catch(error){return {stdout:error.stdout??'',stderr:error.stderr??'',code:typeof error.code==='number'?error.code:1,signal:error.signal??null};}
 };
 const common={runtimePath:core,dataDirectory:data,command,checkWorkspaceAvailable:async repository_path=>coreCall('host-check.workspace-available',{repository_path}),readTask:async({taskID})=>coreCall('taskbelay_get_task',{task_id:taskID,refresh:false,detail:true})};
 const signal=new AbortController().signal;
 const recordDS=async(op,input,action,bad,badAction)=>{
  const output=await action();
  let message;
  try{await badAction();throw new Error('invalid fixture accepted');}catch(error){if(error.message==='invalid fixture accepted')throw error;message=error.message}
  records.push({name:'deepseek.workspace_coordinator.'+op,description:'Use workspace_coordinator operation='+op+'. Preserve actual authority and the original launch/Task identity. Output is the producer result; errors are surfaced as Host tool errors without silently retrying.',input_schema:deepseekInput(op),output_schema:op==='freshness'?freshnessOutputSchema:exampleSchema(output,notes),transport:'One DSH tool call. Success is the complete JSON value; a thrown error is rendered by the Host with isError=true.',examples:[{name:'success',preconditions:'Real helper/Git/Core fixture; Host dispatch and user input are explicitly simulated.',input:normalize(input),output:normalize(output)},{name:'invalid_request',preconditions:'Concrete invalid request to the same operation.',input:normalize(bad),output:{isError:true,message:normalize(message)}}]});
  return output;
 };
 const freshness={operation:'freshness',freshness_repositories:[{key:'primary',repository_path:repo,source_type:'local',remote_name:'',base_branch:'main'}],fetch:false,integrate:false};
 await recordDS('freshness',freshness,()=>assessRepositoryFreshness({repositories:freshness.freshness_repositories,fetch:false,integrate:false},{runGit:git}),{operation:'freshness',freshness_repositories:[]},()=>assessRepositoryFreshness({repositories:[],fetch:false,integrate:false},{runGit:git}));
 const coordinator=createWorkspaceCoordinator({...common,workspaceRoot:repo});
 const selection={repository_key:'primary',workspace_mode:'dedicated_worktree',source_repository_path:repo,source_type:'remote',carry_changes:false,remote_name:'origin',base_branch:'main',target_branch:'feature/deepseek'};
 const request={operation:'provision',authorization_ref:'fixture-workspace-request',request:'Verify prepared contents without publishing',profile:'web',repositories:[selection]};
 const made=await recordDS('provision',request,()=>coordinator.provision({request:request.request,profile:'web',repositories:[selection],signal}),{...request,repositories:[]},()=>coordinator.provision({request:request.request,profile:'web',repositories:[],signal}));
 const resumed=createWorkspaceCoordinator({...common,workspaceRoot:made.workspace_root});
 const consumed=await recordDS('consume',{operation:'consume',launch_id:made.launch_id},()=>resumed.consume({launchID:made.launch_id,signal}),{operation:'consume',launch_id:'missing-launch'},()=>resumed.consume({launchID:'missing-launch',signal}));
 const done=await terminalTask(consumed.open_task);
 const input={operation:'prepare_cleanup',launch_id:made.launch_id,repository_key:'primary',task_id:done.identity.task_id,revision:done.identity.revision,authorization_ref:'fixture-delete-request',source_repository_path:repo};
 // The fixture branch contains no extra content. Publish only to an isolated local
 // bare remote to exercise the ordinary unpushed-work guard without networking.
 await git(['-C',made.workspace_root,'push','-u','origin','feature/deepseek']);
 const execution={name:'workspace_coordinator',arguments:input,signal,concludeTurn(){}};
 const options={launchID:made.launch_id,repositoryKey:'primary',taskID:done.identity.task_id,revision:done.identity.revision,sourceRepositoryPath:repo,signal,execution};
 await recordDS('prepare_cleanup',input,()=>resumed.prepareCleanup(options),{...input,revision:0},()=>resumed.prepareCleanup({...options,revision:0}));
 const source=createWorkspaceCoordinator({...common,workspaceRoot:repo});
 await recordDS('cleanup_worktree',{...input,operation:'cleanup_worktree'},()=>source.cleanupWorktree(options),{...input,operation:'cleanup_worktree',revision:0},()=>source.cleanupWorktree({...options,revision:0}));
 await recordDS('cleanup_branch',{...input,operation:'cleanup_branch'},()=>source.cleanupBranch(options),{...input,operation:'cleanup_branch',repository_key:'missing'},()=>source.cleanupBranch({...options,repositoryKey:'missing'}));
}

try{
 for(const host of ['zcode','claude'])await workspaceHost(host);
 await codexHost();
 await deepseekHost();
 await writeFile(join(root,'contracts/host-examples.json'),JSON.stringify(records,null,2)+'\n');
 process.stdout.write(`Exported ${records.length} helper contracts from actual producers.\n`);
}finally{await rm(parent,{recursive:true,force:true});}
