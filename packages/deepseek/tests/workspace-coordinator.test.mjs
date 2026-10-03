import assert from "node:assert/strict";
import { execFile as execFileCallback } from "node:child_process";
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { readProvisioningReceipt } from "../lib/provisioning-receipt.mjs";
import {
  WORKSPACE_COORDINATOR_TOOL,
  authorizeWorkspaceExecution,
  createWorkspaceCoordinator,
  runClosedCommand,
  workspaceConfirmationText,
  workspaceCleanupText,
  workspaceResumeText,
} from "../lib/workspace-coordinator.mjs";

const execFile = promisify(execFileCallback);
const fixedLaunchID = "11111111-1111-4111-8111-111111111111";

test("workspace confirmation is bound to the current direct user turn", () => {
  const repositories = [{
    repository_key: "primary",
    source_repository_path: "/workspace/source",
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "feature/proof",
  }];
  const expected = workspaceConfirmationText(repositories);
  assert.equal(expected, "/taskbelay confirm-workspace\nrepository=primary;mode=dedicated_worktree;source=remote;carry=false;remote=origin;base=main;target=feature/proof");
  assert.doesNotThrow(() => authorizeWorkspaceExecution(execution(expected, { operation: "provision", repositories })));
  assert.throws(
    () => authorizeWorkspaceExecution(execution("/taskbelay use it", { operation: "provision", repositories })),
    /WORKTREE_CONFIRMATION_REQUIRED/u,
  );
  assert.doesNotThrow(() => authorizeWorkspaceExecution(execution(workspaceResumeText(fixedLaunchID), {
    operation: "consume", launch_id: fixedLaunchID,
  })));
  const cleanup = { launchID: fixedLaunchID, repositoryKey: "primary", taskID: "task-cleanup", revision: 9 };
  assert.doesNotThrow(() => authorizeWorkspaceExecution(execution(workspaceCleanupText("prepare_cleanup", cleanup), {
    operation: "prepare_cleanup", launch_id: fixedLaunchID, repository_key: "primary", task_id: "task-cleanup", revision: 9,
  })));
  assert.doesNotThrow(() => authorizeWorkspaceExecution(execution(workspaceCleanupText("cleanup_worktree", cleanup), {
    operation: "cleanup_worktree", launch_id: fixedLaunchID, repository_key: "primary", task_id: "task-cleanup", revision: 9,
  })));
});

