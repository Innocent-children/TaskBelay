import test from "node:test";
import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createClaudeDriver } from "../lib/hosts/claude.mjs";

function installFixture({ setupOutput, statusOutput } = {}) {
  const calls = [], progress = [], started = [];
  const ready = JSON.stringify({ status: "ready", package_version: "0.1.0", registration: { receipt: true } });
  const driver = createClaudeDriver({ run: async (executable, args) => {
    calls.push([executable, ...args]);
    if (executable === "npm") return { stdout: "" };
    assert.equal(executable, "taskbelay-claude");
    return { stdout: args[0] === "setup" ? setupOutput ?? ready : statusOutput ?? ready };
  } });
  const install = () => driver.execute("install", {
    targetVersion: "0.1.0", observed: { hostAvailable: true, state: "absent" },
    onProgress: step => progress.push(step), onStepStart: step => started.push(step),
  });
  return { install, calls, progress, started };
}

test("Claude confirms setup before reporting registration complete and reading status", async () => {
  const f = installFixture();
  const result = await f.install();
  assert.equal(result.changed, true);
  assert.deepEqual(result.completedSteps, ["claude.install_package", "claude.setup_registration"]);
  assert.deepEqual(f.progress, result.completedSteps);
  assert.deepEqual(f.calls, [
    ["npm", "install", "--global", "taskbelay-claude@0.1.0"],
    ["taskbelay-claude", "setup", "--json"], ["taskbelay-claude", "status", "--json"],
  ]);
});

test("Claude rejects unsuccessful setup output without losing the installed package change", async t => {
  for (const setupOutput of ["", "{", "null", '{"status":"partial","package_version":"0.1.0"}', '{"status":"ready","package_version":"0.0.9"}']) {
    await t.test(JSON.stringify(setupOutput), async () => {
      const f = installFixture({ setupOutput });
      await assert.rejects(f.install, error => {
        assert.match(error.message, /taskbelay-claude setup --json/);
        assert.equal(error.changed, true);
        assert.deepEqual(error.completedSteps, ["claude.install_package"]);
        assert.equal(error.nextStep, "taskbelay repair --host claude --yes");
        return true;
      });
      assert.deepEqual(f.started, ["claude.install_package", "claude.setup_registration"]);
      assert.deepEqual(f.progress, ["claude.install_package"]);
      assert.equal(f.calls.some(call => call[1] === "status"), false);
    });
  }
});

test("Claude names an empty status response after a verified setup", async () => {
  const f = installFixture({ statusOutput: "" });
  await assert.rejects(f.install, error => {
    assert.equal(error.message, "taskbelay-claude status --json returned empty JSON output");
    assert.equal(error.changed, true);
    assert.deepEqual(error.completedSteps, ["claude.install_package", "claude.setup_registration"]);
    return true;
  });
});

