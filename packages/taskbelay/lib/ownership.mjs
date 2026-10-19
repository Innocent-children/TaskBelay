import { createHash, randomBytes } from "node:crypto";
import {
  chmod,
  lstat,
  mkdir,
  readFile,
  realpath,
  rename,
  rm,
  stat,
  unlink,
  writeFile,
} from "node:fs/promises";
import { homedir } from "node:os";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

import {
  cleanupPolicy,
  dataPathPolicy,
  permissionPolicy,
  runtimeDescriptor,
  signalPolicy,
} from "./platform.mjs";

export const DATA_DIRECTORY_ENVIRONMENT = "TASKBELAY_DATA_DIR";

export async function resolveManagerPaths({
  homeDirectory = homedir(),
  environment = process.env,
  platform = process.platform,
  arch = process.arch,
} = {}) {
  const runtime = runtimeDescriptor(platform, arch);
  const dataPaths = dataPathPolicy(platform, arch);
  const permissions = permissionPolicy(platform, arch);
  const signals = signalPolicy(platform, arch);
  const cleanup = cleanupPolicy(platform, arch);
  const canonicalHome = await canonicalExistingDirectory(homeDirectory, "home directory");
  const applicationData = dataPaths.applicationData({ homeDirectory: canonicalHome, environment });
  const applicationDataRoot = applicationData.canonicalizeRoot
    ? await canonicalExistingDirectory(applicationData.path, applicationData.label)
    : ownedPath(canonicalHome, applicationData.path, applicationData.label);
  const applicationDataInspectionRoot = applicationData.canonicalizeRoot
    ? applicationDataRoot
    : ownedPath(canonicalHome, applicationData.inspectionRoot, "application data inspection root");
  const productRoot = ownedPath(applicationDataRoot, await dataPaths.productRoot(applicationDataRoot), "product root");
  const managerRoot = ownedPath(applicationDataRoot, await dataPaths.managerRoot(applicationDataRoot), "manager root");
  const trash = cleanup.trash({
    homeDirectory: canonicalHome,
    managerRoot,
    inspectionRoot: applicationDataInspectionRoot,
  });
  const configurationDirectory = ownedPath(canonicalHome, join(canonicalHome, ".taskbelay"), "configuration directory");
  const configurationPath = ownedPath(configurationDirectory, join(configurationDirectory, "config.json"), "configuration path");
  const defaultDataDirectory = ownedPath(productRoot, join(productRoot, "data"), "default data directory");
  const petDirectory = ownedPath(productRoot, join(productRoot, "pet"), "desktop pet directory");
  const explicitValue = environment[DATA_DIRECTORY_ENVIRONMENT] ?? "";
  const explicitDataDirectory = explicitValue === "" ? null : await canonicalExplicitDirectory(explicitValue);
  return Object.freeze({
    homeDirectory: canonicalHome,
    platform: runtime.platform,
    arch: runtime.arch,
    runtimeKey: runtime.runtimeKey,
    runtimeDirectory: runtime.runtimeDirectory,
    runtimeExecutable: runtime.runtimeExecutable,
    enforcePrivateModes: permissions.enforcePrivateModes,
    requireExecutableMode: permissions.requireExecutableMode,
    forwardedSignals: signals.forwardedSignals,
    recoverableCleanupDescription: cleanup.recoverableCleanupDescription,
    applicationDataRoot,
    applicationDataInspectionRoot,
    productRoot,
    managerRoot,
    runsDirectory: ownedPath(managerRoot, join(managerRoot, "runs"), "runs directory"),
    configurationDirectory,
    configurationPath,
    defaultDataDirectory,
    petDirectory,
    explicitDataDirectory,
    trashDirectory: ownedPath(trash.inspectionRoot, trash.path, "recoverable cleanup directory"),
    trashInspectionRoot: trash.inspectionRoot,
  });
}

export async function ensureManagerDirectories(paths) {
  await rejectSymlinkComponents(paths.applicationDataInspectionRoot, paths.managerRoot);
  await mkdir(paths.runsDirectory, { recursive: true, mode: 0o700 });
  if (paths.enforcePrivateModes) {
    await Promise.all([chmod(paths.managerRoot, 0o700), chmod(paths.runsDirectory, 0o700)]);
  }
}