test("coordinator fetches a frozen base, excludes dirty source state, and emits a consumable relaunch", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-workspace-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const remote = join(root, "remote.git");
  const source = join(root, "source");
  const data = join(root, "data");
  await mkdir(data, { mode: 0o700 });
  await execFile("git", ["init", "--bare", "--initial-branch=main", remote]);
  await execFile("git", ["clone", remote, source]);
  await git(source, ["config", "user.email", "workspace@example.invalid"]);
  await git(source, ["config", "user.name", "Workspace Test"]);
  await git(source, ["config", "core.autocrlf", "false"]);
  await writeFile(join(source, "README.md"), "remote base\n");
  await git(source, ["add", "README.md"]);
  await git(source, ["commit", "-m", "base"]);
  await git(source, ["push", "-u", "origin", "main"]);
  const baseCommit = (await git(source, ["rev-parse", "HEAD"])).stdout.trim();

  await writeFile(join(source, "README.md"), "dirty source\n");
  await writeFile(join(source, "staged.txt"), "staged\n");
  await git(source, ["add", "staged.txt"]);
  await writeFile(join(source, "untracked.txt"), "untracked\n");

  const repositories = [{
    repository_key: "primary",
    source_repository_path: source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "feature/proof",
  }];
  const coordinator = createWorkspaceCoordinator({
    dataDirectory: data,
    workspaceRoot: source,
    launchID: () => fixedLaunchID,
  });
  const result = await coordinator.provision({
    request: "Create an isolated proof.",
    profile: "headless",
    repositories,
  });
  assert.equal(result.status, "relaunch_required");
  assert.deepEqual(result.relaunch, {
    command: "dsh",
    arguments: ["--profile", "headless", `${workspaceResumeText(fixedLaunchID)}\nContinue the confirmed request exactly as assessed:\nCreate an isolated proof.`],
    cwd: result.workspace_root,
  });
  assert.deepEqual(result.source_dirty_paths.primary, ["README.md", "staged.txt", "untracked.txt"]);
  assert.equal(result.source_dirty_paths_truncated.primary, false);
  assert.equal((await git(result.workspace_root, ["rev-parse", "HEAD"])).stdout.trim(), baseCommit);
  assert.equal((await git(result.workspace_root, ["branch", "--show-current"])).stdout.trim(), "feature/proof");
  assert.equal((await git(result.workspace_root, ["status", "--porcelain=v1", "--untracked-files=all"])).stdout, "");
  assert.equal(await readFile(join(result.workspace_root, "README.md"), "utf8"), "remote base\n");
  await assert.rejects(readFile(join(result.workspace_root, "staged.txt")), { code: "ENOENT" });
  await assert.rejects(readFile(join(result.workspace_root, "untracked.txt")), { code: "ENOENT" });

  const receipt = await readProvisioningReceipt(data, fixedLaunchID);
  assert.equal(receipt.operation_status, "provisioned");
  assert.equal(receipt.repositories[0].base_commit, baseCommit);
  assert.equal(JSON.stringify(receipt).includes(source), false);
  assert.equal(Object.hasOwn(receipt.repositories[0], "source_repository_path"), false);
  if (process.platform !== "win32") assert.equal((await stat(join(data, "host-operations", "deepseek", "provisioning", `${fixedLaunchID}.json`))).mode & 0o077, 0);

  const relaunched = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root });
  const consumed = await relaunched.consume({ launchID: fixedLaunchID });
  assert.equal(consumed.status, "consumed");
  assert.deepEqual(Object.keys(consumed.open_task).sort(), [
    "additional_repositories", "primary_repository_key", "repository_path", "workspace_origin",
  ]);
  assert.deepEqual(consumed.open_task.workspace_origin, {
    mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    base_commit: baseCommit,
    task_branch: "feature/proof",
    provisioning_receipt_id: fixedLaunchID,
  });
  assert.equal((await readProvisioningReceipt(data, fixedLaunchID)).operation_status, "consumed");

  const baseTerminalTask = {
    task_id: "task-cleanup", revision: 9, current_cursor: "DONE", primary_repository_key: "primary",
    workspace_origin: { ...consumed.open_task.workspace_origin, canonical_worktree_root: result.workspace_root },
    repository: { current_head: baseCommit, current_branch:"feature/proof", detached:false }, additional_repositories: [],
  };
  const activeCleanup = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root, readTask: async () => ({ ...baseTerminalTask, current_cursor: "IMPLEMENT" }) });
  await assert.rejects(activeCleanup.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: baseTerminalTask.task_id, revision: baseTerminalTask.revision }), /not terminal/u);
  const foreignCleanup = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root, readTask: async () => ({ ...baseTerminalTask, workspace_origin: { ...baseTerminalTask.workspace_origin, provisioning_receipt_id: "another-receipt" } }) });
  await assert.rejects(foreignCleanup.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: baseTerminalTask.task_id, revision: baseTerminalTask.revision }), /does not match the receipt workspace/u);
  const staleHeadCleanup = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root, readTask: async () => ({ ...baseTerminalTask, repository: { current_head: "f".repeat(40), current_branch:"feature/proof", detached:false } }) });
  await assert.rejects(staleHeadCleanup.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: baseTerminalTask.task_id, revision: baseTerminalTask.revision }), /differs from the terminal Core observation/u);
  const guardedCleanup = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root, readTask: async () => baseTerminalTask });
  await assert.rejects(guardedCleanup.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: baseTerminalTask.task_id, revision: baseTerminalTask.revision }), /unpushed task branch/u);
  await writeFile(join(result.workspace_root, "dirty.txt"), "dirty\n");
  await assert.rejects(guardedCleanup.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: baseTerminalTask.task_id, revision: baseTerminalTask.revision }), /dirty terminal worktree/u);
  await rm(join(result.workspace_root, "dirty.txt"));

  await writeFile(join(result.workspace_root, "proof.txt"), "proof\n");
  await git(result.workspace_root, ["add", "proof.txt"]);
  await git(result.workspace_root, ["commit", "-m", "proof"]);
  await git(result.workspace_root, ["push", "-u", "origin", "HEAD"]);
  const terminalHead = (await git(result.workspace_root, ["rev-parse", "HEAD"])).stdout.trim();
  const terminalTask = {
    task_id: "task-cleanup", revision: 9, current_cursor: "DONE", primary_repository_key: "primary",
    workspace_origin: { ...consumed.open_task.workspace_origin, canonical_worktree_root: result.workspace_root },
    repository: { current_head: terminalHead, current_branch:"feature/proof", detached:false }, additional_repositories: [],
  };
  const cleanupFromTask = createWorkspaceCoordinator({
    dataDirectory: data, workspaceRoot: result.workspace_root, readTask: async () => terminalTask,
  });
  const cleanupRelaunch = await cleanupFromTask.prepareCleanup({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: terminalTask.task_id, revision: terminalTask.revision, sourceRepositoryPath: source });
  assert.equal(cleanupRelaunch.status, "cleanup_relaunch_required");
  assert.equal(cleanupRelaunch.relaunch.cwd, source);
  assert.match(cleanupRelaunch.relaunch.arguments[2], /\/taskbelay resume-cleanup/u);
  assert.equal(JSON.stringify(await readProvisioningReceipt(data, fixedLaunchID)).includes(source), false);
  const cleanupFromSource = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: source, readTask: async () => terminalTask });
  assert.equal(relative(source, result.workspace_root).startsWith(".."), true, "Task worktree must remain outside the relaunched fixed Workspace Root");
  const removedWorktree = await cleanupFromSource.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: terminalTask.task_id, revision: terminalTask.revision });
  assert.deepEqual(removedWorktree, { status: "worktree_removed", changed: true, launch_id: fixedLaunchID, repository_key: "primary", branch_retained: true });
  await assert.rejects(stat(result.workspace_root), { code: "ENOENT" });
  assert.equal((await readProvisioningReceipt(data, fixedLaunchID)).repositories[0].operation_status, "worktree_removed");
  await git(source, ["merge", "--ff-only", "feature/proof"]);
  const removedBranch = await cleanupFromSource.cleanupBranch({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: terminalTask.task_id, revision: terminalTask.revision, sourceRepositoryPath: source });
  assert.deepEqual(removedBranch, { status: "branch_removed", changed: true, launch_id: fixedLaunchID, repository_key: "primary" });
  assert.equal((await readProvisioningReceipt(data, fixedLaunchID)).operation_status, "cleaned");
});

