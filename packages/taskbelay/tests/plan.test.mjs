import assert from "node:assert/strict";
import test from "node:test";

import { createLifecyclePlan } from "../lib/plan.mjs";

test("ready repeated install is an idempotent zero-action plan", () => {
  const plan = createLifecyclePlan(request("install", "codex"), observed({ codexState: "ready", codexVersion: "1.2.3" }), {
    targetVersions: { "codex:default": "1.2.3" },
    now: () => new Date("2026-08-25T00:00:00Z"),
  });
  assert.deepEqual(plan.actions, []);
  assert.equal(plan.confirmationClass, "none");
});

test("downgrade and reset use stable plan-bound confirmations", () => {
  const downgrade = createLifecyclePlan(request("upgrade", "codex"), observed({ codexState: "ready", codexVersion: "2.0.0" }), {
    targetVersions: { "codex:default": "1.9.0" },
  });
  assert.equal(downgrade.confirmationClass, "downgrade");
  assert.match(downgrade.downgradeToken, /^DOWNGRADE-/u);

  const resetRequest = { ...request("delete-all", "all"), allKnownProfiles: true };
  const first = createLifecyclePlan(resetRequest, observed({ codexState: "ready", deepseekState: "ready" }));
  const second = createLifecyclePlan(resetRequest, observed({ codexState: "ready", deepseekState: "ready" }));
  assert.equal(first.planId, second.planId);
  assert.equal(first.confirmationToken, second.confirmationToken);
  assert.equal(first.actions.at(-1).operation, "cleanup");
});

test("delete-all blocks a partial Host selection and unknown manager Profile", () => {
  assert.throws(() => createLifecyclePlan(request("delete-all", "codex"), observed({})), /requires all Agents/u);
  const reset = { ...request("delete-all", "all"), profiles: ["web"], allKnownProfiles: false };
  assert.throws(() => createLifecyclePlan(reset, observed({ known: ["web", "other"] })), /every known Profile/u);
});

test("uninstall and delete-all retain an installed Codex package after registration loss", () => {
  const current = observed({ codexState: "absent", codexVersion: "0.7.3", codexPackageInstalled: true });
  const uninstall = createLifecyclePlan(request("uninstall", "codex"), current);
  assert.deepEqual(uninstall.actions.map((action) => action.actionId), ["codex.default.uninstall"]);

  const reset = createLifecyclePlan({ ...request("delete-all", "all"), allKnownProfiles: true }, current);
  assert.deepEqual(reset.actions.map((action) => action.actionId), ["codex.default.uninstall", "manager.cleanup"]);
  assert.ok(reset.impacts.includes("Remove every installed Adapter before shared data cleanup"));
});

test("delete-all reports one exact no-op impact when Adapters and data are absent", () => {
  const plan = createLifecyclePlan({ ...request("delete-all", "all"), allKnownProfiles: true }, observed());
  assert.deepEqual(plan.actions, []);
  assert.equal(plan.confirmationClass, "none");
  assert.deepEqual(plan.impacts, ["No installed Adapter or active TaskBelay data was found"]);
});

test("Windows delete-all previews the product recovery directory instead of macOS Trash", () => {
  const current = observed();
  current.resources.defaultData = {
    label: "default-data",
    path: "C:\\Users\\ordinary\\AppData\\Local\\taskbelay\\data",
    exists: true,
    identity: "volume:file:directory:0:0",
  };
  const plan = createLifecyclePlan(
    { ...request("delete-all", "all"), allKnownProfiles: true },
    current,
    {
      platformKey: "win32-x64",
      recoverableCleanupDescription: "Move confirmed data to the TaskBelay recovery directory",
    },
  );
  assert.equal(plan.impacts.includes("Move confirmed data to the TaskBelay recovery directory"), true);
  assert.equal(plan.impacts.some((impact) => impact.includes("macOS Trash")), false);
});

test("delete-all reports the pet directory as a confirmed cleanup impact", () => {
  const current = observed();
  current.resources.pet = { label: "pet", path: "/tmp/pet", exists: true, identity: "volume:file:directory:0:0" };
  current.resources.petData = [current.resources.pet];
  const plan = createLifecyclePlan({ ...request("delete-all", "all"), allKnownProfiles: true }, current);
  assert.ok(plan.impacts.includes("Clear desktop pet records, preferences, and imported appearances"));
});

function request(operation, host) {
  return { operation, hosts: host === "all" ? ["codex", "deepseek", "claude", "zcode"] : [host], profiles: host === "codex" ? [] : ["web"], targetVersion: "latest", allKnownProfiles: false, adopt: false, permanent: false };
}

function observed({ codexState = "absent", codexVersion = null, codexPackageInstalled = false, deepseekState = "absent", known = ["web"] } = {}) {
  return {
    codex: { host: "codex", profile: null, state: codexState, packageInstalled: codexPackageInstalled, packageVersion: codexVersion, receipt: codexState !== "absent" },
    deepseek: [{ host: "deepseek", profile: "web", state: deepseekState, packageVersion: deepseekState === "ready" ? "1.2.3" : null, receipt: deepseekState !== "absent" ? {} : null }],
    claude: { host: "claude", profile: null, state: "absent", packageInstalled: false, packageVersion: null, receipt: false },
    zcode: { host: "zcode", profile: null, state: "absent", packageInstalled: false, packageVersion: null, receipt: false },
    knownDeepSeekProfiles: known,
    resources: {
      configuration: { label: "configuration", path: "/tmp/config", exists: false, identity: null },
      defaultData: { label: "default-data", path: "/tmp/data", exists: false, identity: null },
      pet: { label: "pet", path: "/tmp/pet", exists: false, identity: null },
      explicitData: null,
    },
  };
}

test("every version replacement requires a separate downgrade confirmation", () => {
  for (const operation of ['install', 'upgrade', 'repair', 'reinstall']) {
    const plan = createLifecyclePlan(request(operation, 'codex'), observed({ codexState: 'ready', codexVersion: '2.0.0' }), {
      targetVersions: { 'codex:default': '1.0.0' },
    });
    assert.equal(plan.confirmationClass, 'downgrade', operation);
  }
});

test("reset plan shares one cleanup target for the same observed canonical directory", () => {
  const current = observed();
  current.resources.defaultData = { label: "default-data", path: "/same/data", exists: true, kind: "directory", identity: "same" };
  current.resources.explicitData = { ...current.resources.defaultData, label: "explicit-data" };
  const plan = createLifecyclePlan({ ...request("delete-all", "all"), allKnownProfiles: true }, current);
  assert.equal(plan.cleanupTargets.length, 1);
  assert.equal(plan.cleanupTargets[0].requiresExplicitConfirmation, true);
  assert.deepEqual(plan.resources, [{ label: "default-data", path: "/same/data" }]);
  current.resources.explicitData.identity = "changed";
  assert.throws(() => createLifecyclePlan({ ...request("delete-all", "all"), allKnownProfiles: true }, current), /changed during observation/);
});
