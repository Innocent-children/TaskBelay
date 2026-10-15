import { createHash, randomUUID } from "node:crypto";
import { isAbsolute, resolve } from "node:path";

import { captureWorkspaceChanges, applyWorkspaceChanges } from "./worktree-snapshot.mjs";
import { assertNoDuplicateJSONMembers } from "./json.mjs";
import { requestDigest } from "./task-admission.mjs";
import {
  buildManagedBootstrapPrompt,
  readTaskHandoff,
  readTaskHandoffDraft,
  taskHandoffDigest,
  writeTaskHandoff,
} from "./task-handoff.mjs";
import {
  createProvisioningReceipt,
  provisioningReceiptPath,
  readProvisioningReceipt,
  updateProvisioningReceipt,
  validateProvisioningReceipt,
  withProvisioningReceiptLock,
  writeProvisioningReceiptAtomic,
} from "./provisioning-receipt.mjs";
import {
  createCliWorktree,
  resolveFrozenBase,
  defaultRunGit,
  initializeManagedWorktree,
  inspectSourceRepository,
  preflightWorktreeSelection,
  preflightLocalBranchSelection,
  prepareLocalBranch,
  removeCliWorktree,
  removeTaskBranch,
} from "./worktree-lifecycle.mjs";

export async function prepareTaskLaunch(input, {
  productSupportRoot,
  enforcePrivateModes = true,
  runGit,
  checkWorkspaceAvailable,
  now = () => new Date(),
  createLaunchId = randomUUID,
} = {}) {
  validatePrepareInput(input);
  const launchId = input.launch_id ?? createLaunchId();
  const receiptInput = { ...input, launch_id: launchId };
  const currentRequestDigest = requestDigest(input.request);
  const authorizationRef = input.authorization_ref;
  const local = input.workspace_mode !== "dedicated_worktree";
  const handoff = local ? null : await readTaskHandoffDraft(input.handoff_file, input.request);
  const handoffDigest = local ? null : taskHandoffDigest(handoff);
  const path = provisioningReceiptPath(productSupportRoot, launchId, input.repository_key);
  const existing = await readProvisioningReceipt(path, { productSupportRoot });
  if (existing !== null) {
    assertInputMatchesReceipt(existing, receiptInput, currentRequestDigest, handoffDigest);
    if (existing.operation_status.phase !== "confirmed") {
      return Object.freeze({ receipt_path: path, receipt: existing, resumed: true, fetch_performed: false });
    }
  }
  const source = await (local ? preflightLocalBranchSelection : preflightWorktreeSelection)({
    repositoryPath: input.repository_path,
    remoteName: input.remote_name,
    sourceType: input.source_type,
    baseBranch: input.base_branch,
    targetBranch: input.target_branch,
    workspaceMode: input.workspace_mode,
    carryChanges: input.carry_changes,
    runGit,
  });
  if (local) await requireAvailableWorkspace(source.canonical_root, checkWorkspaceAvailable);
  let initial = existing;
  if (initial === null) {
    initial = createProvisioningReceipt({
      launchId,
      authorizationRef,
      requestDigest: currentRequestDigest,
      handoffDigest,
      sourceRepositoryIdentity: source.source_repository_identity,
      repositoryKey: input.repository_key,
      workspaceMode: input.workspace_mode,
      remoteName: input.remote_name,
      sourceType: input.source_type,
      carryChanges: input.carry_changes,
      baseBranch: input.base_branch,
      targetBranch: input.target_branch,
      worktreePath: input.worktree_path,
      surface: input.surface,
      createdAt: now().toISOString(),
    });
    try {
      await writeProvisioningReceiptAtomic(path, initial, {
        productSupportRoot,
        enforcePrivateModes,
        createOnly: true,
      });
    } catch (error) {
      if (error?.code !== "EEXIST") throw error;
      const concurrent = await readProvisioningReceipt(path, { productSupportRoot });
      if (concurrent === null) throw error;
      assertInputMatchesReceipt(concurrent, receiptInput, currentRequestDigest, handoffDigest);
      return Object.freeze({ receipt_path: path, receipt: concurrent, resumed: true, fetch_performed: false });
    }
  } else if (initial.source_repository_identity !== source.source_repository_identity) {
    throw new Error("source repository identity changed after the provisioning receipt was created");
  }
  try {
    return await withProvisioningReceiptLock(path, { productSupportRoot, enforcePrivateModes }, async () => {
      const current = await readProvisioningReceipt(path, { productSupportRoot });
      if (current === null) throw new Error("provisioning receipt disappeared before preparation");
      if (current.operation_status.phase !== "confirmed") {
        return Object.freeze({ receipt_path: path, receipt: current, resumed: true, fetch_performed: false });
      }
      assertInputMatchesReceipt(current, receiptInput, currentRequestDigest, handoffDigest);
      if (handoff !== null) await writeTaskHandoff(path, handoff, { enforcePrivateModes });
      const resolving = updateProvisioningReceipt(current, { phase: "resolving", values: {} });
      await writeProvisioningReceiptAtomic(path, resolving, { productSupportRoot, enforcePrivateModes });
      try {
        const resolvedBase = await resolveFrozenBase({
          repositoryPath: source.canonical_root,
          remoteName: input.remote_name,
          sourceType: input.source_type,
          baseBranch: input.base_branch,
          runGit,
        });
        if (resolvedBase.source_repository_identity !== source.source_repository_identity) {
          throw new Error("source repository identity changed during preparation");
        }
        const snapshot = input.carry_changes ? await captureWorkspaceChanges(source.canonical_root, snapshotRunner(runGit)) : null;
        const complete = updateProvisioningReceipt(resolving, {
          phase: "prepared",
          values: { base_commit: resolvedBase.base_commit, snapshot_commit: snapshot },
        });
        await writeProvisioningReceiptAtomic(path, complete, { productSupportRoot, enforcePrivateModes });
        return Object.freeze({
          receipt_path: path,
          receipt: complete,
          resumed: existing !== null,
          fetch_performed: input.source_type === "remote",
          source_dirty: !source.clean,
          source_status_digest: source.status_digest,
        });
      } catch (error) {
        const failed = updateProvisioningReceipt(resolving, { phase: "failed", values: {} });
        await writeProvisioningReceiptAtomic(path, failed, { productSupportRoot, enforcePrivateModes }).catch(() => {});
        throw error;
      }
    });
  } catch (error) {
    if (error?.code !== "ELOCKED") throw error;
    const concurrent = await readProvisioningReceipt(path, { productSupportRoot });
    if (concurrent === null) throw error;
    return Object.freeze({ receipt_path: path, receipt: concurrent, resumed: true, fetch_performed: false });
  }
}

