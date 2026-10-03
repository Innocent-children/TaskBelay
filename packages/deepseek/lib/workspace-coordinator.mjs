import { spawn } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { access, lstat, realpath, stat } from "node:fs/promises";
import { constants as fsConstants } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

import { captureWorkspaceChanges, applyWorkspaceChanges } from "./worktree-snapshot.mjs";
import { currentDirectUserText } from "./authorization.mjs";
import {
  readProvisioningReceipt,
  validateProvisioningReceipt,
  writeProvisioningReceipt,
} from "./provisioning-receipt.mjs";

export const WORKSPACE_COORDINATOR_TOOL = "workspace_coordinator";
const MAX_COMMAND_OUTPUT = 1024 * 1024;
const DEFAULT_TIMEOUT_MS = 120_000;

export function workspaceConfirmationText(repositories) {
  const rows = validateRepositoryRequests(repositories).map((repository) =>
    `repository=${repository.repository_key};mode=${repository.workspace_mode};source=${repository.source_type};carry=${repository.carry_changes};remote=${repository.remote_name};base=${repository.base_branch};target=${repository.target_branch}`,
  );
  return ["/taskbelay confirm-workspace", ...rows].join("\n");
}

export function workspaceResumeText(launchID) {
  assertLaunchID(launchID);
  return `/taskbelay resume-worktree launch=${launchID}`;
}

export function workspaceCleanupText(operation, { launchID, repositoryKey, taskID, revision }) {
  if (!new Set(["prepare_cleanup", "cleanup_worktree", "cleanup_branch"]).has(operation)) throw new Error("cleanup operation is invalid");
  assertLaunchID(launchID);
  if (typeof repositoryKey !== "string" || !/^[a-z0-9][a-z0-9._-]{0,127}$/u.test(repositoryKey)) throw new Error("cleanup repository key is invalid");
  if (typeof taskID !== "string" || taskID.trim() === "" || /\s/u.test(taskID)) throw new Error("cleanup Task identity is invalid");
  if (!Number.isInteger(revision) || revision < 1) throw new Error("cleanup Task revision is invalid");
  return `/taskbelay ${operation.replace("_", "-")} launch=${launchID} repository=${repositoryKey} task=${taskID} revision=${revision}`;
}

export function authorizeWorkspaceExecution(execution) {
  if (execution?.name !== WORKSPACE_COORDINATOR_TOOL) return;
  const operation = execution.arguments?.operation;
  const text = currentDirectUserText(execution);
  if (operation === "provision") {
    const expected = workspaceConfirmationText(execution.arguments?.repositories);
    if (!text.includes(expected)) {
      throw new Error(`TASKBELAY_WORKTREE_CONFIRMATION_REQUIRED: send this exact confirmation in the current direct user turn:\n${expected}`);
    }
    return;
  }
  if (operation === "consume") {
    const expected = workspaceResumeText(execution.arguments?.launch_id);
    if (!text.includes(expected)) {
      throw new Error(`TASKBELAY_WORKTREE_RELAUNCH_REQUIRED: the current direct user turn must include ${expected}`);
    }
    return;
  }
  if (operation === "prepare_cleanup" || operation === "cleanup_worktree" || operation === "cleanup_branch") {
    const expected = workspaceCleanupText(operation, {
      launchID: execution.arguments?.launch_id,
      repositoryKey: execution.arguments?.repository_key,
      taskID: execution.arguments?.task_id,
      revision: execution.arguments?.revision,
    });
    if (!text.includes(expected)) throw new Error(`TASKBELAY_WORKSPACE_CLEANUP_CONFIRMATION_REQUIRED: the current direct user turn must include ${expected}`);
    return;
  }
  throw new Error("TASKBELAY_WORKSPACE_OPERATION_INVALID: operation is not supported");
}

