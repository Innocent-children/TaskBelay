import assert from "node:assert/strict";
import { Readable } from "node:stream";
import { EventEmitter } from "node:events";
import { copyFile, chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import test from "node:test";

import { runMain } from "../lib/lifecycle.mjs";
import { execPortableCommand } from "../lib/command.mjs";
import { resolveManagerPaths } from "../lib/ownership.mjs";
import { runPet, stopPetForCore } from "../lib/pet.mjs";
import { loadPetPlatform } from "../lib/platform.mjs";
import {
  bundledPetExecutable,
  isBundledPetApplicationAvailable,
  launchPet,
  runArguments,
  shutdownPet,
  stopArguments,
} from "../lib/platform/macos/pet.mjs";
import { messagesForLanguage } from "../lib/presentation.mjs";
import { NoRuntimeError } from "../lib/runtime.mjs";
import { ensurePetInstalled } from "../lib/platform/macos/pet-installer.mjs";

const currentPetPlatform = await loadPetPlatform(process.platform, process.arch);

test("macOS application replacement preserves the installed copy when source staging fails", {skip:process.platform==='win32'?'Windows cannot create unprivileged file symlinks':false}, async t => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-mac-pet-copy-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = join(root, "package"), petDirectory = join(root, "pet");
  const app = join(source, "runtime/darwin-arm64/TaskBelayPet.app");
  await mkdir(join(app, "Contents/MacOS"), { recursive: true });
  await writeFile(join(app, "Contents/MacOS/TaskBelayPet"), "original", { mode: 0o755 });
  await ensurePetInstalled({ petDirectory, sourcePackageRoots: [source] });
  await writeFile(join(app, "Contents/MacOS/TaskBelayPet"), "updated");
  await symlink(join(app, "Contents/MacOS/TaskBelayPet"), join(app, "unsafe-link"));
  await assert.rejects(ensurePetInstalled({ petDirectory, sourcePackageRoots: [source], replaceExisting: true }), /cannot contain links/);
  assert.equal(await readFile(join(petDirectory, "TaskBelayPet.app/Contents/MacOS/TaskBelayPet"), "utf8"), "original");
});

const CORE_IDENTITY = "0.6.2";
const DATA_ROOT_DIGEST = "3f8a1c9d".repeat(8);
const ORIGIN = "http://127.0.0.1:41731";
const messages = messagesForLanguage("en").pet;

test("pet accepts exactly start and stop and reports invalid arguments with exit code 2", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform();
  const core = coreRuntime([runtimeState()]);
  for (const args of [[], ["pet"], ["pet", "status"], ["pet", "start", "--json"], ["pet", "start", "--host", "codex"], ["webui", "start"]]) {
    const stdout = output();
    const stderr = output();
    const result = await runPet(args, {
      ...fixture.dependencies,
      stdout,
      stderr,
      platformModule: native.module,
      exec: core.exec,
    });
    assert.equal(result.code, 2, JSON.stringify(args));
    assert.equal(stdout.text(), "");
    assert.equal(stderr.text(), `taskbelay: ${messages.invalidArguments}\n`);
  }
  assert.deepEqual(native.calls.launch, []);
  assert.deepEqual(native.calls.shutdown, []);
  assert.deepEqual(core.calls, []);
});

test("a runtime without the desktop component rejects both pet commands", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform();
  const core = coreRuntime([runtimeState()]);
  const selection = coreSelection(fixture);
  for (const command of ["start", "stop"]) {
    const stdout = output();
    const stderr = output();
    const result = await runPet(["pet", command], {
      ...fixture.dependencies,
      stdout,
      stderr,
      platform: "win32",
      arch: "arm64",
      platformModule: native.module,
      resolveCoreRuntime: selection.resolveCoreRuntime,
      exec: core.exec,
    });
    assert.equal(result.code, 1);
    assert.equal(stdout.text(), "");
    assert.equal(stderr.text(), `taskbelay: ${messages.unsupportedPlatform}\n`);
  }
  assert.deepEqual(selection.calls, []);
  assert.deepEqual(core.calls, []);
  assert.deepEqual(native.calls.launch, []);
});