export async function beginManagedTaskDispatch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "project_id"], "managed dispatch input");
  assertNonEmpty(input.project_id, "project_id");
  return await withLockedReceipt(input, options, async (state) => {
    const receipt = state.receipt;
    const retained = receipt.operation_status.host_request;
    if (retained !== null && retained.target.projectId !== input.project_id) throw new Error("dispatch project conflicts with retained request");
    if (["dispatch_prepared", "dispatching", "queued", "dispatched", "provisioning", "provisioned", "uncertain"].includes(receipt.operation_status.phase)) {
      return Object.freeze({ should_dispatch: false, receipt_path: state.path, receipt, host_request: retained });
    }
    if (receipt.operation_status.phase !== "prepared" || receipt.operation_status.surface !== "managed_worktree") {
      throw new Error("managed dispatch requires one prepared managed-worktree receipt");
    }
    const handoff = await readTaskHandoff(state.path, receipt.handoff_digest);
    const prompt = buildManagedBootstrapPrompt({ launchId: receipt.launch_id, repositoryKey: receipt.repository_key, handoff });
    const attemptId = createHash("sha256").update(`${receipt.launch_id}\0${receipt.repository_key}\0managed-dispatch`).digest("hex");
    const hostRequest = {
      prompt,
      title: `TaskBelay ${receipt.launch_id} ${receipt.repository_key}`,
      target: {
        type: "project", projectId: input.project_id,
        environment: { type: "worktree", startingState: { type: "branch", branchName: receipt.base_commit } },
      },
    };
    const prepared = updateProvisioningReceipt(receipt, {
      phase: "dispatch_prepared", values: { dispatch_attempt_id: attemptId, host_request: hostRequest },
    });
    await persistReceipt(state.path, prepared, options);
    return Object.freeze({ should_dispatch: false, receipt_path: state.path, receipt: prepared, host_request: hostRequest });
  });
}

export async function claimManagedTaskDispatch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "dispatch_attempt_id"], "dispatch call input");
  return await withLockedReceipt(input, options, async (state) => {
    const status = state.receipt.operation_status;
    assertDispatchAttempt(status, input);
    if (status.phase !== "dispatch_prepared") {
      return { should_dispatch: false, receipt_path: state.path, receipt: state.receipt, host_request: status.host_request };
    }
    const next = updateProvisioningReceipt(state.receipt, { phase: "dispatching", values: {} });
    await persistReceipt(state.path, next, options);
    return { should_dispatch: true, receipt_path: state.path, receipt: next, host_request: next.operation_status.host_request };
  });
}

