import { petApplicationsDiffer } from "../../pet-files.mjs";
import { spawn } from "node:child_process";
import { execPortableCommand } from "../../command.mjs";
import { access, constants as fsConstants, lstat } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";

// The macOS implementation of the bundled desktop component.
//
// This module owns the packaged application location, the private native
// argument array, the launch confirmation, and the shutdown result. Process
// handling that only exists on macOS stays here; the launcher decides what the
// user asked for and renders the user-facing text.

export const PET_RUNTIME_DIRECTORY = "darwin-arm64";
export const PET_APPLICATION_NAME = "TaskBelayPet.app";
export const PET_EXECUTABLE_RELATIVE_PATH = join("Contents", "MacOS", "TaskBelayPet");

// The only lines the native `run` entry may confirm a launch with. Anything
// else means the two sides of the private contract drifted apart.
export const LAUNCH_CONFIRMATIONS = Object.freeze(["ready", "restored"]);
export const LAUNCH_TIMEOUT_MILLISECONDS = 10_000;
// The native shutdown entry waits at most five seconds for an orderly exit.
export const SHUTDOWN_TIMEOUT_MILLISECONDS = 15_000;

/**
 * The bundled native observer verifies the saved kernel identity without
 * opening a window, acquiring the instance lock or starting Core.
 */
export async function observePetState({ executable, productRoot, environment = process.env, exec = execPortableCommand }) {
  try {
    const { stdout } = await exec(executable, ["status", "--product-root", productRoot], { env: environment, encoding: "utf8", timeout: 3000, maxBuffer: 4096 });
    const state = stdout.trim();
    return ["running", "stopped", "unknown"].includes(state) ? state : "unknown";
  } catch { return "unknown"; }
}

export async function needsPetInstallation(packageRoot, petDirectory) {
  const source = join(resolve(packageRoot), "runtime", PET_RUNTIME_DIRECTORY, PET_APPLICATION_NAME);
  if (!await isBundledPetApplicationAvailable(bundledPetExecutable(packageRoot))) return false;
  if (!await isBundledPetApplicationAvailable(installedPetExecutable(petDirectory))) return true;
  return await petApplicationsDiffer(source, join(resolve(petDirectory), PET_APPLICATION_NAME));
}

// The installed application location inside ~/.taskbelay/pet/.
export function installedPetExecutable(petDirectory) {
  return join(resolve(petDirectory), PET_APPLICATION_NAME, PET_EXECUTABLE_RELATIVE_PATH);
}

// The packaged application location inside the unified launcher package.
export function bundledPetExecutable(packageRoot) {
  return join(resolve(packageRoot), "runtime", PET_RUNTIME_DIRECTORY, PET_APPLICATION_NAME, PET_EXECUTABLE_RELATIVE_PATH);
}

// Reports whether the bundled application can actually be executed. A package
// without it cannot start a desktop pet, and Adapter maintenance then has
// nothing this package could stop.
export async function isBundledPetApplicationAvailable(executable) {
  try {
    const info = await lstat(executable);
    if (!info.isFile() || info.isSymbolicLink()) return false;
    await access(executable, fsConstants.X_OK);
    return true;
  } catch {
    return false;
  }
}

// The private argument array of the native `run` entry. Every path is already
// absolute because the launcher resolves the Adapter, data, and product
// directories before starting the native process.
export function runArguments(request) {
  return Object.freeze([
    "run",
    "--core-path", request.corePath,
    "--data-dir", request.dataDirectory,
    "--product-root", request.productRoot,
    "--core-identity", request.coreIdentity,
    "--data-root-digest", request.dataRootDigest,
  ]);
}

// The private argument array of the native `stop` entry. `corePath` restricts
// the shutdown to instances using that Core, which is what Adapter maintenance
// needs; `taskbelay pet stop` passes no filter.
export function stopArguments({ productRoot, corePath = null }) {
  const arguments_ = ["stop", "--product-root", productRoot];
  if (corePath !== null && corePath !== undefined) arguments_.push("--core-path", corePath);
  return Object.freeze(arguments_);
}

// Starts the native entry and waits for its single confirmation line.
//
// The desktop process is detached and outlives the launcher: once `ready` or
// `restored` arrives, both pipes are closed and the child is released, so
// later diagnostics belong to the system log instead of this process.
export async function launchPet({
  executable,
  request,
  spawnImpl = spawn,
  environment = process.env,
  timeout = LAUNCH_TIMEOUT_MILLISECONDS,
}) {
  const child = spawnImpl(executable, runArguments(request), {
    cwd: dirname(executable),
    env: environment,
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  return await new Promise((resolvePromise, rejectPromise) => {
    let output = "";
    let detail = "";
    let settled = false;
    const settle = (outcome) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      releaseLaunchChannel(child);
      outcome();
    };
    const timer = setTimeout(() => settle(() => {
      child.kill?.("SIGTERM");
      rejectPromise(new PetNativeError(`no launch confirmation within ${timeout} milliseconds`));
    }), timeout);

    child.stdout?.setEncoding("utf8");
    child.stderr?.setEncoding("utf8");
    child.stdout?.on("data", (chunk) => {
      output += chunk;
      const boundary = output.indexOf("\n");
      if (boundary < 0) return;
      const line = output.slice(0, boundary).trim();
      if (LAUNCH_CONFIRMATIONS.includes(line)) settle(() => resolvePromise(line));
      else settle(() => rejectPromise(new PetNativeError(`unsupported launch confirmation ${line}`)));
    });
    child.stderr?.on("data", (chunk) => { detail += chunk; });
    child.once("error", (error) => settle(() => rejectPromise(new PetNativeError(error.message))));
    child.once("exit", (code) => settle(() => rejectPromise(
      new PetNativeError(firstLine(detail) ?? `the native entry exited with code ${code ?? "unknown"}`),
    )));
  });
}

// Runs the native shutdown entry and reports its exit code with the first
// diagnostic line. The entry is short-lived, so no detached process remains.
export async function shutdownPet({
  executable,
  productRoot,
  corePath = null,
  spawnImpl = spawn,
  environment = process.env,
  timeout = SHUTDOWN_TIMEOUT_MILLISECONDS,
}) {
  const child = spawnImpl(executable, stopArguments({ productRoot, corePath }), {
    cwd: dirname(executable),
    env: environment,
    stdio: ["ignore", "ignore", "pipe"],
  });
  return await new Promise((resolvePromise) => {
    let detail = "";
    let settled = false;
    const settle = (result) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolvePromise(result);
    };
    const timer = setTimeout(() => {
      child.kill?.("SIGTERM");
      settle({ code: 1, detail: `no shutdown result within ${timeout} milliseconds` });
    }, timeout);

    child.stderr?.setEncoding("utf8");
    child.stderr?.on("data", (chunk) => { detail += chunk; });
    child.once("error", (error) => settle({ code: 1, detail: error.message }));
    child.once("exit", (code) => settle({ code: code ?? 1, detail: firstLine(detail) }));
  });
}

export class PetNativeError extends Error {
  constructor(message) {
    super(message);
    this.name = "PetNativeError";
  }
}

function releaseLaunchChannel(child) {
  child.stdout?.removeAllListeners();
  child.stderr?.removeAllListeners();
  child.stdout?.destroy();
  child.stderr?.destroy();
  child.removeAllListeners("error");
  child.removeAllListeners("exit");
  child.unref?.();
}

function firstLine(text) {
  const line = text.split(/\r?\n/u).map((value) => value.trim()).filter(Boolean)[0];
  return line === undefined ? null : line;
}