test("start resolves the existing Adapter Core and hands its verified identity to the native entry", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform();
  const core = coreRuntime([runtimeState()]);
  const service_ = service();
  const selection = coreSelection(fixture);
  const stdout = output();
  const stderr = output();

  const result = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout,
    stderr,
    platformModule: native.module,
    resolveCoreRuntime: selection.resolveCoreRuntime,
    exec: core.exec,
    fetchImpl: service_.fetchImpl,
  });

  assert.equal(result.code, 0);
  assert.equal(stderr.text(), "");
  assert.equal(stdout.text(), `${messages.started}\n`);
  assert.deepEqual(selection.calls, [{
    environment: {},
    homeDirectory: fixture.home,
    platform: fixture.dependencies.platform,
    arch: fixture.dependencies.arch,
    exec: core.exec,
    requireData: true,
    initializeDefaultData: true,
  }]);
  assert.equal(core.calls.length, 1);
  assert.equal(core.calls[0].runtimePath, fixture.selection.runtimePath);
  assert.deepEqual(core.calls[0].args, ["webui", "status", "--json"]);
  assert.equal(core.calls[0].options.cwd, fixture.selection.packageRoot);
  assert.equal(core.calls[0].options.env.TASKBELAY_DATA_DIR, fixture.selection.dataDirectory);
  assert.deepEqual(service_.calls, [{
    url: `${ORIGIN}/api/system/status`,
    options: { signal: service_.calls[0].options.signal, redirect: "error" },
  }]);
  assert.equal(native.calls.launch.length, 1);
  assert.equal(native.calls.launch[0].executable, currentPetPlatform.bundledPetExecutable(fixture.packageRoot));
  assert.deepEqual(native.calls.launch[0].request, {
    corePath: fixture.selection.runtimePath,
    dataDirectory: fixture.selection.dataDirectory,
    productRoot: fixture.paths.productRoot,
    coreIdentity: CORE_IDENTITY,
    dataRootDigest: DATA_ROOT_DIGEST,
  });
});

test("start brings the service up once when Core reports it unavailable and restores a hidden pet", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform({ confirmation: "restored" });
  const core = coreRuntime([
    runtimeState({ readiness: "unavailable", url: "", pid: undefined }),
    runtimeState(),
  ]);
  const service_ = service();
  const stdout = output();

  const result = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout,
    stderr: output(),
    platformModule: native.module,
    exec: core.exec,
    fetchImpl: service_.fetchImpl,
  });

  assert.equal(result.code, 0);
  assert.equal(stdout.text(), `${messages.restored}\n`);
  assert.deepEqual(core.calls.map((call) => call.args), [
    ["webui", "status", "--json"],
    ["webui", "start", "--no-open", "--json"],
  ]);
  assert.equal(native.calls.launch.length, 1);
});

