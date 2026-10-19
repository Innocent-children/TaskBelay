import { lstat, unlink } from "node:fs/promises";
import { dirname, isAbsolute, join, resolve, relative, sep } from "node:path";
import { execPortableCommand } from "./command.mjs";
import { adapterCoreRuntimePath, inspectMaintainedCore } from "./core-runtime.mjs";
import { ensureManagerDirectories, readOwnedJSON, writeOwnedJSON } from "./ownership.mjs";

/**
 * Keep pre-uninstall Core locations across failed resets until data cleanup succeeds.
 * WebUI owns its runtime receipt; platform implementations own process identity and signals.
 */
export async function stopManagedCores({ targets, dataDirectories, paths, environment, processes, run = execPortableCommand, assertNoPendingRuns = assertNoPendingTaskRuns }) {
  const retained = await readResetRecord(paths);
  const candidates = new Map();
  for (const target of targets) {
    if (target.installedRuntime) candidates.set(target.installedRuntime.runtimePath, target.installedRuntime);
    for (const runtimePath of target.registeredCorePaths) {
      if (!candidates.has(runtimePath)) candidates.set(runtimePath, {
        runtimePath, packageRoot: dirname(dirname(dirname(runtimePath))), packageName: `taskbelay-${target.host}`,
      });
    }
  }
  const runtimes = [];
  for (const candidate of candidates.values()) {
    const runtime = await inspectMaintainedCore(candidate, paths);
    if (runtime) runtimes.push(runtime);
  }
  const runtimePaths = [...new Set([...(retained?.runtime_paths ?? []), ...candidates.keys()])].sort();
  const verifiedPaths = new Set(runtimes.map(runtime => runtime.runtimePath));
  const directories = [...new Set(dataDirectories.map(path => resolve(path)))];
  const processOptions = { environment };
  if (runtimePaths.length) {
    const record = { runtime_paths: runtimePaths };
    if (Buffer.byteLength(`${JSON.stringify(record, null, 2)}\n`) > 64 * 1024) throw new Error("Reset Core maintenance record exceeds 64 KiB");
    await ensureManagerDirectories(paths);
    await writeOwnedJSON(resetRecordPath(paths), record, {
      root: paths.managerRoot, enforcePrivateModes: paths.enforcePrivateModes,
    });
  }
  // Retained paths may outlive their package. They can block cleanup, but cannot authorize a signal.
  await processes.assertManagedCoresStopped(runtimePaths.filter(path => !verifiedPaths.has(path)), processOptions);
  await processes.stopStdioCores(runtimes.map(runtime => runtime.runtimePath), processOptions);
  for (const directory of directories) await stopWebUI(directory, runtimes, { environment, run });
  const verifyStopped = async () => {
    await processes.assertManagedCoresStopped(runtimePaths, processOptions);
    for (const directory of directories) {
      if (await hasWebUIReceipt(directory)) throw new Error("A WebUI runtime record appeared after shutdown; close its Host session before resetting");
    }
  };
  await verifyStopped();
  await assertNoPendingRuns(directories, runtimes, { environment, run });
  const completeCleanup = async () => {
    const current = await readResetRecord(paths);
    if (current === null) return;
    if (JSON.stringify(current.runtime_paths) !== JSON.stringify(runtimePaths)) throw new Error("Reset Core maintenance record changed during cleanup");
    await unlink(resetRecordPath(paths)).catch(error => { if (error.code !== "ENOENT") throw error; });
  };
  return { verifyStopped, completeCleanup };
}

function resetRecordPath(paths) {
  return join(paths.managerRoot, "reset-core-maintenance.json");
}

async function readResetRecord(paths) {
  const path = resetRecordPath(paths);
  try {
    const info = await lstat(path);
    if (paths.enforcePrivateModes && (info.mode & 0o077) !== 0) throw new Error("Reset Core maintenance record permissions are unsafe");
  } catch (error) {
    if (error.code === "ENOENT") return null;
    throw error;
  }
  return readOwnedJSON(path, { root: paths.managerRoot, validate: value => {
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).length !== 1 ||
        !Array.isArray(value.runtime_paths) || value.runtime_paths.length === 0 ||
        new Set(value.runtime_paths).size !== value.runtime_paths.length || value.runtime_paths.some(runtimePath =>
          typeof runtimePath !== "string" || runtimePath.includes("\0") || !isAbsolute(runtimePath) || resolve(runtimePath) !== runtimePath ||
          runtimePath !== adapterCoreRuntimePath(dirname(dirname(dirname(runtimePath))), paths))) {
      throw new Error("Reset Core maintenance record is invalid");
    }
    return { runtime_paths: [...value.runtime_paths].sort() };
  } });
}

