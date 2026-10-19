import { petApplicationsDiffer } from "../../pet-files.mjs";
import { execPortableCommand } from "../../command.mjs";
import { randomUUID } from "node:crypto";
import { connect } from "node:net";
import { readFile, unlink } from "node:fs/promises";
import { setTimeout as delay } from "node:timers/promises";
import { spawn } from "node:child_process";
import { access, constants as fsConstants, lstat } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";

// The Windows implementation of the bundled desktop component.
//
// This module owns the packaged application location, the private native
// argument array, the launch confirmation, and the shutdown result. Process
// handling that only exists on Windows stays here; the launcher decides what the
// user asked for and renders the user-facing text.

export const PET_RUNTIME_DIRECTORY = "win32-x64";
export const PET_APPLICATION_NAME = "TaskBelayPet";
export const PET_EXECUTABLE_RELATIVE_PATH = "TaskBelayPet.exe";

// The only lines the native `run` entry may confirm a launch with. Anything
// else means the two sides of the private contract drifted apart.
export const LAUNCH_CONFIRMATIONS = Object.freeze(["ready", "restored"]);
export const LAUNCH_TIMEOUT_MILLISECONDS = 10_000;
// The native shutdown entry waits at most five seconds for an orderly exit.
export const SHUTDOWN_TIMEOUT_MILLISECONDS = 15_000;

/**
 * A status reply must come from the per-instance token on its named pipe.
 * The runtime file alone never establishes that the desktop is running.
 */
export async function observePetState({ productRoot, connectImpl = connect }) {
  let record;
  try { record = JSON.parse(await readFile(join(productRoot, "pet", "runtime.json"), "utf8")); }
  catch (error) { return error.code === "ENOENT" ? "stopped" : "unknown"; }
  if (typeof record.pipe !== "string" || !record.pipe.startsWith("\\\\.\\pipe\\taskbelay-pet-") || typeof record.token !== "string" || !record.token) return "unknown";
  return await new Promise(resolvePromise => {
    const socket = connectImpl(record.pipe);
    let data = "";
    const finish = state => { socket.destroy(); resolvePromise(state); };
    socket.setTimeout(1000);
    socket.on("connect", () => socket.write(JSON.stringify({ token: record.token, operation: "status" }) + "\n"));
    socket.on("data", chunk => {
      data += chunk;
      if (data.length > 4096) return finish("unknown");
      if (!data.includes("\n")) return;
      try { finish(JSON.parse(data).result === "running" ? "running" : "unknown"); }
      catch { finish("unknown"); }
    });
    socket.on("timeout", () => finish("unknown"));
    socket.on("error", error => finish(["ENOENT", "ECONNREFUSED"].includes(error.code) ? "stopped" : "unknown"));
    socket.on("end", () => finish("unknown"));
  });
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
    await access(executable, fsConstants.F_OK);
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
    "--launch-id", request.launchID,
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
  environment = process.env,
  timeout = LAUNCH_TIMEOUT_MILLISECONDS,
}) {
  const launchID = randomUUID();
  const receipt = join(request.productRoot, "pet", `.launch-${launchID}.json`);
  const quote = value => "'" + value.replaceAll("'", "''") + "'";
  const argv = runArguments({ ...request, launchID })
    .map(windowsArgument).join(" ");
  await execPortableCommand("powershell.exe", ["-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
    `Start-Process -FilePath ${quote(executable)} -ArgumentList ${quote(argv)} -WindowStyle Hidden`,
  ], { env: environment, encoding: "utf8", timeout: 5000, windowsHide: true });
  const deadline = Date.now() + timeout + 2000;
  try {
    while (Date.now() < deadline) {
      let result;
      try { result = JSON.parse(await readFile(receipt, "utf8")); }
      catch (error) { if (error.code !== "ENOENT") throw error; }
      if (result) {
        if (LAUNCH_CONFIRMATIONS.includes(result.result)) return result.result;
        throw new PetNativeError(result.error ?? "invalid desktop launch confirmation");
      }
      await delay(50);
    }
    throw new PetNativeError("desktop launcher did not return a confirmation");
  } finally {
    await unlink(receipt).catch(() => {});
  }
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

function firstLine(text) {
  const line = text.split(/\r?\n/u).map((value) => value.trim()).filter(Boolean)[0];
  return line === undefined ? null : line;
}

function windowsArgument(value) {
  const slash = String.fromCharCode(92);
  let result = String.fromCharCode(34), pending = 0;
  for (const character of value) {
    if (character === slash) { pending++; continue; }
    result += slash.repeat(character === String.fromCharCode(34) ? pending * 2 + 1 : pending) + character;
    pending = 0;
  }
  return result + slash.repeat(pending * 2) + String.fromCharCode(34);
}
