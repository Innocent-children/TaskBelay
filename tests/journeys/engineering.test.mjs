import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { core, fixture, check } from './fixtures/engineering.mjs';

const goal = (id = 'goal') => ({ id, statement: 'Requested behavior passes its actual assertion.' });

test('existing user authority starts work and failed acceptance cannot be presented as complete', t => {
  const f = fixture(t, { primary: { 'value.cjs': 'module.exports=1;\n', 'check.cjs': "require('node:assert/strict').equal(require('./value.cjs'),2);\n" } });
  f.open([goal()], [check('check', 'check.cjs')]);
  assert.equal(f.view.decision.advice, 'continue');
  const failure = f.run('check').run;
  assert.equal(failure.status, 'failed');
  assert.equal(f.view.task.usage.runs, 1);
  f.call('taskbelay_accept_known_failure', {...f.identity(),check_id:'check',failed_evidence_id:failure.run_id,comparison_evidence_id:failure.run_id,failures:['required-assertion'],user_ref:'fixture-user',reason:'Attempt to waive required acceptance'}, 'FAILURE_ACCEPTANCE_NOT_ALLOWED');
  f.checkpoint({checks: []}, 'INVALID_ARGUMENT');
  f.finish('ACCEPTANCE_INCOMPLETE');
  const authorization = structuredClone(f.view.task.authorization);
  authorization.goals[0].statement = 'A different requested outcome';
  f.call('taskbelay_revise_task', {...f.identity(),authorization,reason:'No actual user requested this change'}, 'AUTHORIZATION_REQUIRED');
  f.write('value.cjs', 'module.exports=2;\n');
  assert.equal(f.run('check').run.status, 'passed');
  f.review(); f.finish();
});

test('adding a check preserves independent results and completed goal progress', t => {
  const f = fixture(t, { primary: {'unit.cjs':'process.exit(0);\n','integration.cjs':'process.exit(0);\n'} });
  f.open([goal('goal')], [check('unit','unit.cjs')]);
  const unit = f.run('unit').run;
  f.checkpoint({progress:[{goal_id:'goal',status:'done',summary:'Implementation is complete; adding relevant verification.'}]});
  const authorization = structuredClone(f.view.task.authorization);
  f.revise(authorization, [...f.view.task.plan.checks, check('integration','integration.cjs')]);
  assert.equal(f.view.task.progress[0].status,'done');
  assert.equal(f.view.decision.evidence.find(e=>e.evidence_id===unit.run_id).valid,true);
  assert.deepEqual(f.view.decision.missing_checks,['integration']);
  assert.equal(f.view.decision.advice,'verify');
  f.run('integration');f.review();f.finish();
});

test('a user changes one outcome while independent progress and verification survive', t => {
  const f = fixture(t, {primary:{
    'value.cjs':'module.exports=1;\n',
    'value-test.cjs':"require('node:assert/strict').equal(require('./value.cjs'),1);\n",
    'format-test.cjs':"require('node:assert/strict').equal(' example '.trim(),'example');\n",
  }});
  f.open([
    {id:'value',statement:'Return one'},
    {id:'format',statement:'Trim surrounding spaces'},
  ],[
    check('value','value-test.cjs',{goal_ids:['value'],inputs:['value.cjs','value-test.cjs'],input_basis:'This local check imports only the value module.'}),
    check('format','format-test.cjs',{goal_ids:['format'],inputs:['format-test.cjs'],input_basis:'This independent assertion uses Node built-ins only.'}),
  ]);
  const value=f.run('value').run, format=f.run('format').run;
  f.review(); assert.equal(f.view.decision.ready,true);
  const authorization=structuredClone(f.view.task.authorization);
  authorization.user_ref='scripted-user-changes-value-outcome';
  authorization.goals[0].statement='Return two';
  f.call('taskbelay_revise_task',{...f.identity(),authorization,reason:'The user changed the value requirement and retained formatting.'});
  assert.deepEqual(f.view.task.progress.map(p=>p.goal_id),['format']);
  assert.equal(f.view.decision.evidence.find(e=>e.evidence_id===value.run_id).valid,false);
  assert.equal(f.view.decision.evidence.find(e=>e.evidence_id===format.run_id).valid,true);
  assert.equal(f.view.task.usage.runs,2);
  f.write('value.cjs','module.exports=2;\n');
  f.write('value-test.cjs',"require('node:assert/strict').equal(require('./value.cjs'),2);\n");
  f.run('value'); f.review(); f.finish();
  assert.equal(f.view.task.usage.runs,3);
});

