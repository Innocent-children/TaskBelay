import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, stat, symlink, unlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { stopManagedCores } from "../lib/core-maintenance.mjs";

async function fixture(t) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-reset-cores-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const runtime = { packageRoot: join(root, "claude"), packageName: "taskbelay-claude" };
  runtime.runtimePath = join(runtime.packageRoot, "runtime", "darwin-arm64", "taskbelay");
  await mkdir(join(runtime.packageRoot, "runtime", "darwin-arm64"), { recursive: true });
  await writeFile(join(runtime.packageRoot, "package.json"), JSON.stringify({ name: runtime.packageName }));
  await writeFile(runtime.runtimePath, "isolated fixture, never executed");
  await chmod(runtime.runtimePath, 0o755);
  const data = join(root, "data");
  await mkdir(data);
  const receipt = join(data, "webui-runtime.json");
  await writeFile(receipt, "managed WebUI receipt interpreted only by Core");
  const events = [];
  const options = {
    targets: [{ host: "claude", installedRuntime: runtime, registeredCorePaths: [runtime.runtimePath] }],
    dataDirectories: [data, data],
    paths: { runtimeDirectory: "darwin-arm64", runtimeExecutable: "taskbelay", requireExecutableMode: process.platform !== "win32",
      managerRoot: join(root, "manager"), runsDirectory: join(root, "manager", "runs"), applicationDataInspectionRoot: root, enforcePrivateModes: process.platform !== "win32" },
    environment: {},
    processes: {
      stopStdioCores: async paths => { assert.deepEqual(paths, [runtime.runtimePath]); events.push("stdio.stop"); },
      assertManagedCoresStopped: async paths => { if (paths.length) events.push("processes.check"); },
    },
    run: async (exe, args, processOptions) => {
      assert.equal(exe, runtime.runtimePath);
      assert.equal(processOptions.env.TASKBELAY_DATA_DIR, data);
      events.push(`webui.${args[1]}`);
      if (args[1] === "status") return { stdout: '{"readiness":"ready","pid":123}' };
      assert.deepEqual(args, ["webui", "stop", "--json"]);
      await unlink(receipt);
      return { stdout: '{"readiness":"unavailable"}' };
    },
  };
  return { root, runtime, data, receipt, events, options };
}

test("reset stops managed STDIO and WebUI without Codex and retains locations after uninstall", async t => {
  const f = await fixture(t);
  const reset = await stopManagedCores(f.options);
  assert.deepEqual(f.events, ["stdio.stop", "webui.status", "webui.stop", "processes.check"]);
  await rm(f.runtime.packageRoot, { recursive: true });
  await reset.verifyStopped();
  assert.equal(f.events.at(-1), "processes.check");
  f.options.processes.assertManagedCoresStopped = async paths => {
    assert.deepEqual(paths, [f.runtime.runtimePath]);
    throw new Error("Core reconnected");
  };
  await assert.rejects(reset.verifyStopped(), /reconnected/);
});

test("reset rejects stop failure, a new WebUI receipt and unverified package ownership", async t => {
  const f = await fixture(t);
  await assert.rejects(stopManagedCores({ ...f.options, run: async () => { throw new Error("WebUI inspection failed"); } }), /Cannot safely stop/);
  await assert.rejects(stopManagedCores({ ...f.options, processes: { ...f.options.processes,
    stopStdioCores: async () => { throw new Error("Core identity changed"); } } }), /identity changed/);
  await assert.rejects(stopManagedCores({ ...f.options, run: async (_exe, args) => ({ stdout: JSON.stringify({ readiness: args[1] === "status" ? "ready" : "unavailable", pid: 123 }) }) }), /did not stop/);
  const reset = await stopManagedCores(f.options);
  await writeFile(f.receipt, "reconnected");
  await assert.rejects(reset.verifyStopped(), /appeared after shutdown/);
  await writeFile(join(f.runtime.packageRoot, "package.json"), '{"name":"unrelated"}');
  f.events.length = 0;
  await assert.rejects(stopManagedCores(f.options), /belong to its Adapter/);
  assert.deepEqual(f.events, []);
});

test("missing Core plus a retained WebUI record blocks reset before cleanup", async t => {
  const f = await fixture(t);
  await rm(f.runtime.packageRoot, { recursive: true });
  await assert.rejects(stopManagedCores({ ...f.options, processes: {
    assertManagedCoresStopped: async () => {}, stopStdioCores: async paths => assert.deepEqual(paths, []),
  } }), /no managed Core executable/);
});

