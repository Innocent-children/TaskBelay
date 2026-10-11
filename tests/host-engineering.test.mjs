import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,cp,mkdir,copyFile,rm,readFile,writeFile} from 'node:fs/promises';
import {spawnSync} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';

const repository=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const core=process.env.ENGINEERING_CORE??join(repository,process.platform==='win32'?'.taskbelay-engineering.exe':'.taskbelay-engineering');
test('all four independently installed Host packages use actual current Core and complete references',async t=>{
 const root=await mkdtemp(join(tmpdir(),'taskbelay-host-engineering-'));t.after(()=>rm(root,{recursive:true,force:true}));
 for(const host of ['codex','claude','zcode','deepseek'])await t.test(host,async()=>{
  const target=join(root,host);await cp(join(repository,'packages',host),target,{recursive:true,filter:p=>!p.includes('node_modules')&&!p.includes('/tests/')&&!p.includes('\\tests\\')&&!p.includes('/runtime/')&&!p.includes('\\runtime\\')});
  const runtimeDir=join(target,'runtime',process.platform==='win32'?'win32-x64':'darwin-arm64');await mkdir(runtimeDir,{recursive:true});const runtime=join(runtimeDir,process.platform==='win32'?'taskbelay.exe':'taskbelay');await copyFile(core,runtime);const data=join(root,host+'-data');await mkdir(data);const env={...process.env,TASKBELAY_DATA_DIR:data};
  const m=JSON.parse(await readFile(join(target,'package.json'),'utf8'));const skill=['deepseek','zcode'].includes(host)?'skills/taskbelay':'plugin/skills/taskbelay';for(const f of ['engineering.md','protocol.md','workspace.md']){assert.ok(m.files.includes(skill+'/references/'+f));assert.ok((await readFile(join(target,skill,'references',f),'utf8')).length>200)}
  const executable=host==='deepseek'?runtime:process.execPath;const args=host==='deepseek'?['call','taskbelay_server_info']:[join(target,'bin',`taskbelay-${host}.mjs`),'call','taskbelay_server_info'];let result=spawnSync(executable,args,{input:'{}',encoding:'utf8',env,windowsHide:true,maxBuffer:8*1024*1024});assert.equal(result.status,0,result.stderr);const info=JSON.parse(result.stdout);assert.equal(info.ok,true);assert.equal(info.result.tools.length,12);assert.ok(info.result.tools.includes('taskbelay_run_check'));assert.ok(!info.result.tools.some(x=>x.startsWith('taskbelay_submit_')));
  if(host==='deepseek'){const names=await import(pathToFileURL(join(target,'lib/tool-names.mjs')).href);assert.deepEqual(names.TASKBELAY_RAW_TOOL_NAMES,info.result.tools)}
  const repo=join(root,host+'-repo');await mkdir(repo);const git=(...args)=>{const r=spawnSync('git',['-C',repo,...args],{encoding:'utf8',windowsHide:true});assert.equal(r.status,0,r.stderr)};git('init','-b','main');git('config','user.name','Host test');git('config','user.email','host@localhost');await writeFile(join(repo,'check.cjs'),"require('node:assert/strict').equal(2+3,5);\n");git('add','.');git('commit','-m','base');
  const call=(tool,args)=>{const r=spawnSync(executable,host==='deepseek'?['call',tool]:[join(target,'bin',`taskbelay-${host}.mjs`),'call',tool],{input:JSON.stringify(args),encoding:'utf8',env,windowsHide:true,maxBuffer:8*1024*1024});assert.equal(r.status,0,r.stderr+r.stdout);const v=JSON.parse(r.stdout);assert.equal(v.ok,true);return v.result};
  const opened=call('taskbelay_open_task',{operation_id:'host-open',host,repositories:[{key:'primary',repository_path:repo}],authorization:{user_ref:'actual-fixture-user',request:'Verify the local arithmetic helper',goals:[{id:'sum',statement:'Existing helper passes the required assertion',checks:['check']}]},checks:[{id:'check',kind:'command',argv:[process.execPath,'check.cjs'],rationale:'Actual local assertion',conditions:'Node built-in assertion',retry:'repeatable',timeout_ms:3000}]});const executed=call('taskbelay_run_check',{task_id:opened.task.task_id,operation_id:'host-run',revision:opened.task.revision,check_id:'check'});assert.equal(executed.run.status,'passed');assert.equal(executed.run.exit_code,0);assert.equal(executed.view.task.usage.runs,1);
 })
});