test("invalid or occupied target branches stop before a receipt or worktree is created", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-workspace-conflict-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, "source");
  const remote = join(root, "remote.git");
  const data = join(root, "data");
  await mkdir(data);
  await execFile("git", ["init", "--bare", "--initial-branch=main", remote]);
  await execFile("git", ["clone", remote, source]);
  await git(source, ["config", "user.email", "workspace@example.invalid"]);
  await git(source, ["config", "user.name", "Workspace Test"]);
  await writeFile(join(source, "README.md"), "base\n");
  await git(source, ["add", "README.md"]);
  await git(source, ["commit", "-m", "base"]);
  await git(source, ["push", "-u", "origin", "main"]);
  await git(source, ["branch", "feature/existing"]);
  const coordinator = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: source, launchID: () => fixedLaunchID });
  await assert.rejects(coordinator.provision({
    request: "Do not collide.", profile: "headless", repositories: [{
      repository_key: "primary", source_repository_path: source, workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
      base_branch: "main", target_branch: "feature/existing",
    }],
  }), /already exists locally/u);
  assert.equal(await readProvisioningReceipt(data, fixedLaunchID), null);
});

test("a multi-repository fetch failure creates no target branch or worktree", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-workspace-multi-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const workspace = join(root, "workspace");
  const data = join(root, "data");
  await mkdir(workspace);
  await mkdir(data);
  const repositories = [];
  for (const key of ["core", "docs"]) {
    const source = join(workspace, key);
    const remote = join(root, `${key}.git`);
    await execFile("git", ["init", "--bare", "--initial-branch=main", remote]);
    await execFile("git", ["clone", remote, source]);
    await git(source, ["config", "user.email", "workspace@example.invalid"]);
    await git(source, ["config", "user.name", "Workspace Test"]);
    await writeFile(join(source, "README.md"), `${key}\n`);
    await git(source, ["add", "README.md"]);
    await git(source, ["commit", "-m", "base"]);
    await git(source, ["push", "-u", "origin", "main"]);
    repositories.push({ repository_key: key, source_repository_path: source, workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin", base_branch: key === "docs" ? "missing" : "main", target_branch: `feature/${key}` });
  }
  const coordinator = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: workspace, launchID: () => fixedLaunchID });
  await assert.rejects(coordinator.provision({ request: "Change both repositories.", profile: "headless", repositories }), /no Core Task was created/u);
  const receipt = await readProvisioningReceipt(data, fixedLaunchID);
  assert.equal(receipt.operation_status, "failed");
  for (const repository of receipt.repositories) {
    await assert.rejects(stat(repository.worktree_path), { code: "ENOENT" });
    const local = await git(join(workspace, repository.repository_key), ["show-ref", "--verify", `refs/heads/feature/${repository.repository_key}`]).then(() => true, () => false);
    assert.equal(local, false);
  }
});