export function createWorkspaceCoordinator({
  dataDirectory,
  runtimePath,
  checkWorkspaceAvailable,
  workspaceRoot = process.cwd(),
  command = runClosedCommand,
  now = () => new Date(),
  launchID = randomUUID,
  dshExecutable = "dsh",
  readTask,
} = {}) {
  if (typeof dataDirectory !== "string" || !isAbsolute(dataDirectory)) throw new Error("workspace coordinator data directory is required");
  if (typeof workspaceRoot !== "string" || !isAbsolute(workspaceRoot)) throw new Error("workspace coordinator root is required");

  return Object.freeze({
    async provision({ request, profile, repositories, signal } = {}) {
      if (typeof request !== "string" || request.trim() === "" || request !== request.trim()) throw new Error("workspace request is invalid");
      assertProfile(profile);
      const requested = validateRepositoryRequests(repositories);
      const canonicalWorkspaceRoot = await canonicalDirectory(workspaceRoot, "Workspace Root");
      const id = launchID();
      assertLaunchID(id);
      const requestDigest = sha256(request);
      const launchRoot = resolve(dirname(canonicalWorkspaceRoot), ".taskbelay-worktrees", id);
      if (inside(canonicalWorkspaceRoot, launchRoot)) throw new Error("worktree launch root must be outside the current Workspace Root");
      await assertNoSymlinkComponents(dirname(canonicalWorkspaceRoot), launchRoot);

      const observed = [];
      const requireAvailable = async (root) => {
        let result;
        if (typeof checkWorkspaceAvailable === "function") result = await checkWorkspaceAvailable(root);
        else {
          if (typeof runtimePath !== "string" || !isAbsolute(runtimePath)) throw new Error("Core workspace availability check is unavailable");
          const output = await command(runtimePath, ["host-check", "workspace-available"], {
            cwd: root, signal, env: { TASKBELAY_DATA_DIR: dataDirectory },
            input: `${JSON.stringify({ repository_path: root })}\n`,
          });
          result = JSON.parse(output.stdout);
        }
        if (result?.available !== true || result.repository_path !== root) throw new Error("workspace already has an active TaskBelay Task or could not be checked; resolve it before changing branches");
      };
      for (const repository of requested) {
        const source = await observeSourceRepository(repository.source_repository_path, { command, signal });
        if (!inside(canonicalWorkspaceRoot, source.root)) throw new Error(`repository ${repository.repository_key} is outside the current Workspace Root`);
        await validateBranchSelection(source.root, repository, { command, signal });
        const local = repository.workspace_mode !== "dedicated_worktree";
        if (local) await requireAvailable(source.root);
        const worktreePath = local ? source.root : resolve(launchRoot, repository.repository_key);
        if (!local) await assertPathAbsent(worktreePath, `worktree path for ${repository.repository_key}`);
        observed.push({ ...repository, source, worktreePath });
      }

      if (new Set(observed.map((entry) => entry.worktreePath)).size !== observed.length) throw new Error("workspace repository directories must be unique");
      const hasLocal = observed.some((entry) => entry.workspace_mode !== "dedicated_worktree");
      const hasWorktree = observed.some((entry) => entry.workspace_mode === "dedicated_worktree");

      const timestamp = now().toISOString();
      let receipt = validateProvisioningReceipt({
        launch_id: id,
        host: "deepseek",
        request_digest: requestDigest,
        profile,
        workspace_root: hasLocal ? (hasWorktree ? dirname(canonicalWorkspaceRoot) : canonicalWorkspaceRoot) : (observed.length === 1 ? observed[0].worktreePath : launchRoot),
        operation_status: "confirmed",
        repositories: observed.map((repository) => ({
          source_repository_identity: repository.source.identity,
          repository_key: repository.repository_key,
          workspace_mode: repository.workspace_mode,
          source_type: repository.source_type,
          carry_changes: repository.carry_changes,
          remote_name: repository.remote_name,
          base_branch: repository.base_branch,
          target_branch: repository.target_branch,
          base_commit: null,
          snapshot_commit: null,
          worktree_path: repository.worktreePath,
          operation_status: "confirmed",
          created_at: timestamp,
        })),
        created_at: timestamp,
        updated_at: timestamp,
      });
      receipt = await writeProvisioningReceipt(dataDirectory, receipt);

      const provisioned = [];
      try {
        receipt = await setLaunchStatus(receipt, "resolving", now, (repository) => ({ ...repository, operation_status: "resolving" }));
        await writeProvisioningReceipt(dataDirectory, receipt);
        for (const repository of receipt.repositories) {
          const source = observed.find((entry) => entry.repository_key === repository.repository_key).source.root;
          if (repository.source_type === "remote") await git(source, [
            "fetch", "--no-tags", repository.remote_name,
            `refs/heads/${repository.base_branch}:refs/remotes/${repository.remote_name}/${repository.base_branch}`,
          ], { command, signal, mutating: true });
          const baseCommit = (await git(source, [
            "rev-parse", "--verify", repository.source_type === "remote" ? `refs/remotes/${repository.remote_name}/${repository.base_branch}^{commit}` : `refs/heads/${repository.base_branch}^{commit}`,
          ], { command, signal })).stdout.trim();
          if (!/^[0-9a-f]{40,64}$/u.test(baseCommit)) throw new Error(`frozen commit for ${repository.repository_key} is invalid`);
          const snapshot = repository.carry_changes ? await captureWorkspaceChanges(source, snapshotRunner(command, signal)) : null;
          receipt = await updateRepositoryStatus(receipt, repository.repository_key, "prepared", now, { base_commit: baseCommit, snapshot_commit: snapshot });
          await writeProvisioningReceipt(dataDirectory, receipt);
        }

        receipt = await setLaunchStatus(receipt, "provisioning", now, (repository) => ({ ...repository, operation_status: "provisioning" }));
        await writeProvisioningReceipt(dataDirectory, receipt);
        for (const repository of receipt.repositories) {
          const source = observed.find((entry) => entry.repository_key === repository.repository_key).source.root;
          if (repository.workspace_mode === "dedicated_worktree") {
            await ensureTargetStillAvailable(source, repository, { command, signal });
            await git(source, [
              "worktree", "add", "-b", repository.target_branch, repository.worktree_path, repository.base_commit,
            ], { command, signal, mutating: true });
            await verifyProvisionedRepository({ ...repository, carry_changes: false }, { command, signal, sourceRepositoryPath: source });
            await applyWorkspaceChanges(repository.worktree_path, repository.snapshot_commit, snapshotRunner(command, signal));
          } else {
            await requireAvailable(source);
            await validateBranchSelection(source, repository, { command, signal });
            const before = await observeLocalBranch(source, { command, signal });
            if (before.head !== repository.base_commit || before.identity !== repository.source_repository_identity) throw new Error("local workspace changed after preparation");
            if (repository.workspace_mode === "new_branch") await git(source, ["switch", "-c", repository.target_branch, repository.base_commit], { command, signal, mutating: true });
            await verifyProvisionedRepository(repository, { command, signal });
            const after = await observeSourceRepository(source, { command, signal });
            if (after.gitDir !== before.gitDir || after.statusDigest !== before.statusDigest) throw new Error("local branch preparation changed workspace state; inspect the retained directory");
          }
          provisioned.push(repository.repository_key);
          receipt = await updateRepositoryStatus(receipt, repository.repository_key, "provisioned", now);
          await writeProvisioningReceipt(dataDirectory, receipt);
        }
        receipt = await setLaunchStatus(receipt, "provisioned", now, (repository) => ({ ...repository, operation_status: "provisioned" }));
        await writeProvisioningReceipt(dataDirectory, receipt);
      } catch (error) {
        const uncertain = error?.operationUncertain === true;
        if (!uncertain) await compensateProvisioned(receipt, provisioned, observed, { command, signal }).catch(() => {});
        receipt = await setLaunchStatus(receipt, uncertain ? "uncertain" : "failed", now, (repository) =>
          provisioned.includes(repository.repository_key)
            ? repository
            : { ...repository, operation_status: uncertain ? "uncertain" : "failed" },
        );
        await writeProvisioningReceipt(dataDirectory, receipt).catch(() => {});
        const failure = new Error(uncertain
          ? "worktree provisioning result is uncertain; inspect the retained receipt and filesystem before retrying"
          : `worktree provisioning failed; no Core Task was created: ${error.message}`, { cause: error });
        failure.code = uncertain ? "WORKTREE_PROVISIONING_UNCERTAIN" : "WORKTREE_PROVISIONING_FAILED";
        throw failure;
      }

      if (!hasWorktree) {
        receipt = await setLaunchStatus(receipt, "consumed", now, (repository) => ({ ...repository, operation_status: "consumed" }));
        await writeProvisioningReceipt(dataDirectory, receipt);
        return workspaceOpenResult(receipt, "ready");
      }
      const prompt = `${workspaceResumeText(receipt.launch_id)}\nContinue the confirmed request exactly as assessed:\n${request}`;
      return Object.freeze({
        status: "relaunch_required",
        launch_id: receipt.launch_id,
        request_digest: receipt.request_digest,
        workspace_root: receipt.workspace_root,
        source_dirty_paths: Object.fromEntries(observed.map((repository) => [repository.repository_key, repository.source.dirtyPaths])),
        source_dirty_paths_truncated: Object.fromEntries(observed.map((repository) => [repository.repository_key, repository.source.dirtyPathsTruncated])),
        relaunch: Object.freeze({ command: dshExecutable, arguments: Object.freeze(["--profile", profile, prompt]), cwd: receipt.workspace_root }),
      });
    },

    async consume({ launchID: id, signal } = {}) {
      assertLaunchID(id);
      let receipt = await readProvisioningReceipt(dataDirectory, id);
      if (receipt === null) throw new Error("provisioning receipt was not found");
      if (!new Set(["provisioned", "consumed"]).has(receipt.operation_status)) {
        throw new Error(`provisioning receipt is ${receipt.operation_status}; it cannot open a Core Task`);
      }
      const canonicalWorkspaceRoot = await canonicalDirectory(workspaceRoot, "Workspace Root");
      if (canonicalWorkspaceRoot !== resolve(receipt.workspace_root)) {
        throw new Error("DSH must be relaunched from the receipt workspace root");
      }
      for (const repository of receipt.repositories) {
        if (!inside(canonicalWorkspaceRoot, repository.worktree_path)) throw new Error(`repository ${repository.repository_key} is outside the relaunched Workspace Root`);
        await verifyProvisionedRepository(repository, { command, signal });
      }
      if (receipt.operation_status !== "consumed") {
        receipt = await setLaunchStatus(receipt, "consumed", now, (repository) => ({ ...repository, operation_status: "consumed" }));
        await writeProvisioningReceipt(dataDirectory, receipt);
      }
      return workspaceOpenResult(receipt, "consumed");
    },

    async prepareCleanup({ launchID: id, repositoryKey, taskID, revision, sourceRepositoryPath, signal, execution } = {}) {
      const state = await cleanupState({ dataDirectory, workspaceRoot, launchID: id, repositoryKey, taskID, revision, signal, execution, readTask, command });
      if (typeof sourceRepositoryPath !== "string" || !isAbsolute(sourceRepositoryPath)) throw new Error("cleanup relaunch source repository path is required");
      const source = await observeSourceRepository(sourceRepositoryPath, { command, signal });
      if (source.identity !== state.repository.source_repository_identity) throw new Error("cleanup relaunch source does not match the receipt repository group");
      if (source.root === state.repository.worktree_path) throw new Error("cleanup relaunch must use a source checkout outside the Task worktree");
      const prompt = [
        `/taskbelay resume-cleanup launch=${id} repository=${repositoryKey} task=${taskID} revision=${revision}`,
        "The receipt-owned source checkout is now the fixed DSH Workspace Root.",
        `Ask the developer to send exactly: ${workspaceCleanupText("cleanup_worktree", { launchID: id, repositoryKey, taskID, revision })}`,
        "Do not delete the worktree or branch in this relaunch turn.",
      ].join("\n");
      return Object.freeze({ status: "cleanup_relaunch_required", changed: false, launch_id: id, repository_key: repositoryKey, relaunch: Object.freeze({ command: dshExecutable, arguments: Object.freeze(["--profile", state.receipt.profile, prompt]), cwd: source.root }) });
    },

    async cleanupWorktree({ launchID: id, repositoryKey, taskID, revision, signal, execution } = {}) {
      const state = await cleanupState({ dataDirectory, workspaceRoot, launchID: id, repositoryKey, taskID, revision, signal, execution, readTask, command });
      if (state.repository.operation_status === "worktree_removed" || state.repository.operation_status === "branch_removed") {
        return Object.freeze({ status: state.repository.operation_status, changed: false, launch_id: id, repository_key: repositoryKey });
      }
      if (state.repository.operation_status !== "consumed") throw new Error("receipt repository is not ready for terminal cleanup");
      const inspected = await inspectTerminalWorktree(state.repository, state.taskRepository, { command, signal });
      await git(dirname(state.repository.worktree_path), ["--git-dir", inspected.commonDir, "worktree", "remove", state.repository.worktree_path], { command, signal, mutating: true });
      const updated = await updateCleanupStatus(state.receipt, repositoryKey, "worktree_removed", now);
      await writeProvisioningReceipt(dataDirectory, updated);
      return Object.freeze({ status: "worktree_removed", changed: true, launch_id: id, repository_key: repositoryKey, branch_retained: true });
    },

    async cleanupBranch({ launchID: id, repositoryKey, taskID, revision, sourceRepositoryPath, signal, execution } = {}) {
      const state = await cleanupState({ dataDirectory, workspaceRoot, launchID: id, repositoryKey, taskID, revision, signal, execution, readTask, command });
      if (state.repository.operation_status === "branch_removed") {
        return Object.freeze({ status: "branch_removed", changed: false, launch_id: id, repository_key: repositoryKey });
      }
      if (state.repository.operation_status !== "worktree_removed") throw new Error("worktree cleanup requires its own earlier authorization");
      if (typeof sourceRepositoryPath !== "string" || !isAbsolute(sourceRepositoryPath)) throw new Error("branch cleanup source repository path is required");
      const source = await observeSourceRepository(sourceRepositoryPath, { command, signal });
      const canonicalWorkspaceRoot = await canonicalDirectory(workspaceRoot, "Workspace Root");
      if (!inside(canonicalWorkspaceRoot, source.root) || source.identity !== state.repository.source_repository_identity) {
        throw new Error("branch cleanup source does not match the receipt repository group");
      }
      const branchHead = (await git(source.root, ["rev-parse", "--verify", `refs/heads/${state.repository.target_branch}^{commit}`], { command, signal })).stdout.trim();
      if (branchHead !== state.taskRepository.current_head) throw new Error("task branch HEAD differs from the terminal Core observation");
      await assertRemoteHead(source.root, state.repository, branchHead, { command, signal });
      const worktrees = (await git(source.root, ["worktree", "list", "--porcelain"], { command, signal })).stdout;
      if (worktrees.split(/\r?\n/u).some((line) => line === `branch refs/heads/${state.repository.target_branch}`)) throw new Error("task branch is still checked out");
      await git(source.root, ["branch", "-d", state.repository.target_branch], { command, signal, mutating: true });
      const updated = await updateCleanupStatus(state.receipt, repositoryKey, "branch_removed", now);
      await writeProvisioningReceipt(dataDirectory, updated);
      return Object.freeze({ status: "branch_removed", changed: true, launch_id: id, repository_key: repositoryKey });
    },
  });
}