test("start consumes actual Core status and starts its service before handing off to the native app", async (t) => {
  const fixture = await petFixture(t);
  const { runtimePath, dataDirectory } = fixture.selection;
  await mkdir(dirname(runtimePath), { recursive: true });
  await mkdir(dataDirectory, { recursive: true });
  // The aggregate check already built Core after the UI build. Reuse those
  // exact bytes while package fixtures may rebuild their own assets concurrently.
  if (process.env.ENGINEERING_CORE) await copyFile(process.env.ENGINEERING_CORE, runtimePath);
  else await execPortableCommand("go", ["build", "-o", runtimePath, "./cmd/taskbelay"], {
    cwd: resolve(import.meta.dirname, "../../.."), encoding: "utf8", timeout: 120_000,
  });
  const native = nativePlatform();
  const states = [];
  const stderr = output();
  const environment = { ...process.env, HOME:fixture.home, USERPROFILE:fixture.home, TASKBELAY_DATA_DIR: dataDirectory };
  try {
    const result = await runPet(["pet", "start"], {
      ...fixture.dependencies,
      environment,
      stderr,
      platformModule: native.module,
      exec: async (executable, args, options) => {
        const response = await execPortableCommand(executable, args, options);
        states.push(JSON.parse(response.stdout));
        return response;
      },
    });
    let diagnostic=stderr.text();if(result.code!==0){try{diagnostic+="\n"+(await readFile(join(dataDirectory,"webui-runtime.log"),"utf8")).slice(-2000)}catch{}}
    assert.equal(result.code, 0, diagnostic);
    assert.equal(states[0].readiness, "unavailable");
    assert.equal(Object.hasOwn(states[0], "pid"), false);
    assert.equal(Object.hasOwn(states[0], "operation"), false);
    assert.equal(states[1].readiness, "ready");
    const { stdout } = await execPortableCommand(runtimePath, ["version"], { encoding: "utf8" });
    const version = stdout.trim().replace(/^taskbelay /u, "");
    assert.equal(states[0].core_identity, version);
    assert.equal(native.calls.launch.length, 1);
    assert.equal(native.calls.launch[0].request.coreIdentity, version);
    assert.equal(native.calls.launch[0].request.dataRootDigest, states[1].data_root_digest);
  } finally {
    await execPortableCommand(runtimePath, ["webui", "stop", "--json"], {
      env: environment, encoding: "utf8", timeout: 15_000,
    });
  }
});

test("start refuses an unusable service, address, identity, or data directory", async (t) => {
  const cases = [
    {
      name: "read-only storage without a live service",
      states: [runtimeState({ readiness: "read_only" })],
      live: new Error("connect ECONNREFUSED"),
      expected: messages.serviceUnavailable,
    },
    {
      name: "a service answering for another data directory",
      states: [runtimeState()],
      live: { core_identity: CORE_IDENTITY, data_root_digest: "9".repeat(64), url: ORIGIN },
      expected: messages.identityMismatch,
    },
    {
      name: "an incompatible data directory",
      states: [runtimeState({ readiness: "incompatible" })],
      live: null,
      expected: messages.incompatibleRuntime,
    },
    {
      name: "an address outside the loopback origin allowlist",
      states: [runtimeState({ url: "http://192.168.1.20:8080" })],
      live: null,
      expected: messages.serviceUnavailable,
    },
    {
      name: "a runtime state outside the documented contract",
      states: [runtimeState({ core_identity: "0.6" })],
      live: null,
      expected: `${messages.coreCommandFailed}: the Core identity is invalid`,
    },
    {
      name: "the retired product-prefixed identity",
      states: [runtimeState({ core_identity: "taskbelay/0.6.2" })],
      live: null,
      expected: `${messages.coreCommandFailed}: the Core identity is invalid`,
    },
    ...[null, "4242", -1, 1.5].map(pid => ({
      name: `an invalid process field: ${JSON.stringify(pid)}`,
      states: [runtimeState({ pid })],
      live: null,
      expected: `${messages.coreCommandFailed}: the Core reported an invalid service process`,
    })),
  ];
  const fixture = await petFixture(t);
  for (const current of cases) {
    const native = nativePlatform();
    const core = coreRuntime(current.states);
    const service_ = service(current.live);
    const stdout = output();
    const stderr = output();
    const result = await runPet(["pet", "start"], {
      ...fixture.dependencies,
      stdout,
      stderr,
      platformModule: native.module,
      exec: core.exec,
      fetchImpl: service_.fetchImpl,
    });
    assert.equal(result.code, 1, current.name);
    assert.equal(stdout.text(), "", current.name);
    assert.equal(stderr.text(), `taskbelay: ${current.expected}\n`, current.name);
    assert.deepEqual(native.calls.launch, [], current.name);
  }
});

