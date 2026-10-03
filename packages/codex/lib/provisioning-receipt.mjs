import { validateLaunchAdmission } from "./task-admission.mjs";
import { createHash, randomBytes } from "node:crypto";
import { chmod, link, lstat, mkdir, open, readFile, realpath, rename, unlink, writeFile } from "node:fs/promises";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

const digestPattern = /^[0-9a-f]{64}$/u;
const commitPattern = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u;
const identifierPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,191}$/u;
const launchIdPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u;
const repositoryKeyPattern = /^[a-z0-9][a-z0-9._-]{0,127}$/u;
const remoteNamePattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u;

export const PROVISIONING_PHASES = Object.freeze([
  "confirmed",
  "resolving",
  "prepared",
  "dispatch_prepared",
  "dispatching",
  "provisioning",
  "queued",
  "dispatched",
  "provisioned",
  "failed",
  "superseded",
  "uncertain",
  "handoff_dispatching",
  "handoff_pending",
  "handoff_succeeded",
  "handoff_failed",
  "worktree_removed",
  "branch_removed",
]);

const cleanupStates = Object.freeze(["not_requested", "requested", "completed", "failed"]);
const phaseTransitions = Object.freeze({
  confirmed: Object.freeze(["confirmed", "resolving", "failed"]),
  resolving: Object.freeze(["resolving", "prepared", "failed", "uncertain"]),
  prepared: Object.freeze(["prepared", "dispatch_prepared", "provisioning", "failed", "uncertain"]),
  dispatch_prepared: Object.freeze(["dispatch_prepared", "dispatching"]),
  dispatching: Object.freeze(["dispatch_prepared", "dispatching", "queued", "dispatched", "provisioning", "failed", "uncertain"]),
  queued: Object.freeze(["queued", "dispatched", "provisioning", "failed", "uncertain"]),
  dispatched: Object.freeze(["dispatched", "provisioning", "failed", "uncertain"]),
  provisioning: Object.freeze(["provisioning", "provisioned", "failed", "uncertain"]),
  provisioned: Object.freeze(["provisioned", "handoff_dispatching", "worktree_removed"]),
  uncertain: Object.freeze(["uncertain", "queued", "dispatched", "provisioning", "provisioned", "handoff_pending", "failed"]),
  failed: Object.freeze(["failed"]),
  superseded: Object.freeze([]),
  handoff_dispatching: Object.freeze(["handoff_dispatching", "handoff_pending", "handoff_failed", "uncertain"]),
  handoff_pending: Object.freeze(["handoff_pending", "handoff_succeeded", "handoff_failed", "uncertain"]),
  handoff_succeeded: Object.freeze(["handoff_succeeded", "handoff_dispatching", "worktree_removed"]),
  handoff_failed: Object.freeze(["handoff_failed", "handoff_dispatching", "worktree_removed"]),
  worktree_removed: Object.freeze(["worktree_removed", "branch_removed"]),
  branch_removed: Object.freeze(["branch_removed"]),
});

export function createProvisioningReceipt({
  launchId,
  admission,
  requestDigest,
  handoffDigest,
  sourceRepositoryIdentity,
  repositoryKey,
  workspaceMode,
  remoteName,
  sourceType,
  carryChanges,
  baseBranch,
  targetBranch,
  worktreePath = null,
  surface,
  createdAt = new Date().toISOString(),
} = {}) {
  return validateProvisioningReceipt({
    launch_id: launchId,
    admission,
    host: "codex",
    request_digest: requestDigest,
    handoff_digest: handoffDigest,
    source_repository_identity: sourceRepositoryIdentity,
    repository_key: repositoryKey,
    workspace_mode: workspaceMode,
    remote_name: remoteName,
    source_type: sourceType,
    carry_changes: carryChanges,
    snapshot_commit: null,
    base_branch: baseBranch,
    target_branch: targetBranch,
    base_commit: null,
    worktree_path: worktreePath,
    operation_status: {
      phase: "confirmed",
      surface,
      dispatch_attempt_id: null,
      host_request: null,
      dispatch_recovery_reason: null,
      host_thread_id: null,
      host_client_thread_id: null,
      host_operation_id: null,
      host_operation_revision: null,
      relocation_id: null,
      worktree_cleanup: "not_requested",
      branch_cleanup: "not_requested",
    },
    created_at: createdAt,
  });
}

