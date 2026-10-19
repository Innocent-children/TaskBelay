import assert from "node:assert/strict";
import { mkdir, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { runLifecycle } from "../lib/lifecycle.mjs";

test("all-Host install prepares ZCode UI steps and repeated install is zero-change", async (t) => {
  const fixture = await lifecycleFixture(t);
  const request = makeRequest("install");
  const progress = [];
  const first = await runLifecycle(request, {
    ...fixture.dependencies,
    confirmPlan: async () => true,
    onProgress: (event) => progress.push(event),
  });
  assert.equal(first.result.status, "action_required");
  assert.equal(first.code, 0);
  assert.equal(first.result.changed, true);
  assert.deepEqual(progress.filter(event => event.action).map((event) => `${event.type}:${event.action.actionId}`), [
    "action_start:codex.default.install",
    "action_complete:codex.default.install",
    "action_start:deepseek.web.install",
    "action_complete:deepseek.web.install",
    "action_start:claude.default.install",
    "action_complete:claude.default.install",
    "action_start:zcode.default.install",
    "action_complete:zcode.default.install",
  ]);
  const second = await runLifecycle(request, { ...fixture.dependencies, confirmPlan: async () => true });
  assert.equal(second.result.changed, false);
  assert.deepEqual(second.plan.actions, []);
});

async function lifecycleFixture(t) {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-install-"));
  const home = join(root, "home");
  await mkdir(home);
  const states = { codex: "absent", deepseek: "absent", claude: "absent", zcode: "absent" };
  const codexDriver = fakeDriver("codex", states, null);
  const deepseekDriver = fakeDriver("deepseek", states, "web");
  deepseekDriver.knownProfiles = async () => ["web"];
  t.after(async () => { const { rm } = await import("node:fs/promises"); await rm(root, { recursive: true, force: true }); });
  return { dependencies: { homeDirectory: home, environment: {}, platform: "darwin", arch: "arm64", codexDriver, deepseekDriver, claudeDriver: fakeDriver("claude", states, null), zcodeDriver: fakeDriver("zcode", states, null) } };
}

function fakeDriver(host, states, profile) {
  return {
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    knownProfiles: async () => [],
    resolveTargetVersion: async () => "0.8.0",
    observe: async () => ({ host, profile, hostAvailable: true, hostVersion: "1.0.0", state: states[host], packageVersion: states[host] !== "absent" ? "0.8.0" : null, localReady: states[host] === "action_required", coreVersion: host === "codex" && states[host] === "ready" ? "0.6.0" : null, receipt: states[host] !== "absent" ? {} : null }),
    execute: async (operation) => { states[host] = operation === "uninstall" ? "absent" : host === "zcode" ? "action_required" : "ready"; return { changed: true, completedSteps: [`${host}.${operation}`] }; },
  };
}

function makeRequest(operation) {
  return { operation, hosts: ["codex", "deepseek", "claude", "zcode"], profiles: ["web"], targetVersion: "latest", allKnownProfiles: true, adopt: false, permanent: false, yes: true, confirmationToken: null, permanentToken: null, downgradeToken: null, confirmedExplicitData: [], outputMode: "json" };
}
