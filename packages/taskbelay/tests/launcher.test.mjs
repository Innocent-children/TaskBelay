import { createHash } from "node:crypto";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, readFile, realpath, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import test from "node:test";
import { createLauncherDriver } from "../lib/launcher.mjs";
import { inspectUpgradeOptions, runLifecycle } from "../lib/lifecycle.mjs";
import { parseArguments } from "../lib/cli.mjs";

async function launcherFixture(t, local = false) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-self-update-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const packageRoot = join(root, "taskbelay"); await mkdir(packageRoot);
  const manifest = { name: "taskbelay", version: "1.0.0", ...(local ? { taskBelayLocalPackages: {} } : {}) };
  await writeFile(join(packageRoot, "package.json"), JSON.stringify(manifest));
  const calls = [];
  const run = async (exe, args) => {
    calls.push([exe, ...args]);
    if (args[0] === "root") return { stdout: root };
    if (args[0] === "view") return { stdout: '"1.1.0"' };
    if (args[0] === "install") { await writeFile(join(packageRoot, "package.json"), JSON.stringify({ ...manifest, version: "1.1.0" })); return { stdout: "" }; }
    throw new Error("unexpected command");
  };
  return { root, packageRoot, calls, driver: createLauncherDriver({ packageRoot, environment: {}, run }), run };
}

test("npm self update pins the resolved version and verifies installed bytes", async t => {
  const f = await launcherFixture(t);
  const next = await f.driver.resolveTarget();
  assert.equal(next.artifact, "taskbelay@1.1.0");
  assert.deepEqual(await f.driver.execute(next), { changed: true, completedSteps: ["taskbelay.upgrade"], nextSteps: ["Run taskbelay again to use the updated launcher."] });
  assert.ok(f.calls.some(args => args.join(" ") === "npm install --global --ignore-scripts --no-audit --no-fund taskbelay@1.1.0"));
  assert.equal(JSON.parse(await readFile(join(f.packageRoot, "package.json"), "utf8")).version, "1.1.0");
});

test("local launcher never silently switches to npm and rejects a changed distribution", async t => {
  const f = await launcherFixture(t, true);
  await assert.rejects(f.driver.resolveTarget(), /--self-package/);
  assert.equal(f.calls.some(args => args[1] === "view" || args[1] === "install"), false);
  const artifact = join(f.root, "distribution.tgz"); await writeFile(artifact, "original artifact");
  const nextManifest = { name: "taskbelay", version: "1.1.0", taskBelayLocalPackages: Object.fromEntries(["codex", "deepseek", "claude", "zcode"].map(key => [key, { path: `local-packages/${key}.tgz`, version: "1.1.0", sha256: createHash("sha256").update("artifact").digest("hex") }])) };
  const driver = createLauncherDriver({ packageRoot: f.packageRoot, run: async (exe, args) => exe === "tar" ? { stdout: args.at(-1) === "package/package.json" ? JSON.stringify(nextManifest) : Buffer.from("artifact") } : f.run(exe, args) });
  const next = await driver.resolveTarget({ selfPackage: artifact });
  await writeFile(artifact, "changed artifact");
  await assert.rejects(driver.execute(next), /changed after confirmation/);
  assert.equal(f.calls.some(args => args[1] === "install"), false);
});

test("batch upgrade confirms once, skips absent Agents and replaces launcher last", async t => {
  const f = await launcherFixture(t), events = []; let confirmations = 0;
  const states = { codex: "1.0.0", claude: "1.0.0", zcode: null };
  const driver = host => ({
    observe: async () => ({ host, profile: null, state: states[host] ? "ready" : "absent", packageVersion: states[host], packageInstalled: Boolean(states[host]), hostAvailable: Boolean(states[host]) }),
    resolveTargetVersion: async () => { assert.notEqual(host, "zcode"); return "1.1.0"; },
    maintenanceTargets: async () => ({ registeredCorePaths: [], installedRuntime: null }),
    execute: async () => { events.push(host); states[host] = "1.1.0"; return { changed: true }; },
  });
  const launcher = { ...f.driver, execute: async target => { events.push("launcher"); return f.driver.execute(target); } };
  const dependencies = { homeDirectory: f.root, environment: {}, packageRoot: f.packageRoot, platform: "darwin", arch: "arm64",
    codexDriver: driver("codex"), claudeDriver: driver("claude"), zcodeDriver: driver("zcode"), launcherDriver: launcher,
    confirmPlan: async plan => { confirmations++; assert.equal(plan.actions.at(-1).owner, "taskbelay"); return true; } };
  const result = await runLifecycle(parseArguments(["upgrade", "--host", "codex", "--host", "claude", "--host", "zcode", "--self", "--json"]), dependencies);
  assert.equal(result.code, 0); assert.equal(confirmations, 1); assert.deepEqual(events, ["codex", "claude", "launcher"]);
  assert.equal(result.result.launcher.packageVersion, "1.1.0");
});

