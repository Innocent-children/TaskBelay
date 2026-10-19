import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { runLifecycle } from "../lib/lifecycle.mjs";
import { resolveManagerPaths } from "../lib/ownership.mjs";

const installStep = "In ZCode Settings > Plugins, install and enable the prepared local plugin.";
const removeStep = "Remove the ZCode plugin and marketplace, then confirm Host removal.";

test("all-Host installation reports pending ZCode UI steps without reinstalling prepared bytes", async t => {
  const fixture = await lifecycleFixture(t);
  const first = await runLifecycle(request("install", "all"), fixture.dependencies);
  assert.equal(first.code, 0);
  assert.equal(first.result.status, "action_required");
  assert.equal(first.result.failed_action, null);
  assert.equal(first.result.changed, true);
  assert.deepEqual(first.result.targets.map(target => [target.host, target.state]), [
    ["codex", "ready"], ["deepseek", "ready"], ["claude", "ready"], ["zcode", "action_required"],
  ]);
  assert.ok(first.result.next_steps.includes(installStep));
  assert.deepEqual(fixture.operations, ["install"]);
  for (const operation of ["install", "repair"]) {
    const repeated = await runLifecycle(request(operation, "all"), fixture.dependencies);
    assert.equal(repeated.code, 0);
    assert.equal(repeated.result.status, "action_required");
    assert.equal(repeated.result.changed, false);
    assert.deepEqual(repeated.plan.actions, []);
    assert.ok(repeated.result.next_steps.includes(installStep));
  }
  assert.deepEqual(fixture.operations, ["install"]);
  assert.equal(fixture.versionLookups, 1);
});

test("pending UI status does not conceal broken preparation or another Host failure", async t => {
  const fixture = await lifecycleFixture(t, "prepared");
  fixture.state.localReady = false;
  const repaired = await runLifecycle(request("repair"), fixture.dependencies);
  assert.equal(repaired.code, 0);
  assert.equal(repaired.result.changed, true);
  assert.deepEqual(fixture.operations, ["repair"]);

  const partial = await runLifecycle(request("status", "all"), {
    ...fixture.dependencies,
    codexDriver: { observe: async () => ({ host: "codex", profile: null, state: "partial", packageVersion: "0.1.0" }) },
  });
  assert.equal(partial.result.status, "partial");
  assert.ok(partial.result.next_steps.includes(installStep));
  const pending = await runLifecycle(request("status"), fixture.dependencies);
  assert.equal(pending.code, 0);
  assert.equal(pending.result.status, "action_required");
  assert.equal(pending.result.changed, false);
});

test("ZCode uninstall retains package and data until Host removal is confirmed", async t => {
  const fixture = await lifecycleFixture(t, "prepared");
  const first = await runLifecycle(request("uninstall"), fixture.dependencies);
  assert.equal(first.code, 0);
  assert.equal(first.result.status, "action_required");
  assert.equal(first.result.targets[0].package_version, "0.1.0");
  assert.ok(first.result.next_steps.includes(removeStep));
  assert.equal(fixture.state.phase, "removal_required");
  const repeated = await runLifecycle(request("uninstall"), fixture.dependencies);
  assert.equal(repeated.code, 0);
  assert.equal(repeated.result.changed, false);
  assert.equal(repeated.result.status, "action_required");

  // The Adapter's separately authorized confirmation has removed the receipt.
  fixture.state.phase = "package_only";
  const finished = await runLifecycle(request("uninstall"), fixture.dependencies);
  assert.equal(finished.code, 0);
  assert.equal(finished.result.status, "absent");
  assert.deepEqual(finished.result.next_steps, []);
  assert.equal(await readFile(fixture.paths.configurationPath, "utf8"), "configuration bytes\n");
  assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "Task bytes\n");
});