test('actual provenance, scope review and stopping after a pass', t => {
  const f = fixture(t, { primary: { 'value.cjs': 'module.exports=1;\n', 'check.cjs': "require('node:assert/strict').equal(require('./value.cjs'),2);\n", 'README.md': 'base\n' } });
  f.open([goal()], [check('check', 'check.cjs', { inputs: ['value.cjs', 'check.cjs'], input_basis: 'Only local module and Node assertions are used.' })], { allowed_paths: ['value.cjs', 'README.md'], constraints: [{ id: 'scope', kind: 'non_goal', statement: 'Do not add side effects.', paths: ['value.cjs'] }] });
  f.call('taskbelay_record_evidence', { ...f.identity(), check_id: 'check', source: 'claim', status: 'passed', summary: 'Fixture agent claims success.', content_digest: f.view.content_digest });
  f.inspect(); assert.ok(f.view.decision.missing_checks.includes('check'));
  assert.equal(f.run('check').run.status, 'failed'); f.write('value.cjs', 'module.exports=2;\n');
  const request = { ...f.identity(), check_id: 'check', rationale: 'Verify the corrected implementation.' };
  const passed = f.call('check.run', request); assert.equal(passed.run.status, 'passed');
  const replay = f.call('check.run', request); assert.equal(replay.view.replayed, true); assert.equal(replay.view.task.usage.runs, 2);
  f.run('check', {}, 'CHECK_ALREADY_SATISFIED');
  f.write('README.md', 'documented\n'); f.inspect(); assert.equal(f.view.decision.evidence.find(e => e.evidence_id === passed.run.run_id).valid, true);
  f.review('violation'); assert.equal(f.view.decision.ready, false);
  f.write('extra.cjs', 'module.exports=3;\n'); f.inspect(); f.run('check', {}, 'SCOPE_VIOLATION');
  f.call('taskbelay_cancel_task', { ...f.identity(), reason: 'Preserve the detected unrelated fixture change for inspection.' });
});

test('default evidence covers all repositories while justified independent evidence is retained', t => {
  const f = fixture(t, { client: { 'check.cjs': "require('node:assert/strict').equal(require('../server/value.cjs'),1);\n", 'independent.cjs': 'process.exit(0);\n' }, server: { 'value.cjs': 'module.exports=1;\n' } });
  f.open([goal('goal')], [check('integration', 'check.cjs', { repository_key: 'client' }), check('independent', 'independent.cjs', { repository_key: 'client', inputs: ['client::independent.cjs'], input_basis: 'The independent command uses no server code or data.' })]);
  const integration = f.run('integration').run, independent = f.run('independent').run;
  f.write('value.cjs', 'module.exports=2;\n', 'server'); f.inspect();
  assert.equal(f.view.decision.evidence.find(e => e.evidence_id === integration.run_id).valid, false);
  assert.equal(f.view.decision.evidence.find(e => e.evidence_id === independent.run_id).valid, true);
  f.review(); f.finish('ACCEPTANCE_INCOMPLETE'); assert.equal(f.run('integration').run.status, 'failed');
  f.write('value.cjs', 'module.exports=1;\n', 'server'); f.inspect();
  assert.equal(f.view.decision.evidence.find(e => e.evidence_id === integration.run_id).valid, true);
  assert.ok(f.call('taskbelay_get_task',{task_id:f.view.task.task_id,detail:true}).details.task.attempts.some(a => a.status === 'failed')); f.review(); f.finish(); assert.equal(f.view.task.status, 'done');
});

test('passing evidence survives execution-setting revisions and a renamed check can establish its own result', t => {
  const f = fixture(t);
  f.open([goal()], [check('check', 'check.cjs')]);
  const passed = f.run('check').run;
  const checks = structuredClone(f.view.task.plan.checks);
  checks[0].timeout_ms = 6000; checks[0].rationale = 'Clarified verification rationale.';
  f.revise(f.view.task.authorization, checks);
  f.inspect();
  assert.equal(f.view.decision.evidence.find(e => e.evidence_id === passed.run_id).valid, true);
  f.run('check', {}, 'CHECK_ALREADY_SATISFIED');
  f.review(); assert.equal(f.view.decision.ready, true);
  const authorization = structuredClone(f.view.task.authorization);
  checks[0].id = 'renamed';
  f.revise(authorization, checks);
  assert.ok(f.view.decision.missing_checks.includes('renamed'));
  assert.equal(f.run('renamed').run.status, 'passed');
  assert.equal(f.view.task.usage.runs, 2);
  f.review(); f.finish();
});

