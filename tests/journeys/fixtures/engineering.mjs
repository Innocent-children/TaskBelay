import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve, relative, isAbsolute } from 'node:path';

export const core = process.env.ENGINEERING_CORE ?? resolve('.evaluation', process.platform === 'win32' ? 'taskbelay.exe' : 'taskbelay');
export function planFor() {
  return { approach: 'Change the existing implementation where the requested outcome requires it.', verification_basis: 'Use relevant local assertions and review the actual diff.' };
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
      paths: view.task.repositories.flatMap(r => r.changed_paths.map(path => ({ path, goal_ids: view.task.authorization.goals.map(g => g.id), basis: 'Changed file directly participates in this fixture.' }))),
      constraints: (view.task.authorization.constraints ?? []).map(c => ({ id: c.id, verdict, basis: 'Scripted review of the constraint.', uncertainty: '' })),
      facts: view.decision.findings.filter(f => !f.blocking).map(f => ({ id: f.id, verdict: 'satisfied', basis: 'Observed change is needed for the fixture goal.', uncertainty: '' })),
      summary: 'Scripted semantic testimony; not an AI model or real user measurement.',
      review: { summary: 'The existing module fixes the original behavior with relevant assertions and no unrelated features; this structure remains appropriate.', uncertainty: '' },
    } });
  }
  function finish(error) {
    const accepted = view.decision.accepted_failures ?? [];
    return call('taskbelay_finish_task', { ...identity(), summary: 'Fixture delivered with current evidence.', risks: ['Scripted user and semantic inputs.'], acceptance: view.task.authorization.goals.map(g => ({ goal_id: g.id, evidence_ids: view.task.evidence.filter(e => g.checks.includes(e.check_id) && (view.decision.evidence.some(f => f.evidence_id === e.id && f.valid) || accepted.some(x => x.failed_evidence_id === e.id))).map(e => e.id), basis: 'Current executions or explicitly accepted regression results establish this outcome.' })) }, error);
  }
  function write(path, value, key = repositories[0].key) { writeFileSync(join(repositories.find(r => r.key === key).repository_path, path), value); }
  return { root, env, repositories, call, identity, inspect, open, checkpoint, revise, run, review, finish, write, get view() { return view; } };
}
