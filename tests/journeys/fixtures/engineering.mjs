import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve, relative, isAbsolute } from 'node:path';

export const core = process.env.ENGINEERING_CORE ?? resolve('.evaluation', process.platform === 'win32' ? 'taskbelay.exe' : 'taskbelay');
export function planFor(checks = []) {
  return { approach: 'Change the existing implementation where the requested outcome requires it.', verification_basis: 'Use relevant local assertions and review the actual diff.', checks };
}
export function check(id, script, extra = {}) {
  return { id, goal_ids: ['goal'], kind: 'command', argv: [process.execPath, script], rationale: 'Exercise the requested behavior.', conditions: 'Local Node and fixture inputs; no external services.', retry: 'repeatable', timeout_ms: 3000, ...extra };
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
    const r = spawnSync(core, tool === 'check.run' ? ['check','run'] : ['call', tool], { input: JSON.stringify(args), env, encoding: 'utf8', windowsHide: true, timeout: 30000, maxBuffer: 16 * 1024 * 1024 });
    assert.ok(r.stdout, `${tool}: ${r.stderr}`);
    const envelope = JSON.parse(r.stdout);
    if (error) { assert.equal(envelope.ok, false); assert.equal(envelope.error.code, error, JSON.stringify(envelope)); return envelope; }
    assert.equal(envelope.ok, true, JSON.stringify(envelope)); assert.equal(r.status, 0, r.stderr);
    const result = envelope.result;
    const state = result.view ?? (result.identity ? result : null);
    if (state) {
      view = state;
      if (tool !== 'taskbelay_get_task') call('taskbelay_get_task', {task_id:state.identity.task_id,refresh:true,detail:true});
    }
    return result;
  }
  const identity = () => ({ task_id: view.identity.task_id, revision: view.identity.revision, operation_id: `fixture-${++sequence}` });
  const inspect = () => call('taskbelay_get_task', { task_id: view.identity.task_id, detail:true });
  function open(goals, checks, authorization = {}) {
    return call('taskbelay_open_task', { operation_id: 'fixture-open', host: 'cli', repositories, authorization: { user_ref: 'fixture-user-request', request: 'Deliver the listed fixture outcomes.', goals, ...authorization }, plan: planFor(checks) });
  }
  const checkpoint = (data, error) => call('taskbelay_checkpoint', { ...identity(), ...data }, error);
  function revise(authorization = view.context.authorization, checks = view.context.plan.checks, plan = {...view.context.plan, checks}) {
    return call('taskbelay_revise_task', { ...identity(), authorization, plan, reason: 'Fixture user changes this requirement or verification plan.' });
  }
  const run = (id, extra = {}, error) => call('check.run', { ...identity(), check_id: id, rationale: 'Establish the unmet fixture check after relevant inputs or approved requirements changed.', ...extra }, error);
  function review(verdict = 'satisfied') {
    inspect();
    return checkpoint({ progress: view.context.authorization.goals.map(g => ({ goal_id: g.id, status: 'done', summary: 'Fixture submits completion for guard verification.' })), assessment: {
      authorization_digest: view.digests.authorization, content_digest: view.digests.content,
      paths: view.context.repositories.flatMap(r => r.changed_paths.map(path => ({ path, goal_ids: view.context.authorization.goals.map(g => g.id), basis: 'Changed file directly participates in this fixture.' }))),
      constraints: (view.context.authorization.constraints ?? []).map(c => ({ id: c.id, verdict, basis: 'Scripted review of the constraint.', uncertainty: '' })),
      facts: view.details.facts.filter(f => !f.blocking).map(f => ({ id: f.id, verdict: 'satisfied', basis: 'Observed change is needed for the fixture goal.', uncertainty: '' })),
      summary: 'Scripted semantic testimony; not an AI model or real user measurement.',
      review: { summary: 'The existing module fixes the original behavior with relevant assertions and no unrelated features; this structure remains appropriate.' },
    } });
  }
  function finish(error) {
    return call('taskbelay_finish_task', { ...identity(), summary: 'Fixture delivered with current evidence.', acceptance: view.context.authorization.goals.map(g => ({ goal_id: g.id, evidence_ids: view.details.evidence.filter(e => view.context.plan.checks.some(c => c.id === e.check_id && c.purpose === 'acceptance' && c.goal_ids.includes(g.id)) && view.details.evidence_applicability.some(f => f.evidence_id === e.id && f.valid)).map(e => e.id), basis: 'Applicable passing acceptance checks establish this outcome.' })) }, error);
  }
  function write(path, value, key = repositories[0].key) { writeFileSync(join(repositories.find(r => r.key === key).repository_path, path), value); }
  return { root, env, repositories, call, identity, inspect, open, checkpoint, revise, run, review, finish, write, get view() { return view; } };
}