test("reset retains both the distribution and Host cache executables for the final process check", async t => {
  const f = await fixture(t);
  const cacheRoot = join(f.root, "owned-plugin-cache");
  const cacheRuntime = join(cacheRoot, "runtime", "darwin-arm64", "taskbelay");
  await mkdir(join(cacheRoot, "runtime", "darwin-arm64"), { recursive: true });
  await writeFile(join(cacheRoot, "package.json"), '{"name":"taskbelay-claude"}');
  await writeFile(cacheRuntime, "isolated fixture, never executed");
  await chmod(cacheRuntime, 0o755);
  f.options.targets[0].registeredCorePaths.push(cacheRuntime);
  f.options.processes.stopStdioCores = async paths => assert.deepEqual(paths, [f.runtime.runtimePath, cacheRuntime]);
  const checks = [];
  f.options.processes.assertManagedCoresStopped = async paths => { if (paths.length) checks.push(paths); };
  const reset = await stopManagedCores(f.options);
  await rm(f.runtime.packageRoot, { recursive: true });
  await rm(cacheRoot, { recursive: true });
  await reset.verifyStopped();
  assert.deepEqual(checks, [[f.runtime.runtimePath, cacheRuntime], [f.runtime.runtimePath, cacheRuntime]]);
});

test("retained reset paths cannot authorize signalling after package ownership is gone", async t => {
  const f = await fixture(t);
  await stopManagedCores(f.options);
  const recordPath = join(f.options.paths.managerRoot, "reset-core-maintenance.json");
  assert.deepEqual(JSON.parse(await readFile(recordPath, "utf8")), { runtime_paths: [f.runtime.runtimePath] });
  if (process.platform !== "win32") assert.equal((await stat(recordPath)).mode & 0o777, 0o600);
  await rm(f.runtime.packageRoot, { recursive: true });
  const calls = [];
  const retryOptions = { ...f.options, targets: [], processes: {
    assertManagedCoresStopped: async paths => { calls.push(paths); throw new Error("Old Core is still running"); },
    stopStdioCores: async () => assert.fail("missing package must not authorize a signal"),
  } };
  await assert.rejects(stopManagedCores(retryOptions), /Old Core is still running/);
  assert.deepEqual(calls, [[f.runtime.runtimePath]]);
  assert.ok(await readFile(recordPath));
  const completed = await stopManagedCores({ ...retryOptions, processes: {
    assertManagedCoresStopped: async paths => { if (paths.length) assert.deepEqual(paths, [f.runtime.runtimePath]); },
    stopStdioCores: async paths => assert.deepEqual(paths, []),
  } });
  await completed.completeCleanup();
  await assert.rejects(stat(recordPath), { code: "ENOENT" });
});

test("reset rejects invalid or linked private maintenance records before process actions", {skip:process.platform==='win32'?'Windows cannot create unprivileged file symlinks':false}, async t => {
  const f = await fixture(t);
  await stopManagedCores(f.options);
  const recordPath = join(f.options.paths.managerRoot, "reset-core-maintenance.json");
  for (const value of [{ runtime_paths: ["relative/runtime/darwin-arm64/taskbelay"] },
    { runtime_paths: [f.runtime.runtimePath, f.runtime.runtimePath] }, { runtime_paths: [f.runtime.runtimePath], extra: true }]) {
    await writeFile(recordPath, JSON.stringify(value), { mode: 0o600 });
    f.events.length = 0;
    await assert.rejects(stopManagedCores(f.options), /record is invalid/);
    assert.deepEqual(f.events, []);
  }
  await unlink(recordPath);
  const outside = join(f.root, "outside-record.json");
  await writeFile(outside, JSON.stringify({ runtime_paths: [f.runtime.runtimePath] }), { mode: 0o600 });
  await symlink(outside, recordPath);
  await assert.rejects(stopManagedCores(f.options), /permissions are unsafe|bounded regular file/);
});

test("cleanup reads every active task through Core and refuses unresolved executions", async t => {
  const f = await fixture(t);
  await writeFile(join(f.data, "taskbelay.db"), "database bytes interpreted only by Core");
  const { assertNoPendingTaskRuns } = await import("../lib/core-maintenance.mjs");
  const calls = [];
  const run = async (_exe, args, options) => {
    const input = JSON.parse(options.input); calls.push({ tool: args[1], input });
    assert.equal(options.env.TASKBELAY_DATA_DIR, f.data);
    const result = args[1] === "taskbelay_list_tasks" ? { items: [{ task_id: "task-a" }], has_next: false }
      : { decision: { pending_runs: ["run-a"] } };
    return { stdout: JSON.stringify({ ok: true, result }) };
  };
  await assert.rejects(assertNoPendingTaskRuns([f.data], [f.runtime], { environment: {}, run }), /task-a.*unresolved/);
  assert.deepEqual(calls[1], { tool: "taskbelay_get_task", input: { task_id: "task-a", refresh: false } });
  await assert.rejects(assertNoPendingTaskRuns([f.data], [], { environment: {}, run }), /compatible TaskBelay plugin/);
});