test("ZCode reset preflight refusal preserves shared data before any Adapter mutation", async t => {
  for (const phase of ["prepared", "package_only"]) {
    const fixture = await lifecycleFixture(t, phase);
    await assert.rejects(runLifecycle({ ...request("delete-all", "all"), zcodeClosed: true }, fixture.dependencies), /ZCode cache shutdown is unverified/);
    assert.deepEqual(fixture.operations, []);
    assert.equal(await readFile(fixture.paths.configurationPath, "utf8"), "configuration bytes\n");
    assert.equal(await readFile(join(fixture.paths.defaultDataDirectory, "taskbelay.db"), "utf8"), "Task bytes\n");
  }
});

async function lifecycleFixture(t, phase = "absent") {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-zcode-manager-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, "home"), packageRoot = join(root, "manager");
  await mkdir(home); await mkdir(packageRoot);
  await writeFile(join(packageRoot, "package.json"), '{"name":"taskbelay","version":"0.1.0"}\n');
  const paths = await resolveManagerPaths({ homeDirectory: home, environment: {}, platform: process.platform, arch: process.arch });
  await mkdir(paths.configurationDirectory, { recursive: true });
  await mkdir(paths.defaultDataDirectory, { recursive: true });
  await writeFile(paths.configurationPath, "configuration bytes\n");
  await writeFile(join(paths.defaultDataDirectory, "taskbelay.db"), "Task bytes\n");
  const state = { phase, localReady: phase === "prepared" };
  const operations = [];
  let versionLookups = 0;
  const zcodeDriver = {
    observe: async () => {
      const receipt = ["prepared", "removal_required"].includes(state.phase);
      const present = state.phase !== "absent";
      return { host: "zcode", profile: null, hostAvailable: null,
        state: receipt ? "action_required" : present ? "partial" : "absent",
        packageInstalled: present, packageVersion: present ? "0.1.0" : null,
        receipt, localReady: state.phase === "prepared" && state.localReady,
        nextSteps: receipt ? [state.phase === "prepared" ? installStep : removeStep] : [], issues: [] };
    },
    resolveTargetVersion: async () => { versionLookups++; return "0.1.0"; },
    maintenanceTargets: async ({ operation }) => {
      if (operation === "delete-all" && state.phase !== "absent") throw new Error("ZCode cache shutdown is unverified");
      return { registeredCorePaths: [], installedRuntime: null };
    },
    execute: async operation => {
      operations.push(operation);
      if (operation === "uninstall") {
        if (["prepared", "removal_required"].includes(state.phase)) {
          const changed = state.phase !== "removal_required";
          state.phase = "removal_required";
          return { changed, completedSteps: ["zcode.remove_registration"], nextSteps: [removeStep] };
        }
        state.phase = "absent";
      } else { state.phase = "prepared"; state.localReady = true; }
      return { changed: true, completedSteps: [`zcode.${operation}`] };
    },
  };
  const readyDriver = (host, profile = null) => ({
    observe: async () => ({ host, profile, hostAvailable: true, state: "ready", packageInstalled: true, packageVersion: "0.1.0", receipt: true }),
    knownProfiles: async () => ["web"],
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    execute: async () => { assert.fail(`must not mutate unrelated ${host} in this scenario`); },
  });
  return { paths, state, operations, get versionLookups() { return versionLookups; }, dependencies: {
    homeDirectory: home, environment: {}, packageRoot, platform: process.platform, arch: process.arch,
    codexDriver: readyDriver("codex"), deepseekDriver: readyDriver("deepseek", "web"), claudeDriver: readyDriver("claude"), zcodeDriver,
    confirmPlan: async () => true, stopPetForCore: async () => ({ stopped: true }),
  } };
}

function request(operation, host = "zcode") {
  return { operation, hosts: host === "all" ? ["codex", "deepseek", "claude", "zcode"] : [host], profiles: host === "all" ? ["web"] : [], targetVersion: null,
    allKnownProfiles: host === "all", adopt: false, permanent: false, yes: true,
    confirmationToken: null, permanentToken: null, downgradeToken: null, confirmedExplicitData: [], outputMode: "json" };
}