test("start prioritizes installed pet in petDirectory over package bundle", async (t) => {
  const fixture = await petFixture(t);
  const core = coreRuntime([runtimeState()]);
  const native = nativePlatform({ installedAvailable: true });
  const stdout = output();
  const stderr = output();

  const result = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout,
    stderr,
    platformModule: native.module,
    exec: core.exec,
    fetchImpl: service().fetchImpl,
  });

  assert.equal(result.code, 0);
  assert.equal(stdout.text(), "✓ Desktop pet started\n");
  assert.equal(
    native.calls.launch[0].executable,
    currentPetPlatform.installedPetExecutable(fixture.paths.petDirectory),
  );
});

test("start reports a failed launch confirmation and a missing bundled application", async (t) => {
  const fixture = await petFixture(t);

  const failing = nativePlatform({ launchError: new Error("no launch confirmation within 10000 milliseconds") });
  const launchStderr = output();
  const launchResult = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout: output(),
    stderr: launchStderr,
    platformModule: failing.module,
    exec: coreRuntime([runtimeState()]).exec,
    fetchImpl: service().fetchImpl,
  });
  assert.equal(launchResult.code, 1);
  assert.equal(launchStderr.text(), `taskbelay: ${messages.launchFailed}: no launch confirmation within 10000 milliseconds\n`);

  const missing = nativePlatform({ available: false });
  const missingStdout = output();
  const missingStderr = output();
  const missingResult = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout: missingStdout,
    stderr: missingStderr,
    platformModule: missing.module,
    exec: coreRuntime([runtimeState()]).exec,
    fetchImpl: service().fetchImpl,
  });
  assert.equal(missingResult.code, 1);
  assert.equal(missingStdout.text(), "");
  assert.equal(missingStderr.text(), `taskbelay: ${messages.applicationUnavailable}\n`);
  assert.deepEqual(missing.calls.launch, []);
});

test("start requires an installed Adapter and never installs one", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform();
  const core = coreRuntime([runtimeState()]);
  const selection = coreSelection(fixture, new NoRuntimeError("no installed Codex or DeepSeek Adapter provides a Core runtime"));
  const stdout = output();
  const stderr = output();

  const result = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout,
    stderr,
    platformModule: native.module,
    resolveCoreRuntime: selection.resolveCoreRuntime,
    exec: core.exec,
    fetchImpl: service().fetchImpl,
  });

  assert.equal(result.code, 1);
  assert.equal(stdout.text(), "");
  assert.equal(stderr.text(), `taskbelay: ${messages.installAdapterFirst}\n`);
  assert.equal(selection.calls.length, 1);
  assert.deepEqual(core.calls, []);
  assert.deepEqual(native.calls.launch, []);
});

test("stop ends the running pet without resolving an Adapter or touching the service", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform();
  const core = coreRuntime([runtimeState()]);
  const selection = coreSelection(fixture);
  const service_ = service();
  const stdout = output();
  const stderr = output();

  const result = await runPet(["pet", "stop"], {
    ...fixture.dependencies,
    stdout,
    stderr,
    platformModule: native.module,
    resolveCoreRuntime: selection.resolveCoreRuntime,
    exec: core.exec,
    fetchImpl: service_.fetchImpl,
  });

  assert.equal(result.code, 0);
  assert.equal(stderr.text(), "");
  assert.equal(stdout.text(), `${messages.stopped}\n`);
  assert.deepEqual(selection.calls, []);
  assert.deepEqual(core.calls, []);
  assert.deepEqual(service_.calls, []);
  assert.deepEqual(native.calls.shutdown, [{
    executable: currentPetPlatform.bundledPetExecutable(fixture.packageRoot),
    productRoot: fixture.paths.productRoot,
    corePath: null,
    environment: {},
  }]);
});