function workspaceOpenResult(receipt, status) {
  const repositories = receipt.repositories.map((repository) => ({
    key: repository.repository_key,
    repository_path: repository.worktree_path,
    workspace_origin: {
      mode: repository.workspace_mode, source_type: repository.source_type,
      carry_changes: repository.carry_changes, remote_name: repository.remote_name,
      base_branch: repository.base_branch, base_commit: repository.base_commit,
      task_branch: repository.target_branch, provisioning_receipt_id: receipt.launch_id,
    },
  }));
  const [primary, ...additional] = repositories;
  return Object.freeze({
    status, launch_id: receipt.launch_id, request_digest: receipt.request_digest,
    workspace_root: receipt.workspace_root,
    open_task: Object.freeze({
      repository_path: primary.repository_path, primary_repository_key: primary.key,
      workspace_origin: primary.workspace_origin, additional_repositories: additional,
    }),
  });
}

async function observeSourceRepository(path, { command, signal }) {
  if (typeof path !== "string" || !isAbsolute(path)) throw new Error("source repository path must be absolute");
  const root = await canonicalDirectory(path, "source repository");
  const top = resolve((await git(root, ["rev-parse", "--show-toplevel"], { command, signal })).stdout.trim());
  if (top !== root) throw new Error("source repository path must name its canonical worktree root");
  const commonDir = resolve(root, (await git(root, ["rev-parse", "--git-common-dir"], { command, signal })).stdout.trim());
  const gitDir = resolve(root, (await git(root, ["rev-parse", "--absolute-git-dir"], { command, signal })).stdout.trim());
  const dirty = (await git(root, ["status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=none"], { command, signal })).stdout;
  const dirtyPaths = parseDirtyPaths(dirty);
  return Object.freeze({
    root,
    commonDir,
    gitDir,
    statusDigest: sha256(dirty),
    identity: sourceRepositoryIdentity(commonDir),
    dirtyPaths: Object.freeze(dirtyPaths.slice(0, 64)),
    dirtyPathsTruncated: dirtyPaths.length > 64,
  });
}

