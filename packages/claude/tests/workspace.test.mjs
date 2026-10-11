import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, mkdir, readFile, readdir, realpath, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { bindTask, cleanup, inspect, prepare, provision, scope, session, status as launchStatus } from "../lib/workspace.mjs";
import { paths } from "../lib/runtime.mjs";
import { defaultRunGit as git } from "../lib/worktree-lifecycle.mjs";
import { assertSkillResources, examples } from "../../../tests/skills/resources.mjs";
async function fixture(t) {
  const parent = await realpath(await mkdtemp(join(tmpdir(), "claude-workspace-")));
  const root = join(parent, "repo"); await mkdir(root);
  t.after(() => rm(parent, { recursive: true, force: true }));
  await git(["init", "-b", "main", root]); await git(["-C", root, "config", "user.name", "Test"]); await git(["-C", root, "config", "user.email", "test@example.invalid"]);
  await writeFile(join(root, "base.txt"), "base"); await git(["-C", root, "add", "."]); await git(["-C", root, "commit", "-m", "base"]);
  const options = { environment: { ...process.env, HOME: parent, USERPROFILE: parent, LOCALAPPDATA: join(parent, "appdata"), TASKBELAY_DATA_DIR: "" }, checkWorkspaceAvailable: async repository_path => ({ available: true, repository_path }) };
  return { root, options };
}

function example(markdown, kind, operation, name) {
  const found = examples(markdown, kind).find(entry => entry.operation === operation && entry.name === name);
  assert.ok(found, `${kind} ${operation} ${name}`);
  return found.value;
}

function shape(value) {
  if (value === null) return null;
  if (Array.isArray(value)) return value.map(shape);
  if (typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, entry]) => [key, shape(entry)]));
  return typeof value;
}

test("Claude Skill references are packaged and reachable from its entrypoint", async () => {
  await assertSkillResources({
    skillRoot: fileURLToPath(new URL("../plugin/skills/taskbelay/", import.meta.url)),
    packageRoot: fileURLToPath(new URL("../", import.meta.url)),
    repositoryRoot: fileURLToPath(new URL("../../../", import.meta.url)),
  });
});


test("local branch prepares once and keeps one session identity", async t => {
  const { root, options } = await fixture(t), request = "Implement an endpoint";
  const anchor = await inspect({ request, repositories: [{ key: "primary", repository_path: root }] });
  const receipt = await prepare({ request, authorization_ref:"fixture-user-authorized-workspace", repositories: [{ key: "primary", repository_path: root, workspace_mode: "new_branch", source_type: "local", remote_name: "", base_branch: "main", target_branch: "task", carry_changes: false, worktree_path: root }], handoff: null }, options);
  const ready = await provision(receipt.launch_id, options);
  assert.equal(ready.repositories[0].phase, "provisioned");
  assert.equal((await scope(receipt.launch_id, options)).repositories[0].workspace_origin.task_branch, "task");
  await provision(receipt.launch_id, options);
  const launch = await session(receipt.launch_id, "launch", {}, options);
  assert.equal(launch.executable, "claude");
  await assert.rejects(session(receipt.launch_id, "launch", {}, options), /already recorded/);
  assert.equal((await session(receipt.launch_id, "resume", {}, options)).session_id, launch.session_id);
  await assert.rejects(session(receipt.launch_id, "retry-launch", { previous_caller_stopped: true, session_not_started: false, reason: "Unknown outcome" }, options), /proof/);
  const retry = await session(receipt.launch_id, "retry-launch", { previous_caller_stopped: true, session_not_started: true, reason: "Process invocation failed before starting Claude" }, options);
  assert.equal(retry.session_id, launch.session_id);
  assert.equal(retry.arguments[0], "--session-id");
  await bindTask(receipt.launch_id, { task_id: "task-from-successful-core-response" }, options);
  const resumed = await session(receipt.launch_id, "resume", {}, options);
  assert.match(resumed.arguments.at(-1), /task-from-successful-core-response/);
  await assert.rejects(bindTask(receipt.launch_id, { task_id: "task-other" }, options), /another Core Task/);
});

test("uppercase repository keys are rejected before preparation changes Git", async t => {
  const { root, options } = await fixture(t);
  const before = await git(["-C", root, "show-ref"]);
  await assert.rejects(prepare({ request: "Keep repository keys consistent with Core", authorization_ref:"fixture-user-authorized-workspace",
  repositories: [dedicated(root, "Backend")], handoff: null }, options), /Repository key/);
  assert.equal(await git(["-C", root, "show-ref"]), before);
  assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
});

async function prepareSelections(repositories, options) {const request="Complete a multi-repository change";return prepare({request,authorization_ref:"fixture-user-authorized-workspace",repositories,handoff:{discussion:[{role:"user",text:request}]}},options);}