function execution(text, arguments_) {
  const callID = "workspace-call";
  return {
    name: WORKSPACE_COORDINATOR_TOOL,
    callId: callID,
    arguments: arguments_,
    agent: {
      status: "running",
      session: { snapshotEvents: () => [
        { seq: 0, type: "turn/start", data: { turn: 1 } },
        { seq: 1, type: "user/message", data: { id: "user", source: { kind: "user" }, content: [{ type: "text", text }] } },
        { seq: 2, type: "tool/call", data: { turn: 1, callId: callID, name: WORKSPACE_COORDINATOR_TOOL } },
      ] },
    },
  };
}

async function git(cwd, args) {
  return await execFile("git", args, { cwd, encoding: "utf8", env: { ...process.env, GIT_CONFIG_NOSYSTEM: "1" } });
}

for (const carry of [false, true]) test(`local coordinator works without a remote and carry_changes=${carry}`, async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-local-dsh-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, "source");
  const data = join(root, "data");
  await mkdir(data);
  await execFile("git", ["init", "--initial-branch=main", source]);
  await git(source, ["config", "core.autocrlf", "false"]);
  await git(source, ["config", "user.name", "Test"]);
  await git(source, ["config", "user.email", "test@localhost"]);
  await writeFile(join(source, "file.txt"), "base\n");
  await git(source, ["add", "."]);
  await git(source, ["commit", "-m", "base"]);
  await writeFile(join(source, "file.txt"), "staged\n");
  await git(source, ["add", "."]);
  await writeFile(join(source, "file.txt"), "unstaged\n");
  await writeFile(join(source, "new.bin"), Buffer.from([0, 255, 128, 10]));
  const before = (await git(source, ["status", "--porcelain=v2"])).stdout;
  const coordinator = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: source, launchID: () => fixedLaunchID });
  const result = await coordinator.provision({ request: "Local work", profile: "headless", repositories: [{
    repository_key: "primary", source_repository_path: source, workspace_mode: "dedicated_worktree", source_type: "local", carry_changes: carry,
    remote_name: "", base_branch: "main", target_branch: "feature/local",
  }] });
  assert.equal((await git(source, ["status", "--porcelain=v2"])).stdout, before);
  assert.equal(await readFile(join(result.workspace_root, "file.txt"), "utf8"), carry ? "unstaged\n" : "base\n");
  if (carry) {
    assert.equal((await git(result.workspace_root, ["show", ":file.txt"])).stdout, "staged\n");
    assert.deepEqual(await readFile(join(result.workspace_root, "new.bin")), Buffer.from([0, 255, 128, 10]));
  }
  const relaunched = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root });
  const consumed = await relaunched.consume({ launchID: fixedLaunchID });
  assert.equal(consumed.open_task.workspace_origin.carry_changes, carry);
  assert.equal(consumed.open_task.workspace_origin.source_type, "local");
});

