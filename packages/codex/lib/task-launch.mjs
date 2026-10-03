import { createHash, randomUUID } from "node:crypto";
import { lstat } from "node:fs/promises";
import { isAbsolute, resolve } from "node:path";

import { captureWorkspaceChanges, applyWorkspaceChanges } from "./worktree-snapshot.mjs";
import { assertNoDuplicateJSONMembers } from "./json.mjs";
import { requestDigest, validateLaunchAdmission } from "./task-admission.mjs";
import {
  buildManagedBootstrapPrompt,
  readTaskHandoff,
  readTaskHandoffDraft,
  taskHandoffDigest,
  taskHandoffPaths,
  writeTaskHandoff,
} from "./task-handoff.mjs";
import {
  createProvisioningReceipt,
  assertReceiptExecutable,
  receiptDigest,
  provisioningReceiptPath,
  readProvisioningReceipt,
  updateProvisioningReceipt,
  validateProvisioningReceipt,
  withProvisioningReceiptLock,
  writeProvisioningReceiptAtomic,
} from "./provisioning-receipt.mjs";
import {
  coreRepositoryBinding,
  workspaceInstanceIdentity,
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
  const admission = validateLaunchAdmission(input.assessment, input.user_choice);
  const assessmentAnchor = admission.assessment.anchor;
  if (assessmentAnchor.request_digest !== currentRequestDigest) {
    throw new Error("launch request changed after suitability assessment");
  }
  const local = input.workspace_mode !== "dedicated_worktree";
  const handoff = local ? null : await readTaskHandoffDraft(input.handoff_file, input.request);
  const handoffDigest = local ? null : taskHandoffDigest(handoff);
  const assessedRepository = assessmentAnchor.repositories.find((entry) => entry.repository_key === input.repository_key);
  if (assessedRepository === undefined) throw new Error("launch repository was not present in the suitability assessment");
  const path = provisioningReceiptPath(productSupportRoot, launchId, input.repository_key);
  const existing = await readProvisioningReceipt(path, { productSupportRoot });
  if (existing !== null) {
    await assertReceiptExecutable(existing, { productSupportRoot });
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
  const currentSource = await inspectSourceRepository(source.canonical_root, { runGit });
  if (
    assessedRepository.canonical_root !== currentSource.canonical_root ||
    assessedRepository.head !== currentSource.head ||
    assessedRepository.status_digest !== currentSource.status_digest
  ) {
    throw new Error("suitability assessment is stale; reassess before provisioning");
  }
  let initial = existing;
  if (initial === null) {
    initial = createProvisioningReceipt({
      launchId,
      admission,
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
      await assertReceiptExecutable(concurrent, { productSupportRoot });
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
      await assertReceiptExecutable(current, { productSupportRoot });
      if (current.operation_status.phase !== "confirmed") {
        return Object.freeze({ receipt_path: path, receipt: current, resumed: true, fetch_performed: false });
      }
      assertInputMatchesReceipt(current, receiptInput, currentRequestDigest, handoffDigest);
      if (handoff !== null) await writeTaskHandoff(path, handoff, { enforcePrivateModes });
      const resolving = updateProvisioningReceipt(current, { phase: "resolving", values: { target_effects: "not_invoked" } });
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

// Only a permanent predecessor mark authorizes creation of a successor. No Git
// operation is run by this entry; normal preparation consumes the saved choices.
export async function supersedeTaskLaunch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "expected_receipt_digest", "reason", "replacement"], "launch supersession input");
  assertNonEmpty(input.reason, "reason");
  validatePrepareInput(input.replacement);
  const next = input.replacement;
  if (!next.launch_id || next.launch_id === input.launch_id || next.repository_key !== input.repository_key) throw new Error("replacement requires a distinct launch_id and the same repository_key");
  const path = provisioningReceiptPath(options.productSupportRoot, input.launch_id, input.repository_key);
  return await withProvisioningReceiptLock(path, options, async () => {
    const prior = await readProvisioningReceipt(path, options);
    if (prior === null) throw new Error("predecessor receipt does not exist");
    if (prior.supersession !== undefined) throw new Error("launch already superseded; resume its exact saved successor seed");
    await assertReceiptExecutable(prior, options);
    if (receiptDigest(prior) !== input.expected_receipt_digest) throw new Error("receipt changed; read its current digest before superseding");
    const status = prior.operation_status;
    if (!["confirmed", "prepared", "dispatch_prepared", "resolving", "failed"].includes(status.phase) || status.target_effects === "may_have_effects" || status.dispatch_recovery_reason !== null || ["failed", "resolving"].includes(status.phase) && status.target_effects !== "not_invoked" || status.host_thread_id !== null || status.host_client_thread_id !== null || status.host_operation_id !== null) throw new Error("launch has no positive proof of zero target calls; reconcile its existing operation");
    const sourcePath = prior.admission.assessment.anchor.repositories.find((entry) => entry.repository_key === prior.repository_key)?.canonical_root;
    if (sourcePath !== next.repository_path) throw new Error("replacement must retain the confirmed source repository");
    const source = await inspectSourceRepository(sourcePath, { runGit: options.runGit });
    if (source.source_repository_identity !== prior.source_repository_identity) throw new Error("source repository identity changed");
    if (prior.workspace_mode !== "dedicated_worktree") await requireAvailableWorkspace(prior.worktree_path, options.checkWorkspaceAvailable);
    else if (prior.worktree_path !== null) {
      try { await lstat(prior.worktree_path); throw new Error("prior target directory exists; reconcile it before superseding"); }
      catch (error) { if (error?.code !== "ENOENT") throw error; }
    }
    if (prior.workspace_mode !== "current_branch") {
      // --verify --quiet has the distinguished missing-ref exit status 1.
      const run = options.runGit ?? defaultRunGit;
      try { await run(["-C", sourcePath, "show-ref", "--verify", "--quiet", `refs/heads/${prior.target_branch}`]); throw new Error("prior target branch exists; reconcile it before superseding"); }
      catch (error) { if (error?.code !== 1) throw error; }
    }
    const admission = validateLaunchAdmission(next.assessment, next.user_choice);
    const anchor = admission.assessment.anchor;
    const assessed = anchor.repositories.find((entry) => entry.repository_key === next.repository_key);
    if (anchor.request_digest !== requestDigest(next.request) || assessed?.canonical_root !== source.canonical_root || assessed.head !== source.head || assessed.status_digest !== source.status_digest) throw new Error("replacement assessment is stale");
    const local = next.workspace_mode !== "dedicated_worktree";
    await (local ? preflightLocalBranchSelection : preflightWorktreeSelection)({ repositoryPath:sourcePath, workspaceMode:next.workspace_mode, sourceType:next.source_type, remoteName:next.remote_name, baseBranch:next.base_branch, targetBranch:next.target_branch, carryChanges:next.carry_changes, runGit:options.runGit });
    if (local) await requireAvailableWorkspace(next.worktree_path, options.checkWorkspaceAvailable);
    const handoff = local ? null : await readTaskHandoffDraft(next.handoff_file, next.request);
    const successor = createProvisioningReceipt({launchId:next.launch_id, admission, requestDigest:requestDigest(next.request), handoffDigest:handoff === null ? null : taskHandoffDigest(handoff), sourceRepositoryIdentity:source.source_repository_identity, repositoryKey:next.repository_key, workspaceMode:next.workspace_mode, remoteName:next.remote_name, sourceType:next.source_type, carryChanges:next.carry_changes, baseBranch:next.base_branch, targetBranch:next.target_branch, worktreePath:next.worktree_path, surface:next.surface, createdAt:(options.now?.() ?? new Date()).toISOString()});
    successor.predecessor = {launch_id:prior.launch_id, repository_key:prior.repository_key, receipt_digest:input.expected_receipt_digest};
    const seed = {receipt:successor, prepare_input:structuredClone(next), handoff};
    const marked = { ...prior, operation_status:{...status, phase:"superseded"}, supersession:{reason:input.reason, prior_phase:status.phase, receipt_digest:input.expected_receipt_digest, seed_digest:receiptDigest(seed), seed} };
    const successorPath = provisioningReceiptPath(options.productSupportRoot, successor.launch_id, successor.repository_key);
    return await withProvisioningReceiptLock(successorPath, options, async () => {
      if (await readProvisioningReceipt(successorPath, options) !== null) throw new Error("successor launch identity already exists");
      // Linearization point. An interrupted successor write never reactivates prior.
      await writeProvisioningReceiptAtomic(path, marked, options);
      await options.afterSupersession?.();
      return await materializeLaunchSuccessor(marked, successorPath, options);
    });
  });
}

export async function resumeLaunchSupersession(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "seed_digest"], "supersession resume input");
  const path = provisioningReceiptPath(options.productSupportRoot, input.launch_id, input.repository_key);
  return await withProvisioningReceiptLock(path, options, async () => {
    const prior = await readProvisioningReceipt(path, options);
    if (prior?.supersession?.seed_digest !== input.seed_digest) throw new Error("no matching retained successor seed");
    const successor = prior.supersession.seed.receipt;
    const successorPath = provisioningReceiptPath(options.productSupportRoot, successor.launch_id, successor.repository_key);
    return await withProvisioningReceiptLock(successorPath, options, async () => await materializeLaunchSuccessor(prior, successorPath, options));
  });
}