export async function recoverUncalledManagedTaskDispatch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "dispatch_attempt_id", "host_call_not_made", "previous_caller_stopped", "reason"], "dispatch recovery input");
  if (input.host_call_not_made !== true || input.previous_caller_stopped !== true) throw new Error("recovery requires an uncalled Host and stopped previous caller");
  assertNonEmpty(input.reason, "reason");
  return await withLockedReceipt(input, options, async (state) => {
    const status = state.receipt.operation_status;
    assertDispatchAttempt(status, input);
    if (status.phase !== "dispatching" || state.receipt.worktree_path !== null ||
        status.host_thread_id !== null || status.host_client_thread_id !== null) {
      throw new Error("recovery requires an uncalled dispatching receipt without Host resources");
    }
    const next = updateProvisioningReceipt(state.receipt, {
      phase: "dispatch_prepared",
      values: { dispatch_attempt_id: randomUUID(), dispatch_recovery_reason: input.reason },
    });
    await persistReceipt(state.path, next, options);
    return { should_dispatch: false, receipt_path: state.path, receipt: next, host_request: next.operation_status.host_request };
  });
}

function assertDispatchAttempt(status, input) {
  assertNonEmpty(input.dispatch_attempt_id, "dispatch_attempt_id");
  if (status.surface !== "managed_worktree" || status.host_request === null || status.dispatch_attempt_id !== input.dispatch_attempt_id) {
    throw new Error("dispatch attempt does not match the retained managed request");
  }
}

export async function reconcileManagedTaskDispatch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "candidates"], "dispatch reconciliation input");
  if (!Array.isArray(input.candidates)) throw new Error("candidates must be an array");
  for (const candidate of input.candidates) {
    assertExactKeys(candidate, ["thread_id", "initial_prompt"], "Host candidate");
    assertNonEmpty(candidate.thread_id, "thread_id");
    assertNonEmpty(candidate.initial_prompt, "initial_prompt");
  }
  return await withLockedReceipt(input, options, async (state) => {
    const status = state.receipt.operation_status;
    if (status.surface !== "managed_worktree" || status.host_request === null ||
        !["dispatching", "uncertain", "queued", "dispatched"].includes(status.phase)) {
      throw new Error("reconciliation requires a pending managed creation");
    }
    const matches = [...new Set(input.candidates.filter((entry) => entry.initial_prompt === status.host_request.prompt).map((entry) => entry.thread_id))];
    if (matches.length > 1) throw new Error("multiple Host tasks match the saved launch; inspect duplicates before continuing");
    if (matches.length === 0) return { receipt_path: state.path, receipt: state.receipt, matched: false, should_dispatch: false };
    if (status.host_thread_id !== null && status.host_thread_id !== matches[0]) throw new Error("Host task conflicts with retained thread ID");
    const next = updateProvisioningReceipt(state.receipt, { phase: "dispatched", values: { host_thread_id: matches[0] } });
    await persistReceipt(state.path, next, options);
    return { receipt_path: state.path, receipt: next, matched: true, should_dispatch: false };
  });
}

export async function recordManagedTaskDispatch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "host_result"], "managed dispatch result input");
  return await withLockedReceipt(input, options, async (state) => {
    if (![
      "dispatching",
      "queued",
      "uncertain",
    ].includes(state.receipt.operation_status.phase)) {
      return Object.freeze({ receipt_path: state.path, receipt: state.receipt, changed: false });
    }
    const result = normalizeHostCreationResult(input.host_result);
    const next = result.kind === "ready"
      ? updateProvisioningReceipt(state.receipt, {
        phase: "dispatched",
        values: { host_thread_id: result.threadId },
      })
      : result.kind === "queued"
        ? updateProvisioningReceipt(state.receipt, {
          phase: "queued",
          values: { host_client_thread_id: result.clientThreadId },
        })
        : updateProvisioningReceipt(state.receipt, { phase: "uncertain", values: {} });
    await persistReceipt(state.path, next, options);
    return Object.freeze({ receipt_path: state.path, receipt: next, changed: true });
  });
}