export function validateProvisioningReceipt(value) {
  assertExactKeys(value, [
    "launch_id",
    "admission",
    "host",
    "request_digest",
    "handoff_digest",
    "source_repository_identity",
    "repository_key", "workspace_mode",
    "remote_name", "source_type", "carry_changes", "snapshot_commit",
    "base_branch",
    "target_branch",
    "base_commit",
    "worktree_path",
    "operation_status",
    "created_at",
    ...["predecessor", "supersession"].filter((key) => Object.hasOwn(value ?? {}, key)),
  ], "provisioning receipt");
  assertExactKeys(value.admission, ["assessment", "user_choice"], "receipt admission");
  validateLaunchAdmission(value.admission.assessment, value.admission.user_choice);
  if (value.admission.assessment.anchor.request_digest !== value.request_digest || !value.admission.assessment.anchor.repositories.some((entry) => entry.repository_key === value.repository_key)) {
    throw new Error("receipt admission does not match the request and repository");
  }
  assertLaunchID(value.launch_id);
  if (value.host !== "codex") throw new Error("provisioning receipt host must equal codex");
  if (!digestPattern.test(value.request_digest)) throw new Error("provisioning receipt request_digest is invalid");
  if (!["new_branch", "current_branch", "dedicated_worktree"].includes(value.workspace_mode)) throw new Error("workspace mode is invalid");
  if (value.workspace_mode === "dedicated_worktree" ? !digestPattern.test(value.handoff_digest) : value.handoff_digest !== null) throw new Error("provisioning receipt handoff_digest is invalid");
  if (!digestPattern.test(value.source_repository_identity)) {
    throw new Error("provisioning receipt source_repository_identity is invalid");
  }
  if (!repositoryKeyPattern.test(value.repository_key)) throw new Error("provisioning receipt repository_key is invalid");
  if (!["local", "remote"].includes(value.source_type) || typeof value.carry_changes !== "boolean" || value.source_type === "remote" && value.carry_changes || value.source_type === "local" && value.remote_name !== "") throw new Error("invalid workspace source selection");
  if (value.snapshot_commit !== null && (!commitPattern.test(value.snapshot_commit) || !value.carry_changes)) throw new Error("invalid snapshot commit");
  if (value.source_type === "remote" && !remoteNamePattern.test(value.remote_name)) throw new Error("provisioning receipt remote_name is invalid");
  assertBranchText(value.base_branch, "base_branch");
  assertBranchText(value.target_branch, "target_branch");
  if (value.base_commit !== null && !commitPattern.test(value.base_commit)) {
    throw new Error("provisioning receipt base_commit is invalid");
  }
  if (value.worktree_path !== null) assertNormalizedAbsolutePath(value.worktree_path, "worktree_path");
  validateOperationStatus(value.operation_status);
  if (value.workspace_mode !== "dedicated_worktree" && (value.source_type !== "local" || value.operation_status.surface !== "current_session" || value.worktree_path === null) || value.workspace_mode === "dedicated_worktree" && value.operation_status.surface === "current_session") throw new Error("workspace mode does not match the launch surface");
  if (value.workspace_mode === "current_branch" && value.base_branch !== value.target_branch || value.workspace_mode === "new_branch" && value.base_branch === value.target_branch) throw new Error("workspace mode does not match the selected branch");
  if (!Number.isFinite(Date.parse(value.created_at))) throw new Error("provisioning receipt created_at is invalid");
  if (!["confirmed", "resolving", "failed", "superseded"].includes(value.operation_status.phase) && value.base_commit === null) {
    throw new Error("provisioning receipt phase requires base_commit");
  }
  if (["provisioning", "provisioned", "handoff_dispatching", "handoff_pending", "handoff_succeeded", "handoff_failed"].includes(value.operation_status.phase) && value.worktree_path === null) {
    throw new Error("provisioning receipt phase requires worktree_path");
  }
  if (value.predecessor !== undefined) {
    assertExactKeys(value.predecessor, ["launch_id", "repository_key", "receipt_digest"], "receipt predecessor");
    assertLaunchID(value.predecessor.launch_id);
    if (value.predecessor.launch_id === value.launch_id || value.predecessor.repository_key !== value.repository_key || !digestPattern.test(value.predecessor.receipt_digest)) throw new Error("receipt predecessor is invalid");
  }
  if ((value.operation_status.phase === "superseded") !== (value.supersession !== undefined)) throw new Error("superseded receipt requires its permanent successor seed");
  if (value.supersession !== undefined) {
    const mark = value.supersession;
    assertExactKeys(mark, ["reason", "prior_phase", "receipt_digest", "seed_digest", "seed"], "receipt supersession");
    assertExactKeys(mark.seed, ["receipt", "prepare_input", "handoff"], "successor seed");
    if (typeof mark.reason !== "string" || !mark.reason.trim() || mark.reason.length > 4096 || !digestPattern.test(mark.receipt_digest) || mark.seed.receipt?.supersession !== undefined || mark.seed.receipt?.operation_status?.phase !== "confirmed") throw new Error("invalid successor seed");
    const successor = validateProvisioningReceipt(mark.seed.receipt);
    if (successor.predecessor?.launch_id !== value.launch_id || successor.predecessor.repository_key !== value.repository_key || successor.predecessor.receipt_digest !== mark.receipt_digest || receiptDigest(mark.seed) !== mark.seed_digest) throw new Error("successor seed does not match its predecessor");
    const prior = structuredClone(value);
    delete prior.supersession;
    prior.operation_status.phase = mark.prior_phase;
    if (!["confirmed", "resolving", "prepared", "dispatch_prepared", "failed"].includes(mark.prior_phase) || receiptDigest(prior) !== mark.receipt_digest) throw new Error("supersession changed the preceding receipt");
  }
  return structuredClone(value);
}