test("upgrade lookup failure remains unavailable while other updates are selectable", async t => {
  const f = await launcherFixture(t);
  const driver = (host, installed = true) => ({ knownProfiles: async () => [], observe: async profile => ({ host, profile, state: installed ? "ready" : "absent", packageVersion: installed ? "1.0.0" : null, hostAvailable: true }), resolveTargetVersion: async () => { if (host === "codex") throw new Error("registry offline"); return "1.1.0"; } });
  const options = await inspectUpgradeOptions({}, { homeDirectory: f.root, packageRoot: f.packageRoot, environment: { LANG: "zh_CN" }, platform: "darwin", arch: "arm64", launcherDriver: f.driver,
    codexDriver: driver("codex"), claudeDriver: driver("claude"), zcodeDriver: driver("zcode", false), deepseekDriver: driver("deepseek", false) });
  assert.equal(options.find(option => option.value === "codex").disabled, true);
  assert.match(options.find(option => option.value === "codex").hint, /检查失败/);
  assert.equal(options.find(option => option.value === "claude").disabled, false);
  assert.equal(options.some(option => option.value === "zcode"), false);
});

test("interrupting batch maintenance retains completed steps and never starts later components", async t => {
  const { EventEmitter } = await import("node:events");
  const { runMain } = await import("../lib/lifecycle.mjs");
  const f = await launcherFixture(t), signalSource = new EventEmitter(), calls = [];
  let text = "", diagnostics = "";
  const drivers = host => ({ observe: async () => ({ host, profile: null, state: "ready", packageVersion: "1.0.0", packageInstalled: true, hostAvailable: true }), resolveTargetVersion: async () => "1.1.0",
    maintenanceTargets: async () => ({ installedRuntime: null, registeredCorePaths: [] }),
    execute: async () => { calls.push(host); signalSource.emit("SIGINT"); return { changed: true, completedSteps: [host + ".install_package"] }; } });
  const result = await runMain(["upgrade", "--host", "codex", "--host", "claude", "--yes", "--json"], {
    homeDirectory: f.root, environment: {}, packageRoot: f.packageRoot, platform: "darwin", arch: "arm64", signalSource,
    codexDriver: drivers("codex"), claudeDriver: drivers("claude"), output: { write: value => { text += value; } }, errorOutput: { write: value => { diagnostics += value; } }, isTTY: false,
  });
  assert.equal(result.code, 130); assert.deepEqual(calls, ["codex"]);
  const response = JSON.parse(text);
  assert.equal(response.changed, null); assert.equal(response.status, "partial");
  assert.ok(response.completed_actions.includes("codex.install_package"));
  assert.equal(response.failed_action, "claude.default.upgrade");
  assert.match(response.next_step, /doctor/); assert.match(diagnostics, /Stopping/);
  assert.equal(signalSource.listenerCount("SIGINT"), 0);
});

test("a failed npm self replacement does not claim that installation files are unchanged", async t => {
  const f = await launcherFixture(t);
  const driver = createLauncherDriver({ packageRoot: f.packageRoot, run: async (exe, args) => {
    if (args[0] === "install") throw new Error("npm stopped during replacement");
    return f.run(exe, args);
  } });
  const target = await driver.resolveTarget();
  await assert.rejects(driver.execute(target), error => error.uncertain === true && error.nextStep === "taskbelay version");
});
