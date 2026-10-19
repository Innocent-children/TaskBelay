import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { runLifecycle } from "../lib/lifecycle.mjs";
import { resolveManagerPaths } from "../lib/ownership.mjs";

test("delete-all requires all Hosts and moves exact shared data to Trash", async (t) => {
  const fixture = await resetFixture(t);
  const result = await runLifecycle(request(), {
    ...fixture.dependencies,
    confirmPlan: async () => true,
    now: () => new Date("2026-08-25T00:00:00Z"),
    random: () => "fixture",
  });
  assert.equal(result.result.data.policy, "recoverable_delete");
  assert.match(result.result.data.trash_root, /taskbelay-/u);
  assert.equal(result.plan.impacts.includes("Clear desktop pet records, preferences, and imported appearances"), true);
  assert.equal(result.result.data.pet, "absent");
  assert.equal(result.result.completed_actions.includes("manager.trash.pet"), true);
  assert.deepEqual(fixture.events, ["pet.stop:null", "codex.uninstall", "deepseek.uninstall", "claude.uninstall"]);
  await assert.rejects(stat(fixture.paths.configurationPath), { code: "ENOENT" });
  await assert.rejects(stat(fixture.paths.defaultDataDirectory), { code: "ENOENT" });
  await assert.rejects(stat(fixture.paths.petDirectory), { code: "ENOENT" });
  assert.equal(fixture.states.codex, "absent");
  assert.equal(fixture.states.deepseek, "absent");
});

test("delete-all keeps Adapters, data, and the pet directory when the pet does not stop", async (t) => {
  const fixture = await resetFixture(t, { stopPet: async () => { throw new Error("the pet did not stop"); } });
  await assert.rejects(runLifecycle(request(), {
    ...fixture.dependencies,
    confirmPlan: async () => true,
  }), /the pet did not stop/u);
  assert.deepEqual(fixture.events, []);
  assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "old-task\n");
  assert.equal(await readFile(join(fixture.paths.petDirectory, "preferences.json"), "utf8"), "pet-preferences\n");
});

test("an unconfirmed delete-all previews the pet impact and cleans nothing", async (t) => {
  const fixture = await resetFixture(t);
  const result = await runLifecycle(request(), {
    ...fixture.dependencies,
    confirmPlan: async () => false,
  });
  assert.equal(result.code, 3);
  assert.equal(result.plan.impacts.includes("Clear desktop pet records, preferences, and imported appearances"), true);
  assert.equal(result.result.confirmation.impacts.includes("Clear desktop pet records, preferences, and imported appearances"), true);
  assert.deepEqual(fixture.events, []);
  assert.equal(await readFile(join(fixture.paths.petDirectory, "preferences.json"), "utf8"), "pet-preferences\n");
});