function dedicated(root, key = "primary", carry = false) {
  return { key, repository_path: root, workspace_mode: "dedicated_worktree", source_type: "local", remote_name: "", base_branch: "main", target_branch: "task", carry_changes: carry, worktree_path: join(root, "..", "worktree") };
}
test("dedicated worktree can exclude changes from an occupied source directory", async t => {
  for (const sourceType of ["local", "remote"]) await t.test(sourceType, async t => {
    const { root, options } = await fixture(t);
    const selection = { ...dedicated(root), source_type: sourceType };
    if (sourceType === "remote") {
      const remote = await fixture(t);
      await writeFile(join(remote.root, "base.txt"), "remote base");
      await git(["-C", remote.root, "commit", "-am", "remote base"]);
      await git(["-C", root, "remote", "add", "origin", remote.root]);
      selection.remote_name = "origin";
    }
    await writeFile(join(root, "base.txt"), "staged"); await git(["-C", root, "add", "base.txt"]);
    await writeFile(join(root, "base.txt"), "working"); await writeFile(join(root, "new.txt"), "untracked");
    const before = await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]);
    const index = await git(["-C", root, "write-tree"]);
    const head = await git(["-C", root, "rev-parse", "HEAD"]);
    const checks = [];
    const occupied = { ...options, checkWorkspaceAvailable: async repository_path => {
      checks.push(repository_path);
      return { available: false, repository_path, task_id: "existing-source-task" };
    } };
    const receipt = await prepareSelections([selection], occupied);
    assert.equal(receipt.repositories[0].snapshot, null);
    await provision(receipt.launch_id, occupied);
    const bound = await scope(receipt.launch_id, occupied);
    assert.equal(bound.repositories[0].repository_path, selection.worktree_path);
    assert.equal(bound.repositories[0].workspace_origin.source_type, sourceType);
    assert.equal(bound.repositories[0].workspace_origin.carry_changes, false);
    assert.equal(await readFile(join(selection.worktree_path, "base.txt"), "utf8"), sourceType === "remote" ? "remote base" : "base");
    assert.equal(await git(["-C", selection.worktree_path, "status", "--porcelain=v2"]), "");
    await assert.rejects(stat(join(selection.worktree_path, "new.txt")), { code: "ENOENT" });
    assert.deepEqual(checks, []);
    assert.equal(await readFile(join(root, "base.txt"), "utf8"), "working");
    assert.equal(await readFile(join(root, "new.txt"), "utf8"), "untracked");
    assert.equal(await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]), before);
    assert.equal(await git(["-C", root, "write-tree"]), index);
    assert.equal(await git(["-C", root, "rev-parse", "HEAD"]), head);
    assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
  });
});
test("local workspace modes retain initial-change acceptance and source occupancy checks", async t => {
  for (const mode of ["new_branch", "current_branch"]) await t.test(mode, async t => {
    const { root, options } = await fixture(t);
    const selection = { ...dedicated(root), workspace_mode: mode, worktree_path: root,
      target_branch: mode === "current_branch" ? "main" : "task" };
    await writeFile(join(root, "base.txt"), "initial work");
    await assert.rejects(prepareSelections([selection], options), /initial local changes require explicit acceptance/);
    selection.carry_changes = true;
    const checks = [];
    let available = false;
    const checked = { ...options, checkWorkspaceAvailable: async repository_path => {
      checks.push(repository_path);
      return { available, repository_path };
    } };
    await assert.rejects(prepareSelections([selection], checked), /claimed/);
    available = true;
    const receipt = await prepareSelections([selection], checked);
    available = false;
    await assert.rejects(provision(receipt.launch_id, checked), /claimed/);
    assert.equal((await launchStatus(receipt.launch_id, options)).repositories[0].phase, "prepared");
    assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
    assert.equal(await git(["-C", root, "branch", "--list", "task"]), "");
    available = true;
    await provision(receipt.launch_id, checked);
    assert.deepEqual(checks, [root, root, root, root]);
    assert.equal((await scope(receipt.launch_id, options)).repositories[0].workspace_origin.mode, mode);
    assert.equal(await readFile(join(root, "base.txt"), "utf8"), "initial work");
  });
});
test("dedicated worktree preserves staged, unstaged and untracked content with its source intact", async t => {
  const { root, options } = await fixture(t);
  await writeFile(join(root, "base.txt"), "staged"); await git(["-C", root, "add", "base.txt"]);
  await writeFile(join(root, "base.txt"), "working"); await writeFile(join(root, "new.txt"), "untracked");
  const before = await git(["-C", root, "status", "--porcelain=v2"]);
  const selection = dedicated(root, "primary", true);
  const receipt = await prepareSelections([selection], options); await provision(receipt.launch_id, options);
  assert.equal(await readFile(join(selection.worktree_path, "base.txt"), "utf8"), "working");
  assert.equal(String(await git(["-C", selection.worktree_path, "show", ":base.txt"])), "staged");
  assert.equal(await readFile(join(selection.worktree_path, "new.txt"), "utf8"), "untracked");
  assert.equal(await git(["-C", root, "status", "--porcelain=v2"]), before);
  assert.equal((await scope(receipt.launch_id, options)).repositories[0].workspace_origin.carry_changes, true);
});
test("snapshot rejects changed contents even when Git status is unchanged", async t => {
  const { root, options } = await fixture(t);
  await writeFile(join(root, "base.txt"), "staged\n"); await git(["-C", root, "add", "base.txt"]);
  await writeFile(join(root, "base.txt"), "tracked-A\n"); await writeFile(join(root, "new.txt"), "untracked-A\n");
  const before = await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]);
  const index = await git(["-C", root, "write-tree"]);
  const head = await git(["-C", root, "rev-parse", "HEAD"]);
  const stash = await git(["-C", root, "stash", "list"]);
  let captures = 0;
  const runGit = async (args, opts) => {
    const output = await git(args, opts);
    if (args.includes("stash") && args.includes("create") && ++captures === 1) {
      await writeFile(join(root, "base.txt"), "tracked-B\n");
      await writeFile(join(root, "new.txt"), "untracked-B\n");
      assert.equal(await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]), before);
    }
    return output;
  };
  const selection = dedicated(root, "primary", true);
  await assert.rejects(prepareSelections([selection], { ...options, runGit }), /Source workspace changed while capturing/);
  assert.equal(captures, 2);
  const launchRoot = join((await paths(options.environment)).productRoot, "provisioning", "claude");
  const [launchID] = await readdir(launchRoot);
  const receipt = await launchStatus(launchID, options);
  assert.equal(receipt.repositories[0].phase, "uncertain");
  assert.equal(receipt.repositories[0].snapshot, null);
  await assert.rejects(stat(selection.worktree_path), { code: "ENOENT" });
  assert.equal(await git(["-C", root, "branch", "--list", selection.target_branch]), "");
  assert.equal(await readFile(join(root, "base.txt"), "utf8"), "tracked-B\n");
  assert.equal(await readFile(join(root, "new.txt"), "utf8"), "untracked-B\n");
  assert.equal(await git(["-C", root, "write-tree"]), index);
  assert.equal(await git(["-C", root, "rev-parse", "HEAD"]), head);
  assert.equal(await git(["-C", root, "stash", "list"]), stash);
});

