import { installedPackageRoot } from "./local-packages.mjs";
import { describeInterface, interfaceHelpRequest } from "./interface-reference.mjs";
import { spawn } from "node:child_process";
import { execFile as execFileCallback } from "node:child_process";
import { readFile } from "node:fs/promises";
import { promisify } from "node:util";

import { createHostDrivers } from "./hosts/index.mjs";
import { assertCanonicalDirectory, inspectCoreRuntime } from "./core-runtime.mjs";

import { renderHelp } from "./cli.mjs";
import { resolveLanguage } from "./presentation.mjs";
import { ensureDefaultDataDirectory, resolveManagerPaths } from "./ownership.mjs";

const execFile = promisify(execFileCallback);
const packageVersion = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8")).version;

export async function runTaskBelay(arguments_, dependencies = {}) {
  const stdout = dependencies.stdout ?? process.stdout;
  const stderr = dependencies.stderr ?? process.stderr;
  const environment = dependencies.environment ?? process.env;
  if (["help", "--help", "-h"].includes(arguments_[0])) {
    stdout.write(renderHelp(null, resolveLanguage(environment)));
    return { code: 0, signal: null };
  }
  try {
    const help = interfaceHelpRequest(arguments_);
    if (help?.name) {
      stdout.write(JSON.stringify(await describeInterface(installedPackageRoot,help.name),null,2)+"\n");
      return {code:0,signal:null};
    }
    if (arguments_[0] === "webui") assertWebUIArguments(arguments_);
    if (["version", "--version"].includes(arguments_[0])) {
      let selection;
      try {
        selection = await (dependencies.resolveCoreRuntime ?? resolveCoreRuntime)({
          environment,
          homeDirectory: dependencies.homeDirectory,
          platform: dependencies.platform,
          arch: dependencies.arch,
          exec: dependencies.exec,
          requireData: false,
        });
      } catch (error) {
        if (!(error instanceof NoRuntimeError)) throw error;
      }
      stdout.write(selection
        ? `taskbelay ${packageVersion} (core ${selection.version}, ${selection.source})\n`
        : `taskbelay ${packageVersion} (core unavailable)\n`);
      return { code: 0, signal: null };
    }
    const selection = await (dependencies.resolveCoreRuntime ?? resolveCoreRuntime)({
      environment,
      homeDirectory: dependencies.homeDirectory,
      platform: dependencies.platform,
      arch: dependencies.arch,
      exec: dependencies.exec,
      requireData: true,
      initializeDefaultData: arguments_[0] === "webui" && arguments_[1] === "start",
    });
    return await launchCore(selection, arguments_, {
      environment,
      spawnImpl: dependencies.spawnImpl ?? spawn,
      signalSource: dependencies.signalSource ?? process,
    });
  } catch (error) {
    if (arguments_.includes("--json")) stdout.write(`${JSON.stringify({ status: "failed", error: { code: error.code ?? "RUNTIME_UNAVAILABLE", message: error.message }, next_step: "taskbelay doctor --host all" })}\n`);
    else stderr.write(`taskbelay: ${error.message}\ntaskbelay doctor --host all\n`);
    return { code: error.exitCode ?? 1, signal: null };
  }
}

export async function resolveCoreRuntime({
  host = "all",
  environment = process.env,
  homeDirectory,
  platform = process.platform,
  arch = process.arch,
  exec = execFile,
  requireData = true,
  initializeDefaultData = false,
} = {}) {
  const paths = await resolveManagerPaths({ homeDirectory, environment, platform, arch });
  const dataDirectory = paths.explicitDataDirectory ?? paths.defaultDataDirectory;
  const candidates = [];

  const drivers = createHostDrivers({ paths, environment });
  if (host !== "all" && !Object.hasOwn(drivers, host)) throw new Error(`unsupported runtime Host ${host}`);
  for (const driver of host === "all" ? Object.values(drivers) : [drivers[host]]) {
    for (const candidate of await driver.runtimeCandidates()) {
      candidates.push(await inspectCoreRuntime(candidate, { exec, environment, requireExecutableMode: paths.requireExecutableMode }));
    }
  }

  if (candidates.length === 0) throw new NoRuntimeError(host === "all"
    ? "no installed Codex, DeepSeek, Claude or ZCode Adapter provides a Core runtime"
    : `no installed ${host} Adapter provides a Core runtime`);
  if (requireData) {
    if (initializeDefaultData && paths.explicitDataDirectory === null) await ensureDefaultDataDirectory(paths);
    else await assertCanonicalDirectory(dataDirectory, "TaskBelay data directory");
  }
  candidates.sort((left, right) => compareSemver(right.version, left.version) || left.source.localeCompare(right.source));
  return Object.freeze({
    ...candidates[0],
    dataDirectory,
    platform: paths.platform,
    arch: paths.arch,
    runtimeKey: paths.runtimeKey,
    forwardedSignals: paths.forwardedSignals,
  });
}

export class NoRuntimeError extends Error {
  constructor(message) {
    super(message);
    this.name = "NoRuntimeError";
  }
}

function assertWebUIArguments(arguments_) {
  const allowed = arguments_[1] === "start" ? ["--no-open", "--plain", "--json"] : ["--plain", "--json"];
  const options = arguments_.slice(2);
  if (!["start", "open", "status", "stop"].includes(arguments_[1]) || options.some(option => !allowed.includes(option)) ||
      new Set(options).size !== options.length || options.includes("--plain") && options.includes("--json")) {
    const error = new Error("invalid WebUI arguments; run taskbelay help");
    error.exitCode = 2;
    throw error;
  }
}

async function launchCore(selection, arguments_, { environment, spawnImpl, signalSource }) {
  if (!Array.isArray(selection.forwardedSignals) || selection.forwardedSignals.length === 0) {
    throw new Error("platform signal contract is unavailable");
  }
  const child = spawnImpl(selection.runtimePath, arguments_, {
    cwd: selection.packageRoot,
    env: { ...environment, TASKBELAY_DATA_DIR: selection.dataDirectory },
    stdio: "inherit",
    shell: false,
    windowsHide: true,
  });
  const handlers = new Map();
  for (const signal of selection.forwardedSignals) {
    const handler = () => child.kill(signal);
    handlers.set(signal, handler);
    signalSource.on(signal, handler);
  }
  const cleanup = () => { for (const [signal, handler] of handlers) signalSource.off(signal, handler); };
  return await new Promise((resolvePromise) => {
    child.once("error", () => { cleanup(); resolvePromise({ code: 1, signal: null }); });
    child.once("exit", (code, signal) => { cleanup(); resolvePromise({ code: code ?? 1, signal }); });
  });
}

function compareSemver(left, right) {
  const a = left.split(".").map(Number);
  const b = right.split(".").map(Number);
  for (let index = 0; index < 3; index += 1) if (a[index] !== b[index]) return a[index] - b[index];
  return 0;
}