export async function bootstrapManagedTask(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "worktree_path"], "managed bootstrap input");
  assertAbsolutePath(input.worktree_path, "worktree_path");
  return await withLockedReceipt(input, options, async (state) => {
    const receipt = state.receipt;
    if (receipt.operation_status.phase === "provisioned") {
      return Object.freeze({
        receipt_path: state.path,
        receipt,
        workspace_origin: workspaceOriginFromReceipt(receipt),
      });
    }
    if (receipt.operation_status.surface !== "managed_worktree" || !["dispatching", "queued", "dispatched", "uncertain"].includes(receipt.operation_status.phase)) {
      throw new Error("managed bootstrap requires a dispatched or uncertain managed receipt");
    }
    const provisioning = updateProvisioningReceipt(receipt, {
      phase: "provisioning",
      values: { worktree_path: resolve(input.worktree_path) },
    });
    await persistReceipt(state.path, provisioning, options);
    try {
      const verified = await initializeManagedWorktree({
        worktreePath: input.worktree_path,
        baseCommit: provisioning.base_commit,
        targetBranch: provisioning.target_branch,
        sourceRepositoryIdentity: provisioning.source_repository_identity,
        runGit: options.runGit,
      });
      await applyWorkspaceChanges(verified.canonical_root, provisioning.snapshot_commit, snapshotRunner(options.runGit));
      const provisioned = updateProvisioningReceipt(provisioning, {
        phase: "provisioned",
        values: { worktree_path: verified.canonical_root },
      });
      await persistReceipt(state.path, provisioned, options);
      return Object.freeze({
        receipt_path: state.path,
        receipt: provisioned,
        workspace_origin: workspaceOriginFromReceipt(provisioned),
      });
    } catch (error) {
      const failed = updateProvisioningReceipt(provisioning, { phase: "failed", values: {} });
      await persistReceipt(state.path, failed, options).catch(() => {});
      throw error;
    }
  });
}

export async function provisionCliTask(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "additional_worktree_paths"], "CLI provision input");
  if (!Array.isArray(input.additional_worktree_paths)) throw new Error("additional_worktree_paths must be an array");
  for (const path of input.additional_worktree_paths) assertAbsolutePath(path, "additional worktree path");
  return await withLockedReceipt(input, options, async (state) => {
    const receipt = state.receipt;
    const handoff = await readTaskHandoff(state.path, receipt.handoff_digest);
    if (receipt.operation_status.phase === "provisioned") {
      return cliProvisionResult(state.path, receipt, input, handoff);
    }
    if (receipt.operation_status.phase !== "prepared" || receipt.operation_status.surface !== "cli_worktree" || receipt.worktree_path === null) {
      throw new Error("CLI provisioning requires one prepared receipt with a worktree path");
    }
    const provisioning = updateProvisioningReceipt(receipt, { phase: "provisioning", values: {} });
    await persistReceipt(state.path, provisioning, options);
    try {
      const verified = await createCliWorktree({
        repositoryPath: options.sourceRepositoryPath,
        worktreePath: provisioning.worktree_path,
        baseCommit: provisioning.base_commit,
        targetBranch: provisioning.target_branch,
        sourceRepositoryIdentity: provisioning.source_repository_identity,
        runGit: options.runGit,
      });
      await applyWorkspaceChanges(verified.canonical_root, provisioning.snapshot_commit, snapshotRunner(options.runGit));
      const provisioned = updateProvisioningReceipt(provisioning, {
        phase: "provisioned",
        values: { worktree_path: verified.canonical_root },
      });
      await persistReceipt(state.path, provisioned, options);
      return cliProvisionResult(state.path, provisioned, input, handoff);
    } catch (error) {
      const failed = updateProvisioningReceipt(provisioning, { phase: "failed", values: {} });
      await persistReceipt(state.path, failed, options).catch(() => {});
      throw error;
    }
  });
}