async function observeLocalBranch(root, { command, signal }) {
  const source = await observeSourceRepository(root, { command, signal });
  const head = (await git(root, ["rev-parse", "HEAD"], { command, signal })).stdout.trim();
  const branch = (await git(root, ["branch", "--show-current"], { command, signal })).stdout.trim();
  return { ...source, head, branch };
}

async function validateBranchSelection(root, repository, { command, signal }) {
  if (repository.source_type === "remote") assertRemoteName(repository.remote_name);
  for (const branch of [repository.base_branch, repository.target_branch]) {
    await git(root, ["check-ref-format", "--branch", branch], { command, signal });
  }
  if (repository.source_type === "remote") await git(root, ["remote", "get-url", repository.remote_name], { command, signal });
  else await git(root, ["rev-parse", "--verify", `refs/heads/${repository.base_branch}^{commit}`], { command, signal });
  if (repository.workspace_mode !== "dedicated_worktree") {
    const source = await observeLocalBranch(root, { command, signal });
    if (repository.source_type !== "local" || !source.branch || source.branch !== repository.base_branch) throw new Error("local mode requires the current named branch as its base");
    if (source.dirtyPaths.length > 0 && !repository.carry_changes) throw new Error("initial local changes require explicit acceptance");
    if (repository.workspace_mode === "current_branch") {
      if (repository.target_branch !== source.branch) throw new Error("current_branch must retain the current branch");
      return;
    }
  }
  await ensureTargetStillAvailable(root, repository, { command, signal });
}