test("stop reports a failed shutdown and a missing bundled application", async (t) => {
  const fixture = await petFixture(t);

  const failing = nativePlatform({ shutdown: { code: 1, detail: "the pet did not exit in time" } });
  const failingStderr = output();
  const failingResult = await runPet(["pet", "stop"], {
    ...fixture.dependencies,
    stdout: output(),
    stderr: failingStderr,
    platformModule: failing.module,
  });
  assert.equal(failingResult.code, 1);
  assert.equal(failingStderr.text(), `taskbelay: ${messages.stopFailed}: the pet did not exit in time\n`);

  const missing = nativePlatform({ available: false });
  const missingStderr = output();
  const missingResult = await runPet(["pet", "stop"], {
    ...fixture.dependencies,
    stdout: output(),
    stderr: missingStderr,
    platformModule: missing.module,
  });
  assert.equal(missingResult.code, 1);
  assert.equal(missingStderr.text(), `taskbelay: ${messages.applicationUnavailable}\n`);
  assert.deepEqual(missing.calls.shutdown, []);
});

test("pet results render in the language the launcher already resolved", async (t) => {
  const fixture = await petFixture(t);
  const native = nativePlatform();
  const stdout = output();
  const result = await runPet(["pet", "start"], {
    ...fixture.dependencies,
    stdout,
    stderr: output(),
    language: "zh-CN",
    platformModule: native.module,
    exec: coreRuntime([runtimeState()]).exec,
    fetchImpl: service().fetchImpl,
  });
  assert.equal(result.code, 0);
  assert.equal(stdout.text(), `${messagesForLanguage("zh-CN").pet.started}\n`);
  assert.notEqual(messagesForLanguage("zh-CN").pet.started, messages.started);
});

test("Adapter maintenance stops only a pet that runs the maintained Core", async (t) => {
  const fixture = await petFixture(t);
  const maintainedCore = fixture.selection.runtimePath;
  const native = nativePlatform();

  const stopped = await stopPetForCore({
    corePath: maintainedCore,
    environment: {},
    homeDirectory: fixture.home,
    platform: fixture.dependencies.platform,
    arch: fixture.dependencies.arch,
    packageRoot: fixture.packageRoot,
    platformModule: native.module,
  });
  assert.deepEqual(stopped, { stopped: true, reason: null });
  assert.deepEqual(native.calls.shutdown, [{
    executable: currentPetPlatform.bundledPetExecutable(fixture.packageRoot),
    productRoot: fixture.paths.productRoot,
    corePath: maintainedCore,
    environment: {},
  }]);

  const failing = nativePlatform({ shutdown: { code: 1, detail: "another Core is in use" } });
  await assert.rejects(stopPetForCore({
    corePath: maintainedCore,
    environment: {},
    homeDirectory: fixture.home,
    platform: fixture.dependencies.platform,
    arch: fixture.dependencies.arch,
    packageRoot: fixture.packageRoot,
    platformModule: failing.module,
  }), /another Core is in use/u);
});

test("Adapter maintenance skips a runtime or package that has no desktop component", async (t) => {
  const fixture = await petFixture(t);

  const windows = nativePlatform();
  assert.deepEqual(await stopPetForCore({
    corePath: null,
    environment: {},
    homeDirectory: fixture.home,
    platform: "win32",
    arch: "arm64",
    packageRoot: fixture.packageRoot,
    platformModule: windows.module,
  }), { stopped: false, reason: "unsupported-platform" });
  assert.deepEqual(windows.calls.shutdown, []);

  const missing = nativePlatform({ available: false });
  assert.deepEqual(await stopPetForCore({
    corePath: null,
    environment: {},
    homeDirectory: fixture.home,
    platform: fixture.dependencies.platform,
    arch: fixture.dependencies.arch,
    packageRoot: fixture.packageRoot,
    platformModule: missing.module,
  }), { stopped: false, reason: "application-unavailable" });
  assert.deepEqual(missing.calls.shutdown, []);
});

test("the macOS packaged application location requires a regular file", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-pet-bundle-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  assert.equal(
    bundledPetExecutable(join(root, "package")),
    join(root, "package", "runtime", "darwin-arm64", "TaskBelayPet.app", "Contents", "MacOS", "TaskBelayPet"),
  );
  const executable = join(root, "TaskBelayPet");
  await writeFile(executable, "#!/bin/sh\n");
  await chmod(executable, 0o755);
  assert.equal(await isBundledPetApplicationAvailable(executable), true);
  assert.equal(await isBundledPetApplicationAvailable(join(root, "absent")), false);
  assert.equal(await isBundledPetApplicationAvailable(root), false);
});

