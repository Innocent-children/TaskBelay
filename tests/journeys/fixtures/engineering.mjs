import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve, relative, isAbsolute } from 'node:path';

export const core = process.env.ENGINEERING_CORE ?? resolve('.evaluation', process.platform === 'win32' ? 'taskbelay.exe' : 'taskbelay');
export function planFor(goals) {
  return {
    understanding: 'Deliver the named goals and preserve unrelated behavior.',
    root_cause: 'The existing behavior must meet the requested outcome.',
    approach: 'Change the existing implementation only where required.',
    simpler_alternative: 'No framework, dependency or compatibility layer is needed.',
    verification_basis: 'Use local assertions and their actual dependencies, without an unrelated matrix.',
    completion: 'Current evidence covers all goals and the actual diff is reviewed.',
    work: [{ id: 'work', goal_ids: goals.map(g => g.id), change: 'Implement and verify the requested behavior.', owner: 'Existing fixture module.', necessity: 'These changes establish the requested outcome.' }],
  };
}
export function check(id, script, extra = {}) {
  return { id, kind: 'command', argv: [process.execPath, script], rationale: 'Exercise the requested behavior.', conditions: 'Local Node and fixture inputs; no external services.', retry: 'repeatable', timeout_ms: 3000, ...extra };
}
export function fixture(t, files = { primary: { 'check.cjs': 'process.exit(0);\n' } }) {
  const root = mkdtempSync(join(tmpdir(), 'taskbelay-approved-'));
  t.after(() => {
    const scope = relative(resolve(tmpdir()), root);
    assert.ok(scope && !scope.startsWith('..') && !isAbsolute(scope));
    rmSync(root, { recursive: true, force: true });
  });
  const data = join(root, 'data'); mkdirSync(data);
  const env = { ...process.env, TASKBELAY_DATA_DIR: data };
  const repositories = Object.entries(files).map(([key, entries]) => {
    const path = join(root, key); mkdirSync(path);
    const git = (...args) => {
      const r = spawnSync('git', ['-C', path, ...args], { encoding: 'utf8', windowsHide: true });
      assert.equal(r.status, 0, r.stderr);
    };
    git('init', '-b', 'main'); git('config', 'user.name', 'TaskBelay fixture'); git('config', 'user.email', 'fixture@localhost');
    writeFileSync(join(path, '.gitignore'), '*.marker\n');
    for (const [name, value] of Object.entries(entries)) writeFileSync(join(path, name), value);
    git('add', '.'); git('commit', '-m', 'isolated fixture');
    return { key, repository_path: path };
  });
  let view, sequence = 0;
  function call(tool, args, error) {
    const r = spawnSync(core, ['call', tool], { input: JSON.stringify(args), env, encoding: 'utf8', windowsHide: true, timeout: 30000, maxBuffer: 16 * 1024 * 1024 });
    assert.ok(r.stdout, `${tool}: ${r.stderr}`);
    const envelope = JSON.parse(r.stdout);
    if (error) { assert.equal(envelope.ok, false); assert.equal(envelope.error.code, error, JSON.stringify(envelope)); return envelope; }
    assert.equal(envelope.ok, true, JSON.stringify(envelope)); assert.equal(r.status, 0, r.stderr);
    const result = envelope.result;
    if (result.view) view = result.view; else if (result.task) view = result;
    return result;
  }
  const identity = () => ({ task_id: view.task.task_id, revision: view.task.revision, operation_id: `fixture-${++sequence}` });
  const inspect = () => call('taskbelay_get_task', { task_id: view.task.task_id });
  const confirm = () => call('taskbelay_confirm_plan', { ...identity(), plan_digest: view.plan_digest, user_ref: 'fixture-user-confirmation', response: 'I understand and approve the displayed plan.' });
  function open(goals, checks, authorization = {}) {
    return call('taskbelay_open_task', { operation_id: 'fixture-open', host: 'cli', repositories, authorization: { user_ref: 'fixture-user-request', request: 'Deliver the listed fixture outcomes.', goals, ...authorization }, checks, plan: planFor(goals) });
  }
  const checkpoint = (data, error) => call('taskbelay_checkpoint', { ...identity(), ...data }, error);
  function revise(authorization = view.task.authorization, checks = view.task.checks, plan = planFor(authorization.goals)) {
    return call('taskbelay_revise_task', { ...identity(), authorization, checks, plan, reason: 'Fixture user changes this requirement or verification plan.' });
  }
  const run = (id, extra = {}, error) => call('taskbelay_run_check', { ...identity(), check_id: id, rationale: 'Establish the unmet fixture check after relevant inputs or approved requirements changed.', ...extra }, error);
  function review(verdict = 'satisfied') {
    inspect();
    return checkpoint({ progress: view.task.authorization.goals.map(g => ({ goal_id: g.id, status: 'done', summary: 'Fixture submits completion for guard verification.' })), assessment: {
      authorization_digest: view.authorization_digest, content_digest: view.content_digest,
      paths: view.task.repositories.flatMap(r => r.binding.task_surface.map(e => ({ path: repositories.length > 1 ? `${r.key}::${e.path}` : e.path, goal_ids: view.task.authorization.goals.map(g => g.id), work_id: 'work', basis: 'Changed file directly participates in this fixture.', necessity: 'Required by the approved fixture work.' }))),
      constraints: (view.task.authorization.constraints ?? []).map(c => ({ id: c.id, verdict, basis: 'Scripted review of the constraint.', uncertainty: '' })),
      facts: view.task.facts.filter(f => !f.blocking).map(f => ({ id: f.id, verdict: 'satisfied', basis: 'Observed change is needed for the fixture goal.', uncertainty: '' })),
      summary: 'Scripted semantic testimony; not an AI model or real user measurement.',
      review: { root_cause: 'Targets the original behavior.', responsibilities: 'Existing module owns it.', proportionality: 'No unrelated feature was added.', verification: 'Assertions cover the fixture.', would_choose_again: true, uncertainty: '' },
    } });
  }
  function finish(error) {
    const accepted = view.decision.accepted_failures ?? [];
    return call('taskbelay_finish_task', { ...identity(), summary: 'Fixture delivered with current evidence.', risks: ['Scripted user and semantic inputs.'], acceptance: view.task.authorization.goals.map(g => ({ goal_id: g.id, evidence_ids: view.task.evidence.filter(e => g.checks.includes(e.check_id) && (view.decision.evidence.some(f => f.evidence_id === e.id && f.valid) || accepted.some(x => x.failed_evidence_id === e.id))).map(e => e.id), basis: 'Current executions or explicitly accepted regression results establish this outcome.' })) }, error);
  }
  function write(path, value, key = repositories[0].key) { writeFileSync(join(repositories.find(r => r.key === key).repository_path, path), value); }
  return { root, env, repositories, call, identity, inspect, confirm, open, checkpoint, revise, run, review, finish, write, get view() { return view; } };
}