test('unchanged failure retries need information and labels cannot reset their count', t => {
  const f = fixture(t, { primary: { 'check.cjs': 'process.exit(1);\n' } });
  f.open([goal()], [check('check', 'check.cjs', { full_suite: true })]);
  f.run('check', {}, 'INVALID_ARGUMENT');
  let failed = f.run('check', { full_suite_basis: 'Test the explicit full-suite boundary.' }).run;
  f.run('check', { full_suite_basis: 'Same boundary.' }, 'RETRY_BASIS_REQUIRED');
  for (let n = 1; n <= 2; n++) failed = f.run('check', { full_suite_basis: 'Investigate this suite result.', previous_run_id: failed.run_id, new_information: `Fixture diagnostic observation ${n}.` }).run;
  const duplicates=structuredClone(f.view.task.plan.checks);duplicates[0].goal_ids=['goal','goal'];
  f.call('taskbelay_revise_task',{...f.identity(),authorization:f.view.task.authorization,plan:f.view.task.plan,checks:duplicates,reason:'Try duplicate bookkeeping references'},'INVALID_ARGUMENT');
  const a = structuredClone(f.view.task.authorization), checks = structuredClone(f.view.task.plan.checks);
  checks[0].id = 'renamed'; checks[0].rationale = 'Changed display label';
  f.revise(a, checks);
  f.run('renamed', { full_suite_basis: 'Another repetition', previous_run_id: failed.run_id, new_information: 'Labels do not change actual inputs.' }, 'REPEATED_FAILURE');
  assert.equal(f.view.task.usage.runs, 3);
});

test('known failure acceptance retains failed provenance and expires on changed content', t => {
  const f = fixture(t, { primary: { 'regression.cjs': 'process.exit(1);\n', 'compare.cjs': 'process.exit(0);\n', 'feature.cjs': 'process.exit(0);\n' } });
  f.open([goal('goal')], [check('feature', 'feature.cjs'), check('regression', 'regression.cjs', { purpose: 'regression' }), check('compare', 'compare.cjs', { purpose: 'diagnostic', goal_ids: ['goal'], compares: 'regression' })]);
  f.run('feature'); const failed = f.run('regression').run, comparison = f.run('compare').run; f.review(); f.finish('ACCEPTANCE_INCOMPLETE');
  f.call('taskbelay_accept_known_failure', { ...f.identity(), check_id: 'regression', failed_evidence_id: failed.run_id, comparison_evidence_id: comparison.run_id, failures: ['fixture-existing-failure'], user_ref: 'fixture-user-accepts-existing-failure', reason: 'The approved comparison reports no new failures; this is scripted semantic input.' });
  assert.equal(f.view.decision.accepted_failures.length, 1); assert.equal(f.view.task.evidence.find(e => e.id === failed.run_id).status, 'failed');
  assert.equal(f.view.decision.evidence.find(e => e.evidence_id === failed.run_id).valid, false);
  assert.equal(f.view.decision.ready, true);
  f.call('taskbelay_revise_task',{...f.identity(),plan:{...f.view.task.plan,approach:'Clarify the existing implementation approach.'},reason:'Routine method clarification.'});
  assert.equal(f.view.decision.accepted_failures.length,1);
  f.write('feature.cjs', 'process.exit(0); // changed\n'); f.inspect();
  assert.equal(f.view.decision.accepted_failures.length, 0); f.finish('ACCEPTANCE_INCOMPLETE');
});