test("the macOS packaged application requires POSIX executable permission", { skip: process.platform !== "darwin" }, async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-pet-mode-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const executable = join(root, "TaskBelayPet");
  await writeFile(executable, "#!/bin/sh\n", { mode: 0o755 });
  assert.equal(await isBundledPetApplicationAvailable(executable), true);
  await chmod(executable, 0o644);
  assert.equal(await isBundledPetApplicationAvailable(executable), false);
});

test("the native argument arrays are the exact private entry contract", () => {
  assert.deepEqual(runArguments({
    corePath: "/adapter/runtime/darwin-arm64/taskbelay",
    dataDirectory: "/product/data",
    productRoot: "/product",
    coreIdentity: CORE_IDENTITY,
    dataRootDigest: DATA_ROOT_DIGEST,
  }), [
    "run",
    "--core-path", "/adapter/runtime/darwin-arm64/taskbelay",
    "--data-dir", "/product/data",
    "--product-root", "/product",
    "--core-identity", CORE_IDENTITY,
    "--data-root-digest", DATA_ROOT_DIGEST,
  ]);
  assert.deepEqual(stopArguments({ productRoot: "/product" }), ["stop", "--product-root", "/product"]);
  assert.deepEqual(
    stopArguments({ productRoot: "/product", corePath: "/adapter/runtime/darwin-arm64/taskbelay" }),
    ["stop", "--product-root", "/product", "--core-path", "/adapter/runtime/darwin-arm64/taskbelay"],
  );
});

