import test from 'node:test';import assert from 'node:assert/strict';import {requestDigest} from '../lib/task-admission.mjs';
test('workspace inspection retains exact request identity without a suitability gate',()=>{assert.equal(requestDigest('authorized request').length,64);assert.notEqual(requestDigest('one'),requestDigest('two'));assert.throws(()=>requestDigest(''));});

import {assessRepositoryFreshness as assessRaw} from '../lib/repository-freshness.mjs';
import {randomUUID} from 'node:crypto';
const assessRepositoryFreshness=async(request,options={})=>assessRaw(request,{receiptRoot:await realpath(dirname(request.repositories[0].repository_path)),...options});
import {mkdtemp,mkdir,writeFile,readFile,rm,realpath,symlink} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {tmpdir} from 'node:os';import {join,dirname} from 'node:path';
const git=(root,...args)=>execFileSync('git',['-C',root,...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
async function remoteFixture(t,name='repo'){
 const root=await mkdtemp(join(tmpdir(),'taskbelay-freshness-'));t.after(()=>rm(root,{recursive:true,force:true}));
 const remote=join(root,'remote'),seed=join(root,'seed'),local=join(root,name);await mkdir(remote);git(remote,'init','--bare','-b','main');git(root,'clone',remote,seed);git(seed,'config','user.name','fixture');git(seed,'config','user.email','fixture@localhost');await writeFile(join(seed,'file.txt'),'base\n');git(seed,'add','.');git(seed,'commit','-m','base');git(seed,'push','origin','main');git(root,'clone',remote,local);git(local,'config','user.name','fixture');git(local,'config','user.email','fixture@localhost');
 return {root,remote,seed,local,head:git(local,'rev-parse','HEAD'),update:async()=>{await writeFile(join(seed,'file.txt'),'remote update\n');git(seed,'commit','-am','remote update');git(seed,'push','origin','main')}};
}
const input=(local,extra={})=>({repositories:[{key:'primary',repository_path:local}],authorization_ref:'actual-fixture-fetch-authority',...(extra.integrate?{integration_authorization_ref:'actual-fixture-integrate-authority',operation_id:randomUUID()}:{}),...extra});
test('fetch detects the corresponding task upstream and separate authorized integration fast-forwards',async t=>{
 const f=await remoteFixture(t);git(f.local,'switch','-c','repair');git(f.local,'branch','--set-upstream-to=origin/main');await f.update();
 const before=await readFile(join(f.local,'file.txt'),'utf8');const report=await assessRepositoryFreshness(input(f.local));assert.equal(report.repositories[0].remote_freshness.state,'behind');assert.equal(report.repositories[0].remote_freshness.upstream,'refs/remotes/origin/main');assert.equal(report.repositories[0].remote_freshness.remote_name,'origin');assert.equal(report.repositories[0].remote_freshness.remote_ref,'refs/heads/main');assert.ok(Date.parse(report.repositories[0].remote_freshness.fetched_at)<=Date.parse(report.repositories[0].remote_freshness.checked_at));assert.equal(git(f.local,'rev-parse','HEAD'),f.head);assert.equal(await readFile(join(f.local,'file.txt'),'utf8'),before);
 const merged=await assessRepositoryFreshness(input(f.local,{integrate:true}));assert.equal(merged.repositories[0].integrated,true);assert.equal(merged.repositories[0].remote_freshness.state,'current');assert.equal(await readFile(join(f.local,'file.txt'),'utf8'),'remote update\n');
});
test('dirty or divergent repositories are fetched but never auto-integrated; multi-root selection is preflighted',async t=>{
 const a=await remoteFixture(t,'a'),b=await remoteFixture(t,'b');await a.update();await b.update();await writeFile(join(b.local,'untracked.txt'),'preserve me');await writeFile(join(b.local,'staged.txt'),'preserve index');git(b.local,'add','staged.txt');const staged=git(b.local,'diff','--cached');
 const request={repositories:[{key:'a',repository_path:a.local},{key:'b',repository_path:b.local}],authorization_ref:'fixture',integration_authorization_ref:'fixture-integrate',operation_id:randomUUID(),integrate:true};const report=await assessRepositoryFreshness(request);assert.equal(report.integration_blocked,true);assert.equal(git(a.local,'rev-parse','HEAD'),a.head);assert.equal(git(b.local,'rev-parse','HEAD'),b.head);assert.equal(await readFile(join(b.local,'untracked.txt'),'utf8'),'preserve me');assert.equal(git(b.local,'diff','--cached'),staged);
 git(a.local,'commit','--allow-empty','-m','local advance');const current=git(a.local,'rev-parse','HEAD');const diverged=await assessRepositoryFreshness(input(a.local,{integrate:true}));assert.equal(diverged.repositories[0].remote_freshness.state,'diverged');assert.equal(git(a.local,'rev-parse','HEAD'),current);
});
test('offline, skipped fetch, detached and absent upstream states are honest and retain work',async t=>{
 const f=await remoteFixture(t);const skipped=await assessRepositoryFreshness(input(f.local,{fetch:false}));assert.equal(skipped.repositories[0].remote_freshness.state,'not_fetched');assert.equal(skipped.repositories[0].remote_freshness.fetched,false);
 git(f.local,'remote','set-url','origin',join(f.root,'missing-remote'));const failed=await assessRepositoryFreshness(input(f.local,{integrate:true}));assert.equal(failed.repositories[0].remote_freshness.state,'fetch_failed');assert.equal(failed.repositories[0].remote_freshness.fetched_at,undefined);assert.equal(git(f.local,'rev-parse','HEAD'),f.head);
 git(f.local,'branch','--unset-upstream');const none=await assessRepositoryFreshness(input(f.local));assert.equal(none.repositories[0].remote_freshness.state,'no_upstream');
 git(f.local,'switch','--detach');const detached=await assessRepositoryFreshness(input(f.local));assert.equal(detached.repositories[0].remote_freshness.state,'detached');
});
test('missing authorization, duplicate roots and unknown fields never issue a fetch or merge',async t=>{
 const f=await remoteFixture(t);await assert.rejects(assessRepositoryFreshness({repositories:[{key:'a',repository_path:f.local}]}),/authorization/);
 await assert.rejects(assessRepositoryFreshness({...input(f.local),integrate:true}),/separate integration authorization/);
 await assert.rejects(assessRepositoryFreshness({...input(f.local),unexpected:true}),/closed/);
 await assert.rejects(assessRepositoryFreshness({repositories:[{key:'a',repository_path:f.local},{key:'b',repository_path:f.local}],authorization_ref:'fixture'}),/duplicate/);
});

test('an actual unresolved merge conflict is preserved during upstream assessment',async t=>{
 const f=await remoteFixture(t);await writeFile(join(f.local,'file.txt'),'local conflicting update\n');git(f.local,'commit','-am','local change');await f.update();git(f.local,'fetch','origin');assert.throws(()=>git(f.local,'merge','origin/main'));const status=git(f.local,'status','--porcelain=v2');const conflict=await readFile(join(f.local,'file.txt'),'utf8');
 const result=await assessRepositoryFreshness(input(f.local,{integrate:true}));assert.equal(result.integration_blocked,true);assert.equal(git(f.local,'status','--porcelain=v2'),status);assert.equal(await readFile(join(f.local,'file.txt'),'utf8'),conflict);
});

test('a task branch without upstream can explicitly recheck its retained development base',async t=>{
 const f=await remoteFixture(t);git(f.local,'switch','-c','repair-without-upstream');await f.update();
 const report=await assessRepositoryFreshness({repositories:[{key:'primary',repository_path:f.local,base_branch:'main'}],authorization_ref:'actual-fixture-fetch'});
 assert.equal(report.repositories[0].remote_freshness.state,'behind');assert.equal(report.repositories[0].remote_freshness.upstream,'refs/remotes/origin/main');assert.equal(report.repositories[0].remote_freshness.remote_name,'origin');assert.equal(report.repositories[0].remote_freshness.remote_ref,'refs/heads/main');assert.ok(Date.parse(report.repositories[0].remote_freshness.fetched_at)<=Date.parse(report.repositories[0].remote_freshness.checked_at));assert.equal(git(f.local,'rev-parse','HEAD'),f.head);
});

import {defaultRunGit} from '../lib/worktree-lifecycle.mjs';
import {spawnSync} from 'node:child_process';

test('bound source stays main even when the task branch gets its own upstream', async t => {
 const f=await remoteFixture(t);git(f.local,'switch','-c','repair-own');git(f.local,'push','-u','origin','repair-own');await f.update();
 const request={repositories:[{key:'primary',repository_path:f.local,source_type:'local',remote_name:'',base_branch:'main'}],authorization_ref:'fixture-fetch'};
 const report=await assessRepositoryFreshness(request);assert.equal(report.repositories[0].remote_freshness.upstream,'refs/remotes/origin/main');assert.equal(report.repositories[0].remote_freshness.state,'behind');assert.equal(git(f.local,'rev-parse','HEAD'),f.head);
});
test('explicit remote release source works without a local release branch', async t => {
 const f=await remoteFixture(t);git(f.seed,'switch','-c','release');git(f.seed,'push','origin','release');git(f.local,'fetch','origin');
 const root=join(f.root,'release-worktree');git(f.local,'worktree','add','--detach',root,'origin/release');git(root,'switch','--no-track','-c','repair-release');assert.throws(()=>git(root,'show-ref','--verify','refs/heads/release'));
 const request={repositories:[{key:'primary',repository_path:root,source_type:'remote',remote_name:'origin',base_branch:'release'}],authorization_ref:'fixture-fetch'};
 const report=await assessRepositoryFreshness(request);assert.equal(report.repositories[0].remote_freshness.upstream,'refs/remotes/origin/release');assert.equal(report.repositories[0].remote_freshness.state,'current');assert.equal(git(root,'rev-parse','HEAD'),f.head);assert.equal(git(root,'branch','--show-current'),'repair-release');
});
test('multi-repository partial success is durable, returned and never replayed',async t=>{
 const a=await remoteFixture(t,'a'),b=await remoteFixture(t,'b');await a.update();await b.update();
 const request=input(a.local,{integrate:true,operation_id:'partial-batch'});request.repositories.push({key:'b',repository_path:b.local});const canonicalA=await realpath(a.local);let merges=0,fetches=0;
 const runGit=async(args,options)=>{
  if(args[2]==='fetch')fetches++;
  if(args[2]==='merge'){
   const intent=JSON.parse(await readFile(join(await realpath(a.root),'freshness','partial-batch.json'),'utf8'));assert.equal(intent.repositories[0].integration.state,'applying');assert.equal(intent.repositories[0].integration.target_head,git(a.local,'rev-parse','origin/main'));merges++;
  }
  const result=await defaultRunGit(args,options);if(args[1]===canonicalA&&args[2]==='merge')await writeFile(join(b.local,'concurrent.txt'),'preserve concurrent work');return result;
 };
 const partial=await assessRepositoryFreshness(request,{runGit});assert.equal(partial.operation.status,'partial');assert.deepEqual(partial.repositories.map(x=>x.integration.state),['applied','blocked']);assert.equal(merges,1);assert.notEqual(git(a.local,'rev-parse','HEAD'),a.head);assert.equal(git(b.local,'rev-parse','HEAD'),b.head);
 const receipt=JSON.parse(await readFile(partial.operation.receipt_path,'utf8'));assert.equal(receipt.repositories[0].integration.state,'applied');assert.equal(receipt.repositories[1].source.head,b.head);assert.equal(receipt.owner_pid,0);
 const before={merges,fetches};const replay=await assessRepositoryFreshness(request,{runGit});assert.equal(replay.operation.replayed,true);assert.equal(replay.operation.status,'partial');await assessRepositoryFreshness({...request,recover:true},{runGit});assert.deepEqual({merges,fetches},before);assert.equal(await readFile(join(b.local,'concurrent.txt'),'utf8'),'preserve concurrent work');
 await assert.rejects(assessRepositoryFreshness({...request,integration_authorization_ref:'changed-request'},{runGit}),/identity\/request/);assert.equal(merges,1);
});
test('actual child exit after merge leaves an intent; stopped-owner recovery is read-only and repeatable',async t=>{
 const f=await remoteFixture(t);await f.update();const request=input(f.local,{integrate:true,operation_id:'crash-after-merge'});const receiptRoot=await realpath(f.root);const moduleURL=new URL('../lib/repository-freshness.mjs',import.meta.url).href,gitURL=new URL('../lib/worktree-lifecycle.mjs',import.meta.url).href;
 const code=`import {assessRepositoryFreshness} from ${JSON.stringify(moduleURL)};import {defaultRunGit} from ${JSON.stringify(gitURL)};await assessRepositoryFreshness(${JSON.stringify(request)},{receiptRoot:${JSON.stringify(receiptRoot)},runGit:async(a,o)=>{const r=await defaultRunGit(a,o);if(a[2]==='merge')process.exit(42);return r}});`;
 const child=spawnSync(process.execPath,['--input-type=module','--eval',code],{encoding:'utf8',windowsHide:true});assert.equal(child.status,42,child.stderr);const target=git(f.local,'rev-parse','HEAD');assert.notEqual(target,f.head);
 const path=join(receiptRoot,'freshness','crash-after-merge.json');const original=JSON.parse(await readFile(path,'utf8'));assert.equal(original.repositories[0].integration.state,'applying');assert.equal(original.repositories[0].integration.target_head,target);
 let mutations=0;const runGit=async(a,o)=>{if(['fetch','merge','switch','reset'].includes(a[2])){mutations++;throw new Error('recovery attempted a Git mutation')}return await defaultRunGit(a,o)};
 const replay=await assessRepositoryFreshness(request,{runGit});assert.equal(replay.operation.status,'integrating');assert.equal(mutations,0);
 const recovered=await assessRepositoryFreshness({...request,recover:true},{runGit});assert.equal(recovered.operation.status,'reconciled');assert.equal(recovered.integration_blocked,true);assert.equal(recovered.repositories[0].integration.state,'unknown');assert.equal(recovered.repositories[0].integration.recovery.observation,'target_present');assert.equal(recovered.repositories[0].remote_freshness.fetched,false);assert.equal(git(f.local,'rev-parse','HEAD'),target);
 const again=await assessRepositoryFreshness({...request,recover:true},{runGit});assert.deepEqual(again,recovered);assert.equal(mutations,0);
});
test('unknown result with concurrent user work is retained; frozen target and new identity are not guessed',async t=>{
 const f=await remoteFixture(t);await f.update();const request=input(f.local,{integrate:true,operation_id:'unknown-drift'});const receiptRoot=await realpath(f.root);const moduleURL=new URL('../lib/repository-freshness.mjs',import.meta.url).href,gitURL=new URL('../lib/worktree-lifecycle.mjs',import.meta.url).href;
 const child=spawnSync(process.execPath,['--input-type=module','--eval',`import{assessRepositoryFreshness}from${JSON.stringify(moduleURL)};import{defaultRunGit}from${JSON.stringify(gitURL)};await assessRepositoryFreshness(${JSON.stringify(request)},{receiptRoot:${JSON.stringify(receiptRoot)},runGit:async(a,o)=>{const r=await defaultRunGit(a,o);if(a[2]==='merge')process.exit(43);return r}})`],{encoding:'utf8'});assert.equal(child.status,43,child.stderr);
 const head=git(f.local,'rev-parse','HEAD');await writeFile(join(f.local,'user-work.txt'),'retain after unknown result');await writeFile(join(f.seed,'file.txt'),'later remote update');git(f.seed,'commit','-am','later');git(f.seed,'push','origin','main');git(f.local,'fetch','origin');const latest=git(f.local,'rev-parse','origin/main');assert.notEqual(latest,head);
 const recovered=await assessRepositoryFreshness({...request,recover:true});assert.equal(recovered.repositories[0].integration.target_head,head);assert.equal(recovered.repositories[0].integration.recovery.observation,'workspace_drift');assert.equal(recovered.repositories[0].integration.state,'unknown');assert.equal(git(f.local,'rev-parse','HEAD'),head);assert.equal(await readFile(join(f.local,'user-work.txt'),'utf8'),'retain after unknown result');
});
test('a live integration owner and a concurrent claim cannot be recovered or duplicated',async t=>{
 const f=await remoteFixture(t);await f.update();const request=input(f.local,{integrate:true,operation_id:'live-owner'});let attempted,continueMerge;const waiting=new Promise(resolve=>attempted=resolve);const gate=new Promise(resolve=>continueMerge=resolve);let merges=0;
 const operation=assessRepositoryFreshness(request,{runGit:async(a,o)=>{if(a[2]==='merge'){merges++;attempted();await gate}return await defaultRunGit(a,o)}});await waiting;
 try {await assert.rejects(assessRepositoryFreshness({...request,recover:true}),/still running/);const read=await assessRepositoryFreshness(request);assert.equal(read.operation.replayed,true);assert.equal(read.operation.status,'integrating');assert.equal(merges,1);}finally{continueMerge()}
 const done=await operation;assert.equal(done.operation.status,'complete');assert.equal(merges,1);
});
test('invalid, changed or symbolic receipt paths stop before another Git mutation',async t=>{
 const f=await remoteFixture(t);await f.update();const root=await realpath(f.root);const request=input(f.local,{integrate:true,operation_id:'invalid-record'});await mkdir(join(root,'freshness'));await writeFile(join(root,'freshness','invalid-record.json'),'{}');let mutations=0;const runGit=async(a,o)=>{if(['fetch','merge'].includes(a[2]))mutations++;return await defaultRunGit(a,o)};
 await assert.rejects(assessRepositoryFreshness(request,{runGit}),/identity/);await rm(join(root,'freshness','invalid-record.json'));await symlink(join(root,'missing.json'),join(root,'freshness','invalid-record.json'));await assert.rejects(assessRepositoryFreshness(request,{runGit}),/regular/);assert.equal(mutations,0);assert.equal(git(f.local,'rev-parse','HEAD'),f.head);
 await assert.rejects(assessRepositoryFreshness(input(f.local,{integrate:true,operation_id:undefined})),/stable operation_id/);
});

test('a deleted local source is not replaced by a similarly prefixed branch',async t=>{
 const f=await remoteFixture(t);git(f.local,'switch','-c','repair');git(f.local,'branch','-D','main');git(f.local,'branch','main/other');git(f.local,'branch','--set-upstream-to=origin/main','main/other');
 const report=await assessRepositoryFreshness({repositories:[{key:'primary',repository_path:f.local,source_type:'local',remote_name:'',base_branch:'main'}],authorization_ref:'fixture-fetch'});assert.equal(report.repositories[0].remote_freshness.state,'no_upstream');assert.equal(report.repositories[0].remote_freshness.fetched,false);
});

test('an actual second Git command failure retains first success, original error and user commit',async t=>{
 const a=await remoteFixture(t,'a'),b=await remoteFixture(t,'b');await a.update();await b.update();const request=input(a.local,{integrate:true,operation_id:'second-command-fails'});request.repositories.push({key:'b',repository_path:b.local});const canonicalB=await realpath(b.local);let userHead,failureStderr;
 const report=await assessRepositoryFreshness(request,{runGit:async(args,options)=>{if(args[1]===canonicalB&&args[2]==='merge'){git(b.local,'commit','--allow-empty','-m','concurrent user commit');userHead=git(b.local,'rev-parse','HEAD')}try{return await defaultRunGit(args,options)}catch(error){failureStderr=error.stderr;throw error}}});
 assert.equal(report.operation.status,'uncertain');assert.equal(report.repositories[0].integration.state,'applied');assert.equal(report.repositories[1].integration.state,'unknown');assert.ok(typeof failureStderr==='string'&&failureStderr.length>0);assert.equal(report.repositories[1].integration.failure.stderr,failureStderr);assert.equal(git(b.local,'rev-parse','HEAD'),userHead);assert.notEqual(git(a.local,'rev-parse','HEAD'),a.head);
 const read=JSON.parse(await readFile(report.operation.receipt_path,'utf8'));assert.equal(read.repositories[0].integration.state,'applied');assert.equal(read.repositories[1].integration.failure.code,128);
});

for(const stopBefore of ['fetch','merge'])test(`actual child exit before ${stopBefore} is retained as unknown without replay`,async t=>{
 const f=await remoteFixture(t);await f.update();const request=input(f.local,{integrate:true,operation_id:'crash-before-'+stopBefore});const receiptRoot=await realpath(f.root);const moduleURL=new URL('../lib/repository-freshness.mjs',import.meta.url).href,gitURL=new URL('../lib/worktree-lifecycle.mjs',import.meta.url).href;
 const code=`import{assessRepositoryFreshness}from${JSON.stringify(moduleURL)};import{defaultRunGit}from${JSON.stringify(gitURL)};await assessRepositoryFreshness(${JSON.stringify(request)},{receiptRoot:${JSON.stringify(receiptRoot)},runGit:async(a,o)=>{if(a[2]===${JSON.stringify(stopBefore)})process.exit(44);return await defaultRunGit(a,o)}})`;
 const child=spawnSync(process.execPath,['--input-type=module','--eval',code],{encoding:'utf8'});assert.equal(child.status,44,child.stderr);assert.equal(git(f.local,'rev-parse','HEAD'),f.head);
 let changes=0;const runGit=async(a,o)=>{if(['merge','fetch'].includes(a[2])){changes++;throw new Error('recovery repeated mutation')}return await defaultRunGit(a,o)};
 const recovered=await assessRepositoryFreshness({...request,recover:true},{runGit});assert.equal(recovered.operation.status,'reconciled');assert.equal(recovered.repositories[0].integration.recovery.observation,'original_state_present');if(stopBefore==='merge')assert.equal(recovered.repositories[0].integration.state,'unknown');assert.equal(recovered.repositories[0].remote_freshness.fetched,false);assert.equal(changes,0);assert.equal(git(f.local,'rev-parse','HEAD'),f.head);
});

test('an actual recovery child exit leaves a fenced lock; a stopped owner can be recovered again without mutation',async t=>{
 const f=await remoteFixture(t);await f.update();const request=input(f.local,{integrate:true,operation_id:'recovery-interrupted'});const root=await realpath(f.root);const moduleURL=new URL('../lib/repository-freshness.mjs',import.meta.url).href,gitURL=new URL('../lib/worktree-lifecycle.mjs',import.meta.url).href;const path=join(root,'freshness','recovery-interrupted.json');
 const imports=`import{assessRepositoryFreshness}from${JSON.stringify(moduleURL)};import{defaultRunGit}from${JSON.stringify(gitURL)};import{existsSync}from'node:fs';`;
 const first=spawnSync(process.execPath,['--input-type=module','--eval',imports+`await assessRepositoryFreshness(${JSON.stringify(request)},{receiptRoot:${JSON.stringify(root)},runGit:async(a,o)=>{const r=await defaultRunGit(a,o);if(a[2]==='merge')process.exit(42);return r}})`],{encoding:'utf8'});assert.equal(first.status,42,first.stderr);
 const second=spawnSync(process.execPath,['--input-type=module','--eval',imports+`await assessRepositoryFreshness(${JSON.stringify({...request,recover:true})},{receiptRoot:${JSON.stringify(root)},runGit:async(a,o)=>{if(existsSync(${JSON.stringify(path+'.recovery.lock')}))process.exit(45);return await defaultRunGit(a,o)}})`],{encoding:'utf8'});assert.equal(second.status,45,second.stderr);
 const retainedLock=JSON.parse(await readFile(path+'.recovery.lock','utf8'));assert.equal(retainedLock.pid,second.pid);assert.equal(typeof retainedLock.token,'string');
 let mutations=0;const runGit=async(a,o)=>{if(['fetch','merge','switch','reset'].includes(a[2])){mutations++;throw new Error('recovery mutated Git')}return await defaultRunGit(a,o)};
 const recovered=await assessRepositoryFreshness({...request,recover:true},{runGit});assert.equal(recovered.operation.status,'reconciled');assert.equal(recovered.repositories[0].integration.state,'unknown');assert.equal(recovered.repositories[0].integration.recovery.observation,'target_present');assert.equal(mutations,0);await assert.rejects(readFile(path+'.recovery.lock'),{code:'ENOENT'});
 const again=await assessRepositoryFreshness({...request,recover:true},{runGit});assert.deepEqual(again,recovered);assert.equal(mutations,0);
});
test('live, unknown and replaced recovery locks are retained rather than cleared by age',async t=>{
 const f=await remoteFixture(t);await f.update();const request=input(f.local,{integrate:true,operation_id:'recovery-lock-guards'});const root=await realpath(f.root);const moduleURL=new URL('../lib/repository-freshness.mjs',import.meta.url).href,gitURL=new URL('../lib/worktree-lifecycle.mjs',import.meta.url).href;
 const child=spawnSync(process.execPath,['--input-type=module','--eval',`import{assessRepositoryFreshness}from${JSON.stringify(moduleURL)};import{defaultRunGit}from${JSON.stringify(gitURL)};await assessRepositoryFreshness(${JSON.stringify(request)},{receiptRoot:${JSON.stringify(root)},runGit:async(a,o)=>{const r=await defaultRunGit(a,o);if(a[2]==='merge')process.exit(42);return r}})`],{encoding:'utf8'});assert.equal(child.status,42,child.stderr);
 const lock=join(root,'freshness','recovery-lock-guards.json.recovery.lock');const live=JSON.stringify({pid:process.pid,token:randomUUID()});await writeFile(lock,live,{mode:0o600});await assert.rejects(assessRepositoryFreshness({...request,recover:true}),/still running/);assert.equal(await readFile(lock,'utf8'),live);
 await writeFile(lock,'{}');await assert.rejects(assessRepositoryFreshness({...request,recover:true}),/unknown/);assert.equal(await readFile(lock,'utf8'),'{}');await rm(lock);
 let replaced=false;const foreign=JSON.stringify({pid:process.pid,token:randomUUID()});await assert.rejects(assessRepositoryFreshness({...request,recover:true},{runGit:async(a,o)=>{if(!replaced){try{await readFile(lock);await rm(lock);await writeFile(lock,foreign,{mode:0o600});replaced=true}catch(e){if(e.code!=='ENOENT')throw e}}return await defaultRunGit(a,o)}}),/lock changed/);assert.equal(replaced,true);assert.equal(await readFile(lock,'utf8'),foreign);
});
