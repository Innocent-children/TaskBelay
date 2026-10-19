import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, unlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { createZCodeDriver } from "../lib/hosts/zcode.mjs";

async function fixture(t, { setupOutput, statusOutput } = {}) {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-zcode-driver-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { productRoot: join(root, "taskbelay"), runtimeKey: "win32-x64", runtimeDirectory: "win32-x64", runtimeExecutable: "taskbelay.exe" };
  const packageRoot = join(root, "npm", "taskbelay-zcode"), receiptPath = join(paths.productRoot, "registrations", "zcode.json");
  await mkdir(join(paths.productRoot, "registrations"), { recursive: true });
  let installed = false, receipt = null, damaged = false;
  const calls = [];
  const report = () => ({ status: receipt && !damaged ? "action_required" : "partial", package_version: "0.1.0", core_version: "0.18.0", issues: damaged ? ["Package payload differs from preparation"] : [],
    registration: { receipt: receipt !== null, phase: receipt?.phase ?? null }, next_steps: [receipt?.phase === "removal_required" ? "Remove the plugin in ZCode, then confirm Host removal." : "Install the prepared source in ZCode."] });
  const save = async phase => {
    receipt = { product: { name: "taskbelay-zcode", version: "0.1.0", core_version: "0.18.0" },
      host: { surface: "zcode-ui", os: "win32", arch: "x64" }, phase, package_digest: "a".repeat(64),
      paths: { package_root: packageRoot, runtime_path: join(packageRoot, "runtime/win32-x64/taskbelay.exe"),
        data_dir: join(root, "data"), receipt_path: receiptPath, marketplace_path: join(packageRoot, "marketplace.json") } };
    await writeFile(receiptPath, JSON.stringify(receipt));
  };
  const run = async (executable, args) => {
    calls.push([executable, ...args]);
    if (executable === "npm") {
      if (args[0] === "install") { installed = true; return { stdout: "" }; }
      if (args[0] === "uninstall") { installed = false; return { stdout: "" }; }
      if (args[0] === "root") return { stdout: join(root, "npm") };
      if (args[0] === "view") return { stdout: '"0.1.0"' };
      if (args[0] === "list") return { stdout: JSON.stringify({ dependencies: installed ? { "taskbelay-zcode": { version: "0.1.0" } } : {} }) };
    }
    assert.equal(executable, "taskbelay-zcode", "the driver must not invent a ZCode CLI");
    if (!installed) throw new Error("Adapter is absent");
    if (args[0] === "setup") { await save("prepared"); return { stdout: setupOutput ?? JSON.stringify({ ...report(), changed: true }) }; }
    if (args[0] === "remove") { await save("removal_required"); return { stdout: JSON.stringify({ ...report(), changed: true }) }; }
    assert.deepEqual(args, ["status", "--json"]);
    return { stdout: statusOutput ?? JSON.stringify(report()) };
  };
  const driver = createZCodeDriver({ paths, environment: {}, run, localPackage: { path: join(root, "local.tgz"), version: "0.1.0" } });
  return { driver, calls, receiptPath, paths, save, localPackagePath: join(root, "local.tgz"),
    damagePayload: () => { damaged = true; },
    confirmRemoval: async () => { await unlink(receiptPath); receipt = null; } };
}

test("ZCode prepares a local source and reports UI work without inventing Host availability", async t => {
  const f = await fixture(t);
  assert.equal((await f.driver.observe()).state, "absent");
  assert.equal(await f.driver.resolveTargetVersion("latest"), "0.1.0");
  const result = await f.driver.execute("install", { targetVersion: "0.1.0", observed: await f.driver.observe() });
  assert.equal(result.changed, true);
  const observed = await f.driver.observe();
  assert.equal(observed.state, "action_required");
  assert.equal(observed.hostAvailable, null);
  assert.equal(observed.localReady, true);
  assert.equal((await f.driver.runtimeCandidates())[0].source, "zcode");
  assert.deepEqual(f.calls.find(call => call[0] === "npm" && call[1] === "install"), ["npm", "install", "--global", f.localPackagePath]);
  assert.ok(result.nextSteps[0].includes("ZCode"));
});