export function provisioningReceiptPath(productSupportRoot, launchId, repositoryKey) {
  assertNormalizedAbsolutePath(productSupportRoot, "product support root");
  assertLaunchID(launchId);
  if (!repositoryKeyPattern.test(repositoryKey)) throw new Error("repository_key is invalid");
  const root = join(productSupportRoot, "provisioning", "codex");
  return containedPath(root, join(root, launchId, `${repositoryKey}.json`));
}

export async function readProvisioningReceipt(path, { productSupportRoot } = {}) {
  const expectedRoot = receiptRoot(productSupportRoot);
  containedPath(expectedRoot, path);
  await assertNoSymbolicLinkComponents(productSupportRoot, dirname(path));
  await rejectSymbolicLink(path);
  let contents;
  try {
    contents = await readFile(path, "utf8");
  } catch (error) {
    if (error?.code === "ENOENT") return null;
    throw new Error("read provisioning receipt", { cause: error });
  }
  try {
    return validateProvisioningReceipt(JSON.parse(contents));
  } catch (error) {
    throw new Error(`invalid provisioning receipt: ${error.message}`, { cause: error });
  }
}

export async function writeProvisioningReceiptAtomic(path, receipt, {
  productSupportRoot,
  enforcePrivateModes = true,
  createOnly = false,
} = {}) {
  const expectedRoot = receiptRoot(productSupportRoot);
  containedPath(expectedRoot, path);
  const validated = validateProvisioningReceipt(receipt);
  const expectedPath = provisioningReceiptPath(
    productSupportRoot,
    validated.launch_id,
    validated.repository_key,
  );
  if (path !== expectedPath) throw new Error("provisioning receipt path does not match its identity");
  await ensureOwnedDirectory(productSupportRoot, enforcePrivateModes);
  await ensureOwnedDirectory(join(productSupportRoot, "provisioning"), enforcePrivateModes, productSupportRoot);
  await ensureOwnedDirectory(expectedRoot, enforcePrivateModes, productSupportRoot);
  const parent = dirname(path);
  await ensureOwnedDirectory(parent, enforcePrivateModes, productSupportRoot);
  await rejectSymbolicLink(path);
  if (createOnly) {
    try {
      await lstat(path);
      const conflict = new Error("provisioning receipt already exists");
      conflict.code = "EEXIST";
      throw conflict;
    } catch (error) {
      if (error?.code !== "ENOENT") throw error;
    }
  }
  const temporaryPath = join(parent, `.${basename(path)}.tmp-${process.pid}-${randomBytes(8).toString("hex")}`);
  try {
    await writeFile(temporaryPath, `${JSON.stringify(validated, null, 2)}\n`, {
      encoding: "utf8",
      mode: 0o600,
      flag: "wx",
      flush: true,
    });
    if (createOnly) {
      await link(temporaryPath, path);
      await unlink(temporaryPath);
    } else await rename(temporaryPath, path);
    if (enforcePrivateModes) await chmod(path, 0o600);
  } catch (error) {
    await unlink(temporaryPath).catch(() => {});
    throw error;
  }
  return validated;
}

