#!/usr/bin/env node

import { execFile as execFileCallback, spawn } from "node:child_process";
import { constants as fsConstants, realpathSync } from "node:fs";
import { access, lstat, readFile, stat } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { promisify } from "node:util";

import {
  CODEX_MCP_INSTRUCTIONS,
  inspectRegistrationStatus,
  inspectCoreVersion,
  removeRegistration,
  setupRegistration,
} from "../lib/lifecycle.mjs";
import {
  buildSetupSuccessResult,
  ensureUserConfiguration,
  renderSetup,
  renderSetupPlain,
  resolveSetupLanguage,
  selectSetupPresentationMode,
} from "../lib/install-experience.mjs";
import {
  ensureDefaultDataDirectory,
  resolveProductPaths,
} from "../lib/paths.mjs";
import { runHook } from "../plugin/hooks/pre-tool-use.mjs";
import { assertNoDuplicateJSONMembers } from "../lib/json.mjs";
import { execPortableCommand } from "../lib/command.mjs";
import { inspectAdmissionAnchor } from "../lib/task-admission.mjs";
import { HOST_LAUNCH_OPERATIONS, hostLaunchHelp } from "../lib/host-launch-contract.mjs";
import { ARTIFACT_OPERATIONS, artifactsHelp } from "../lib/artifacts-help.mjs";
import {
  beginManagedTaskDispatch,
  claimManagedTaskDispatch,
  reconcileManagedTaskDispatch,
  recoverUncalledManagedTaskDispatch,
  beginTaskHandoff,
  bootstrapManagedTask,
  cleanupCliTaskWorktree,
  cleanupTaskBranch,
  prepareTaskLaunch,
  supersedeTaskLaunch,
  resumeLaunchSupersession,
  readOpenTaskRepositoryScope,
  provisionCliTask,
  provisionLocalTask,
  recordManagedTaskDispatch,
  recordTaskHandoff,
  recordTaskHandoffStatus,
} from "../lib/task-launch.mjs";
import { provisioningReceiptPath, readProvisioningReceipt, receiptDigest } from "../lib/provisioning-receipt.mjs";
import { terminalCleanupDecision } from "../lib/worktree-lifecycle.mjs";

const execFile = promisify(execFileCallback);
const NPM_UNINSTALL_HANDOFF = "Run npm uninstall -g taskbelay-codex separately after deregistration.";
const CODEX_MCP_INSTRUCTIONS_ENVIRONMENT = "TASKBELAY_CODEX_MCP_INSTRUCTIONS";