async function materializeLaunchSuccessor(prior, path, options) {
  const seed = prior.supersession.seed;
  validatePrepareInput(seed.prepare_input);
  let receipt = await readProvisioningReceipt(path, options);
  if (receipt !== null) {
    await assertReceiptExecutable(receipt, options);
    if (receiptDigest(receipt.predecessor) !== receiptDigest(seed.receipt.predecessor)) throw new Error("successor identity belongs to another predecessor");
  } else {
    if (seed.handoff !== null) {
      if (taskHandoffDigest(seed.handoff) !== seed.receipt.handoff_digest) throw new Error("successor handoff differs from its seed");
      await writeTaskHandoff(path, seed.handoff, options);
    }
    receipt = await writeProvisioningReceiptAtomic(path, seed.receipt, {...options, createOnly:true});
  }
  const prepareInput = {...seed.prepare_input, handoff_file:seed.handoff === null ? null : taskHandoffPaths(path).json_path};
  return {receipt_path:path, receipt, receipt_digest:receiptDigest(receipt), predecessor:prior, prepare_input:prepareInput};
}

export async function claimManagedTaskDispatch(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "dispatch_attempt_id"], "dispatch call input");
  return await withLockedReceipt(input, options, async (state) => {
    const status = state.receipt.operation_status;
    assertDispatchAttempt(status, input);
    if (status.phase !== "dispatch_prepared") {
      return { should_dispatch: false, receipt_path: state.path, receipt: state.receipt, host_request: status.host_request };
    }
    const next = updateProvisioningReceipt(state.receipt, { phase: "dispatching", values: { target_effects: "may_have_effects" } });
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
        status.host_thread_id !== null || status.host_client_thread_id !== null || status.host_operation_id !== null || status.relocation_id !== null) {
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
    if (status.surface !== "managed_worktree" || status.host_request === null || status.relocation_id !== null ||
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
      ...(state.receipt.operation_status.relocation_id === null ? ["uncertain"] : []),
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
        values: { worktree_path: verified.canonical_root, worktree_identity:await workspaceInstanceIdentity(verified) },
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
    let provisioning = updateProvisioningReceipt(receipt, { phase: "provisioning", values: { target_effects: "not_invoked" } });
    await persistReceipt(state.path, provisioning, options);
    try {
      const verified = await createCliWorktree({
        repositoryPath: options.sourceRepositoryPath,
        worktreePath: provisioning.worktree_path,
        baseCommit: provisioning.base_commit,
        targetBranch: provisioning.target_branch,
        sourceRepositoryIdentity: provisioning.source_repository_identity,
        runGit: options.runGit,
        beforeTargetEffect: async () => {
          provisioning = updateProvisioningReceipt(provisioning, { phase: "provisioning", values: { target_effects: "may_have_effects" } });
          await persistReceipt(state.path, provisioning, options);
        },
      });
      await applyWorkspaceChanges(verified.canonical_root, provisioning.snapshot_commit, snapshotRunner(options.runGit));
      const provisioned = updateProvisioningReceipt(provisioning, {
        phase: "provisioned",
        values: { worktree_path: verified.canonical_root, worktree_identity:await workspaceInstanceIdentity(verified) },
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
    let provisioning = updateProvisioningReceipt(receipt, { phase: "provisioning", values: { target_effects: "not_invoked" } });
    await persistReceipt(state.path, provisioning, options);
    try {
      await prepareLocalBranch({
        repositoryPath: receipt.worktree_path, workspaceMode: receipt.workspace_mode,
        baseBranch: receipt.base_branch, targetBranch: receipt.target_branch, baseCommit: receipt.base_commit,
        sourceRepositoryIdentity: receipt.source_repository_identity, carryChanges: receipt.carry_changes, runGit: options.runGit,
        beforeTargetEffect: async () => {
          provisioning = updateProvisioningReceipt(provisioning, { phase: "provisioning", values: { target_effects: "may_have_effects" } });
          await persistReceipt(state.path, provisioning, options);
        },
      });
      const complete = updateProvisioningReceipt(provisioning, { phase: "provisioned", values: {worktree_identity:await workspaceInstanceIdentity(await inspectSourceRepository(receipt.worktree_path,{runGit:options.runGit}))} });
      await persistReceipt(state.path, complete, options);
      return Object.freeze({ receipt_path: state.path, receipt: complete, workspace_origin: workspaceOriginFromReceipt(complete) });
    } catch (error) {
      await persistReceipt(state.path, updateProvisioningReceipt(provisioning, { phase: provisioning.operation_status.target_effects === "not_invoked" ? "failed" : "uncertain", values: {} }), options).catch(() => {});
      throw error;
    }
  });
}

async function requireAvailableWorkspace(root, check) {
  if (typeof check !== "function") throw new Error("Core workspace availability check is required before local branch preparation");
  const result = await check(root);
  if (result?.available !== true || result.repository_path !== root) throw new Error("workspace is unavailable or already has an active TaskBelay Task; resume or resolve that Task before changing branches");
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
  const result = {
    repository_path: primary.repository_path,
    workspace_origin: primary.workspace_origin,
  };
  if (entries.length > 1) {
    result.primary_repository_key = primaryRepositoryKey;
    result.additional_repositories = entries
      .filter((entry) => entry.key !== primaryRepositoryKey)
      .sort((left, right) => left.key.localeCompare(right.key));
  }
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

export async function beginTaskHandoff(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "relocation_id", "thread_id"], "handoff input");
  assertNonEmpty(input.relocation_id, "relocation_id");
  assertNonEmpty(input.thread_id, "thread_id");
  return await withLockedReceipt(input, options, async (state) => {
    const receipt = state.receipt;
    if (receipt.workspace_mode !== "dedicated_worktree") throw new Error("local branch Tasks retain their original directory and do not support worktree handoff");
    if (["handoff_dispatching", "handoff_pending"].includes(receipt.operation_status.phase)) {
      return Object.freeze({ should_dispatch: false, receipt_path: state.path, receipt });
    }
    if (["handoff_succeeded", "handoff_failed"].includes(receipt.operation_status.phase) && receipt.operation_status.relocation_id === input.relocation_id) {
      return Object.freeze({ should_dispatch: false, receipt_path: state.path, receipt });
    }
    if (!["provisioned", "handoff_succeeded", "handoff_failed"].includes(receipt.operation_status.phase)) {
      throw new Error("handoff requires a provisioned Task receipt");
    }
    const next = updateProvisioningReceipt(receipt, {
      phase: "handoff_dispatching",
      values: {
        relocation_id: input.relocation_id,
        host_operation_id: null,
        host_operation_revision: null,
      },
    });
    await persistReceipt(state.path, next, options);
    return Object.freeze({
      should_dispatch: true,
      receipt_path: state.path,
      receipt: next,
      host_request: Object.freeze({
        threadId: input.thread_id,
        followUpPrompt: `Resume TaskBelay relocation ${input.relocation_id}; inspect the Host result, then resolve the Core blocker with the exact destination repository paths.`,
      }),
    });
  });
}

export async function recordTaskHandoff(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "host_result"], "handoff result input");
  return await withLockedReceipt(input, options, async (state) => {
    if (![
      "handoff_dispatching",
      ...(state.receipt.operation_status.relocation_id === null ? [] : ["uncertain"]),
    ].includes(state.receipt.operation_status.phase)) {
      return Object.freeze({ receipt_path: state.path, receipt: state.receipt, changed: false });
    }
    const hostResult = normalizedStructuredResult(input.host_result);
    const operationId = hostResult?.operationId;
    const revision = hostResult?.revision;
    const valid = typeof operationId === "string" && operationId !== "" && Number.isSafeInteger(revision) && revision >= 0;
    const next = valid
      ? updateProvisioningReceipt(state.receipt, {
        phase: "handoff_pending",
        values: { host_operation_id: operationId, host_operation_revision: revision },
      })
      : updateProvisioningReceipt(state.receipt, { phase: "uncertain", values: {} });
    await persistReceipt(state.path, next, options);
    return Object.freeze({ receipt_path: state.path, receipt: next, changed: true });
  });
}