async function ensureTargetStillAvailable(sourceRepositoryPath, repository, { command, signal }) {
  const local = await git(sourceRepositoryPath, ["show-ref", "--verify", "--quiet", `refs/heads/${repository.target_branch}`], {
    command, signal, allowExitCodes: [0, 1],
  });
  if (local.code === 0) throw new Error(`target branch ${repository.target_branch} already exists locally`);
  if (repository.source_type === "remote") {
    const remote = await git(sourceRepositoryPath, ["ls-remote", "--exit-code", "--heads", repository.remote_name, `refs/heads/${repository.target_branch}`], {
      command, signal, allowExitCodes: [0, 2],
  });
  if (remote.code === 0) throw new Error(`target branch ${repository.target_branch} already exists on ${repository.remote_name}`);
  }
  const worktrees = (await git(sourceRepositoryPath, ["worktree", "list", "--porcelain"], { command, signal })).stdout;
  if (worktrees.split(/\r?\n/u).some((line) => line === `branch refs/heads/${repository.target_branch}`)) {
    throw new Error(`target branch ${repository.target_branch} is already checked out`);
  }
}

async function verifyProvisionedRepository(repository, { command, signal, sourceRepositoryPath = null }) {
  const root = await canonicalDirectory(repository.worktree_path, `worktree ${repository.repository_key}`);
  const targetCommon = resolve(root, (await git(root, ["rev-parse", "--git-common-dir"], { command, signal })).stdout.trim());
  if (sourceRepositoryIdentity(targetCommon) !== repository.source_repository_identity) throw new Error("Task worktree does not belong to the source repository group");
  const targetGit = resolve(root, (await git(root, ["rev-parse", "--absolute-git-dir"], { command, signal })).stdout.trim());
  if (repository.workspace_mode === "dedicated_worktree" && targetGit === targetCommon) throw new Error("dedicated worktree requires a linked Git directory");
  if (sourceRepositoryPath !== null) {
    if (root === resolve(sourceRepositoryPath)) throw new Error("Task worktree must differ from the source checkout");
    const sourceGit = resolve(sourceRepositoryPath, (await git(sourceRepositoryPath, ["rev-parse", "--absolute-git-dir"], { command, signal })).stdout.trim());
    if (sourceGit === targetGit) throw new Error("Task worktree Git directory is not a new instance");
  }
  const head = (await git(root, ["rev-parse", "HEAD"], { command, signal })).stdout.trim();
  const branch = (await git(root, ["branch", "--show-current"], { command, signal })).stdout.trim();
  const status = (await git(root, ["status", "--porcelain=v2", "--untracked-files=all", "--ignore-submodules=none"], { command, signal })).stdout;
  if (head !== repository.base_commit || branch !== repository.target_branch || !repository.carry_changes && status !== "") {
    throw new Error(`Task worktree ${repository.repository_key} failed branch, HEAD, or clean-state verification`);
  }
  await access(root, fsConstants.R_OK | fsConstants.W_OK);
  return Object.freeze({ root, head, branch, commonDir: targetCommon, gitDir: targetGit });
}