export async function withProvisioningReceiptLock(
  path,
  { productSupportRoot, enforcePrivateModes = true } = {},
  operation,
) {
  if (typeof operation !== "function") throw new Error("provisioning receipt lock operation is required");
  const expectedRoot = receiptRoot(productSupportRoot);
  containedPath(expectedRoot, path);
  const parent = dirname(path);
  await ensureOwnedDirectory(productSupportRoot, enforcePrivateModes);
  await ensureOwnedDirectory(join(productSupportRoot, "provisioning"), enforcePrivateModes, productSupportRoot);
  await ensureOwnedDirectory(expectedRoot, enforcePrivateModes, productSupportRoot);
  await ensureOwnedDirectory(parent, enforcePrivateModes, productSupportRoot);
  const lockPath = `${path}.lock`;
  containedPath(expectedRoot, lockPath);
  const handle = await acquireReceiptLock(lockPath, enforcePrivateModes);
  try {
    return await operation();
  } finally {
    await handle.close().catch(() => {});
    await unlink(lockPath).catch(() => {});
  }
}

async function acquireReceiptLock(lockPath, enforcePrivateModes, reclaimAttempted = false) {
  let handle;
  try {
    handle = await open(lockPath, "wx", 0o600);
    await handle.writeFile(`${JSON.stringify({ pid: process.pid, created_at: new Date().toISOString() })}\n`, "utf8");
    await handle.sync();
    if (enforcePrivateModes) await chmod(lockPath, 0o600);
    return handle;
  } catch (error) {
    await handle?.close().catch(() => {});
    if (handle !== undefined) await unlink(lockPath).catch(() => {});
    if (error?.code !== "EEXIST" || reclaimAttempted) {
      if (error?.code === "EEXIST") {
        throw lockedReceiptError();
      }
      throw error;
    }
  }

  let retained;
  try {
    const info = await lstat(lockPath);
    if (!info.isFile() || info.isSymbolicLink()) throw new Error("provisioning lock is not a regular file");
    retained = JSON.parse(await readFile(lockPath, "utf8"));
  } catch (error) {
    throw new Error("inspect existing provisioning lock", { cause: error });
  }
  if (!Number.isSafeInteger(retained?.pid) || retained.pid <= 0 || typeof retained.created_at !== "string") {
    throw new Error("existing provisioning lock is invalid; read the receipt instead");
  }
  if (processIsAlive(retained.pid)) {
    throw lockedReceiptError();
  }
  await unlink(lockPath);
  return await acquireReceiptLock(lockPath, enforcePrivateModes, true);
}

function lockedReceiptError() {
  const error = new Error("provisioning operation is already in progress; read the receipt instead");
  error.code = "ELOCKED";
  return error;
}

function processIsAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return error?.code === "EPERM";
  }
}