test("snapshot rejects changed contents even when Git status is unchanged", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-changing-dsh-snapshot-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, "source"), data = join(root, "data");
  await mkdir(data);
  await execFile("git", ["init", "--initial-branch=main", source]);
  await git(source, ["config", "user.name", "Test"]);
  await git(source, ["config", "user.email", "test@example.invalid"]);
  await writeFile(join(source, "file.txt"), "base\n");
  await git(source, ["add", "."]); await git(source, ["commit", "-m", "base"]);
  await writeFile(join(source, "file.txt"), "staged\n"); await git(source, ["add", "file.txt"]);
  await writeFile(join(source, "file.txt"), "tracked-A\n"); await writeFile(join(source, "new.txt"), "untracked-A\n");
  const before = (await git(source, ["status", "--porcelain=v2", "--untracked-files=all"])).stdout;
  const index = (await git(source, ["write-tree"])).stdout;
  const head = (await git(source, ["rev-parse", "HEAD"])).stdout;
  const stash = (await git(source, ["stash", "list"])).stdout;
  let captures = 0;
  const command = async (executable, args, options) => {
    const result = await runClosedCommand(executable, args, options);
    if (executable === "git" && args.includes("stash") && args.includes("create") && ++captures === 1) {
      await writeFile(join(source, "file.txt"), "tracked-B\n");
      await writeFile(join(source, "new.txt"), "untracked-B\n");
      assert.equal((await git(source, ["status", "--porcelain=v2", "--untracked-files=all"])).stdout, before);
    }
    return result;
  };
  const coordinator = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: source, launchID: () => fixedLaunchID, command });
  await assert.rejects(coordinator.provision({ request: "Carry one stable source snapshot", profile: "headless", repositories: [{
    repository_key: "primary", source_repository_path: source, workspace_mode: "dedicated_worktree", source_type: "local", carry_changes: true,
    remote_name: "", base_branch: "main", target_branch: "feature/snapshot",
  }] }), /Source workspace changed while capturing/);
  assert.equal(captures, 2);
  const receipt = await readProvisioningReceipt(data, fixedLaunchID);
  assert.equal(receipt.operation_status, "failed");
  assert.equal(receipt.repositories[0].snapshot_commit, null);
  await assert.rejects(stat(receipt.repositories[0].worktree_path), { code: "ENOENT" });
  assert.equal((await git(source, ["branch", "--list", "feature/snapshot"])).stdout, "");
  assert.equal(await readFile(join(source, "file.txt"), "utf8"), "tracked-B\n");
  assert.equal(await readFile(join(source, "new.txt"), "utf8"), "untracked-B\n");
  assert.equal((await git(source, ["write-tree"])).stdout, index);
  assert.equal((await git(source, ["rev-parse", "HEAD"])).stdout, head);
  assert.equal((await git(source, ["stash", "list"])).stdout, stash);
});

