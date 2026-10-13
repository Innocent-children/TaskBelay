import test from 'node:test';import assert from 'node:assert/strict';import {requestDigest} from '../lib/task-admission.mjs';
test('workspace inspection retains exact request identity without a suitability gate',()=>{assert.equal(requestDigest('authorized request').length,64);assert.notEqual(requestDigest('one'),requestDigest('two'));assert.throws(()=>requestDigest(''));});

import {assessRepositoryFreshness} from '../lib/repository-freshness.mjs';
import {mkdtemp,mkdir,writeFile,readFile,rm} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {tmpdir} from 'node:os';import {join} from 'node:path';
const git=(root,...args)=>execFileSync('git',['-C',root,...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
async function remoteFixture(t,name='repo'){
 const root=await mkdtemp(join(tmpdir(),'taskbelay-freshness-'));t.after(()=>rm(root,{recursive:true,force:true}));
 const remote=join(root,'remote'),seed=join(root,'seed'),local=join(root,name);await mkdir(remote);git(remote,'init','--bare','-b','main');git(root,'clone',remote,seed);git(seed,'config','user.name','fixture');git(seed,'config','user.email','fixture@localhost');await writeFile(join(seed,'file.txt'),'base\n');git(seed,'add','.');git(seed,'commit','-m','base');git(seed,'push','origin','main');git(root,'clone',remote,local);git(local,'config','user.name','fixture');git(local,'config','user.email','fixture@localhost');
 return {root,remote,seed,local,head:git(local,'rev-parse','HEAD'),update:async()=>{await writeFile(join(seed,'file.txt'),'remote update\n');git(seed,'commit','-am','remote update');git(seed,'push','origin','main')}};
}
const input=(local,extra={})=>({repositories:[{key:'primary',repository_path:local}],authorization_ref:'actual-fixture-fetch-authority',...(extra.integrate?{integration_authorization_ref:'actual-fixture-integrate-authority'}:{}),...extra});
test('fetch detects the corresponding task upstream and separate authorized integration fast-forwards',async t=>{
 const f=await remoteFixture(t);git(f.local,'switch','-c','repair');git(f.local,'branch','--set-upstream-to=origin/main');await f.update();
 const before=await readFile(join(f.local,'file.txt'),'utf8');const report=await assessRepositoryFreshness(input(f.local));assert.equal(report.repositories[0].remote_freshness.state,'behind');assert.equal(report.repositories[0].remote_freshness.upstream,'refs/remotes/origin/main');assert.equal(report.repositories[0].remote_freshness.remote_name,'origin');assert.equal(report.repositories[0].remote_freshness.remote_ref,'refs/heads/main');assert.ok(Date.parse(report.repositories[0].remote_freshness.fetched_at)<=Date.parse(report.repositories[0].remote_freshness.checked_at));assert.equal(git(f.local,'rev-parse','HEAD'),f.head);assert.equal(await readFile(join(f.local,'file.txt'),'utf8'),before);
 const merged=await assessRepositoryFreshness(input(f.local,{integrate:true}));assert.equal(merged.repositories[0].integrated,true);assert.equal(merged.repositories[0].remote_freshness.state,'current');assert.equal(await readFile(join(f.local,'file.txt'),'utf8'),'remote update\n');
});
test('dirty or divergent repositories are fetched but never auto-integrated; multi-root selection is preflighted',async t=>{
 const a=await remoteFixture(t,'a'),b=await remoteFixture(t,'b');await a.update();await b.update();await writeFile(join(b.local,'untracked.txt'),'preserve me');await writeFile(join(b.local,'staged.txt'),'preserve index');git(b.local,'add','staged.txt');const staged=git(b.local,'diff','--cached');
 const request={repositories:[{key:'a',repository_path:a.local},{key:'b',repository_path:b.local}],authorization_ref:'fixture',integration_authorization_ref:'fixture-integrate',integrate:true};const report=await assessRepositoryFreshness(request);assert.equal(report.integration_blocked,true);assert.equal(git(a.local,'rev-parse','HEAD'),a.head);assert.equal(git(b.local,'rev-parse','HEAD'),b.head);assert.equal(await readFile(join(b.local,'untracked.txt'),'utf8'),'preserve me');assert.equal(git(b.local,'diff','--cached'),staged);
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