async function orphanFixture(t, foreign = false) {
  const root = await mkdtemp(join(tmpdir(), "claude-orphan-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { productRoot: join(root, "taskbelay"), homeDirectory: root };
  const receiptPath = join(paths.productRoot, "registrations", "claude.json");
  await mkdir(join(paths.productRoot, "registrations"), { recursive: true });
  const packageRoot = join(root, "removed-package");
  await writeFile(receiptPath, JSON.stringify({ product: { name: "taskbelay-claude" }, paths: { package_root: packageRoot, receipt_path: receiptPath, config_root: join(root, ".claude") } }));
  const effects = [];
  const run = async (exe, args) => {
    if (exe === "taskbelay-claude") throw new Error("package missing");
    if (exe === "npm") return { stdout: '{"dependencies":{}}' };
    if (args[0] === "--version") return { stdout: "2.1.270 (Claude Code)" };
    if (args[1] === "list") return { stdout: JSON.stringify([{ id: "taskbelay-claude@taskbelay-claude-local", scope: "user" }]) };
    if (args[1] === "marketplace" && args[2] === "list") return { stdout: JSON.stringify([{ name: "taskbelay-claude-local", source: "directory", path: foreign ? join(root, "foreign") : packageRoot }]) };
    effects.push([exe, ...args]); return { stdout: '{"outcome":"ok"}' };
  };
  return { driver: createClaudeDriver({ paths, environment: {}, run }), receiptPath, effects };
}
test("Claude orphan removal verifies ownership and does not need the missing package executable", async t => {
  const f = await orphanFixture(t);
  const observed = await f.driver.observe();
  assert.equal(observed.orphanedRegistration, true); assert.equal(observed.state, "partial");
  await f.driver.execute("uninstall", { observed });
  await assert.rejects(readFile(f.receiptPath), { code: "ENOENT" });
  assert.equal(f.effects.length, 2);
  assert.ok(f.effects.every(args => args[0] === "claude"));
});
test("Claude orphan removal refuses a replacement marketplace without mutation", async t => {
  const f = await orphanFixture(t, true);
  await assert.rejects(f.driver.execute("uninstall", { observed: await f.driver.observe() }), /ownership changed/);
  assert.equal(f.effects.length, 0); assert.ok(await readFile(f.receiptPath));
});

test("Claude reset includes the owned user plugin cache Core before uninstall", async t => {
  const root = await mkdtemp(join(tmpdir(), "claude-reset-cache-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { productRoot: join(root, "taskbelay"), homeDirectory: root, runtimeDirectory: "darwin-arm64", runtimeExecutable: "taskbelay" };
  const npmRoot = join(root, "npm"), packageRoot = join(npmRoot, "taskbelay-claude");
  const cachedRoot = join(root, "claude-cache", "taskbelay-claude");
  const runtimePath = join(packageRoot, "runtime", "darwin-arm64", "taskbelay");
  await mkdir(join(packageRoot, "runtime", "darwin-arm64"), { recursive: true });
  await writeFile(runtimePath, "fixture");
  await mkdir(join(paths.productRoot, "registrations"), { recursive: true });
  await writeFile(join(paths.productRoot, "registrations", "claude.json"), JSON.stringify({ paths: { package_root: packageRoot, runtime_path: runtimePath } }));
  let marketplacePath = packageRoot, scope = "user", installPath = cachedRoot;
  const calls = [];
  const run = async (exe, args) => {
    calls.push([exe, ...args]);
    if (exe === "npm") { assert.deepEqual(args, ["root", "--global"]); return { stdout: npmRoot }; }
    assert.equal(exe, "claude");
    if (args[1] === "marketplace") return { stdout: JSON.stringify([{ name: "taskbelay-claude-local", source: "directory", path: marketplacePath }]) };
    assert.deepEqual(args, ["plugin", "list", "--json"]);
    return { stdout: JSON.stringify([{ id: "taskbelay-claude@taskbelay-claude-local", scope, installPath }]) };
  };
  const driver = createClaudeDriver({ paths, environment: {}, run });
  const observed = { packageInstalled: true, receipt: true };
  const replacement = await driver.maintenanceTargets({ observed, operation: "upgrade" });
  assert.deepEqual(replacement.registeredCorePaths, [runtimePath]);
  assert.equal(calls.length, 1);
  const reset = await driver.maintenanceTargets({ observed, operation: "delete-all" });
  assert.deepEqual(reset.registeredCorePaths, [runtimePath, join(cachedRoot, "runtime", "darwin-arm64", "taskbelay")]);
  assert.equal(reset.installedRuntime.runtimePath, runtimePath);
  marketplacePath = join(root, "foreign-source");
  await assert.rejects(driver.maintenanceTargets({ observed, operation: "delete-all" }), /ownership changed/);
  marketplacePath = packageRoot;
  scope = "project";
  await assert.rejects(driver.maintenanceTargets({ observed, operation: "delete-all" }), /Other plugin scopes/);
  scope = "user"; installPath = "relative-cache";
  await assert.rejects(driver.maintenanceTargets({ observed, operation: "delete-all" }), /path is unavailable/);
});