async function compensateProvisioned(receipt, keys, observed, { command, signal }) {
  for (const key of [...keys].reverse()) {
    const repository = receipt.repositories.find((entry) => entry.repository_key === key);
    const source = observed.find((entry) => entry.repository_key === key)?.source.root;
    if (!repository || !source) continue;
    if (repository.workspace_mode !== "dedicated_worktree") continue;
    try {
      await verifyProvisionedRepository(repository, { command, signal, sourceRepositoryPath: source });
      await git(source, ["worktree", "remove", repository.worktree_path], { command, signal, mutating: true });
    } catch {
      // Preserve resources whenever cleanup safety cannot be proven.
    }
  }
}

async function cleanupState({ dataDirectory, launchID, repositoryKey, taskID, revision, signal, execution, readTask, command }) {
  assertLaunchID(launchID);
  if (typeof readTask !== "function") throw new Error("terminal Core Task reader is unavailable");
  const receipt = await readProvisioningReceipt(dataDirectory, launchID);
  if (receipt === null) throw new Error("provisioning receipt was not found");
  if (!new Set(["consumed", "cleaned"]).has(receipt.operation_status)) throw new Error("provisioning receipt is not eligible for terminal cleanup");
  const repository = receipt.repositories.find((entry) => entry.repository_key === repositoryKey);
  if (!repository) throw new Error("cleanup repository is not owned by the receipt");
  if (repository.workspace_mode !== "dedicated_worktree") throw new Error("local Task directories and branches are retained; workspace cleanup does not apply");
  const task = await readTask({ taskID, signal, execution });
  if (task === null || typeof task !== "object" || !new Set(["DONE", "CANCELLED"]).has(task.current_cursor)) {
    throw new Error("Core Task is not terminal");
  }
  if (task.task_id !== taskID || task.revision !== revision) throw new Error("terminal Core Task identity or revision changed");
  const primaryKey = task.primary_repository_key ?? "primary";
  const taskRepository = repositoryKey === primaryKey
    ? { origin: task.workspace_origin, binding: task.repository }
    : (task.additional_repositories ?? []).filter((entry) => entry.key === repositoryKey)
      .map((entry) => ({ origin: entry.workspace_origin, binding: entry.repository }))[0];
  if (!taskRepository || taskRepository.origin?.provisioning_receipt_id !== launchID ||
      taskRepository.origin?.canonical_worktree_root !== repository.worktree_path ||
      taskRepository.origin?.task_branch !== repository.target_branch ||
      taskRepository.binding?.current_head === undefined) {
    throw new Error("terminal Core Task does not match the receipt workspace");
  }
  if (taskRepository.binding.detached || typeof taskRepository.binding.current_branch!=="string" || !taskRepository.binding.current_branch || taskRepository.binding.current_branch.startsWith("-")) throw new Error("Terminal Core effective branch is required");
  return { receipt, repository:{...repository,target_branch:taskRepository.binding.current_branch}, taskRepository: { ...taskRepository.binding, origin: taskRepository.origin }, command };
}

async function inspectTerminalWorktree(repository, taskRepository, { command, signal }) {
  const root = await canonicalDirectory(repository.worktree_path, `worktree ${repository.repository_key}`);
  const commonDir = resolve(root, (await git(root, ["rev-parse", "--git-common-dir"], { command, signal })).stdout.trim());
  if (sourceRepositoryIdentity(commonDir) !== repository.source_repository_identity) throw new Error("worktree no longer belongs to the receipt repository group");
  const head = (await git(root, ["rev-parse", "HEAD"], { command, signal })).stdout.trim();
  const branch = (await git(root, ["branch", "--show-current"], { command, signal })).stdout.trim();
  const status = (await git(root, ["status", "--porcelain=v2", "--untracked-files=all", "--ignore-submodules=none"], { command, signal })).stdout;
  if (head !== taskRepository.current_head || branch !== repository.target_branch) throw new Error("worktree branch or HEAD differs from the terminal Core observation");
  if (status !== "") throw new Error("dirty terminal worktree is retained");
  await assertRemoteHead(root, repository, head, { command, signal });
  return { root, commonDir, head };
}