export async function recordTaskHandoffStatus(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "status", "revision", "worktree_path"], "handoff status input");
  if (!["succeeded", "failed", "pending"].includes(input.status)) throw new Error("handoff status is invalid");
  if (!Number.isSafeInteger(input.revision) || input.revision < 0) throw new Error("handoff revision is invalid");
  if (input.worktree_path !== null) assertAbsolutePath(input.worktree_path, "worktree_path");
  return await withLockedReceipt(input, options, async (state) => {
    if (state.receipt.operation_status.phase !== "handoff_pending") {
      return Object.freeze({ receipt_path: state.path, receipt: state.receipt, changed: false });
    }
    const phase = input.status === "succeeded" ? "handoff_succeeded" : input.status === "failed" ? "handoff_failed" : "handoff_pending";
    const values = { host_operation_revision: input.revision };
    if (input.worktree_path !== null) values.worktree_path = input.worktree_path;
    const next = updateProvisioningReceipt(state.receipt, { phase, values });
    await persistReceipt(state.path, next, options);
    return Object.freeze({
      receipt_path: state.path,
      receipt: next,
      changed: true,
      relocation_id: next.operation_status.relocation_id,
    });
  });
}

export async function cleanupCliTaskWorktree(input, options = {}) {
  assertExactKeys(input, ["launch_id", "repository_key", "terminal", "authorized", "core_task"], "worktree cleanup input");
  if (input.terminal !== true || input.authorized !== true) {
    throw new Error("worktree cleanup requires terminal state and explicit authorization");
  }
  return await withLockedReceipt(input, options, async (state) => {
    if (!["DONE","CANCELLED"].includes(input.core_task?.current_cursor)) throw new Error("Actual terminal Core Task is required for cleanup");
    const binding=coreRepositoryBinding(input.core_task,{host:"codex",repositoryKey:state.receipt.repository_key,receiptId:provisioningReceiptID(state.receipt.launch_id,state.receipt.repository_key),creationBranch:state.receipt.target_branch,worktreePath:state.receipt.worktree_path});
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
    const actual=await inspectSourceRepository(state.receipt.worktree_path,{runGit:options.runGit});
    if (!state.receipt.operation_status.worktree_identity || await workspaceInstanceIdentity(actual)!==state.receipt.operation_status.worktree_identity) throw new Error("Cleanup workspace instance is unverified or changed; retain it for manual inspection");
    if (actual.branch!==binding.current_branch || actual.head!==binding.current_head) throw new Error("Cleanup workspace differs from the terminal Core binding");
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
  assertExactKeys(input, ["launch_id", "repository_key", "terminal", "authorized", "core_task"], "branch cleanup input");
  if (input.terminal !== true || input.authorized !== true) {
    throw new Error("branch cleanup requires separate explicit authorization");
  }
  return await withLockedReceipt(input, options, async (state) => {
    if (!["DONE","CANCELLED"].includes(input.core_task?.current_cursor)) throw new Error("Actual terminal Core Task is required for cleanup");
    const binding=coreRepositoryBinding(input.core_task,{host:"codex",repositoryKey:state.receipt.repository_key,receiptId:provisioningReceiptID(state.receipt.launch_id,state.receipt.repository_key),creationBranch:state.receipt.target_branch,worktreePath:state.receipt.worktree_path});
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
        targetBranch: binding.current_branch,
        expectedHead: binding.current_head,
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
    await assertReceiptExecutable(receipt, options);
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
    "request", "assessment", "user_choice", "repository_key", "repository_path", "workspace_mode", "source_type", "carry_changes", "remote_name", "base_branch", "target_branch",
    "surface", "worktree_path", "handoff_file",
  ];
  if (Object.hasOwn(value ?? {}, "launch_id")) keys.push("launch_id");
  assertExactKeys(value, keys, "launch preparation input");
  assertNonEmpty(value.request, "request");
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
    admission: { assessment: input.assessment, user_choice: input.user_choice },
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
    admission: receipt.admission,
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