export async function provisionLocalTask(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key"], "local provision input");
  return await withLockedReceipt(input, options, async (state) => {
    const receipt = state.receipt;
    if (receipt.operation_status.surface !== "current_session") throw new Error("local provisioning requires a current-session receipt");
    if (receipt.operation_status.phase === "provisioned") return Object.freeze({ receipt_path: state.path, receipt, workspace_origin: workspaceOriginFromReceipt(receipt) });
    if (receipt.operation_status.phase !== "prepared") throw new Error("local provisioning is incomplete or uncertain; inspect the retained operation before continuing");
    await requireAvailableWorkspace(receipt.worktree_path, options.checkWorkspaceAvailable);
    const provisioning = updateProvisioningReceipt(receipt, { phase: "provisioning", values: {} });
    await persistReceipt(state.path, provisioning, options);
    try {
      await prepareLocalBranch({
        repositoryPath: receipt.worktree_path, workspaceMode: receipt.workspace_mode,
        baseBranch: receipt.base_branch, targetBranch: receipt.target_branch, baseCommit: receipt.base_commit,
        sourceRepositoryIdentity: receipt.source_repository_identity, carryChanges: receipt.carry_changes, runGit: options.runGit,
      });
      const complete = updateProvisioningReceipt(provisioning, { phase: "provisioned", values: {} });
      await persistReceipt(state.path, complete, options);
      return Object.freeze({ receipt_path: state.path, receipt: complete, workspace_origin: workspaceOriginFromReceipt(complete) });
    } catch (error) {
      await persistReceipt(state.path, updateProvisioningReceipt(provisioning, { phase: "uncertain", values: {} }), options).catch(() => {});
      throw error;
    }
  });
}

async function requireAvailableWorkspace(root, check) {
  if (typeof check !== "function") throw new Error("Core workspace availability check is required before local branch preparation");
  const result = await check(root);
  if (result?.available !== true || result.repositories?.length !== 1 || result.repositories[0].repository_path !== root) throw new Error("workspace is unavailable or already has an active TaskBelay Task; resume or resolve that Task before changing branches");
}

function cliProvisionResult(path, receipt, input, handoff) {
  return Object.freeze({
    receipt_path: path,
    receipt,
    workspace_origin: workspaceOriginFromReceipt(receipt),
    relaunch: buildCliRelaunchDescriptor({
      worktreePath: receipt.worktree_path,
      additionalWorktreePaths: input.additional_worktree_paths,
      prompt: buildManagedBootstrapPrompt({
        launchId: receipt.launch_id,
        repositoryKey: receipt.repository_key,
        handoff,
      }),
    }),
  });
}

export function workspaceOriginFromReceipt(receipt) {
  const value = validateProvisioningReceipt(receipt);
  if (value.operation_status.phase !== "provisioned") throw new Error("workspace origin requires a provisioned receipt");
  return Object.freeze({
    mode: value.workspace_mode,
    source_type: value.source_type,
    carry_changes: value.carry_changes,
    remote_name: value.remote_name,
    base_branch: value.base_branch,
    base_commit: value.base_commit,
    task_branch: value.target_branch,
    provisioning_receipt_id: provisioningReceiptID(value.launch_id, value.repository_key),
  });
}

export function provisioningReceiptID(launchId, repositoryKey) {
  assertNonEmpty(launchId, "launchId");
  assertNonEmpty(repositoryKey, "repositoryKey");
  return `codex-${createHash("sha256").update(`${launchId}\0${repositoryKey}`).digest("hex")}`;
}

export async function readOpenTaskRepositoryScope(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_keys", "primary_repository_key"], "open Task scope input");
  if (!Array.isArray(input.repository_keys) || input.repository_keys.length === 0 || input.repository_keys.length > 8 ||
      new Set(input.repository_keys).size !== input.repository_keys.length || !input.repository_keys.includes(input.primary_repository_key)) {
    throw new Error("scope requires one to eight unique repository keys including the primary");
  }
  const receipts = [];
  for (const key of input.repository_keys) {
    const path = provisioningReceiptPath(options.productSupportRoot, input.launch_id, key);
    const receipt = await readProvisioningReceipt(path, { productSupportRoot: options.productSupportRoot });
    if (receipt === null) throw new Error(`provisioning receipt is missing for repository ${key}`);
    if (receipt.launch_id !== input.launch_id || receipt.repository_key !== key) {
      throw new Error("provisioning receipt does not match the requested launch/repository");
    }
    receipts.push(receipt);
  }
  return buildOpenTaskRepositoryScope(receipts, { primaryRepositoryKey: input.primary_repository_key });
}