async function assertRemoteHead(root, repository, head, { command, signal }) {
  if (repository.source_type === "local") throw new Error("local-source task branch is retained; review it before separate manual cleanup");
  const remote = await git(root, ["ls-remote", "--exit-code", "--heads", repository.remote_name, `refs/heads/${repository.target_branch}`], {
    command, signal, allowExitCodes: [0, 2],
  });
  if (remote.code !== 0) throw new Error("unpushed task branch is retained");
  const rows = remote.stdout.trim().split(/\r?\n/u).filter(Boolean);
  if (rows.length !== 1 || rows[0].split(/\s+/u)[0] !== head) throw new Error("remote task branch differs from the terminal HEAD");
}

async function updateCleanupStatus(receipt, repositoryKey, status, now) {
  const repositories = receipt.repositories.map((repository) => repository.repository_key === repositoryKey
    ? { ...repository, operation_status: status }
    : repository);
  return validateProvisioningReceipt({
    ...receipt,
    operation_status: repositories.every((repository) => repository.operation_status === "branch_removed") ? "cleaned" : "consumed",
    repositories,
    updated_at: now().toISOString(),
  });
}

async function updateRepositoryStatus(receipt, key, status, now, patch = {}) {
  return validateProvisioningReceipt({
    ...receipt,
    operation_status: status === "provisioned" && receipt.repositories.every((entry) => entry.repository_key === key || entry.operation_status === "provisioned") ? "provisioned" : receipt.operation_status,
    repositories: receipt.repositories.map((repository) => repository.repository_key === key
      ? { ...repository, ...patch, operation_status: status }
      : repository),
    updated_at: now().toISOString(),
  });
}

async function setLaunchStatus(receipt, status, now, mapRepository) {
  return validateProvisioningReceipt({
    ...receipt,
    operation_status: status,
    repositories: receipt.repositories.map(mapRepository),
    updated_at: now().toISOString(),
  });
}

function validateRepositoryRequests(value) {
  if (!Array.isArray(value) || value.length < 1 || value.length > 8) throw new Error("one to eight confirmed repositories are required");
  const keys = new Set();
  return value.map((entry) => {
    const expected = ["repository_key", "workspace_mode", "source_repository_path", "source_type", "carry_changes", "remote_name", "base_branch", "target_branch"];
    if (entry === null || typeof entry !== "object" || Array.isArray(entry) || JSON.stringify(Object.keys(entry).sort()) !== JSON.stringify(expected.sort())) {
      throw new Error("workspace repository fields are invalid");
    }
    if (typeof entry.repository_key !== "string" || !/^[a-z0-9][a-z0-9._-]{0,127}$/u.test(entry.repository_key) || keys.has(entry.repository_key)) {
      throw new Error("workspace repository key is invalid or duplicated");
    }
    if (typeof entry.source_repository_path !== "string" || !isAbsolute(entry.source_repository_path) || entry.source_repository_path.includes("\0")) {
      throw new Error(`repository ${entry.repository_key} source path is invalid`);
    }
    if (!["new_branch", "current_branch", "dedicated_worktree"].includes(entry.workspace_mode)) throw new Error("workspace_mode is invalid");
    if (!["local", "remote"].includes(entry.source_type) || typeof entry.carry_changes !== "boolean" || entry.source_type === "remote" && entry.carry_changes || entry.source_type === "local" && entry.remote_name !== "") throw new Error("invalid workspace source selection");
    if (entry.source_type === "remote") assertRemoteName(entry.remote_name);
    for (const field of ["base_branch", "target_branch"]) {
      if (typeof entry[field] !== "string" || entry[field] === "" || entry[field].length > 255 || /[\0\r\n;=]/u.test(entry[field])) {
        throw new Error(`repository ${entry.repository_key} ${field} is invalid`);
      }
    }
    keys.add(entry.repository_key);
    return Object.freeze({ ...entry });
  });
}

function assertRemoteName(value) {
  if (typeof value !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u.test(value)) throw new Error("remote name is invalid");
}

function assertProfile(value) {
  if (typeof value !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/u.test(value)) throw new Error("DSH Profile is invalid");
}

function assertLaunchID(value) {
  if (typeof value !== "string" || !/^[0-9a-f-]{36}$/u.test(value)) throw new Error("provisioning launch identity is invalid");
}

async function canonicalDirectory(path, label) {
  const canonical = await realpath(path).catch((error) => { throw new Error(`${label} is unavailable`, { cause: error }); });
  if (canonical !== resolve(path) || !(await stat(canonical)).isDirectory()) throw new Error(`${label} must be a canonical non-symlink directory`);
  return canonical;
}

async function assertPathAbsent(path, label) {
  try {
    await lstat(path);
  } catch (error) {
    if (error?.code === "ENOENT") return;
    throw error;
  }
  throw new Error(`${label} already exists`);
}