export function updateProvisioningReceipt(receipt, patch) {
  const current = validateProvisioningReceipt(receipt);
  assertExactKeys(patch, ["phase", "values"], "provisioning receipt update");
  if (!PROVISIONING_PHASES.includes(patch.phase)) throw new Error("provisioning receipt update phase is invalid");
  if (!phaseTransitions[current.operation_status.phase].includes(patch.phase)) {
    throw new Error(`invalid provisioning phase transition ${current.operation_status.phase} -> ${patch.phase}`);
  }
  if (patch.values === null || typeof patch.values !== "object" || Array.isArray(patch.values)) {
    throw new Error("provisioning receipt update values must be an object");
  }
  const allowed = new Set([
    "base_commit", "snapshot_commit", "worktree_path", "dispatch_attempt_id", "host_thread_id",
    "host_client_thread_id", "host_operation_id", "host_operation_revision", "relocation_id",
    "worktree_cleanup", "branch_cleanup", "host_request", "dispatch_recovery_reason", "target_effects", "worktree_identity",
  ]);
  for (const key of Object.keys(patch.values)) {
    if (!allowed.has(key)) throw new Error(`provisioning receipt update cannot change ${key}`);
  }
  if (current.operation_status.target_effects === "may_have_effects" && patch.values.target_effects === "not_invoked") throw new Error("target-call evidence cannot be reset");
  const next = structuredClone(current);
  for (const field of ["base_commit", "snapshot_commit", "worktree_path"]) {
    if (Object.hasOwn(patch.values, field)) next[field] = patch.values[field];
  }
  next.operation_status.phase = patch.phase;
  for (const [field, value] of Object.entries(patch.values)) {
    if (field !== "base_commit" && field !== "snapshot_commit" && field !== "worktree_path") next.operation_status[field] = value;
  }
  return validateProvisioningReceipt(next);
}

function validateOperationStatus(value) {
  if (value?.worktree_identity !== undefined && !/^[a-f0-9]{64}$/.test(value.worktree_identity)) throw new Error("worktree_identity is invalid");
  assertExactKeys(value, [
    "phase", "surface", "dispatch_attempt_id", "host_thread_id", "host_client_thread_id",
    "host_operation_id", "host_operation_revision", "relocation_id", "worktree_cleanup", "branch_cleanup", "host_request", "dispatch_recovery_reason",
    ...(Object.hasOwn(value ?? {}, "target_effects") ? ["target_effects"] : []),
    ...(Object.hasOwn(value ?? {}, "worktree_identity") ? ["worktree_identity"] : []),
  ], "provisioning operation_status");
  if (value.target_effects !== undefined && !["not_invoked", "may_have_effects"].includes(value.target_effects)) throw new Error("invalid target call evidence");
  if (value.host_request !== null) {
    const request = value.host_request;
    assertExactKeys(request, ["prompt", "title", "target"], "host_request");
    for (const field of ["prompt", "title"]) {
      if (typeof request[field] !== "string" || request[field].trim() === "") throw new Error(`host_request.${field} is invalid`);
    }
    assertExactKeys(request.target, ["type", "projectId", "environment"], "host_request.target");
    assertExactKeys(request.target.environment, ["type", "startingState"], "host_request.environment");
    assertExactKeys(request.target.environment.startingState, ["type", "branchName"], "host_request.startingState");
    if (request.target.type !== "project" || typeof request.target.projectId !== "string" || request.target.projectId.trim() === "" ||
        request.target.environment.type !== "worktree" || request.target.environment.startingState.type !== "branch") {
      throw new Error("host_request target is invalid");
    }
    assertBranchText(request.target.environment.startingState.branchName, "host_request branchName");
  }
  if (["dispatch_prepared", "dispatching"].includes(value.phase) && (value.host_request === null || value.dispatch_attempt_id === null)) {
    throw new Error("dispatch requires a retained host_request and attempt ID");
  }
  if (value.dispatch_recovery_reason !== null && (typeof value.dispatch_recovery_reason !== "string" || value.dispatch_recovery_reason.trim() === "")) {
    throw new Error("dispatch_recovery_reason is invalid");
  }
  if (!PROVISIONING_PHASES.includes(value.phase)) throw new Error("provisioning operation phase is invalid");
  if (!["managed_worktree", "cli_worktree", "current_session"].includes(value.surface)) {
    throw new Error("provisioning surface is invalid");
  }
  for (const field of [
    "dispatch_attempt_id", "host_thread_id", "host_client_thread_id", "host_operation_id", "relocation_id",
  ]) {
    if (value[field] !== null) assertIdentifier(value[field], `operation_status.${field}`);
  }
  if (value.host_operation_revision !== null && (!Number.isSafeInteger(value.host_operation_revision) || value.host_operation_revision < 0)) {
    throw new Error("operation_status.host_operation_revision is invalid");
  }
  if (!cleanupStates.includes(value.worktree_cleanup) || !cleanupStates.includes(value.branch_cleanup)) {
    throw new Error("provisioning cleanup state is invalid");
  }
}

