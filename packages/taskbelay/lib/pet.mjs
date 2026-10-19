import { describeInterface } from "./interface-reference.mjs";
import { execFile as execFileCallback } from "node:child_process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { lstat } from "node:fs/promises";
import { join } from "node:path";

import { resolveManagerPaths } from "./ownership.mjs";
import { supportsDesktopPet, loadPetPlatform, loadPetInstaller } from "./platform.mjs";
import { messagesForLanguage, resolveLanguage } from "./presentation.mjs";
import { NoRuntimeError, resolveCoreRuntime } from "./runtime.mjs";

// The desktop pet launcher.
//
// It resolves the Core runtime an installed Adapter already provides, confirms
// a connectable local service, and hands verified absolute paths to the bundled
// native application. Task state, node, blocker, and terminal outcome stay with
// Core; this module decides only whether the desktop component may start, and
// which user-facing text describes the result.

const execFile = promisify(execFileCallback);
const packageRoot = fileURLToPath(new URL("../", import.meta.url));

const CORE_INVOCATION_TIMEOUT_MILLISECONDS = 10_000;
const SERVICE_TIMEOUT_MILLISECONDS = 3_000;
const CORE_IDENTITY_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u;
const DATA_ROOT_DIGEST_PATTERN = /^[0-9a-f]{64}$/u;
const LOOPBACK_ORIGIN_PATTERN = /^http:\/\/127\.0\.0\.1:(\d{1,5})$/u;
const READINESS_VALUES = Object.freeze(["ready", "read_only", "incompatible", "unavailable"]);

/**
 * Runtime state is observed by the platform implementation; missing records
 * mean stopped, while unavailable native observation remains unknown.
 */
export async function observePetState({ paths, packageRoot: root = packageRoot, environment = process.env }) {
  if (!supportsDesktopPet(paths.platform, paths.arch)) return "unknown";
  try { const info = await lstat(join(paths.petDirectory, "runtime.json")); if (!info.isFile() || info.isSymbolicLink()) return "unknown"; }
  catch (error) { return error.code === "ENOENT" ? "stopped" : "unknown"; }
  const platform = await loadPetPlatform(paths.platform, paths.arch);
  return await platform.observePetState({ executable: platform.bundledPetExecutable(root), productRoot: paths.productRoot, environment });
}

export class PetRequestError extends Error {
  constructor(message, exitCode = 1) {
    super(message);
    this.name = "PetRequestError";
    this.exitCode = exitCode;
  }
}

// Runs `taskbelay pet start` and `taskbelay pet stop`. Success is written to
// stdout, failure to stderr; the exit code is 0, 1, or 2 for invalid arguments.
export async function runPet(arguments_, dependencies = {}) {
  const stdout = dependencies.stdout ?? process.stdout;
  const stderr = dependencies.stderr ?? process.stderr;
  const environment = dependencies.environment ?? process.env;
  const platform = dependencies.platform ?? process.platform;
  const arch = dependencies.arch ?? process.arch;
  const messages = messagesForLanguage(dependencies.language ?? resolveLanguage(environment)).pet;
  try {
    if (arguments_[0] === "pet" && arguments_[2] === "--help" && (arguments_.length === 3 || arguments_.length === 4 && arguments_[3] === "--json")) {
      stdout.write(JSON.stringify(await describeInterface(packageRoot,"taskbelay.pet."+arguments_[1]),null,2)+"\n");
      return { code:0, signal:null };
    }
    const command = parsePetArguments(arguments_, messages);
    assertSupportedPlatform(platform, arch, messages);
    const text = command === "start"
      ? await startPet({ messages, environment, platform, arch, dependencies })
      : await stopPet({ messages, environment, platform, arch, dependencies });
    stdout.write(`${text}\n`);
    return { code: 0, signal: null };
  } catch (error) {
    stderr.write(`taskbelay: ${error.message}\n`);
    return { code: error instanceof PetRequestError ? error.exitCode : 1, signal: null };
  }
}

// Stops the desktop pet that uses `corePath` before a confirmed operation
// changes that Adapter's Core. A platform without the desktop component, or a
// package without the bundled application, has nothing this launcher could run.
// A failed shutdown is reported so the caller keeps the Adapter and can retry.
export async function stopPetForCore({
  corePath,
  environment = process.env,
  homeDirectory,
  platform = process.platform,
  arch = process.arch,
  packageRoot: root = packageRoot,
  spawnImpl,
  platformModule,
} = {}) {
  if (!supportsDesktopPet(platform, arch)) return Object.freeze({ stopped: false, reason: "unsupported-platform" });
  const paths = await resolveManagerPaths({ homeDirectory, environment, platform, arch });
  const petPlatform = platformModule ?? await loadPetPlatform(platform, arch);
  const installed = petPlatform.installedPetExecutable ? petPlatform.installedPetExecutable(paths.petDirectory) : null;
  const executable = installed && (await petPlatform.isBundledPetApplicationAvailable(installed))
    ? installed
    : petPlatform.bundledPetExecutable(root);
  if (!(await petPlatform.isBundledPetApplicationAvailable(executable))) return Object.freeze({ stopped: false, reason: "application-unavailable" });
  const result = await petPlatform.shutdownPet({
    executable,
    productRoot: paths.productRoot,
    corePath,
    ...(spawnImpl === undefined ? {} : { spawnImpl }),
    environment,
  });
  if (result.code !== 0) throw new PetRequestError(result.detail ?? "the desktop pet did not stop");
  return Object.freeze({ stopped: true, reason: null });
}