test("deleting data retains installed plugins and initializes empty data without reinstall", async (t) => {
  const fixture = await resetFixture(t);
  const result = await runLifecycle(request("delete-data"), {
    ...fixture.dependencies,
    confirmPlan: async () => true,
    now: () => new Date("2026-08-25T00:00:00Z"),
    random: () => "fixture-reinstall",
  });
  assert.equal(result.result.status, "restart_required");
  assert.equal(result.code, 0);
  assert.equal(fixture.states.zcode, "absent");
  assert.deepEqual(JSON.parse(await readFile(fixture.paths.configurationPath, "utf8")), {});
  await assert.rejects(readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db")), { code: "ENOENT" });
  assert.equal(fixture.states.codex, "ready");
  assert.equal(fixture.states.deepseek, "ready");
  assert.deepEqual(fixture.events, ["pet.stop:null"]);
});

test("explicit data is blocked until its exact canonical path is confirmed", async (t) => {
  const fixture = await resetFixture(t, { explicit: true });
  await assert.rejects(runLifecycle(request(), {
    ...fixture.dependencies, confirmPlan: async () => true,
  }), /confirm-explicit-data/u);
  assert.equal(await readFile(join(fixture.paths.explicitDataDirectory, "taskbelay.db"), "utf8"), "explicit-task\n");
});

test("delete-all uninstalls a Codex package after its registration is already absent", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-reset-package-only-"));
  const home = join(root, "home");
  await mkdir(home);
  let packageInstalled = true;
  const operations = [];
  const codexDriver = {
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    knownProfiles: async () => [],
    resolveTargetVersion: async () => "0.7.3",
    observe: async () => ({
      host: "codex",
      profile: null,
      hostAvailable: true,
      hostVersion: "0.147.0",
      state: "absent",
      packageInstalled,
      packageVersion: packageInstalled ? "0.7.3" : null,
      coreVersion: packageInstalled ? "0.6.2" : null,
      receipt: false,
    }),
    execute: async (operation) => {
      operations.push(operation);
      packageInstalled = false;
      return { changed: true, completedSteps: ["codex.uninstall_package"] };
    },
  };
  const deepseekDriver = {
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    knownProfiles: async () => [],
    resolveTargetVersion: async () => "0.7.3",
    observe: async (profile) => ({ host: "deepseek", profile, hostAvailable: true, state: "absent", packageVersion: null, coreVersion: null, receipt: null }),
  };

  const result = await runLifecycle(request(), {
    homeDirectory: home,
    environment: {},
    platform: process.platform,
    arch: process.arch,
    codexDriver,
    deepseekDriver,
    claudeDriver: { observe: async () => ({ host: "claude", profile: null, hostAvailable: true, state: "absent", packageVersion: null, receipt: null }) },
    zcodeDriver: { observe: async () => ({ host: "zcode", profile: null, hostAvailable: null, state: "absent", packageInstalled: false, packageVersion: null, receipt: false }) },
    confirmPlan: async () => true,
  });

  assert.deepEqual(operations, ["uninstall"]);
  assert.equal(packageInstalled, false);
  assert.equal(result.result.changed, true);
  assert.equal(result.result.status, "absent");
  assert.equal(result.result.completed_actions.includes("codex.uninstall_package"), true);
  t.after(async () => { const { rm } = await import("node:fs/promises"); await rm(root, { recursive: true, force: true }); });
});

async function resetFixture(t, { explicit = false, stopPet = null } = {}) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-reset-")));
  const home = join(root, "home");
  let explicitData = join(root, "explicit-data");
  await mkdir(home);
  if (explicit) {
    await mkdir(explicitData);
    explicitData = await realpath(explicitData);
  }
  const environment = explicit ? { TASKBELAY_DATA_DIR: explicitData } : {};
  const paths = await resolveManagerPaths({ homeDirectory: home, environment, platform: process.platform, arch: process.arch });
  await mkdir(paths.configurationDirectory);
  await mkdir(paths.defaultDataDirectory, { recursive: true });
  await mkdir(paths.petDirectory, { recursive: true });
  await writeFile(paths.configurationPath, "old-config\n");
  await writeFile(join(paths.defaultDataDirectory, "taskbelay.db"), "old-task\n");
  await writeFile(join(paths.petDirectory, "preferences.json"), "pet-preferences\n");
  if (explicit) await writeFile(join(explicitData, "taskbelay.db"), "explicit-task\n");
  const states = { codex: "ready", deepseek: "ready", claude: "ready", zcode: "absent" };
  const events = [];
  const codexDriver = driver("codex", null, states, events);
  const deepseekDriver = driver("deepseek", "web", states, events);
  deepseekDriver.knownProfiles = async () => ["web"];
  t.after(async () => { const { rm } = await import("node:fs/promises"); await rm(root, { recursive: true, force: true }); });
  return {
    paths,
    states,
    events,
    dependencies: {
      assertNoPendingRuns: async () => {},
      homeDirectory: home,
      environment,
      platform: process.platform,
      arch: process.arch,
      codexDriver,
      deepseekDriver,
      claudeDriver: driver("claude", null, states, events),
      zcodeDriver: driver("zcode", null, states, events),
      stopPetForCore: stopPet ?? (async (options) => {
        events.push(`pet.stop:${options.corePath}`);
        return { stopped: true, reason: null };
      }),
    },
  };
}