export async function runCLI(arguments_, dependencies = {}) {
  const stdout = dependencies.stdout ?? process.stdout;
  const stderr = dependencies.stderr ?? process.stderr;
  const environment = dependencies.environment ?? process.env;
  const resolvePaths = dependencies.resolvePaths ?? (() => resolveProductPaths({ environment }));
  const readPackageVersion = dependencies.readPackageVersion ?? readInstalledPackageVersion;
  const inspectVersion = dependencies.inspectCoreVersion ?? inspectCoreVersion;
  const inspectStatus = dependencies.inspectRegistrationStatus ?? inspectRegistrationStatus;
  const ensureDataDirectory = dependencies.ensureDefaultDataDirectory ?? ensureDefaultDataDirectory;
  const setup = dependencies.setupRegistration ?? setupRegistration;
  const ensureConfiguration = dependencies.ensureUserConfiguration ?? ensureUserConfiguration;
  const renderSetupResult = dependencies.renderSetup ?? renderSetup;
  let completedSetupChanges = [];
  let setupAttempted = false;

  if (Array.isArray(arguments_) && arguments_.length === 1 && arguments_[0] === "--help") {
    stdout.write("Usage: taskbelay-codex status|setup|remove [--json]\n       taskbelay-codex --version\n       taskbelay-codex mcp\n       taskbelay-codex artifacts <collect|prepare> [--help]\n       taskbelay-codex hook pre-tool-use\n       taskbelay-codex host-check pre-file-write\n       taskbelay-codex host-check workspace-available\n\n" + artifactsHelp() + "\n" + hostLaunchHelp());
    return { code: 0, signal: null };
  }
  if (Array.isArray(arguments_) && arguments_[0] === "host-launch" && (
    arguments_.length === 2 && arguments_[1] === "--help" ||
    arguments_.length === 3 && HOST_LAUNCH_OPERATIONS.includes(arguments_[1]) && arguments_[2] === "--help"
  )) {
    stdout.write(hostLaunchHelp(arguments_.length === 3 ? arguments_[1] : undefined));
    return { code: 0, signal: null };
  }
  if (Array.isArray(arguments_) && arguments_[0] === "artifacts" && (
    arguments_.length === 2 && arguments_[1] === "--help" ||
    arguments_.length === 3 && ARTIFACT_OPERATIONS.includes(arguments_[1]) && arguments_[2] === "--help"
  )) {
    stdout.write(artifactsHelp(arguments_.length === 3 ? arguments_[1] : undefined));
    return { code: 0, signal: null };
  }

  if (!isProductionCommand(arguments_)) {
    stderr.write("taskbelay-codex: invalid arguments; expected status [--json], setup [--json], remove [--json], mcp, artifacts <collect|prepare>, hook pre-tool-use, host-check <pre-file-write|workspace-available>, host-launch <operation>, --version, or --help\n");
    return { code: 2, signal: null };
  }

  try {
    if (arguments_.length === 2 && arguments_[0] === "hook" && arguments_[1] === "pre-tool-use") {
      const invokeHook = dependencies.runPreToolUseHook ?? (() => runHook({
        output: stdout,
        error: stderr,
        environment,
      }));
      return { code: await invokeHook(), signal: null };
    }
    const paths = await resolvePaths();
    if (arguments_.length === 2 && arguments_[0] === "artifacts") {
      return await launchPackagedCore(paths, arguments_, {
        environment,
        spawnImpl: dependencies.spawnImpl ?? spawn,
        signalSource: dependencies.signalSource ?? process,
      });
    }
    if (arguments_.length === 1 && arguments_[0] === "mcp") {
      if (paths.usesDefaultDataDirectory) await ensureDataDirectory(paths);
      return await launchPackagedCore(paths, ["mcp", "--stdio"], {
        environment,
        spawnImpl: dependencies.spawnImpl ?? spawn,
        signalSource: dependencies.signalSource ?? process,
      });
    }
    if (arguments_.length === 2 && arguments_[0] === "host-check") {
      return await launchPackagedCore(paths, arguments_, {
        environment,
        spawnImpl: dependencies.spawnImpl ?? spawn,
        signalSource: dependencies.signalSource ?? process,
      });
    }
    if (arguments_.length === 2 && arguments_[0] === "host-launch") {
      const input = await readClosedStandardInput(dependencies.readInput);
      const result = await runHostLaunchCommand(arguments_[1], input, paths, dependencies);
      stdout.write(`${JSON.stringify(result)}\n`);
      return { code: 0, signal: null };
    }

    const packageVersion = await readPackageVersion(paths.packageRoot);
    if (arguments_.length === 1 && arguments_[0] === "--version") {
      const coreVersion = await inspectVersion(paths.runtimePath, {
        environment,
        currentDirectory: paths.packageRoot,
        requireExecutableMode: paths.requireExecutableMode,
      });
      stdout.write(`taskbelay-codex ${packageVersion} (core ${coreVersion})\n`);
      return { code: 0, signal: null };
    }

    const json = arguments_.at(-1) === "--json";
    if (arguments_[0] === "status") {
      const coreVersion = await inspectVersion(paths.runtimePath, {
        environment,
        currentDirectory: paths.packageRoot,
        requireExecutableMode: paths.requireExecutableMode,
      });
      const result = await inspectStatus({
        paths,
        packageVersion,
        codexExecutable: dependencies.codexExecutable ?? "codex",
        environment,
      });
      writeStatusSuccess(stdout, result, {
        packageVersion,
        coreVersion,
        receiptPath: paths.receiptPath,
        json,
      });
      return { code: 0, signal: null };
    }
    if (arguments_[0] === "setup") {
      setupAttempted = true;
      const configuration = await ensureConfiguration(paths, { environment });
      if (configuration.fileChange) completedSetupChanges = [configuration.fileChange];
      const result = await setup({
        paths,
        packageVersion,
        codexExecutable: dependencies.codexExecutable ?? "codex",
        environment,
        now: dependencies.now,
      });
      const setupResult = buildSetupSuccessResult(result, configuration, paths.receiptPath);
      writeSetupSuccess(stdout, setupResult, json, {
        environment,
        renderSetupResult,
      });
      return { code: 0, signal: null };
    }

    const stopWebUI = dependencies.stopPackagedWebUI ?? stopPackagedWebUI;
    await stopWebUI(paths, { environment });
    const remove = dependencies.removeRegistration ?? removeRegistration;
    const result = await remove({
      paths,
      packageVersion,
      codexExecutable: dependencies.codexExecutable ?? "codex",
      environment,
    });
    writeLifecycleSuccess(stdout, "remove", result, paths.receiptPath, json);
    return { code: 0, signal: null };
  } catch (error) {
    if (setupAttempted && completedSetupChanges.length > 0) {
      stderr.write(
        `taskbelay-codex: ${error.message}; created ${completedSetupChanges[0].path}; ` +
        "Codex registration is incomplete; run taskbelay-codex setup again\n",
      );
    } else {
      stderr.write(`taskbelay-codex: ${error.message}\n`);
    }
    return { code: 1, signal: null };
  }
}