// Stable across JSON object key order; used for explicit receipt CAS and saved seeds.
export function receiptDigest(value) {
  const canonical = (entry) => Array.isArray(entry) ? entry.map(canonical) : entry !== null && typeof entry === "object" ? Object.fromEntries(Object.keys(entry).sort().map((key) => [key, canonical(entry[key])])) : entry;
  return createHash("sha256").update(JSON.stringify(canonical(value))).digest("hex");
}

export async function assertReceiptExecutable(receipt, options) {
  if (receipt.operation_status.phase === "superseded") throw new Error("launch was permanently superseded; read its successor seed");
  if (receipt.predecessor === undefined) return;
  const prior = await readProvisioningReceipt(provisioningReceiptPath(options.productSupportRoot, receipt.predecessor.launch_id, receipt.predecessor.repository_key), options);
  const seed = prior?.supersession?.seed?.receipt;
  if (seed === undefined || receiptDigest(seed.predecessor) !== receiptDigest(receipt.predecessor) || seed.launch_id !== receipt.launch_id) throw new Error("successor has no matching permanent predecessor");
  for (const field of ["admission", "host", "request_digest", "handoff_digest", "source_repository_identity", "repository_key", "workspace_mode", "remote_name", "source_type", "carry_changes", "base_branch", "target_branch", "created_at"]) {
    if (receiptDigest(seed[field]) !== receiptDigest(receipt[field])) throw new Error("successor differs from its retained seed");
  }
}

function receiptRoot(productSupportRoot) {
  assertNormalizedAbsolutePath(productSupportRoot, "product support root");
  return join(productSupportRoot, "provisioning", "codex");
}

async function ensureOwnedDirectory(path, enforcePrivateModes, boundary = path) {
  containedPath(boundary, path);
  await assertNoSymbolicLinkComponents(boundary, path);
  await mkdir(path, { recursive: true, mode: 0o700 });
  await assertNoSymbolicLinkComponents(boundary, path);
  const canonical = await realpath(path);
  if (canonical !== path) throw new Error("provisioning directory must be canonical");
  if (enforcePrivateModes) await chmod(path, 0o700);
}

async function assertNoSymbolicLinkComponents(root, candidate) {
  const canonicalRoot = resolve(root);
  const canonicalCandidate = containedPath(canonicalRoot, candidate);
  const components = relative(canonicalRoot, canonicalCandidate).split(sep).filter(Boolean);
  let current = canonicalRoot;
  for (const component of components) {
    current = join(current, component);
    try {
      const info = await lstat(current);
      if (info.isSymbolicLink()) throw new Error("provisioning path contains a symbolic link");
    } catch (error) {
      if (error?.code === "ENOENT") return;
      throw error;
    }
  }
}

async function rejectSymbolicLink(path) {
  try {
    if ((await lstat(path)).isSymbolicLink()) throw new Error("provisioning receipt must not be a symbolic link");
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
  }
}

function containedPath(root, candidate) {
  const canonicalRoot = resolve(root);
  const canonicalCandidate = resolve(candidate);
  const offset = relative(canonicalRoot, canonicalCandidate);
  if (offset === ".." || offset.startsWith(`..${sep}`) || isAbsolute(offset)) {
    throw new Error("provisioning receipt escapes its owned root");
  }
  return canonicalCandidate;
}

function assertExactKeys(value, keys, label) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error(`${label} must be an object`);
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error(`${label} has an invalid closed shape`);
}

function assertIdentifier(value, label) {
  if (typeof value !== "string" || !identifierPattern.test(value)) throw new Error(`${label} is invalid`);
}

function assertLaunchID(value) {
  if (typeof value !== "string" || !launchIdPattern.test(value)) throw new Error("launch_id is invalid");
}

function assertBranchText(value, label) {
  if (typeof value !== "string" || value === "" || value.includes("\0") || value.includes("\n") || value.includes("\r")) {
    throw new Error(`provisioning receipt ${label} is invalid`);
  }
}

function assertNormalizedAbsolutePath(value, label) {
  if (typeof value !== "string" || value.includes("\0") || !isAbsolute(value) || resolve(value) !== value) {
    throw new Error(`${label} must be a normalized absolute path`);
  }
}