function driver(host, profile, states, events) {
  return {
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    knownProfiles: async () => [], resolveTargetVersion: async () => "0.8.0",
    observe: async () => ({ host, profile, hostAvailable: true, hostVersion: "1.0.0", state: states[host], packageVersion: states[host] !== "absent" ? "0.8.0" : null, localReady: states[host] === "action_required", coreVersion: null, receipt: states[host] !== "absent" ? {} : null }),
    execute: async (operation) => {
      events.push(`${host}.${operation}`);
      states[host] = operation === "uninstall" ? "absent" : host === "zcode" ? "action_required" : "ready";
      return { changed: true, completedSteps: [`${host}.${operation}`] };
    },
  };
}

function request(operation = "delete-all") {
  return { operation, hosts: ["codex", "deepseek", "claude", "zcode"], profiles: ["web"], targetVersion: "latest", allKnownProfiles: true, adopt: false, permanent: false, yes: true, confirmationToken: "test", permanentToken: null, downgradeToken: null, confirmedExplicitData: [], outputMode: "json" };
}

test("reset without explicit data approval leaves both Adapters installed", async t => {
  const fixture = await resetFixture(t, { explicit: true });
  await assert.rejects(runLifecycle(request(), { ...fixture.dependencies, confirmPlan: async () => true }), /confirm-explicit-data/u);
  assert.deepEqual(fixture.events, []);
  assert.equal(fixture.states.codex, 'ready');
  assert.equal(fixture.states.deepseek, 'ready');
});

test("repeating a completed reset is a zero-write no-op", async t => {
  const fixture = await resetFixture(t);
  await runLifecycle(request(), { ...fixture.dependencies, confirmPlan: async () => true });
  fixture.events.length = 0;
  const result = await runLifecycle(request(), fixture.dependencies);
  assert.equal(result.code, 0);
  assert.equal(result.result.changed, false);
  assert.equal(result.plan.confirmationClass, 'none');
  assert.deepEqual(fixture.events, []);
});

test("reset permits owned runtime records to disappear while stopping services", async t => {
  const fixture = await resetFixture(t);
  const runtime = join(fixture.paths.petDirectory, "runtime.json");
  await writeFile(runtime, '{}\n');
  const result = await runLifecycle(request(), {
    ...fixture.dependencies, confirmPlan: async () => true,
    stopPetForCore: async () => { const { unlink } = await import('node:fs/promises'); await unlink(runtime); },
  });
  assert.equal(result.code, 0);
  assert.equal(result.result.data.pet, "absent");
  assert.equal(await readFile(join(result.result.data.trash_root, 'pet', 'preferences.json'), 'utf8'), 'pet-preferences\n');
});

test("reset without Codex stops Core before uninstall and verifies again before cleanup", async t => {
  const fixture = await resetFixture(t);
  fixture.states.codex = "absent";
  const result = await runLifecycle(request(), {
    ...fixture.dependencies, confirmPlan: async () => true,
    stopManagedCores: async ({ targets, dataDirectories }) => {
      assert.deepEqual(targets.map(target => target.host), ["deepseek", "claude"]);
      assert.deepEqual(dataDirectories, [fixture.paths.defaultDataDirectory]);
      fixture.events.push("core.stop");
      return { completeCleanup: async () => {}, verifyStopped: async () => {
        assert.equal(fixture.states.deepseek, "absent");
        assert.equal(fixture.states.claude, "absent");
        assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "old-task\n");
        fixture.events.push("core.verify");
      } };
    },
  });
  assert.equal(result.code, 0);
  assert.deepEqual(fixture.events, ["pet.stop:null", "core.stop", "deepseek.uninstall", "claude.uninstall", "core.verify"]);
});

test("reset preserves shared data after Core stop failure or post-uninstall reconnection", async t => {
  for (const phase of ["stop", "verify"]) {
    const fixture = await resetFixture(t);
    await assert.rejects(runLifecycle(request(), {
      ...fixture.dependencies, confirmPlan: async () => true,
      stopManagedCores: async () => {
        if (phase === "stop") throw new Error("Core shutdown failed");
        return { completeCleanup: async () => {}, verifyStopped: async () => { throw new Error("Core reconnected"); } };
      },
    }), /Core shutdown failed|Core reconnected/);
    assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "old-task\n");
    assert.equal(await readFile(fixture.paths.configurationPath, "utf8"), "old-config\n");
    if (phase === "stop") assert.equal(fixture.states.claude, "ready");
  }
});