export async function stopPackagedWebUI(
  paths,
  { environment = process.env, exec = execFile } = {},
) {
  try {
    if (!(await stat(paths.dataDirectory)).isDirectory()) throw new Error("data path is not a directory");
  } catch (error) {
    if (error?.code === "ENOENT" && paths.usesDefaultDataDirectory) return;
    throw new Error("inspect packaged WebUI data directory", { cause: error });
  }
  // A missing binary is removable only when there is no managed WebUI record.
  // Do not turn a failed stop or an unreadable record into permission to delete.
  try { await lstat(paths.runtimePath); }
  catch (error) {
    if (error.code !== "ENOENT") throw error;
    try { await lstat(join(paths.dataDirectory, "webui-runtime.json")); }
    catch (recordError) { if (recordError.code === "ENOENT") return; throw recordError; }
    throw new Error("Core runtime is missing while a WebUI runtime record remains; restore the runtime before removal");
  }
  await assertExecutableRuntime(paths.runtimePath, paths.requireExecutableMode);
  try {
    await exec(paths.runtimePath, ["webui", "stop", "--json"], {
      cwd: paths.packageRoot,
      env: { ...environment, TASKBELAY_DATA_DIR: paths.dataDirectory },
      encoding: "utf8",
      maxBuffer: 64 * 1024,
    });
  } catch (error) {
    throw new Error("stop packaged WebUI", { cause: error });
  }
}

export async function launchPackagedCore(
  paths,
  arguments_,
  {
    environment = process.env,
    spawnImpl = spawn,
    signalSource = process,
  } = {},
) {
  await assertExecutableRuntime(paths.runtimePath, paths.requireExecutableMode);
  if (!Array.isArray(paths.forwardedSignals) || paths.forwardedSignals.length === 0) {
    throw new Error("platform signal contract is unavailable");
  }
  let child;
  try {
    child = spawnImpl(paths.runtimePath, arguments_, {
      cwd: paths.packageRoot,
      env: {
        ...environment,
        TASKBELAY_DATA_DIR: paths.dataDirectory,
        [CODEX_MCP_INSTRUCTIONS_ENVIRONMENT]: CODEX_MCP_INSTRUCTIONS,
      },
      stdio: "inherit",
      shell: false,
      windowsHide: true,
    });
  } catch (error) {
    throw new Error("start packaged Core", { cause: error });
  }

  const handlers = new Map();
  for (const signal of paths.forwardedSignals) {
    const handler = () => child.kill(signal);
    handlers.set(signal, handler);
    signalSource.on(signal, handler);
  }
  const cleanup = () => {
    for (const [signal, handler] of handlers) signalSource.off(signal, handler);
  };

  return await new Promise((resolve, reject) => {
    child.once("error", (error) => {
      cleanup();
      reject(new Error("packaged Core process failed to start", { cause: error }));
    });
    child.once("exit", (code, signal) => {
      cleanup();
      resolve({ code, signal });
    });
  });
}

async function readInstalledPackageVersion(packageRoot) {
  let manifest;
  try {
    manifest = JSON.parse(await readFile(new URL("../package.json", pathToFileURL(`${packageRoot}/bin/`)), "utf8"));
  } catch (error) {
    throw new Error("read installed package identity", { cause: error });
  }
  if (manifest.name !== "taskbelay-codex" || typeof manifest.version !== "string") {
    throw new Error("installed package identity is invalid");
  }
  return manifest.version;
}