test("ZCode rejects unsuccessful setup output without reporting local preparation complete", async t => {
  for (const setupOutput of ["", "{", "null", JSON.stringify({ status: "partial" }),
    JSON.stringify({ status: "action_required", package_version: "0.1.0", registration: { phase: "removal_required" }, next_steps: [] })]) {
    await t.test(JSON.stringify(setupOutput), async t => {
      const f = await fixture(t, { setupOutput });
      const progress = [], started = [];
      await assert.rejects(f.driver.execute("install", {
        targetVersion: "0.1.0", observed: {}, onProgress: step => progress.push(step), onStepStart: step => started.push(step),
      }), error => {
        assert.match(error.message, /taskbelay-zcode setup --json/);
        assert.equal(error.changed, true);
        assert.deepEqual(error.completedSteps, ["zcode.install_package"]);
        assert.equal(error.nextStep, "taskbelay repair --host zcode --yes");
        return true;
      });
      assert.deepEqual(started, ["zcode.install_package", "zcode.setup_registration"]);
      assert.deepEqual(progress, ["zcode.install_package"]);
      assert.equal(f.calls.some(call => call[1] === "status"), false);
      assert.equal(JSON.parse(await readFile(f.receiptPath, "utf8")).phase, "prepared");
    });
  }
});

test("ZCode names an empty status response after a verified local preparation", async t => {
  const f = await fixture(t, { statusOutput: "" });
  await assert.rejects(f.driver.execute("install", { targetVersion: "0.1.0", observed: {} }), error => {
    assert.equal(error.message, "taskbelay-zcode status --json returned empty JSON output");
    assert.equal(error.changed, true);
    assert.deepEqual(error.completedSteps, ["zcode.install_package", "zcode.setup_registration"]);
    return true;
  });
});

test("ZCode keeps the removal command until UI removal is confirmed, then uninstalls only its package", async t => {
  const f = await fixture(t);
  await f.driver.execute("install", { targetVersion: "0.1.0", observed: await f.driver.observe() });
  const removing = await f.driver.execute("uninstall", { observed: await f.driver.observe() });
  assert.equal(removing.changed, true);
  assert.equal(f.calls.some(call => call[1] === "uninstall"), false);
  assert.equal((await f.driver.observe()).state, "action_required");
  assert.deepEqual(await f.driver.runtimeCandidates(), []);
  await f.confirmRemoval();
  const completed = await f.driver.execute("uninstall", { observed: await f.driver.observe() });
  assert.deepEqual(completed.completedSteps, ["zcode.uninstall_package"]);
  assert.equal((await f.driver.observe()).state, "absent");
  const repeated = await f.driver.execute("uninstall", { observed: await f.driver.observe() });
  assert.equal(repeated.changed, false);
});

test("ZCode refuses shared reset while cached Host processes cannot be verified", async t => {
  const f = await fixture(t);
  await f.driver.execute("install", { targetVersion: "0.1.0", observed: await f.driver.observe() });
  const before = f.calls.length;
  await assert.rejects(f.driver.maintenanceTargets({ operation: "delete-all", observed: await f.driver.observe() }), /cannot be verified automatically/u);
  assert.equal(f.calls.slice(before).some(call => ["uninstall", "remove"].includes(call[1])), false);
  assert.equal(JSON.parse(await readFile(f.receiptPath, "utf8")).phase, "prepared");
});

test("ZCode rejects a receipt whose runtime escapes its owned package layout", async t => {
  const f = await fixture(t);
  await f.save("prepared");
  const receipt = JSON.parse(await readFile(f.receiptPath, "utf8"));
  receipt.paths.runtime_path = join(f.paths.productRoot, "foreign.exe");
  await writeFile(f.receiptPath, JSON.stringify(receipt));
  await assert.rejects(f.driver.runtimeCandidates(), /owned paths/u);
});

test("ZCode propagates a failed local package check despite an older prepared receipt", async t => {
  const f = await fixture(t);
  await f.driver.execute("install", { targetVersion: "0.1.0", observed: await f.driver.observe() });
  f.damagePayload();
  const observed = await f.driver.observe();
  assert.equal(observed.state, "partial");
  assert.equal(observed.localReady, false);
  assert.ok(observed.issues.some(issue => issue.code === "adapter_check_failed" && issue.message.includes("payload")));
});