test("reset merges identical explicit and default data while preserving explicit approval", async t => {
  for (const permanent of [false, true]) {
    const fixture = await resetFixture(t);
    const dependencies = { ...fixture.dependencies, environment: { TASKBELAY_DATA_DIR: fixture.paths.defaultDataDirectory } };
    const command = { ...request(), permanent };
    const preview = await runLifecycle(command, { ...dependencies, confirmPlan: async () => false });
    assert.equal(preview.plan.cleanupTargets.filter(target => target.path === fixture.paths.defaultDataDirectory).length, 1);
    assert.match(preview.result.next_step, /--confirm-explicit-data/);
    await assert.rejects(runLifecycle(command, { ...dependencies, confirmPlan: async () => true }), /confirm-explicit-data/);
    assert.deepEqual(fixture.events, []);
    const result = await runLifecycle({ ...command, confirmedExplicitData: [fixture.paths.defaultDataDirectory] }, {
      ...dependencies, confirmPlan: async () => true,
    });
    assert.equal(result.code, 0);
    assert.equal(result.result.completed_actions.filter(value => /manager\.(trash|remove)\.default-data/.test(value)).length, 1);
    await assert.rejects(stat(fixture.paths.defaultDataDirectory), { code: "ENOENT" });
  }
});

test("independent reset retries retain a reconnected Core after Adapter records are removed", async t => {
  const fixture = await resetFixture(t);
  fixture.states.codex = "absent";
  fixture.states.deepseek = "absent";
  const packageRoot = join(fixture.paths.homeDirectory, "isolated-claude");
  const runtimePath = join(packageRoot, "runtime", fixture.paths.runtimeDirectory, fixture.paths.runtimeExecutable);
  await mkdir(join(packageRoot, "runtime", fixture.paths.runtimeDirectory), { recursive: true });
  await writeFile(join(packageRoot, "package.json"), '{"name":"taskbelay-claude"}');
  await writeFile(runtimePath, "isolated fixture, never executed");
  await chmod(runtimePath, 0o755);
  let alive = true, signalCount = 0;
  const original = fixture.dependencies.claudeDriver;
  const dependencies = { ...fixture.dependencies, confirmPlan: async () => true,
    claudeDriver: { ...original,
      maintenanceTargets: async () => ({ registeredCorePaths: [runtimePath], installedRuntime: {
        packageName: "taskbelay-claude", packageRoot, runtimePath,
      } }),
      execute: async (operation, options) => {
        const result = await original.execute(operation, options);
        // A reconnect started before unlink can keep running after all installation records vanish.
        alive = true;
        await rm(packageRoot, { recursive: true });
        return result;
      },
    },
    loadMaintenancePlatform: async () => ({
      stopStdioCores: async paths => { if (paths.length) { signalCount++; alive = false; } },
      assertManagedCoresStopped: async paths => { if (alive && paths.includes(runtimePath)) throw new Error("Core reconnected"); },
    }),
  };
  const command = request();
  await assert.rejects(runLifecycle(command, dependencies), /Core reconnected/);
  assert.equal(fixture.states.claude, "absent");
  await assert.rejects(stat(packageRoot), { code: "ENOENT" });
  const recordPath = join(fixture.paths.managerRoot, "reset-core-maintenance.json");
  assert.deepEqual(JSON.parse(await readFile(recordPath, "utf8")), { runtime_paths: [runtimePath] });
  assert.equal(signalCount, 1);

  // A fresh call observes no installed Adapter and must still consult the private reset record.
  await assert.rejects(runLifecycle(command, dependencies), /Core reconnected/);
  assert.equal(signalCount, 1);
  assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "old-task\n");
  assert.ok(await readFile(recordPath));

  alive = false;
  const completed = await runLifecycle(command, dependencies);
  assert.equal(completed.code, 0);
  assert.equal(signalCount, 1);
  await assert.rejects(stat(recordPath), { code: "ENOENT" });
  await assert.rejects(stat(fixture.paths.defaultDataDirectory), { code: "ENOENT" });
});