export function buildOpenTaskRepositoryScope(receipts, { primaryRepositoryKey = "primary" } = {}) {
  if (!Array.isArray(receipts) || receipts.length === 0 || receipts.length > 8) {
    throw new Error("open Task scope requires one to eight provisioning receipts");
  }
  assertNonEmpty(primaryRepositoryKey, "primaryRepositoryKey");
  const entries = receipts.map((receipt) => {
    const value = validateProvisioningReceipt(receipt);
    if (value.operation_status.phase !== "provisioned" || value.worktree_path === null) {
      throw new Error("every repository must be provisioned before Core Task creation");
    }
    return {
      key: value.repository_key,
      repository_path: value.worktree_path,
      workspace_origin: workspaceOriginFromReceipt(value),
    };
  });
  if (new Set(entries.map((entry) => entry.key)).size !== entries.length) {
    throw new Error("open Task repository keys must be unique");
  }
  if (new Set(entries.map((entry) => entry.repository_path)).size !== entries.length) {
    throw new Error("open Task worktree paths must be unique");
  }
  if (new Set(receipts.map((entry) => entry.launch_id)).size !== 1 || new Set(receipts.map((entry) => entry.request_digest)).size !== 1) {
    throw new Error("open Task receipts must belong to one confirmed launch and request");
  }
  const primary = entries.find((entry) => entry.key === primaryRepositoryKey);
  if (primary === undefined) throw new Error("primary repository receipt is missing");
  const result = { repositories: [primary, ...entries.filter(entry => entry.key !== primaryRepositoryKey)] };
  return Object.freeze(result);
}

export function buildCliRelaunchDescriptor({ worktreePath, additionalWorktreePaths = [], prompt } = {}) {
  assertAbsolutePath(worktreePath, "worktreePath");
  assertNonEmpty(prompt, "prompt");
  if (!Array.isArray(additionalWorktreePaths) || additionalWorktreePaths.length > 7) {
    throw new Error("additionalWorktreePaths must contain at most seven paths");
  }
  const arguments_ = ["-C", worktreePath];
  for (const path of additionalWorktreePaths) {
    assertAbsolutePath(path, "additional worktree path");
    arguments_.push("--add-dir", path);
  }
  arguments_.push("--", prompt);
  return Object.freeze({ executable: "codex", arguments: Object.freeze(arguments_) });
}




export async function cleanupCliTaskWorktree(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "terminal", "authorized"], "worktree cleanup input");
  if (input.terminal !== true || input.authorized !== true) {
    throw new Error("worktree cleanup requires terminal state and explicit authorization");
  }
  return await withLockedReceipt(input, options, async (state) => {
    if (state.receipt.workspace_mode !== "dedicated_worktree") throw new Error("local Task directories and branches are retained; workspace cleanup does not apply");
    if (state.receipt.operation_status.surface !== "cli_worktree") {
      throw new Error("managed worktree cleanup belongs to the Codex Host");
    }
    if (state.receipt.operation_status.worktree_cleanup === "requested") {
      return Object.freeze({ changed: false, uncertain: true, receipt_path: state.path, receipt: state.receipt });
    }
    if (state.receipt.operation_status.worktree_cleanup === "completed") {
      return Object.freeze({ changed: false, uncertain: false, receipt_path: state.path, receipt: state.receipt });
    }
    const requested = updateProvisioningReceipt(state.receipt, {
      phase: state.receipt.operation_status.phase,
      values: { worktree_cleanup: "requested" },
    });
    await persistReceipt(state.path, requested, options);
    try {
      await removeCliWorktree({
        repositoryPath: options.sourceRepositoryPath,
        worktreePath: requested.worktree_path,
        sourceRepositoryIdentity: requested.source_repository_identity,
        terminal: input.terminal,
        authorized: input.authorized,
        runGit: options.runGit,
      });
      const completed = updateProvisioningReceipt(requested, {
        phase: "worktree_removed",
        values: { worktree_cleanup: "completed" },
      });
      await persistReceipt(state.path, completed, options);
      return Object.freeze({ changed: true, uncertain: false, receipt_path: state.path, receipt: completed });
    } catch (error) {
      const failed = updateProvisioningReceipt(requested, {
        phase: requested.operation_status.phase,
        values: { worktree_cleanup: "failed" },
      });
      await persistReceipt(state.path, failed, options).catch(() => {});
      throw error;
    }
  });
}