async function assertExecutableRuntime(runtimePath, requireExecutableMode = true) {
  let info;
  try {
    info = await stat(runtimePath);
    await access(runtimePath, requireExecutableMode ? fsConstants.X_OK : fsConstants.F_OK);
  } catch (error) {
    throw new Error("packaged Core must exist and be executable", { cause: error });
  }
  if (!info.isFile() || requireExecutableMode && (info.mode & 0o111) === 0) {
    throw new Error("packaged Core must exist and be executable");
  }
}

function writeLifecycleSuccess(stdout, operation, result, receiptPath, json) {
  if (json) {
    const output = {
      operation,
      status: result.status,
      changed: result.changed,
      receipt_path: receiptPath,
    };
    if (operation === "remove") output.next_step = NPM_UNINSTALL_HANDOFF;
    stdout.write(`${JSON.stringify(output)}\n`);
    return;
  }
  stdout.write(`taskbelay-codex ${operation}: ${result.status}\n`);
  if (operation === "remove") stdout.write(`${NPM_UNINSTALL_HANDOFF}\n`);
}

function writeStatusSuccess(stdout, result, { packageVersion, coreVersion, receiptPath, json }) {
  const output = {
    operation: "status",
    status: result.status,
    changed: false,
    package_version: packageVersion,
    core_version: coreVersion,
    receipt_path: receiptPath,
    registration: {
      receipt: result.receipt,
      marketplace: result.marketplace,
      plugin: result.plugin,
    },
  };
  if (json) {
    stdout.write(`${JSON.stringify(output)}\n`);
    return;
  }
  stdout.write(`taskbelay-codex status: ${result.status}\n`);
  stdout.write(`Package ${packageVersion} · Core ${coreVersion}\n`);
}

function writeSetupSuccess(stdout, result, json, { environment, renderSetupResult }) {
  if (json) {
    stdout.write(`${JSON.stringify(result)}\n`);
    return;
  }
  const language = resolveSetupLanguage(environment);
  const mode = selectSetupPresentationMode(stdout, environment);
  try {
    stdout.write(renderSetupResult(result, { language, mode }));
  } catch {
    stdout.write(renderSetupPlain(result, language));
  }
}

function isProductionCommand(arguments_) {
  if (!Array.isArray(arguments_)) return false;
  if (arguments_.length === 2 && arguments_[0] === "artifacts" && ["collect", "prepare"].includes(arguments_[1])) return true;
  if (arguments_.length === 1 && ["mcp", "--version"].includes(arguments_[0])) return true;
  if (arguments_.length === 2 && arguments_[0] === "hook" && arguments_[1] === "pre-tool-use") return true;
  if (arguments_.length === 2 && arguments_[0] === "host-check" && ["pre-file-write", "workspace-available"].includes(arguments_[1])) return true;
  if (arguments_.length === 2 && arguments_[0] === "host-launch" && HOST_LAUNCH_OPERATIONS.includes(arguments_[1])) return true;
  return (
    (arguments_.length === 1 || arguments_.length === 2 && arguments_[1] === "--json") &&
    ["status", "setup", "remove"].includes(arguments_[0])
  );
}

