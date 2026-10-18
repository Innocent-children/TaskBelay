import { mkdirSync, realpathSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { assessRepositoryFreshness } from '../../packages/host-workspace/repository-freshness.mjs';

const root = realpathSync(process.argv[2]);
const source = join(root, 'source'), remote = join(root, 'remote.git'), local = join(root, 'local');
const config = join(root, 'gitconfig');
writeFileSync(config, '');
const env = { ...process.env, GIT_CONFIG_GLOBAL: config, GIT_CONFIG_SYSTEM: config, GIT_TERMINAL_PROMPT: '0' };
const git = args => {
  try { return execFileSync('git', args, { encoding: 'utf8', env, stdio: ['ignore', 'pipe', 'pipe'] }); }
  catch (error) { error.code = error.status; throw error; }
};
git(['init', '--bare', '-b', 'main', remote]);
git(['init', '-b', 'main', source]);
git(['-C', source, 'config', 'user.name', 'Fixture']);
git(['-C', source, 'config', 'user.email', 'fixture@localhost']);
git(['-C', source, 'remote', 'add', 'origin', remote]);
function advance(content) {
  writeFileSync(join(source, 'file.txt'), content);
  git(['-C', source, 'add', 'file.txt']);
  git(['-C', source, 'commit', '-m', 'fixture update']);
  git(['-C', source, 'push', 'origin', 'main']);
}
advance('initial\n');
git(['clone', remote, local]);
const receiptRoot = join(root, 'receipts');
mkdirSync(receiptRoot);
const request = { repositories: [{ key: 'primary', repository_path: local }], authorization_ref: 'fixture-fetch' };
const integration = { ...request, integrate: true, integration_authorization_ref: 'fixture-integration' };
const outputs = [];
async function record(name, input, runGit = git) {
  const output = await assessRepositoryFreshness(input, { runGit, receiptRoot });
  outputs.push({ name, output });
  return output;
}
await record('skipped_fetch', { ...request, fetch: false });
advance('remote update\n');
const fetched = await record('fetched', request);
assert.equal(fetched.repositories[0].fetch_status, 'completed');
assert.ok(fetched.repositories[0].remote_freshness.fetched_at);
assert.equal((await record('integrated', { ...integration, operation_id: 'merge-success' })).repositories[0].integrated, true);
assert.equal((await record('replayed', { ...integration, operation_id: 'merge-success' })).operation.replayed, true);
const failed = await record('fetch_failed', request, args => {
  if (args[2] === 'fetch') throw Object.assign(new Error('fixture fetch denied'), { code: 'EACCES', stderr: 'fixture diagnostic' });
  return git(args);
});
assert.equal(failed.repositories[0].remote_freshness.state, 'fetch_failed');
advance('another update\n');
const unknown = { ...integration, operation_id: 'merge-unknown' };
const interrupted = await record('integration_unknown', unknown, args => {
  if (args[2] === 'merge') throw Object.assign(new Error('fixture merge interrupted'), { code: 1, stderr: 'fixture diagnostic' });
  return git(args);
});
assert.equal(interrupted.operation.status, 'uncertain');
assert.equal((await record('recovered', { ...unknown, recover: true })).operation.status, 'reconciled');
git(['-C', local, 'branch', '--unset-upstream']);
await record('no_upstream', request);
git(['-C', local, 'switch', '--detach']);
await record('detached', request);
process.stdout.write(JSON.stringify(outputs));