export async function cleanupTaskBranch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "terminal", "authorized"], "branch cleanup input");
  if (input.terminal !== true || input.authorized !== true) {
    throw new Error("branch cleanup requires separate explicit authorization");
  }
  return await withLockedReceipt(input, options, async (state) => {
    if (state.receipt.workspace_mode !== "dedicated_worktree") throw new Error("local Task directories and branches are retained; workspace cleanup does not apply");
    if (state.receipt.operation_status.surface !== "cli_worktree") {
      throw new Error("managed branch cleanup belongs to the Codex Host");
    }
    if (state.receipt.operation_status.branch_cleanup === "requested") {
      return Object.freeze({ changed: false, uncertain: true, receipt_path: state.path, receipt: state.receipt });
    }
    if (state.receipt.operation_status.branch_cleanup === "completed") {
      return Object.freeze({ changed: false, uncertain: false, receipt_path: state.path, receipt: state.receipt });
    }
    if (state.receipt.operation_status.worktree_cleanup !== "completed") {
      throw new Error("branch cleanup requires completed worktree cleanup");
    }
    const requested = updateProvisioningReceipt(state.receipt, {
      phase: state.receipt.operation_status.phase,
      values: { branch_cleanup: "requested" },
    });
    await persistReceipt(state.path, requested, options);
    try {
      await removeTaskBranch({
        repositoryPath: options.sourceRepositoryPath,
        targetBranch: requested.target_branch,
        sourceRepositoryIdentity: requested.source_repository_identity,
        terminal: input.terminal,
        authorized: input.authorized,
        runGit: options.runGit,
      });
      const completed = updateProvisioningReceipt(requested, {
        phase: "branch_removed",
        values: { branch_cleanup: "completed" },
      });
      await persistReceipt(state.path, completed, options);
      return Object.freeze({ changed: true, uncertain: false, receipt_path: state.path, receipt: completed });
    } catch (error) {
      const failed = updateProvisioningReceipt(requested, {
        phase: requested.operation_status.phase,
        values: { branch_cleanup: "failed" },
      });
      await persistReceipt(state.path, failed, options).catch(() => {});
      throw error;
    }
  });
}

async function withLockedReceipt(input, options, operation) {
  const path = provisioningReceiptPath(options.productSupportRoot, input.launch_id, input.repository_key);
  return await withProvisioningReceiptLock(path, {
    productSupportRoot: options.productSupportRoot,
    enforcePrivateModes: options.enforcePrivateModes ?? true,
  }, async () => {
    const receipt = await readProvisioningReceipt(path, { productSupportRoot: options.productSupportRoot });
    if (receipt === null) throw new Error("provisioning receipt does not exist");
    return await operation({ path, receipt });
  });
}

async function persistReceipt(path, receipt, options) {
  return await writeProvisioningReceiptAtomic(path, receipt, {
    productSupportRoot: options.productSupportRoot,
    enforcePrivateModes: options.enforcePrivateModes ?? true,
  });
}

function normalizeHostCreationResult(value) {
  if (value?.isError === true) return { kind: "uncertain" };
  let result = normalizedStructuredResult(value);
  if (result === value && Array.isArray(value?.content)) {
    const texts = value.content.filter((entry) => entry?.type === "text");
    if (texts.length !== 1 || typeof texts[0].text !== "string") return { kind: "uncertain" };
    try {
      assertNoDuplicateJSONMembers(texts[0].text);
      result = normalizedStructuredResult(JSON.parse(texts[0].text));
    } catch {
      return { kind: "uncertain" };
    }
  }
  if (result?.isError === true) return { kind: "uncertain" };
  if (result && typeof result.threadId === "string" && result.threadId !== "") {
    return { kind: "ready", threadId: result.threadId };
  }
  if (result && typeof result.clientThreadId === "string" && result.clientThreadId !== "") {
    return { kind: "queued", clientThreadId: result.clientThreadId };
  }
  return { kind: "uncertain" };
}

function normalizedStructuredResult(value) {
  return value?.structuredContent?.result ?? value?.structuredContent ?? value?.result ?? value;
}

function validatePrepareInput(value) {
  const keys = [
    "request", "authorization_ref", "repository_key", "repository_path", "workspace_mode", "source_type", "carry_changes", "remote_name", "base_branch", "target_branch",
    "surface", "worktree_path", "handoff_file",
  ];
  if (Object.hasOwn(value ?? {}, "launch_id")) keys.push("launch_id");
  assertExactKeys(value, keys, "launch preparation input");
  assertNonEmpty(value.request, "request");
  assertNonEmpty(value.authorization_ref, "authorization_ref");
  if (!["new_branch", "current_branch", "dedicated_worktree"].includes(value.workspace_mode)) throw new Error("workspace_mode is invalid");
  if (value.workspace_mode === "dedicated_worktree") assertAbsolutePath(value.handoff_file, "handoff_file");
  else if (value.handoff_file !== null) throw new Error("current-session launch requires handoff_file=null");
  assertNonEmpty(value.repository_key, "repository_key");
  assertAbsolutePath(value.repository_path, "repository_path");
  if (!["local", "remote"].includes(value.source_type) || typeof value.carry_changes !== "boolean" || value.source_type === "remote" && value.carry_changes || value.source_type === "local" && value.remote_name !== "") throw new Error("invalid workspace source selection");
  if (value.source_type === "remote") assertNonEmpty(value.remote_name, "remote_name");
  assertNonEmpty(value.base_branch, "base_branch");
  assertNonEmpty(value.target_branch, "target_branch");
  if (!["managed_worktree", "cli_worktree", "current_session"].includes(value.surface)) throw new Error("surface is invalid");
  if (value.workspace_mode !== "dedicated_worktree" && (value.surface !== "current_session" || value.source_type !== "local" || value.worktree_path !== value.repository_path) || value.workspace_mode === "dedicated_worktree" && value.surface === "current_session") throw new Error("workspace mode does not match the launch surface");
  if (value.surface === "cli_worktree") assertAbsolutePath(value.worktree_path, "worktree_path");
  if (value.surface === "managed_worktree" && value.worktree_path !== null) {
    throw new Error("managed worktree path must be discovered from the Host");
  }
  if (Object.hasOwn(value, "launch_id")) assertNonEmpty(value.launch_id, "launch_id");
}