test("all repositories are required and replacement worktree instances are rejected", async t => {
  const one = await fixture(t), two = await fixture(t);
  const selected = [dedicated(one.root), dedicated(two.root, "secondary")];
  const receipt = await prepareSelections(selected, one.options); await provision(receipt.launch_id, one.options);
  const bound = await scope(receipt.launch_id, one.options);
  assert.equal(bound.repositories.slice(1)[0].key, "secondary");
  await git(["-C", one.root, "worktree", "remove", selected[0].worktree_path]);
  await git(["-C", one.root, "worktree", "add", selected[0].worktree_path, "task"]);
  await assert.rejects(scope(receipt.launch_id, one.options), /identity/);
});
test("a partial provisioning failure retains the first workspace and cannot expose a partial Core scope", async t => {
  const one = await fixture(t), two = await fixture(t);
  const selected = [dedicated(one.root), dedicated(two.root, "secondary")];
  const receipt = await prepareSelections(selected, one.options);
  const failSecond = { ...one.options, runGit: async (args, opts) => {
    if (args.includes(two.root) && args.includes("worktree") && args.includes("add")) throw new Error("Injected second-workspace failure");
    return git(args, opts);
  } };
  await assert.rejects(provision(receipt.launch_id, failSecond), /second-workspace/);
  assert.equal(await readFile(join(selected[0].worktree_path, "base.txt"), "utf8"), "base");
  await assert.rejects(scope(receipt.launch_id, one.options));
  await assert.rejects(provision(receipt.launch_id, one.options), /uncertain/);
});

test("current-branch and remote-base modes preserve their confirmed origins", async t => {
  const local = await fixture(t);
  const current = { ...dedicated(local.root), workspace_mode: "current_branch", target_branch: "main", worktree_path: local.root };
  const localReceipt = await prepareSelections([current], local.options);
  await provision(localReceipt.launch_id, local.options);
  assert.equal((await scope(localReceipt.launch_id, local.options)).repositories[0].workspace_origin.mode, "current_branch");
  const source = await fixture(t), remote = await fixture(t);
  await writeFile(join(remote.root, "remote.txt"), "remote-only");
  await git(["-C", remote.root, "add", "remote.txt"]); await git(["-C", remote.root, "commit", "-m", "remote content"]);
  await git(["-C", source.root, "remote", "add", "origin", remote.root]);
  const selection = { ...dedicated(source.root), source_type: "remote", remote_name: "origin" };
  const receipt = await prepareSelections([selection], source.options); await provision(receipt.launch_id, source.options);
  assert.equal(await readFile(join(selection.worktree_path, "remote.txt"), "utf8"), "remote-only");
  assert.equal((await scope(receipt.launch_id, source.options)).repositories[0].workspace_origin.source_type, "remote");
});