async function assertNoSymlinkComponents(root, candidate) {
  const canonicalRoot = resolve(root);
  const offset = relative(canonicalRoot, resolve(candidate));
  if (offset === ".." || offset.startsWith(`..${sep}`) || isAbsolute(offset)) throw new Error("worktree launch path escapes its parent");
  let current = canonicalRoot;
  for (const part of offset.split(sep).filter(Boolean)) {
    current = join(current, part);
    try {
      if ((await lstat(current)).isSymbolicLink()) throw new Error(`worktree launch path contains a symbolic link: ${current}`);
    } catch (error) {
      if (error?.code === "ENOENT") return;
      throw error;
    }
  }
}

function inside(root, candidate) {
  const offset = relative(resolve(root), resolve(candidate));
  return offset === "" || !(offset === ".." || offset.startsWith(`..${sep}`) || isAbsolute(offset));
}

function parseDirtyPaths(status) {
  const paths = [];
  const records = status.split("\0");
  for (let index = 0; index < records.length; index += 1) {
    const record = records[index];
    if (!record) continue;
    if (record.startsWith("? ") || record.startsWith("! ")) {
      paths.push(record.slice(2));
      continue;
    }
    if (record.startsWith("1 ") || record.startsWith("u ")) {
      paths.push(record.split(" ").slice(record.startsWith("1 ") ? 8 : 10).join(" "));
      continue;
    }
    if (record.startsWith("2 ")) {
      paths.push(record.split(" ").slice(9).join(" "));
      if (records[index + 1]) paths.push(records[index + 1]);
      index += 1;
    }
  }
  return [...new Set(paths)].sort();
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function sourceRepositoryIdentity(commonDirectory) {
  return sha256(`taskbelay/source-repository\0${commonDirectory}`);
}

async function git(cwd, arguments_, options) {
  return await options.command("git", arguments_, {
    cwd,
    signal: options.signal,
    timeoutMs: DEFAULT_TIMEOUT_MS,
    maxOutputBytes: MAX_COMMAND_OUTPUT,
    allowExitCodes: options.allowExitCodes ?? [0],
    mutating: options.mutating === true,
    env: options.env,
  });
}

export async function runClosedCommand(executable, arguments_, {
  cwd,
  signal,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  maxOutputBytes = MAX_COMMAND_OUTPUT,
  allowExitCodes = [0],
  mutating = false,
  env = {},
  input,
} = {}) {
  if (typeof executable !== "string" || executable === "" || executable.includes("\0") || !Array.isArray(arguments_) || arguments_.some((value) => typeof value !== "string" || value.includes("\0"))) {
    throw new Error("command arguments must be closed strings");
  }
  if (signal?.aborted) throw signal.reason ?? new Error("command aborted");
  return await new Promise((resolvePromise, reject) => {
    const child = spawn(executable, arguments_, {
      cwd,
      env: { ...process.env, ...env, GIT_TERMINAL_PROMPT: "0" },
      stdio: [input === undefined ? "ignore" : "pipe", "pipe", "pipe"],
      shell: false,
      windowsHide: true,
    });
    child.stdin?.on("error", () => {});
    child.stdin?.end(input);
    const stdout = [];
    const stderr = [];
    let bytes = 0;
    let interrupted = false;
    let settled = false;
    const cleanup = () => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", interrupt);
    };
    const closeStreams = () => {
      child.stdin?.destroy();
      child.stdout.destroy();
      child.stderr.destroy();
    };
    const complete = (code, exitSignal) => {
      if (settled) return;
      settled = true;
      cleanup();
      const result = { code: code ?? -1, signal: exitSignal, stdout: Buffer.concat(stdout).toString("utf8"), stderr: Buffer.concat(stderr).toString("utf8") };
      if (!interrupted && !signal?.aborted && exitSignal === null && allowExitCodes.includes(result.code)) {
        resolvePromise(result);
        return;
      }
      const error = new Error(`${executable} command failed`);
      error.exitCode = result.code;
      error.operationUncertain = mutating && (interrupted || signal?.aborted || exitSignal !== null);
      reject(error);
    };
    const interrupt = () => {
      if (settled) return;
      interrupted = true;
      child.kill("SIGKILL");
      // Descendants may retain the pipes after the direct child has exited.
      closeStreams();
      complete(child.exitCode, child.signalCode);
    };
    const collect = (target) => (chunk) => {
      if (settled) return;
      bytes += chunk.length;
      if (bytes > maxOutputBytes) {
        interrupt();
      } else target.push(Buffer.from(chunk));
    };
    child.stdout.on("data", collect(stdout));
    child.stderr.on("data", collect(stderr));
    signal?.addEventListener("abort", interrupt, { once: true });
    const timer = setTimeout(interrupt, timeoutMs);
    child.once("error", (error) => {
      if (settled) return;
      settled = true;
      cleanup();
      closeStreams();
      reject(error);
    });
    child.once("close", complete);
  });
}

function snapshotRunner(command, signal) {
  return async (root, args, env) => (await git(root, args, { command, signal, env, mutating: true })).stdout;
}
