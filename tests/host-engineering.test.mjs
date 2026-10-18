import test from 'node:test';
import {planFor} from './journeys/fixtures/engineering.mjs';
import assert from 'node:assert/strict';
import {mkdtemp,realpath,cp,mkdir,copyFile,rm,readFile,writeFile,symlink} from 'node:fs/promises';
import {mkdirSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';

const repository=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const core=process.env.ENGINEERING_CORE??join(repository,'.evaluation',process.platform==='win32'?'taskbelay.exe':'taskbelay');
test('all four independently installed Host packages use actual current Core and complete references',async t=>{
 if(!process.env.ENGINEERING_CORE){mkdirSync(join(repository,'.evaluation'),{recursive:true});const built=spawnSync('go',['build','-o',core,'./cmd/taskbelay'],{cwd:repository,encoding:'utf8',windowsHide:true});assert.equal(built.status,0,built.stderr)}
 const root=await realpath(await mkdtemp(join(tmpdir(),'taskbelay-host-engineering-')));t.after(()=>rm(root,{recursive:true,force:true}));
 for(const host of ['codex','claude','zcode','deepseek'])await t.test(host,async()=>{
  const target=join(root,host);await cp(join(repository,'packages',host),target,{recursive:true,filter:p=>!p.includes('node_modules')&&!p.includes('/tests/')&&!p.includes('\\tests\\')&&!p.includes('/runtime/')&&!p.includes('\\runtime\\')});
  const runtimeDir=join(target,'runtime',process.platform==='win32'?'win32-x64':'darwin-arm64');await mkdir(runtimeDir,{recursive:true});const runtime=join(runtimeDir,process.platform==='win32'?'taskbelay.exe':'taskbelay');await copyFile(core,runtime);const data=join(root,host+'-data');await mkdir(data);const env={...process.env,TASKBELAY_DATA_DIR:data};
  const m=JSON.parse(await readFile(join(target,'package.json'),'utf8'));const skill=['deepseek','zcode'].includes(host)?'skills/taskbelay':'plugin/skills/taskbelay';for(const f of ['engineering.md','protocol.md','workspace.md']){assert.ok(m.files.includes(skill+'/references/'+f));assert.ok((await readFile(join(target,skill,'references',f),'utf8')).length>200)}
  const executable=host==='deepseek'?runtime:process.execPath;const args=host==='deepseek'?['call','taskbelay_server_info']:[join(target,'bin',`taskbelay-${host}.mjs`),'call','taskbelay_server_info'];let result=spawnSync(executable,args,{input:'{}',encoding:'utf8',env,windowsHide:true,maxBuffer:8*1024*1024});assert.equal(result.status,0,result.stderr);const info=JSON.parse(result.stdout);assert.equal(info.ok,true);assert.equal(info.result.tools.length,15);assert.ok(info.result.tools.includes('taskbelay_resolve_issue'));assert.ok(!info.result.tools.includes('taskbelay_run_check'));assert.ok(!info.result.tools.some(x=>x.startsWith('taskbelay_submit_')));
  if(host==='deepseek'){const names=await import(pathToFileURL(join(target,'lib/tool-names.mjs')).href);assert.deepEqual(names.TASKBELAY_RAW_TOOL_NAMES,info.result.tools)}
  const repo=join(root,host+'-repo');await mkdir(repo);const git=(...args)=>{const r=spawnSync('git',['-C',repo,...args],{encoding:'utf8',windowsHide:true});assert.equal(r.status,0,r.stderr);return r.stdout.trim()};git('init','-b','main');git('config','user.name','Host test');git('config','user.email','host@localhost');await writeFile(join(repo,'check.cjs'),"require('node:assert/strict').equal(2+3,5);\n");git('add','.');git('commit','-m','base');
  // Exercise the shipped freshness entrypoints, not only their shared source.
  assert.ok(m.files.includes('lib/integration-receipt.mjs'));
  const remote=join(root,host+'-remote.git'),seed=join(root,host+'-seed');let created=spawnSync('git',['init','--bare','-b','main',remote],{encoding:'utf8'});assert.equal(created.status,0,created.stderr);git('remote','add','origin',remote);git('push','-u','origin','main');created=spawnSync('git',['clone',remote,seed],{encoding:'utf8'});assert.equal(created.status,0,created.stderr);
  const seedGit=(...args)=>{const r=spawnSync('git',['-C',seed,...args],{encoding:'utf8'});assert.equal(r.status,0,r.stderr);return r.stdout.trim()};seedGit('config','user.name','Host fixture');seedGit('config','user.email','fixture@localhost');await writeFile(join(seed,'remote-file.txt'),'actual remote change\n');seedGit('add','.');seedGit('commit','-m','remote change');seedGit('push','origin','main');const expected=seedGit('rev-parse','HEAD');
  const freshRequest={repositories:[{key:'primary',repository_path:repo,source_type:'local',remote_name:'',base_branch:'main'}],fetch:true,integrate:true,authorization_ref:'actual-fixture-fetch',integration_authorization_ref:'actual-fixture-integration',operation_id:'packaged-'+host};
  let freshness,replay;
  if(host==='deepseek'){
   // Resolve the package's actual declared SDK dependency in this isolated installation.
   await symlink(join(repository,'packages','deepseek','node_modules'),join(target,'node_modules'),process.platform==='win32'?'junction':'dir');
   const {registerWorkspaceCoordinator}=await import(pathToFileURL(join(target,'lib/workspace-tool.mjs')).href);let registered;
   const dispose=registerWorkspaceCoordinator({tools:{guard(){return()=>{}},register(value){registered=value;return()=>{}}}},{dataDirectory:data,workspaceRoot:repo});
   const arguments_={operation:'freshness',freshness_repositories:freshRequest.repositories,fetch:true,integrate:true,authorization_ref:freshRequest.authorization_ref,integration_authorization_ref:freshRequest.integration_authorization_ref,operation_id:freshRequest.operation_id};
   freshness=await registered.execute(arguments_,{name:'workspace_coordinator',arguments:arguments_});replay=await registered.execute(arguments_,{name:'workspace_coordinator',arguments:arguments_});dispose();
  }else{
   const launch=()=>{const r=spawnSync(process.execPath,[join(target,'bin',`taskbelay-${host}.mjs`),'host-launch','freshness'],{input:JSON.stringify(freshRequest),encoding:'utf8',env,windowsHide:true,maxBuffer:8*1024*1024});assert.equal(r.status,0,r.stderr+r.stdout);return JSON.parse(r.stdout)};
   freshness=launch();replay=launch();
  }
  assert.equal(freshness.operation.status,'complete');assert.equal(freshness.repositories[0].integration.state,'applied');assert.equal(freshness.repositories[0].remote_freshness.upstream,'refs/remotes/origin/main');assert.equal(git('rev-parse','HEAD'),expected);assert.equal(replay.operation.replayed,true);assert.equal(replay.operation.operation_id,freshRequest.operation_id);assert.equal(JSON.parse(await readFile(freshness.operation.receipt_path,'utf8')).repositories[0].integration.state,'applied');
  const call=(tool,args)=>{const invocation=tool==='check.run'?['check','run']:['call',tool];const r=spawnSync(executable,host==='deepseek'?invocation:[join(target,'bin',`taskbelay-${host}.mjs`),...invocation],{input:JSON.stringify(args),encoding:'utf8',env,windowsHide:true,maxBuffer:8*1024*1024});assert.equal(r.status,0,r.stderr+r.stdout);const v=JSON.parse(r.stdout);assert.equal(v.ok,true);return v.result};
  const opened=call('taskbelay_open_task',{operation_id:'host-open',host,repositories:[{key:'primary',repository_path:repo}],authorization:{user_ref:'actual-fixture-user',request:'Verify the local arithmetic helper',goals:[{id:'sum',statement:'Existing helper passes the required assertion'}]},plan:planFor([{goal_ids:['sum'],id:'check',kind:'command',argv:[process.execPath,'check.cjs'],rationale:'Actual local assertion',conditions:'Node built-in assertion',retry:'repeatable',timeout_ms:3000}])});const executed=call('check.run',{task_id:opened.identity.task_id,operation_id:'host-run',revision:opened.identity.revision,check_id:'check',rationale:'Establish the requested actual arithmetic assertion.'});assert.equal(executed.run.status,'passed');assert.equal(executed.run.exit_code,0);assert.equal(executed.view.usage.runs,1);
 })
});