test('interrupted real execution is recovered without a passing claim or cost reset', async t => {
  const f = fixture(t, { primary: { 'slow.cjs': "require('node:fs').writeFileSync('started.marker','started');setTimeout(()=>process.exit(0),1500);\n" } });
  f.open([goal()], [check('check', 'slow.cjs', { timeout_ms: 5000 })]);
  const child = spawn(core, ['check','run'], { env: f.env, stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
  child.stdin.end(JSON.stringify({ ...f.identity(), check_id: 'check', rationale: 'Verify actual process interruption and recovery.' }));
  const marker = join(f.repositories[0].repository_path, 'started.marker'), deadline = Date.now() + 10000;
  while (!existsSync(marker) && Date.now() < deadline) await new Promise(r => setTimeout(r, 20));
  assert.ok(existsSync(marker)); const exited = new Promise(r => child.once('exit', r)); child.kill('SIGKILL'); await exited;
  f.inspect(); const id = f.view.decision.pending_runs[0]; assert.ok(id); f.run('check', {}, 'EXECUTION_PENDING');
  const pending = f.call('taskbelay_get_run', {run_id:id});
  assert.ok(pending.process?.pid > 0);
  if (process.platform === 'darwin') {
    f.call('taskbelay_recover_run', { ...f.identity(), run_id:id, resolution_basis:'The Core stopped; the command is still alive.', user_ref:'' }, 'EXECUTION_RUNNING');
    f.call('taskbelay_cancel_task', {...f.identity(),reason:'Cannot release a running process.'}, 'EXECUTION_PENDING');
    f.finish('ACCEPTANCE_INCOMPLETE');
    await new Promise(resolve => setTimeout(resolve, 1700));
  }
  f.call('taskbelay_recover_run', { ...f.identity(), run_id: id, resolution_basis: 'Stopped owner only wrote the ignored fixture marker.', user_ref: '' });
  assert.equal(f.view.task.usage.uncertain_ms, 5000); assert.equal(f.view.task.usage.runs, 1);
  const result = f.run('check', { previous_run_id: id, new_information: 'The stopped execution was inspected and its repeatable operation is safe.' });
  assert.equal(result.run.status, 'passed'); assert.equal(f.view.task.usage.runs, 2); f.review(); f.finish();
  if (process.env.ENGINEERING_REPORT) writeFileSync(process.env.ENGINEERING_REPORT, JSON.stringify({ platform: process.platform, actual_process_interruption: true, cumulative_cost: f.view.task.usage, limitations: ['Scripted authorization and semantic reviews; not a native Host conversation.'] }, null, 2) + '\n');
});

test('macOS recovery retains descendants after the command leader and Core exit', {skip:process.platform !== 'darwin'}, async t => {
  const f = fixture(t, {primary:{
    'parent.cjs': "const {spawn}=require('node:child_process');const fs=require('node:fs');const child=spawn(process.execPath,['descendant.cjs'],{stdio:'ignore'});fs.writeFileSync('child.marker',String(child.pid));setTimeout(()=>process.exit(0),500);\n",
    'descendant.cjs': "setTimeout(()=>{require('node:fs').writeFileSync('finished.marker','done');process.exit(0)},4000);\n",
  }});
  f.open([goal()], [check('check','parent.cjs',{timeout_ms:6000})]);
  const child = spawn(core,['check','run'],{env:f.env,stdio:['pipe','ignore','ignore']});
  child.stdin.end(JSON.stringify({...f.identity(),check_id:'check',rationale:'Check descendant recovery after forced owner loss.'}));
  const marker=join(f.repositories[0].repository_path,'child.marker');
  const deadline=Date.now()+10000;
  while(!existsSync(marker)&&Date.now()<deadline) await new Promise(r=>setTimeout(r,20));
  assert.ok(existsSync(marker));
  const descendant=Number(readFileSync(marker,'utf8'));
  t.after(()=>{try{process.kill(descendant,'SIGKILL')}catch{}});
  const exited=new Promise(r=>child.once('exit',r));child.kill('SIGKILL');await exited;
  f.inspect();const id=f.view.decision.pending_runs[0];assert.ok(id);
  await new Promise(r=>setTimeout(r,700));
  const request=()=>({...f.identity(),run_id:id,resolution_basis:'Inspect the retained command group.',user_ref:''});
  f.call('taskbelay_recover_run',request(),'EXECUTION_RUNNING');
  assert.deepEqual(f.view.decision.pending_runs,[id]);
  const finished=join(f.repositories[0].repository_path,'finished.marker');
  while(!existsSync(finished)&&Date.now()<deadline) await new Promise(r=>setTimeout(r,20));
  assert.ok(existsSync(finished));
  await new Promise(r=>setTimeout(r,100));
  const result=f.call('taskbelay_recover_run',request());
  assert.equal(result.run.status,'interrupted_unknown');
  assert.deepEqual(f.view.decision.pending_runs,[]);
});