async function runHostLaunchCommand(operation, input, paths, dependencies) {
  const common = {
    productSupportRoot: paths.productSupportRoot,
    enforcePrivateModes: paths.enforcePrivateModes,
    runGit: dependencies.runGit,
    checkWorkspaceAvailable: dependencies.checkWorkspaceAvailable ?? (async (repositoryPath) => {
      if (paths.usesDefaultDataDirectory) await (dependencies.ensureDefaultDataDirectory ?? ensureDefaultDataDirectory)(paths);
      const result = await execPortableCommand(paths.runtimePath, ["host-check", "workspace-available"], {
        cwd: paths.packageRoot,
        env: { ...(dependencies.environment ?? process.env), TASKBELAY_DATA_DIR: paths.dataDirectory },
        input: `${JSON.stringify({ repository_path: repositoryPath })}\n`,
        encoding: "utf8", timeout: 30_000, maxBuffer: 1024 * 1024,
      });
      return JSON.parse(result.stdout);
    }),
  };
  if (operation === "inspect") {
    assertClosedObject(input, ["request", "repositories"], "host-launch inspect input");
    return await inspectAdmissionAnchor({ ...input, runGit: dependencies.runGit });
  }
  if (operation === "prepare") {
    return await prepareTaskLaunch(input, {
      ...common,
      now: dependencies.now,
      createLaunchId: dependencies.createLaunchId,
    });
  }
  if (operation === "scope") return await readOpenTaskRepositoryScope(input, common);
  if (operation === "supersede") return await supersedeTaskLaunch(input, common);
  if (operation === "supersede-resume") return await resumeLaunchSupersession(input, common);
  if (operation === "local-provision") return await provisionLocalTask(input, common);
  if (operation === "status") {
    assertClosedObject(input, ["launch_id", "repository_key"], "host-launch status input");
    const receiptPath = provisioningReceiptPath(paths.productSupportRoot, input.launch_id, input.repository_key);
    const receipt = await readProvisioningReceipt(receiptPath, { productSupportRoot: paths.productSupportRoot });
    return {
      receipt_path: receiptPath,
      receipt,
      receipt_digest: receipt === null ? null : receiptDigest(receipt),
    };
  }
  if (operation === "dispatch-start") return await beginManagedTaskDispatch(input, common);
  if (operation === "dispatch-reconcile") return await reconcileManagedTaskDispatch(input, common);
  if (operation === "dispatch-call") return await claimManagedTaskDispatch(input, common);
  if (operation === "dispatch-recover") return await recoverUncalledManagedTaskDispatch(input, common);
  if (operation === "dispatch-result") return await recordManagedTaskDispatch(input, common);
  if (operation === "handoff-start") return await beginTaskHandoff(input, common);
  if (operation === "handoff-result") return await recordTaskHandoff(input, common);
  if (operation === "handoff-status") return await recordTaskHandoffStatus(input, common);
  if (operation === "cleanup-decision") {
    assertClosedObject(input, ["lifecycle", "surface", "clean", "pushed", "stateCertain"], "cleanup decision input");
    return terminalCleanupDecision(input);
  }
  if (operation === "bootstrap") return await bootstrapManagedTask(input, common);

  const sourceRepositoryPath = input?.source_repository_path;
  if (typeof sourceRepositoryPath !== "string") throw new Error(`${operation} requires source_repository_path`);
  const operationInput = { ...input };
  delete operationInput.source_repository_path;
  const repositoryOptions = { ...common, sourceRepositoryPath };
  if (operation === "cli-provision") return await provisionCliTask(operationInput, repositoryOptions);
  if (operation === "cleanup-worktree") return await cleanupCliTaskWorktree(operationInput, repositoryOptions);
  if (operation === "cleanup-branch") return await cleanupTaskBranch(operationInput, repositoryOptions);
  throw new Error(`unsupported host-launch operation ${operation}`);
}

async function readStandardInput() {
  const chunks = [];
  let byteLength = 0;
  for await (const chunk of process.stdin) {
    byteLength += chunk.length;
    if (byteLength > 1024 * 1024) {
      throw new Error("host-launch input must be UTF-8 JSON no larger than 1 MiB");
    }
    chunks.push(chunk);
  }
  try {
    return new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(Buffer.concat(chunks, byteLength));
  } catch (error) {
    throw new Error("host-launch input must be UTF-8 JSON", { cause: error });
  }
}

async function readClosedStandardInput(readInput = readStandardInput) {
  const raw = await readInput();
  if (typeof raw !== "string" || Buffer.byteLength(raw) > 1024 * 1024) {
    throw new Error("host-launch input must be UTF-8 JSON no larger than 1 MiB");
  }
  let value;
  try {
    assertNoDuplicateJSONMembers(raw);
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`host-launch input must be one JSON object: ${error.message}`, { cause: error });
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("host-launch input must be one JSON object");
  }
  return value;
}

function assertClosedObject(value, keys, label) {
  const actual = Object.keys(value ?? {}).sort();
  const expected = [...keys].sort();
  if (value === null || typeof value !== "object" || Array.isArray(value) || JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${label} has an invalid closed shape`);
  }
}

function isMainModule() {
  if (!process.argv[1]) return false;
  try {
    return pathToFileURL(realpathSync(process.argv[1])).href === import.meta.url;
  } catch {
    return false;
  }
}

if (isMainModule()) {
  const result = await runCLI(process.argv.slice(2));
  if (result.signal) {
    try {
      process.kill(process.pid, result.signal);
    } catch {
      process.exitCode = 1;
    }
  } else {
    process.exitCode = result.code ?? 1;
  }
}