test("data-only cleanup keeps registration, profile ownership and desktop executable and preserves the result", async t => {
  const fixture = await resetFixture(t, { explicit: true });
  const app = join(fixture.paths.petDirectory, "TaskBelayPet.app");
  await mkdir(app); await writeFile(join(app, "retained-program"), "application bytes");
  await mkdir(join(fixture.paths.productRoot, "registrations"));
  const registration = join(fixture.paths.productRoot, "registrations", "codex.json");
  await writeFile(registration, "registration bytes");
  const { createRun, recordRun, readRun } = await import("../lib/journal.mjs");
  const old = await createRun(fixture.paths, { planId: "previous", digest: "previous" }, { operationId: "completed-maintenance" });
  await recordRun(fixture.paths, old, { next_step: "complete" });
  const pending = await createRun(fixture.paths, { planId: "pending", digest: "pending" }, { operationId: "unresolved-maintenance" });
  const result = await runLifecycle({ ...request("delete-data"), confirmedExplicitData: [fixture.paths.explicitDataDirectory] }, { ...fixture.dependencies, confirmPlan: async () => true });
  assert.equal(result.code, 0);
  assert.deepEqual(fixture.events, ["pet.stop:null"]);
  assert.equal(await readFile(registration, "utf8"), "registration bytes");
  assert.equal(await readFile(join(app, "retained-program"), "utf8"), "application bytes");
  await assert.rejects(readFile(join(fixture.paths.petDirectory, "preferences.json")), { code: "ENOENT" });
  await assert.rejects(readFile(join(fixture.paths.explicitDataDirectory, "taskbelay.db")), { code: "ENOENT" });
  assert.ok((await stat(fixture.paths.explicitDataDirectory)).isDirectory());
  assert.deepEqual(await readRun(fixture.paths, pending.operation_id), pending);
  assert.equal(await readRun(fixture.paths, old.operation_id), null);
  assert.equal((await readRun(fixture.paths, result.result.operation_id)).next_step, "complete");
  assert.deepEqual(fixture.states, { codex: "ready", deepseek: "ready", claude: "ready", zcode: "absent" });
});

test("data-only cleanup refuses overlapping installation roots before changing files", async t => {
  const fixture = await resetFixture(t);
  await assert.rejects(runLifecycle({ ...request("delete-data"), confirmedExplicitData: [fixture.paths.configurationDirectory] }, {
    ...fixture.dependencies, environment: { TASKBELAY_DATA_DIR: fixture.paths.configurationDirectory }, confirmPlan: async () => true,
  }), /retained installation resource/);
  assert.deepEqual(fixture.events, []);
  assert.equal(await readFile(fixture.paths.configurationPath, "utf8"), "old-config\n");
});

test("unresolved execution inspection refuses data removal while plugin ownership remains intact", async t => {
  const fixture = await resetFixture(t);
  await assert.rejects(runLifecycle(request("delete-data"), {
    ...fixture.dependencies, confirmPlan: async () => true,
    assertNoPendingRuns: async () => { throw new Error("Task has unresolved executions"); },
  }), /unresolved executions/);
  assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "old-task\n");
  assert.equal(fixture.states.codex, "ready");
});

test("data deletion rejects a Git workspace used as the explicit data directory", async t => {
  const fixture = await resetFixture(t, { explicit: true });
  await mkdir(join(fixture.paths.explicitDataDirectory, ".git"));
  await writeFile(join(fixture.paths.explicitDataDirectory, "source.go"), "project code");
  await assert.rejects(runLifecycle({ ...request("delete-data"), confirmedExplicitData: [fixture.paths.explicitDataDirectory] }, {
    ...fixture.dependencies, confirmPlan: async () => true,
  }), /Git workspace/);
  assert.equal(await readFile(join(fixture.paths.explicitDataDirectory, "source.go"), "utf8"), "project code");
  assert.deepEqual(fixture.events, []);
});