test("launch resolves on the first confirmation line and releases the detached native process", async (t) => {
  const executable = "/package/runtime/darwin-arm64/TaskBelayPet.app/Contents/MacOS/TaskBelayPet";
  const request = {
    corePath: "/adapter/runtime/darwin-arm64/taskbelay",
    dataDirectory: "/product/data",
    productRoot: "/product",
    coreIdentity: CORE_IDENTITY,
    dataRootDigest: DATA_ROOT_DIGEST,
  };
  const spawned = spawnStub();
  const promise = launchPet({ executable, request, spawnImpl: spawned.spawnImpl, environment: { LANG: "en_US.UTF-8" } });
  const child = spawned.children[0];
  assert.deepEqual(spawned.calls[0].args, runArguments(request));
  assert.deepEqual(spawned.calls[0].options, {
    cwd: dirname(executable),
    env: { LANG: "en_US.UTF-8" },
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout.emit("data", "rea");
  child.stdout.emit("data", "dy\nignored later output\n");
  assert.equal(await promise, "ready");
  assert.equal(child.released, true);
  assert.equal(child.stdout.destroyed, true);
  assert.equal(child.stderr.destroyed, true);
});

test("launch refuses an unsupported confirmation and reports the native diagnostic on exit", async (t) => {
  const executable = "/package/TaskBelayPet";

  const unsupported = spawnStub();
  const unsupportedPromise = launchPet({ executable, request: launchRequest(), spawnImpl: unsupported.spawnImpl });
  unsupported.children[0].stdout.emit("data", "started\n");
  await assert.rejects(unsupportedPromise, /unsupported launch confirmation started/u);

  const exited = spawnStub();
  const exitedPromise = launchPet({ executable, request: launchRequest(), spawnImpl: exited.spawnImpl });
  exited.children[0].stderr.emit("data", "another instance holds a different Core\nsecond line\n");
  exited.children[0].emit("exit", 1);
  await assert.rejects(exitedPromise, /another instance holds a different Core/u);
});

test("launch gives up when the native entry sends no confirmation", async (t) => {
  const spawned = spawnStub();
  const promise = launchPet({
    executable: "/package/TaskBelayPet",
    request: launchRequest(),
    spawnImpl: spawned.spawnImpl,
    timeout: 40,
  });
  await assert.rejects(promise, /no launch confirmation within 40 milliseconds/u);
  assert.deepEqual(spawned.children[0].killed, ["SIGTERM"]);
});

test("shutdown reports the native exit code with its first diagnostic line", async (t) => {
  const executable = "/package/runtime/darwin-arm64/TaskBelayPet.app/Contents/MacOS/TaskBelayPet";

  const stopped = spawnStub();
  const stoppedPromise = shutdownPet({ executable, productRoot: "/product", spawnImpl: stopped.spawnImpl, environment: {} });
  assert.deepEqual(stopped.calls[0].args, ["stop", "--product-root", "/product"]);
  assert.deepEqual(stopped.calls[0].options, { cwd: dirname(executable), env: {}, stdio: ["ignore", "ignore", "pipe"] });
  stopped.children[0].emit("exit", 0);
  assert.deepEqual(await stoppedPromise, { code: 0, detail: null });

  const filtered = spawnStub();
  const filteredPromise = shutdownPet({
    executable,
    productRoot: "/product",
    corePath: "/adapter/runtime/darwin-arm64/taskbelay",
    spawnImpl: filtered.spawnImpl,
  });
  assert.deepEqual(filtered.calls[0].args, ["stop", "--product-root", "/product", "--core-path", "/adapter/runtime/darwin-arm64/taskbelay"]);
  filtered.children[0].stderr.emit("data", "the pet did not exit in time\n");
  filtered.children[0].emit("exit", 1);
  assert.deepEqual(await filteredPromise, { code: 1, detail: "the pet did not exit in time" });

  const timedOut = spawnStub();
  const timedOutPromise = shutdownPet({ executable, productRoot: "/product", spawnImpl: timedOut.spawnImpl, timeout: 40 });
  assert.deepEqual(await timedOutPromise, { code: 1, detail: "no shutdown result within 40 milliseconds" });
  assert.deepEqual(timedOut.children[0].killed, ["SIGTERM"]);
});

test("an interactive pet selection reuses the pet launcher and returns to the menu", async () => {
  const stdout = output();
  const stderr = output();
  const delegated = [];
  const result = await runMain([], {
    input: Readable.from([]),
    output: stdout,
    errorOutput: stderr,
    environment: {},
    resolveManagerPaths: async () => { throw new Error("installation unavailable"); },
    isTTY: true,
    platform: "darwin",
    arch: "arm64",
    promptForRequest: async (options) => {
      assert.equal(options.platform, "darwin");
      assert.equal(options.arch, "arm64");
      return delegated.length ? { cancelled: true } : { pet: "start" };
    },
    runPet: async (args, options) => {
      delegated.push({ args, options });
      return { code: 0, signal: null };
    },
  });
  assert.equal(result.code, 0);
  assert.equal(delegated.length, 1);
  assert.deepEqual(delegated[0].args, ["pet", "start"]);
  assert.equal(delegated[0].options.stdout, stdout);
  assert.equal(delegated[0].options.stderr, stderr);
  assert.equal(delegated[0].options.language, "en");
});

async function petFixture(t) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-pet-")));
  const home = join(root, "home");
  const packageRoot = join(root, "package");
  await mkdir(home);
  await mkdir(packageRoot);
  const paths = await resolveManagerPaths({ homeDirectory: home, environment: {}, platform: process.platform, arch: process.arch });
  const selection = Object.freeze({
    source: "codex",
    packageRoot: join(root, "adapter", "taskbelay-codex"),
    runtimePath: join(root, "adapter", "taskbelay-codex", "runtime", paths.runtimeDirectory, paths.runtimeExecutable),
    version: "0.6.2",
    dataDirectory: paths.defaultDataDirectory,
    platform: paths.platform,
    arch: paths.arch,
    runtimeKey: paths.runtimeKey,
    forwardedSignals: paths.forwardedSignals,
  });
  t.after(async () => {
    const { rm } = await import("node:fs/promises");
    await rm(root, { recursive: true, force: true });
  });
  return {
    root,
    home,
    packageRoot,
    paths,
    selection,
    dependencies: {
      stdout: output(),
      stderr: output(),
      environment: {},
      language: "en",
      platform: paths.platform,
      arch: paths.arch,
      homeDirectory: home,
      packageRoot,
      resolveCoreRuntime: async () => selection,
    },
  };
}

function coreSelection(fixture, error = null) {
  const calls = [];
  return {
    calls,
    resolveCoreRuntime: async (options) => {
      calls.push(options);
      if (error !== null) throw error;
      return fixture.selection;
    },
  };
}

function coreRuntime(states) {
  const calls = [];
  const queue = [...states];
  return {
    calls,
    exec: async (runtimePath, args, options) => {
      calls.push({ runtimePath, args, options });
      const state = queue.length > 1 ? queue.shift() : queue[0];
      if (state instanceof Error) throw state;
      return { stdout: `${JSON.stringify(state)}\n`, stderr: "" };
    },
  };
}

function service(live = {
  ok: true,
  request_id: "status-1",
  readiness: "ready",
  core_identity: CORE_IDENTITY,
  data_root_digest: DATA_ROOT_DIGEST,
  url: ORIGIN,
}) {
  const calls = [];
  return {
    calls,
    fetchImpl: async (url, options) => {
      calls.push({ url, options });
      if (live instanceof Error) throw live;
      return { ok: true, json: async () => live };
    },
  };
}

function nativePlatform({ available = true, confirmation = "ready", shutdown = { code: 0, detail: null }, launchError = null, installedAvailable = false } = {}) {
  const calls = { launch: [], shutdown: [] };
  let installedExecutable;
  return {
    calls,
    module: {
      bundledPetExecutable: currentPetPlatform.bundledPetExecutable,
      installedPetExecutable: (petDirectory) => {
        installedExecutable = currentPetPlatform.installedPetExecutable(petDirectory);
        return installedExecutable;
      },
      isBundledPetApplicationAvailable: async (candidate) => {
        if (!available) return false;
        if (candidate === installedExecutable) return installedAvailable;
        return true;
      },
      launchPet: async (options) => {
        calls.launch.push(options);
        if (launchError !== null) throw launchError;
        return confirmation;
      },
      shutdownPet: async (options) => {
        calls.shutdown.push(options);
        return shutdown;
      },
    },
  };
}

function launchRequest() {
  return {
    corePath: "/adapter/runtime/darwin-arm64/taskbelay",
    dataDirectory: "/product/data",
    productRoot: "/product",
    coreIdentity: CORE_IDENTITY,
    dataRootDigest: DATA_ROOT_DIGEST,
  };
}

function runtimeState(overrides = {}) {
  return {
    readiness: "ready",
    core_identity: CORE_IDENTITY,
    data_root_digest: DATA_ROOT_DIGEST,
    url: ORIGIN,
    pid: 4242,
    ...overrides,
  };
}

function output() {
  const chunks = [];
  return { write: (value) => { chunks.push(value); return true; }, text: () => chunks.join("") };
}

function spawnStub() {
  const calls = [];
  const children = [];
  return {
    calls,
    children,
    spawnImpl: (executable, args, options) => {
      calls.push({ executable, args, options });
      const child = new EventEmitter();
      child.stdout = pipeStub();
      child.stderr = pipeStub();
      child.killed = [];
      child.kill = (signal) => { child.killed.push(signal); };
      child.released = false;
      child.unref = () => { child.released = true; };
      children.push(child);
      return child;
    },
  };
}

function pipeStub() {
  const pipe = new EventEmitter();
  pipe.encodings = [];
  pipe.setEncoding = (encoding) => { pipe.encodings.push(encoding); };
  pipe.destroyed = false;
  pipe.destroy = () => { pipe.destroyed = true; };
  return pipe;
}