function parsePetArguments(arguments_, messages) {
  const invalid = !Array.isArray(arguments_)
    || arguments_[0] !== "pet"
    || !["start", "stop"].includes(arguments_[1])
    || arguments_.length !== 2
    || arguments_.some((value) => typeof value !== "string" || value.includes("\0"));
  if (invalid) throw new PetRequestError(messages.invalidArguments, 2);
  return arguments_[1];
}

function assertSupportedPlatform(platform, arch, messages) {
  if (!supportsDesktopPet(platform, arch)) throw new PetRequestError(messages.unsupportedPlatform);
}

async function startPet({ messages, environment, platform, arch, dependencies }) {
  const paths = await resolveManagerPaths({
    homeDirectory: dependencies.homeDirectory,
    environment,
    platform,
    arch,
  });
  let selection;
  try {
    selection = await (dependencies.resolveCoreRuntime ?? resolveCoreRuntime)({
      environment,
      homeDirectory: dependencies.homeDirectory,
      platform,
      arch,
      exec: dependencies.exec,
      requireData: true,
      initializeDefaultData: true,
    });
  } catch (error) {
    if (error instanceof NoRuntimeError) throw new PetRequestError(messages.installAdapterFirst);
    throw error;
  }

  const petPlatform = dependencies.platformModule ?? await loadPetPlatform(platform, arch);
  const installed = petPlatform.installedPetExecutable ? petPlatform.installedPetExecutable(paths.petDirectory) : null;
  let executable = installed && (await petPlatform.isBundledPetApplicationAvailable(installed))
    ? installed
    : petPlatform.bundledPetExecutable(dependencies.packageRoot ?? packageRoot);
  if (!(await petPlatform.isBundledPetApplicationAvailable(executable))) {
    const installer = dependencies.petInstaller ?? await loadPetInstaller(platform, arch).catch(() => null);
    if (installer?.ensurePetInstalled) {
      await installer.ensurePetInstalled({
        petDirectory: paths.petDirectory,
        sourcePackageRoots: [
          dependencies.packageRoot ?? packageRoot,
          selection.packageRoot,
        ].filter(Boolean),
        enforcePrivateModes: paths.enforcePrivateModes,
      }).catch(() => {});
      if (installed && (await petPlatform.isBundledPetApplicationAvailable(installed))) {
        executable = installed;
      }
    }
  }
  if (!(await petPlatform.isBundledPetApplicationAvailable(executable))) throw new PetRequestError(messages.applicationUnavailable);

  const state = await confirmConnectableService({
    selection,
    environment,
    runCore: dependencies.exec ?? execFile,
    fetchImpl: dependencies.fetchImpl,
    messages,
  });

  let confirmation;
  try {
    confirmation = await petPlatform.launchPet({
      executable,
      request: {
        corePath: selection.runtimePath,
        dataDirectory: selection.dataDirectory,
        productRoot: paths.productRoot,
        coreIdentity: state.core_identity,
        dataRootDigest: state.data_root_digest,
      },
      ...(dependencies.spawnImpl === undefined ? {} : { spawnImpl: dependencies.spawnImpl }),
      environment,
    });
  } catch (error) {
    throw new PetRequestError(withDetail(messages.launchFailed, error.message));
  }
  return confirmation === "restored" ? messages.restored : messages.started;
}

async function stopPet({ messages, environment, platform, arch, dependencies }) {
  // Shutdown only ends an existing desktop pet. It never re-resolves an Adapter
  // and never starts the local service.
  const paths = await resolveManagerPaths({
    homeDirectory: dependencies.homeDirectory,
    environment,
    platform,
    arch,
  });
  const petPlatform = dependencies.platformModule ?? await loadPetPlatform(platform, arch);
  const installed = petPlatform.installedPetExecutable ? petPlatform.installedPetExecutable(paths.petDirectory) : null;
  const executable = installed && (await petPlatform.isBundledPetApplicationAvailable(installed))
    ? installed
    : petPlatform.bundledPetExecutable(dependencies.packageRoot ?? packageRoot);
  if (!(await petPlatform.isBundledPetApplicationAvailable(executable))) throw new PetRequestError(messages.applicationUnavailable);

  const result = await petPlatform.shutdownPet({
    executable,
    productRoot: paths.productRoot,
    corePath: null,
    ...(dependencies.spawnImpl === undefined ? {} : { spawnImpl: dependencies.spawnImpl }),
    environment,
  });
  if (result.code !== 0) throw new PetRequestError(withDetail(messages.stopFailed, result.detail));
  return messages.stopped;
}

