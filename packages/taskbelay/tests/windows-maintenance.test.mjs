import assert from "node:assert/strict";
import { mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { createHostDrivers } from "../lib/hosts/index.mjs";
import { runLifecycle } from "../lib/lifecycle.mjs";
import { resolveManagerPaths } from "../lib/ownership.mjs";
import * as windows from "../lib/platform/windows/maintenance.mjs";
import { assertManagedCoresStopped, stopStdioCores } from "../lib/platform/windows/core-processes.mjs";

test("Windows maintenance simulation installs and repairs only the selected Claude package", async t => {
  for (const initiallyInstalled of [false, true]) {
    const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-windows-claude-")));
    t.after(() => rm(root, { recursive: true, force: true }));
    const home = join(root, "home"), npmRoot = join(root, "npm");
    const environment = { LOCALAPPDATA: join(home, "AppData", "Local"), SystemRoot: join(root, "Windows") };
    await mkdir(environment.LOCALAPPDATA, { recursive: true });
    const paths = await resolveManagerPaths({ homeDirectory: home, environment, platform: "win32", arch: "x64" });
    await mkdir(paths.defaultDataDirectory, { recursive: true });
    const packageRoot = join(npmRoot, "taskbelay-claude");
    const runtimePath = join(packageRoot, "runtime", "win32-x64", "taskbelay.exe");
    if (initiallyInstalled) {
      await mkdir(join(packageRoot, "runtime", "win32-x64"), { recursive: true });
      await writeFile(runtimePath, "simulated Core executable");
      await writeFile(join(packageRoot, "package.json"), JSON.stringify({ name: "taskbelay-claude", version: "0.1.0" }));
    }
    let installed = initiallyInstalled, ready = false;
    const events = [], driverPaths = [];
    const runClaudeChild = async (executable, args) => {
      if (executable === "claude") { assert.deepEqual(args, ["--version"]); return { stdout: "2.1.270" }; }
      if (executable === "npm") {
        if (args[0] === "root") { events.push("claude.locate"); return { stdout: npmRoot }; }
        if (args[0] === "list") return { stdout: JSON.stringify({ dependencies: installed ? { "taskbelay-claude": { version: "0.1.0" } } : {} }) };
        if (args[0] === "view") return { stdout: '"0.2.0"' };
        assert.deepEqual(args, ["install", "--global", "taskbelay-claude@0.2.0"]);
        events.push("claude.install"); installed = true; return { stdout: "" };
      }
      assert.equal(executable, "taskbelay-claude");
      if (args[0] === "setup") { events.push("claude.setup"); ready = true; return { stdout: JSON.stringify({status:"ready",package_version:"0.2.0"}) }; }
      assert.equal(args[0], "status");
      if (!ready) throw Object.assign(new Error("registration not ready"), { code: "ENOENT" });
      return { stdout: JSON.stringify({ status: "ready", package_version: "0.2.0", core_version: "0.6.0", registration: { receipt: true } }) };
    };
    const failOtherHost = async () => { throw new Error("must not invoke another Host"); };
    const result = await runLifecycle(request(initiallyInstalled ? "repair" : "install"), {
      homeDirectory: home, environment, platform: "win32", arch: "x64", runClaudeChild,
      runCodexChild: failOtherHost, runDeepSeekChild: failOtherHost, runZCodeChild: failOtherHost,
      confirmPlan: async () => true,
      stopPetForCore: async () => { throw new Error("no registered Core exists"); },
      createHostDrivers: options => { driverPaths.push(options.paths); return createHostDrivers(options); },
      loadMaintenancePlatform: async () => ({
        ...windows,
        prepareReplacement: options => windows.prepareReplacement({ ...options, run: async (executable, args) => {
          if (executable === runtimePath) { assert.deepEqual(args, ["webui", "status", "--json"]); return { stdout: '{"pid":null}' }; }
          assert.match(args.at(-1), /Get-CimInstance Win32_Process/);
          assert.ok(args.at(-1).includes(runtimePath));
          events.push("claude.stop-stdio"); return { stdout: "" };
        } }),
      }),
    });
    assert.equal(result.code, 0);
    assert.equal(driverPaths.length, 2);
    assert.notEqual(driverPaths[0], driverPaths[1]);
    assert.equal(driverPaths[1].productRoot, paths.productRoot);
    assert.deepEqual(events, initiallyInstalled
      ? ["claude.locate", "claude.stop-stdio", "claude.install", "claude.setup"]
      : ["claude.install", "claude.setup"]);
  }
});

test("Windows replacement simulation validates package identity and stops the exact owned Core", async t => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-windows-replacement-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const runtimePath = join(root, "runtime", "win32-x64", "taskbelay.exe");
  await mkdir(join(root, "runtime", "win32-x64"), { recursive: true });
  await writeFile(runtimePath, "simulated executable");
  const manifest = join(root, "package.json");
  const paths = { runtimeDirectory: "win32-x64", runtimeExecutable: "taskbelay.exe", defaultDataDirectory: root };
  const runtime = { packageName: "taskbelay-claude", packageRoot: root, runtimePath };
  const environment = { SystemRoot: root };
  const calls = [];
  const run = async (executable, args) => {
    calls.push(args);
    if (executable === runtimePath) return { stdout: JSON.stringify(args[1] === "status" ? { pid: 123, readiness: "ready" } : { readiness: "unavailable" }) };
    return { stdout: args.at(-1).includes("Get-Process -Id 123") ? runtimePath : "" };
  };
  await writeFile(manifest, JSON.stringify({ name: "taskbelay-deepseek" }));
  await assert.rejects(windows.prepareReplacement({ runtime, paths, environment, run }), /must belong to its Adapter/);
  assert.equal(calls.length, 0);
  await writeFile(manifest, JSON.stringify({ name: "taskbelay-claude" }));
  await windows.prepareReplacement({ runtime, paths, environment, run });
  assert.equal(calls.length, 4);
  assert.match(calls[0].at(-1), /StartTime.*CreationDate/s);
  assert.deepEqual(calls.at(-1), ["webui", "stop", "--json"]);
});

function request(operation) {
  return { operation, hosts: ["claude"], profiles: [], allKnownProfiles: false, targetVersion: "0.2.0", adopt: false,
    permanent: false, yes: true, confirmedExplicitData: [], outputMode: "json" };
}

test("Windows process command simulation binds exact image, complete arguments and startup identity", async () => {
  const calls = [];
  const options = { environment: { SystemRoot: "C:\\Windows" }, run: async (_exe, args) => { calls.push(args.at(-1)); return { stdout: "" }; } };
  const runtimePaths = ["C:\\managed package\\runtime\\win32-x64\\taskbelay.exe"];
  await stopStdioCores(runtimePaths, options);
  await assertManagedCoresStopped(runtimePaths, options);
  for (const script of calls) {
    assert.match(script, /ExecutablePath -ine \$expected/);
    assert.match(script, /\[Regex\]::Escape\(\$expected\)/);
    assert.match(script, /StartTime.*CreationDate/s);
    assert.match(script, /difference -ge 10.*identity changed/);
    assert.ok(script.includes("\\s*$"));
  }
  assert.match(calls[0], /WaitForExit\(5000\)/);
  assert.match(calls[1], /webui\\s\+serve/);
  assert.doesNotMatch(calls[1], /\.Kill\(/);
  for (const message of ["Managed Core process identity changed", "The managed STDIO Core did not exit", "Core reconnected"]) {
    await assert.rejects(stopStdioCores(runtimePaths, { ...options, run: async () => { throw new Error(message); } }), new RegExp(message));
  }
});