export async function ensureDefaultDataDirectory(paths) {
  if (paths.explicitDataDirectory !== null) throw new OwnershipError("refusing to create an explicit data directory");
  const expected = ownedPath(paths.productRoot, join(paths.productRoot, "data"), "default data directory");
  if (paths.defaultDataDirectory !== expected) throw new OwnershipError("default data directory differs from the product-owned path");
  await rejectSymlinkComponents(paths.applicationDataInspectionRoot, expected);
  await mkdir(expected, { recursive: true, mode: 0o700 });
  await rejectSymlinkComponents(paths.applicationDataInspectionRoot, expected);
  const canonical = await realpath(expected);
  const info = await stat(canonical);
  if (canonical !== expected || !info.isDirectory()) throw new OwnershipError("default data directory must be canonical");
  if (paths.enforcePrivateModes) await chmod(expected, 0o700);
  return expected;
}

export async function inspectResource(path, label) {
  let info;
  try {
    info = await lstat(path);
  } catch (error) {
    if (error?.code === "ENOENT") return Object.freeze({ label, path: resolve(path), exists: false, identity: null });
    throw new Error(`inspect ${label}`, { cause: error });
  }
  if (info.isSymbolicLink()) throw new Error(`${label} must not be a symbolic link`);
  const canonical = await realpath(path);
  if (canonical !== resolve(path)) throw new Error(`${label} must be canonical`);
  return Object.freeze({
    label,
    path: canonical,
    exists: true,
    // Directory identity survives managed service files being removed during shutdown.
    // Individual file targets also bind their size and modification time.
    identity: info.isDirectory() ? `${info.dev}:${info.ino}:${info.mode}` : `${info.dev}:${info.ino}:${info.mode}:${info.size}:${info.mtimeMs}`,
    kind: info.isDirectory() ? "directory" : info.isFile() ? "file" : "unsupported",
  });
}

export async function assertResourceUnchanged(expected) {
  const current = await inspectResource(expected.path, expected.label);
  if (current.exists !== expected.exists || current.identity !== expected.identity || current.kind !== expected.kind) {
    throw new OwnershipError(`${expected.label} changed after plan creation`);
  }
  return current;
}

export async function writeOwnedJSON(path, value, { root, enforcePrivateModes = true }) {
  const target = ownedPath(root, path, "JSON target");
  await rejectSymlinkComponents(root, dirname(target));
  await mkdir(dirname(target), { recursive: true, mode: 0o700 });
  const temporary = join(dirname(target), `.${basename(target)}.${process.pid}.${randomBytes(8).toString("hex")}.tmp`);
  try {
    await writeFile(temporary, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600, flag: "wx" });
    await rename(temporary, target);
    if (enforcePrivateModes) await chmod(target, 0o600);
  } catch (error) {
    await unlink(temporary).catch(() => {});
    throw error;
  }
}

export async function readOwnedJSON(path, { root, validate }) {
  const target = ownedPath(root, path, "JSON target");
  let text;
  try {
    const info = await lstat(target);
    if (!info.isFile() || info.isSymbolicLink() || info.size > 64 * 1024) throw new OwnershipError("owned JSON must be a bounded regular file");
    text = await readFile(target, "utf8");
  } catch (error) {
    if (error?.code === "ENOENT") return null;
    throw error;
  }
  let value;
  try {
    value = JSON.parse(text);
  } catch (error) {
    throw new OwnershipError("owned JSON is invalid", { cause: error });
  }
  return validate(value);
}