// Reads the runtime state of the selected Core and confirms that the same
// identity and address answer over the loopback service. Only this explicit
// start may ask Core to bring the service up; a read-only local storage without
// a connectable service does not activate the desktop pet.
//
// `runCore` is `execFile` with an argv array, never a shell command string.
async function confirmConnectableService({ selection, environment, runCore, fetchImpl, messages }) {
  let state = await readRuntimeState({
    selection,
    environment,
    runCore,
    arguments_: ["webui", "status", "--json"],
    failureMessage: messages.coreCommandFailed,
    messages,
  });
  if (state.readiness === "unavailable") {
    state = await readRuntimeState({
      selection,
      environment,
      runCore,
      arguments_: ["webui", "start", "--no-open", "--json"],
      failureMessage: messages.serviceStartFailed,
      messages,
    });
  }
  if (state.readiness === "incompatible") throw new PetRequestError(messages.incompatibleRuntime);
  if (state.readiness !== "ready" && state.readiness !== "read_only") throw new PetRequestError(messages.serviceUnavailable);
  if (!isConfirmedLoopbackOrigin(state.url)) throw new PetRequestError(messages.serviceUnavailable);
  await verifyLiveService({
    origin: state.url,
    coreIdentity: state.core_identity,
    dataRootDigest: state.data_root_digest,
    fetchImpl: fetchImpl ?? fetch,
    messages,
  });
  return state;
}

async function readRuntimeState({ selection, environment, runCore, arguments_, failureMessage, messages }) {
  let result;
  try {
    result = await runCore(selection.runtimePath, arguments_, {
      cwd: selection.packageRoot,
      encoding: "utf8",
      windowsHide: true,
      maxBuffer: 64 * 1024,
      timeout: CORE_INVOCATION_TIMEOUT_MILLISECONDS,
      env: { ...environment, TASKBELAY_DATA_DIR: selection.dataDirectory },
    });
  } catch (error) {
    // A failed Core invocation does not guarantee JSON on stdout, so the exit
    // code and the reported output are checked together.
    throw new PetRequestError(withDetail(failureMessage, error?.stderr || error?.message));
  }
  return validatedRuntimeState(result?.stdout, messages);
}

// Checks the documented runtime-state fields by type. A value outside the
// contract is reported as a failed Core invocation naming the exact field, so a
// drifted Core output can never be handed to the desktop component.
function validatedRuntimeState(output, messages) {
  const invalid = (detail) => new PetRequestError(withDetail(messages.coreCommandFailed, detail));
  let state;
  try {
    state = JSON.parse(typeof output === "string" ? output : "");
  } catch {
    throw invalid("the Core runtime state is not valid JSON");
  }
  if (state === null || typeof state !== "object" || Array.isArray(state)) throw invalid("the Core runtime state is not an object");
  if (!READINESS_VALUES.includes(state.readiness)) throw invalid("the Core reported an unknown readiness");
  if (typeof state.core_identity !== "string" || !CORE_IDENTITY_PATTERN.test(state.core_identity)) throw invalid("the Core identity is invalid");
  if (typeof state.data_root_digest !== "string" || !DATA_ROOT_DIGEST_PATTERN.test(state.data_root_digest)) {
    throw invalid("the data directory digest is invalid");
  }
  if (typeof state.url !== "string") throw invalid("the Core reported an invalid service address");
  if (state.pid !== undefined && (!Number.isInteger(state.pid) || state.pid < 0)) {
    throw invalid("the Core reported an invalid service process");
  }
  return Object.freeze({
    readiness: state.readiness,
    core_identity: state.core_identity,
    data_root_digest: state.data_root_digest,
    url: state.url,
    pid: state.pid,
  });
}

// Confirms the running service is the one Core just described, so a stale
// receipt address can never be handed to the desktop component.
async function verifyLiveService({ origin, coreIdentity, dataRootDigest, fetchImpl, messages }) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), SERVICE_TIMEOUT_MILLISECONDS);
  let live;
  try {
    const response = await fetchImpl(`${origin}/api/system/status`, { signal: controller.signal, redirect: "error" });
    if (!response.ok) throw new PetRequestError(messages.serviceUnavailable);
    live = await response.json();
  } catch (error) {
    if (error instanceof PetRequestError) throw error;
    throw new PetRequestError(messages.serviceUnavailable);
  } finally {
    clearTimeout(timer);
  }
  if (live?.core_identity !== coreIdentity || live?.data_root_digest !== dataRootDigest || live?.url !== origin) {
    throw new PetRequestError(messages.identityMismatch);
  }
}

function isConfirmedLoopbackOrigin(value) {
  const match = LOOPBACK_ORIGIN_PATTERN.exec(value);
  if (match === null) return false;
  const port = Number(match[1]);
  return port > 0 && port <= 65535;
}

function withDetail(message, detail) {
  const value = firstLine(detail);
  return value === null ? message : `${message}: ${value}`;
}

function firstLine(value) {
  if (typeof value !== "string") return null;
  const line = value.split(/\r?\n/u).map((entry) => entry.trim()).filter(Boolean)[0];
  return line === undefined ? null : line;
}
