import { createHash, randomUUID } from "node:crypto";
import { lstat, mkdir, open, readFile, rename, unlink } from "node:fs/promises";
import { isAbsolute, join, relative, resolve } from "node:path";
import { inspectSourceRepository, preflightLocalBranchSelection, preflightWorktreeSelection, resolveFrozenBase, prepareLocalBranch, createCliWorktree, removeCliWorktree, removeTaskBranch, defaultRunGit } from "./worktree-lifecycle.mjs";
import { captureWorkspaceChanges, applyWorkspaceChanges } from "./worktree-snapshot.mjs";
import { paths, coreJSON } from "./runtime.mjs";
const hash = value => createHash("sha256").update(typeof value === "string" ? value : JSON.stringify(value)).digest("hex");
const identifier = value => { if (typeof value !== "string" || !/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(value)) throw new Error("Invalid launch, task or relocation identity"); return value; };
const repositoryKey = value => { if (typeof value !== "string" || !/^[a-z0-9][a-z0-9._-]{0,127}$/.test(value)) throw new Error("Repository key must use 1–128 lowercase ASCII letters, digits, dots, underscores or hyphens and begin with a letter or digit"); return value; };
const exact = (v, keys) => { if (!v || typeof v !== "object" || Array.isArray(v) || Object.keys(v).some(k => !keys.includes(k)) || keys.some(k => !(k in v))) throw new Error("Input does not match the closed contract: " + keys.join(", ")); };
async function atomic(path, value) {
  const temp = path + "." + randomUUID() + ".tmp";
  const handle = await open(temp, "wx", 0o600);
  try { await handle.writeFile(JSON.stringify(value) + "\n"); await handle.sync(); } finally { await handle.close(); }
  try { await rename(temp, path); } finally { await unlink(temp).catch(() => {}); }
}
async function directory(launchID, environment) {
  const p = await paths(environment);
  let current = p.productRoot;
  for (const part of ["", "provisioning", "zcode", identifier(launchID)]) {
    if (part) current = join(current, part);
    try { const info = await lstat(current); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Launch path must be an owned regular directory"); }
    catch (error) { if (error.code !== "ENOENT") throw error; }
  }
  return current;
}
export async function inspect(input, { runGit = defaultRunGit } = {}) {
  exact(input, ["request", "repositories"]);
  if (typeof input.request !== "string" || !input.request.trim()) throw new Error("Request is required");
  if (!Array.isArray(input.repositories) || input.repositories.length < 1 || input.repositories.length > 8) throw new Error("One to eight repositories required");
  const repositories = [];
  for (const item of input.repositories) {
    exact(item, ["key", "repository_path"]); repositoryKey(item.key);
    const observation = await inspectSourceRepository(item.repository_path, { runGit });
    if (repositories.some(r => r.key === item.key || r.repository_path === observation.canonical_root)) throw new Error("Duplicate repository");
    repositories.push({ key: item.key, repository_path: observation.canonical_root, head: observation.head, branch: observation.branch, status_digest: observation.status_digest, clean: observation.clean, source_identity: observation.source_repository_identity });
  }
  return { request_digest: hash(input.request), repositories };
}
export async function prepare(input, options = {}) {
  exact(input, ["request", "authorization_ref", "repositories", "handoff"]);
  if (typeof input.authorization_ref !== "string" || !input.authorization_ref.trim()) throw new Error("Actual workspace authorization reference required");
  const anchor = await inspect({ request: input.request, repositories: input.repositories.map(r => ({ key: r.key, repository_path: r.repository_path })) }, options);
  const runGit = options.runGit ?? defaultRunGit;
  const repositories = [];
  for (const selection of input.repositories) {
    exact(selection, ["key", "repository_path", "workspace_mode", "source_type", "remote_name", "base_branch", "target_branch", "carry_changes", "worktree_path"]);
    const observed = anchor.repositories.find(r => r.key === selection.key);
    if (typeof selection.carry_changes !== "boolean") throw new Error("Explicit carry choice required");
    if (!isAbsolute(selection.worktree_path)) throw new Error("Worktree path must be absolute");
    if (selection.workspace_mode === "dedicated_worktree") {
      if (selection.source_type === "remote" && selection.carry_changes) throw new Error("Remote source cannot carry changes");
      await preflightWorktreeSelection({ repositoryPath: observed.repository_path, remoteName: selection.remote_name, sourceType: selection.source_type, baseBranch: selection.base_branch, targetBranch: selection.target_branch, runGit });
    } else {
      if (selection.source_type !== "local" || selection.remote_name !== "" || resolve(selection.worktree_path) !== observed.repository_path) throw new Error("Local work must retain its source directory");
      await preflightLocalBranchSelection({ repositoryPath: observed.repository_path, workspaceMode: selection.workspace_mode, baseBranch: selection.base_branch, targetBranch: selection.target_branch, carryChanges: selection.carry_changes, runGit });
      const available = await (options.checkWorkspaceAvailable ?? (root => coreJSON(["host-check", "workspace-available"], { repository_path: root }, options)))(observed.repository_path);
      if (available.available !== true) throw new Error("Repository is claimed by an active Core Task");
    }
    if (repositories.some(r => resolve(r.worktree_path) === resolve(selection.worktree_path))) throw new Error("Duplicate destination");
    repositories.push({ ...selection, repository_path: observed.repository_path, source_identity: observed.source_identity, base_commit: null, snapshot: null, phase: "confirmed" });
  }
  if (repositories.some(r => r.workspace_mode === "dedicated_worktree") && (!input.handoff || !Array.isArray(input.handoff.discussion) || !input.handoff.discussion.length)) throw new Error("Dedicated workspace needs complete original requirement discussion");
  const launchID = randomUUID(), dir = await directory(launchID, options.environment);
  await mkdir(dir, { recursive: true, mode: 0o700 });
  const receipt = { launch_id: launchID, host: "zcode", request: input.request, authorization_ref: input.authorization_ref, request_digest: hash(input.request), handoff: input.handoff, handoff_digest: hash(input.handoff), repositories, relocation: null };
  await atomic(join(dir, "receipt.json"), receipt);
  return locked(launchID, options, async (r, save) => {
    for (const repo of r.repositories) {
      repo.phase = "resolving"; await save();
      try {
        const base = await resolveFrozenBase({ repositoryPath: repo.repository_path, remoteName: repo.remote_name, sourceType: repo.source_type, baseBranch: repo.base_branch, runGit });
        repo.base_commit = base.base_commit;
        if (repo.carry_changes && repo.workspace_mode === "dedicated_worktree") repo.snapshot = await captureWorkspaceChanges(repo.repository_path, snapshotRun(runGit));
        repo.phase = "prepared"; await save();
      } catch (error) { repo.phase = "uncertain"; await save(); throw error; }
    }
    return r;
  }).catch(error => {
    error.launch_id = launchID;
    error.receipt_path = join(dir, "receipt.json");
    throw error;
  });
}
const snapshotRun = runGit => async (root, args, env) => runGit(["-C", root, ...args], { env });
async function workspaceIdentity(observed) {
  const directory = await lstat(observed.canonical_root, { bigint: true });
  const gitDirectory = await lstat(observed.worktree_git_dir, { bigint: true });
  return hash([observed.canonical_root, observed.worktree_git_dir,
    String(directory.dev), String(directory.ino), String(directory.birthtimeNs),
    String(gitDirectory.dev), String(gitDirectory.ino), String(gitDirectory.birthtimeNs)]);
}
async function locked(id, options, operation) {
  const dir = await directory(id, options.environment), lockPath = join(dir, "operation.lock");
  const lock = await acquireLock(lockPath);
  try {
    const r = JSON.parse(await readFile(join(dir, "receipt.json"), "utf8"));
    validateReceipt(r, id);
    return await operation(r, async () => {
      await verifyLock(lockPath, lock);
      await atomic(join(dir, "receipt.json"), r);
    });
  } finally {
    try {
      await verifyLock(lockPath, lock);
      await unlink(lockPath);
    } finally { await lock.handle.close(); }
  }
}
async function acquireLock(path) {
  try {
    const handle = await open(path, "wx", 0o600);
    const token = randomUUID();
    try {
      await handle.writeFile(JSON.stringify({ pid: process.pid, token }));
      await handle.sync();
      return { handle, token, identity: await handle.stat() };
    } catch (error) { await handle.close(); throw error; }
  } catch (error) {
    if (error.code !== "EEXIST") throw error;
    throw new Error(`Launch operation lock remains at ${path}; inspect the saved receipt and workspace and establish that its previous owner stopped before explicitly clearing the lock. Do not replay an uncertain operation.`);
  }
}
async function verifyLock(path, lock) {
  const current = await lstat(path);
  if (!current.isFile() || current.isSymbolicLink() || current.dev !== lock.identity.dev ||
      current.ino !== lock.identity.ino || current.birthtimeMs !== lock.identity.birthtimeMs ||
      JSON.parse(await readFile(path, "utf8")).token !== lock.token) {
    throw new Error("Launch operation lock changed; retain the lock and inspect the saved operation");
  }
}
export async function status(id, options = {}) {
  const dir = await directory(id, options.environment);
  const r = JSON.parse(await readFile(join(dir, "receipt.json"), "utf8"));
  validateReceipt(r, id);
  return r;
}
function validateReceipt(receipt, id) {
  if (receipt.host !== "zcode" || receipt.launch_id !== id ||
      hash(receipt.request) !== receipt.request_digest || !receipt.authorization_ref ||
      hash(receipt.handoff) !== receipt.handoff_digest) throw new Error("Receipt identity or retained discussion changed");
}
export async function bindTask(id, input, options = {}) {
  exact(input, ["task_id"]); identifier(input.task_id);
  await scope(id, options);
  return locked(id, options, async (receipt, save) => {
    if (receipt.core_task_id && receipt.core_task_id !== input.task_id) throw new Error("Launch already belongs to another Core Task");
    receipt.core_task_id = input.task_id; await save(); return receipt;
  });
}
export async function provision(id, options = {}) {
  const runGit = options.runGit ?? defaultRunGit;
  return locked(id, options, async (r, save) => {
    for (const repo of r.repositories) {
      if (repo.phase === "provisioned") continue;
      if (repo.phase !== "prepared") throw new Error("Provisioning is incomplete or uncertain; inspect retained state");
      if (repo.workspace_mode !== "dedicated_worktree") {
        const available = await (options.checkWorkspaceAvailable ?? (root => coreJSON(["host-check", "workspace-available"], { repository_path: root }, options)))(repo.repository_path);
        if (available.available !== true) throw new Error("Workspace is already claimed");
      }
      repo.phase = "provisioning"; await save();
      try {
        const args = { repositoryPath: repo.repository_path, workspaceMode: repo.workspace_mode, baseBranch: repo.base_branch, targetBranch: repo.target_branch, baseCommit: repo.base_commit, sourceRepositoryIdentity: repo.source_identity, carryChanges: repo.carry_changes, runGit };
        if (repo.workspace_mode === "dedicated_worktree") {
          await createCliWorktree({ ...args, worktreePath: repo.worktree_path });
          await applyWorkspaceChanges(repo.worktree_path, repo.snapshot, snapshotRun(runGit));
        } else await prepareLocalBranch(args);
        repo.worktree_identity = await workspaceIdentity(await inspectSourceRepository(repo.worktree_path, { runGit }));
        repo.phase = "provisioned"; await save();
      } catch (error) { repo.phase = "uncertain"; await save(); throw error; }
    }
    return r;
  });
}
function origin(r, repo) {
  if (repo.phase !== "provisioned") throw new Error("Every repository must be provisioned");
  return { mode: repo.workspace_mode, source_type: repo.source_type, carry_changes: repo.carry_changes, remote_name: repo.remote_name,
    base_branch: repo.base_branch, base_commit: repo.base_commit, task_branch: repo.target_branch, provisioning_receipt_id: "zcode-" + hash(r.launch_id + "\0" + repo.key) };
}
export async function scope(id, options = {}) {
  const r = await status(id, options), [primary, ...additional] = r.repositories;
  if (r.repositories.some(repo => repo.phase !== "provisioned")) throw new Error("Every repository must be provisioned before exposing a Core scope");
  for (const repo of r.repositories) {
    const actual = await inspectSourceRepository(repo.worktree_path, options);
    if (actual.source_repository_identity !== repo.source_identity || actual.branch !== repo.target_branch || await workspaceIdentity(actual) !== repo.worktree_identity) throw new Error("Workspace identity or branch changed");
  }
  return { repositories: [primary, ...additional].map(repo => ({key: repo.key, repository_path:repo.worktree_path, workspace_origin:origin(r,repo)})) };
}

// ZCode's documented desktop integration does not expose a session-launch CLI.
// This descriptor is retained-data guidance, not a claim that a UI action ran.
export async function session(id, operation, options = {}) {
  if (!["open", "resume"].includes(operation)) throw new Error("Use open or resume for ZCode UI guidance");
  await scope(id, options);
  const receipt = await status(id, options);
  const receiptPath = join(await directory(id, options.environment), "receipt.json");
  const taskInstruction = receipt.core_task_id
    ? `Read the saved Core Task ${receipt.core_task_id} by ID with host=zcode, handle its blocker/recovery before work, and never create another Task for this launch.`
    : "Check Core for an existing Task in these exact repositories before creation. An unbound launch receipt does not prove that Core creation never occurred. If a previous open result is uncertain, recover it first. After a successful open, save the actual task_id with host-launch bind-task.";
  const prompt = `Continue this TaskBelay task in its prepared workspace. First read the entire retained request, authorization and original discussion at ${receiptPath}. Read taskbelay-zcode host-launch status and scope for launch ${id}; verify every actual workspace and its permissions, and preserve existing content. Discover the actual plugin MCP tools and perform server_info before work. ${taskInstruction} Follow the current authorized goals, observed changes and evidence decision and preserve the user's requirements and corrections.`;
  return {
    operation, status: "action_required", host: "zcode", launch_id: id,
    receipt_path: receiptPath, task_id: receipt.core_task_id ?? null,
    workspace_paths: receipt.repositories.map(repo => repo.worktree_path), prompt,
    next_steps: [
      "In ZCode, open the listed prepared workspace directories and grant only the required workspace permissions. If the current session already has access to all of them, continue there.",
      "Use the installed TaskBelay Skill in that workspace and provide the complete prompt above. This command has not opened or resumed a ZCode session.",
    ],
  };
}

export async function cleanup(id, operation, input, options = {}) {
  if (!["cleanup-worktree", "cleanup-branch"].includes(operation)) throw new Error("Unknown cleanup operation");
  exact(input, ["repository_key", "task_id", "revision", "terminal", "authorized"]);
  return locked(id, options, async (r, save) => {
    const repo = r.repositories.find(v => v.key === input.repository_key);
    if (!repo || repo.workspace_mode !== "dedicated_worktree") throw new Error("Only receipt-owned dedicated worktrees can be cleaned");
    if (input.terminal !== true || input.authorized !== true) throw new Error("Terminal Task and separate deletion authorization required");
    const response = await (options.readTask ?? (taskID => coreJSON(["call","taskbelay_get_task"], {task_id:taskID,refresh:false}, options)))(input.task_id);
    const task = response?.result?.task;
    if (!task || !["done","cancelled"].includes(task.status) || task.revision !== input.revision || task.task_id !== input.task_id || !task.repositories?.some(entry => entry.key === repo.key && entry.workspace_origin.provisioning_receipt_id === id && entry.workspace_origin.canonical_worktree_root === repo.worktree_path)) throw new Error("Current terminal Core task does not own this workspace");
    const key = operation === "cleanup-worktree" ? "worktree_cleanup" : "branch_cleanup";
    if (repo[key] === "completed") return r;
    if (repo[key]) throw new Error("Cleanup has already been requested; inspect its outcome");
    if (key === "branch_cleanup" && repo.worktree_cleanup !== "completed") throw new Error("Remove worktree before branch");
    if (key === "worktree_cleanup" && await workspaceIdentity(await inspectSourceRepository(repo.worktree_path, options)) !== repo.worktree_identity) throw new Error("Cleanup workspace instance changed");
    repo[key] = "requested"; await save();
    const args = { repositoryPath: repo.repository_path, worktreePath: repo.worktree_path, targetBranch: repo.target_branch, sourceRepositoryIdentity: repo.source_identity, terminal: true, authorized: true, runGit: options.runGit };
    await (key === "worktree_cleanup" ? removeCliWorktree : removeTaskBranch)(args);
    repo[key] = "completed"; await save(); return r;
  });
}


async function verifyWorkspace(repo, runGit) {
  const actual = await inspectSourceRepository(repo.worktree_path, { runGit });
  if (actual.source_repository_identity !== repo.source_identity || actual.branch !== repo.target_branch ||
      await workspaceIdentity(actual) !== repo.worktree_identity) throw new Error("Relocation workspace identity changed");
}