export async function moveTargetsToTrash(paths, targets, { now = () => new Date(), random = () => randomBytes(6).toString("hex") } = {}) {
  await rejectSymlinkComponents(paths.trashInspectionRoot, paths.trashDirectory);
  await mkdir(paths.trashDirectory, { recursive: true, mode: 0o700 });
  await rejectSymlinkComponents(paths.trashInspectionRoot, paths.trashDirectory);
  const stamp = now().toISOString().replace(/[:.]/gu, "-");
  const trashRoot = ownedPath(paths.trashDirectory, join(paths.trashDirectory, `taskbelay-${stamp}-${random()}`), "Trash root");
  await mkdir(trashRoot, { mode: 0o700 });
  const moved = [];
  try {
    for (const target of targets) {
      if (!target.exists) continue;
      await assertResourceUnchanged(target);
      const destination = ownedPath(trashRoot, join(trashRoot, safeLabel(target.label)), "Trash target");
      await rename(target.path, destination);
      moved.push({ label: target.label, source: target.path, destination });
    }
    return { trashRoot, moved };
  } catch (error) {
    throw new CleanupPartialError("recoverable cleanup stopped", { cause: error, trashRoot, moved });
  }
}

export async function permanentlyRemoveTargets(targets, { allowedPaths }) {
  const allowed = new Set(allowedPaths.map(path => resolve(path)));
  const removed = [];
  try {
    for (const target of targets) {
      if (!target.exists) continue;
      if (!allowed.has(resolve(target.path))) throw new OwnershipError(`${target.label} is not in the confirmed cleanup set`);
      await assertResourceUnchanged(target);
      await rm(target.path, { recursive: target.kind === "directory", force: false });
      removed.push({ label: target.label, path: target.path });
    }
    return removed;
  } catch (error) {
    error.changed = removed.length > 0;
    error.completedSteps = removed.map(entry => `manager.remove.${entry.label}`);
    if (removed.length) error.exitCode = 5;
    throw error;
  }
}

export function planDigest(value) {
  return createHash("sha256").update(stableJSON(value)).digest("hex");
}

export class OwnershipError extends Error {
  constructor(message, options) {
    super(message, options);
    this.name = "OwnershipError";
    this.exitCode = 4;
  }
}

export class CleanupPartialError extends Error {
  constructor(message, { cause, trashRoot, moved }) {
    super(message, { cause });
    this.name = "CleanupPartialError";
    this.exitCode = 5;
    this.trashRoot = trashRoot;
    this.moved = moved;
    this.completedSteps = moved.map(entry => `manager.trash.${entry.label}`);
    this.changed = moved.length > 0;
  }
}

function safeLabel(value) {
  return value.replace(/[^A-Za-z0-9._-]+/gu, "-").replace(/^-+|-+$/gu, "") || "resource";
}

function ownedPath(root, candidate, label) {
  const canonicalRoot = resolve(root);
  const canonicalCandidate = resolve(candidate);
  const offset = relative(canonicalRoot, canonicalCandidate);
  if (offset === ".." || offset.startsWith(`..${sep}`) || isAbsolute(offset)) throw new OwnershipError(`${label} escapes its owned root`);
  return canonicalCandidate;
}

async function canonicalExistingDirectory(path, label) {
  if (!isAbsolute(path)) throw new OwnershipError(`${label} must be absolute`);
  try {
    const canonical = await realpath(path);
    if (!(await stat(canonical)).isDirectory()) throw new Error("not a directory");
    return canonical;
  } catch (error) {
    throw new OwnershipError(`${label} must name an existing directory`, { cause: error });
  }
}

async function canonicalExplicitDirectory(path) {
  if (!isAbsolute(path)) throw new OwnershipError(`${DATA_DIRECTORY_ENVIRONMENT} must be absolute`);
  const canonical = await canonicalExistingDirectory(path, DATA_DIRECTORY_ENVIRONMENT);
  if (canonical !== resolve(path)) throw new OwnershipError(`${DATA_DIRECTORY_ENVIRONMENT} must be canonical and non-symlink`);
  return canonical;
}

async function rejectSymlinkComponents(root, candidate) {
  const canonicalRoot = resolve(root);
  const target = ownedPath(canonicalRoot, candidate, "owned path");
  const parts = relative(canonicalRoot, target).split(sep).filter(Boolean);
  let current = canonicalRoot;
  for (const part of parts) {
    current = join(current, part);
    try {
      if ((await lstat(current)).isSymbolicLink()) throw new OwnershipError(`owned path contains a symbolic link: ${current}`);
    } catch (error) {
      if (error?.code === "ENOENT") return;
      throw error;
    }
  }
}

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value !== null && typeof value === "object") return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}