for (const mode of ["new_branch", "current_branch"]) {
  test(`${mode} returns ready in the existing DSH session and retains staged and ignored contents`, async (t) => {
    const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-local-branch-")));
    t.after(() => rm(root, { recursive: true, force: true }));
    const source = join(root, "source"), data = join(root, "data");
    await mkdir(source); await mkdir(data);
    await git(source, ["init", "-b", "main"]);
    await git(source, ["config", "user.name", "Workspace Test"]);
    await git(source, ["config", "user.email", "workspace@example.invalid"]);
    await writeFile(join(source, "file.txt"), "base\n");
    await git(source, ["add", "file.txt"]); await git(source, ["commit", "-m", "base"]);
    await writeFile(join(source, ".git", "info", "exclude"), "ignored.txt\n");
    await writeFile(join(source, "ignored.txt"), "local config\n");
    await writeFile(join(source, "file.txt"), "staged\n"); await git(source, ["add", "file.txt"]);
    await writeFile(join(source, "file.txt"), "unstaged\n");
    await writeFile(join(source, "new.txt"), "untracked\n");
    const status = (await git(source, ["status", "--porcelain=v2", "--untracked-files=all"])).stdout;
    const index = (await git(source, ["write-tree"])).stdout;
    const repository = { repository_key: "primary", workspace_mode: mode, source_repository_path: source,
      source_type: "local", carry_changes: true, remote_name: "", base_branch: "main", target_branch: mode === "new_branch" ? "feature/local" : "main" };
    const choices = [repository];
    assert.match(workspaceConfirmationText(choices), new RegExp(`mode=${mode}`));
    const options = { dataDirectory: data, workspaceRoot: source, launchID: () => fixedLaunchID, readTask: async () => assert.fail("local cleanup must stop before reading a terminal Task") };
    const denied = createWorkspaceCoordinator({ ...options, checkWorkspaceAvailable: async (path) => ({ available: false, repository_path: path }) });
    await assert.rejects(denied.provision({ request: "Work locally", profile: "headless", repositories: choices }), /active TaskBelay Task/);
    assert.equal((await git(source, ["branch", "--show-current"])).stdout.trim(), "main");
    const coordinator = createWorkspaceCoordinator({ ...options, checkWorkspaceAvailable: async (path) => ({ available: true, repository_path: path }) });
    await assert.rejects(coordinator.provision({ request: "Work locally", profile: "headless", repositories: [{ ...repository, carry_changes: false }] }), /initial local changes/);
    const result = await coordinator.provision({ request: "Work locally", profile: "headless", repositories: choices });
    assert.equal(result.status, "ready");
    assert.equal(Object.hasOwn(result, "relaunch"), false);
    assert.equal(result.workspace_root, source);
    assert.equal(result.open_task.repository_path, source);
    assert.equal(result.open_task.workspace_origin.mode, mode);
    assert.equal((await git(source, ["branch", "--show-current"])).stdout.trim(), repository.target_branch);
    assert.equal((await git(source, ["status", "--porcelain=v2", "--untracked-files=all"])).stdout, status);
    assert.equal((await git(source, ["write-tree"])).stdout, index);
    assert.equal(await readFile(join(source, "ignored.txt"), "utf8"), "local config\n");
    assert.equal((await readProvisioningReceipt(data, fixedLaunchID)).operation_status, "consumed");
    await assert.rejects(coordinator.cleanupWorktree({ launchID: fixedLaunchID, repositoryKey: "primary", taskID: "task-local", revision: 1 }), /cleanup does not apply/);
  });
}

test("mixed local and dedicated repositories relaunch once with every directory inside the new root", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-mixed-workspace-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const workspace = join(root, "workspace"), data = join(root, "data");
  await mkdir(workspace); await mkdir(data);
  const repositories = [];
  for (const key of ["api", "docs"]) {
    const source = join(workspace, key); await mkdir(source);
    await git(source, ["init", "-b", "main"]);
    await git(source, ["-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "base"]);
    repositories.push({ repository_key: key, workspace_mode: key === "api" ? "new_branch" : "dedicated_worktree",
      source_repository_path: source, source_type: "local", carry_changes: false, remote_name: "", base_branch: "main", target_branch: `feature/${key}` });
  }
  const coordinator = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: workspace, launchID: () => fixedLaunchID,
    checkWorkspaceAvailable: async (path) => ({ available: true, repository_path: path }) });
  const result = await coordinator.provision({ request: "Update API and documentation", profile: "web", repositories });
  assert.equal(result.status, "relaunch_required");
  assert.equal(result.relaunch.cwd, root);
  const destination = createWorkspaceCoordinator({ dataDirectory: data, workspaceRoot: result.workspace_root });
  const ready = await destination.consume({ launchID: result.launch_id });
  assert.equal(ready.open_task.repository_path, join(workspace, "api"));
  assert.equal(ready.open_task.workspace_origin.mode, "new_branch");
  assert.equal(ready.open_task.additional_repositories[0].workspace_origin.mode, "dedicated_worktree");
  const docs = ready.open_task.additional_repositories[0].repository_path;
  assert.equal(docs, join(root, ".taskbelay-worktrees", fixedLaunchID, "docs"));
  assert.notEqual(docs, join(workspace, "docs"));
  assert.equal((await git(join(workspace, "docs"), ["branch", "--show-current"])).stdout.trim(), "main");
});