async function stopWebUI(directory, runtimes, { environment, run }) {
  if (!await hasWebUIReceipt(directory)) return;
  let detail = "no managed Core executable is available";
  for (const runtime of runtimes) {
    const options = { env: { ...environment, TASKBELAY_DATA_DIR: directory }, cwd: runtime.packageRoot,
      encoding: "utf8", timeout: 10000, maxBuffer: 64 * 1024, windowsHide: true };
    let state;
    try { state = JSON.parse((await run(runtime.runtimePath, ["webui", "status", "--json"], options)).stdout); }
    catch (error) { detail = error.message; continue; }
    if (!await hasWebUIReceipt(directory)) return;
    if (state.readiness !== "ready" || !Number.isInteger(state.pid) || state.pid <= 0) {
      detail = `managed WebUI is ${state.readiness ?? "unknown"}`;
      continue;
    }
    const stopped = JSON.parse((await run(runtime.runtimePath, ["webui", "stop", "--json"], options)).stdout);
    if (stopped.readiness !== "unavailable" || await hasWebUIReceipt(directory)) throw new Error("The managed WebUI did not stop");
    return;
  }
  throw new Error(`Cannot safely stop the WebUI before resetting ${directory}: ${detail}`);
}

async function hasWebUIReceipt(directory) {
  try {
    const info = await lstat(join(directory, "webui-runtime.json"));
    if (!info.isFile() || info.isSymbolicLink()) throw new Error("WebUI runtime record must be a regular file");
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}

/**
 * A stopped Core can leave an external check running or its outcome unresolved.
 * Read through Core's public saved-state API before removing the task database.
 */
export async function assertNoPendingTaskRuns(directories, runtimes, { environment, run }) {
  for (const directory of directories) {
    const database = await lstat(join(directory, "taskbelay.db")).catch(error => { if (error.code !== "ENOENT") throw error; });
    if (!database) continue;
    if (!database.isFile() || database.isSymbolicLink()) throw new Error("Task database must be a regular file before cleanup");
    const runtime = runtimes[0];
    if (!runtime) throw new Error(`Cannot inspect unresolved executions in ${directory}; install a compatible TaskBelay plugin before deleting its data`);
    const call = async (tool, input) => {
      const output = await run(runtime.runtimePath, ["call", tool], { env: { ...environment, TASKBELAY_DATA_DIR: directory }, cwd: runtime.packageRoot,
        input: JSON.stringify(input), encoding: "utf8", timeout: 10000, maxBuffer: 8 * 1024 * 1024, windowsHide: true });
      const response = JSON.parse(output.stdout);
      if (response.ok !== true) throw new Error(response.error?.message ?? "Core could not inspect saved executions");
      return response.result;
    };
    for (let page = 1; ; page++) {
      const tasks = await call("taskbelay_list_tasks", { page, page_size: 100, include_archived: true });
      if (!Array.isArray(tasks.items)) throw new Error("Core returned an invalid task list before cleanup");
      for (const task of tasks.items) {
        for (const repository of task.repositories ?? []) {
          const offset = relative(directory, repository.repository_path);
          if (offset === "" || offset !== ".." && !offset.startsWith(`..${sep}`) && !isAbsolute(offset)) throw new Error(`Task data directory contains project code: ${repository.repository_path}`);
        }
        if (task.status && task.status !== "active") continue;
        const view = await call("taskbelay_get_task", { task_id: task.task_id, refresh: false });
        if (!Array.isArray(view.decision?.pending_runs)) throw new Error("Core did not report pending executions before cleanup");
        if (view.decision.pending_runs.length) throw new Error(`Task ${task.task_id} has running or unresolved executions; recover them before deleting data`);
      }
      if (!tasks.has_next) break;
    }
  }
}