function assertInputMatchesReceipt(receipt, input, requestDigest, handoffDigest) {
  const requested = {
    launch_id: input.launch_id,
    authorization_ref: input.authorization_ref,
    request_digest: requestDigest,
    handoff_digest: handoffDigest,
    repository_key: input.repository_key,
    workspace_mode: input.workspace_mode,
    source_type: input.source_type,
    carry_changes: input.carry_changes,
    remote_name: input.remote_name,
    base_branch: input.base_branch,
    target_branch: input.target_branch,
    requested_worktree_path: input.worktree_path,
    surface: input.surface,
  };
  const retained = {
    launch_id: receipt.launch_id,
    authorization_ref: receipt.authorization_ref,
    request_digest: receipt.request_digest,
    handoff_digest: receipt.handoff_digest,
    repository_key: receipt.repository_key,
    workspace_mode: receipt.workspace_mode,
    source_type: receipt.source_type,
    carry_changes: receipt.carry_changes,
    remote_name: receipt.remote_name,
    base_branch: receipt.base_branch,
    target_branch: receipt.target_branch,
    requested_worktree_path: receipt.operation_status.surface === "managed_worktree" ? null : receipt.worktree_path,
    surface: receipt.operation_status.surface,
  };
  if (stableJSON(requested) !== stableJSON(retained)) {
    throw new Error("launch identity conflicts with the existing provisioning receipt");
  }
}

function assertExactKeys(value, keys, label) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error(`${label} must be an object`);
  if (stableJSON(Object.keys(value).sort()) !== stableJSON([...keys].sort())) throw new Error(`${label} has an invalid closed shape`);
}

function assertNonEmpty(value, label) {
  if (typeof value !== "string" || value.trim() === "" || value.includes("\0")) throw new Error(`${label} must be a non-empty string`);
}

function assertAbsolutePath(value, label) {
  if (typeof value !== "string" || value.includes("\0") || !isAbsolute(value) || resolve(value) !== value) {
    throw new Error(`${label} must be a normalized absolute path`);
  }
}

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

export function validateWorkspaceOrigin(value) {
  assertExactKeys(value, ["mode", "source_type", "carry_changes", "remote_name", "base_branch", "base_commit", "task_branch", "provisioning_receipt_id"], "workspace origin");
  if (!["local", "remote"].includes(value.source_type) || typeof value.carry_changes !== "boolean" || value.source_type === "remote" && (value.carry_changes || !value.remote_name) || value.source_type === "local" && value.remote_name !== "") throw new Error("invalid workspace source selection");
  if (value.source_type === "remote") assertNonEmpty(value.remote_name, "remote_name");
  if (!["new_branch", "current_branch", "dedicated_worktree"].includes(value.mode)) throw new Error("workspace origin mode is invalid");
  if (value.mode !== "dedicated_worktree" && value.source_type !== "local" || value.mode === "current_branch" && value.base_branch !== value.task_branch || value.mode === "new_branch" && value.base_branch === value.task_branch) throw new Error("workspace origin branch selection is invalid");
  for (const field of ["base_branch", "task_branch", "provisioning_receipt_id"]) assertNonEmpty(value[field], field);
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u.test(value.base_commit)) throw new Error("workspace origin base_commit is invalid");
  return structuredClone(value);
}

function snapshotRunner(runGit = defaultRunGit) {
  return async (root, args, env) => await runGit(["-C", root, ...args], { env });
}
