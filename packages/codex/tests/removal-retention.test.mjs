import { planFor } from "../../../tests/journeys/fixtures/engineering.mjs";
import { admissionFixture, receiptAdmissionFixture } from "./fixtures/task-handoff.mjs";
import assert from "node:assert/strict";
import { execFile as execFileCallback, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createInterface } from "node:readline";
import test from "node:test";

import { writeHandoffFixture } from "./fixtures/task-handoff.mjs";

const execFile = promisify(execFileCallback);
const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const repositoryRoot = join(packageRoot, "..", "..");
const buildScript = join(repositoryRoot, "scripts", "build-codex-local.sh");
const fakeCodexPath = join(packageRoot, "tests", "fixtures", "fake-codex.mjs");
const supportedMachine = process.platform === "darwin" && process.arch === "arm64";

test("packaged Core task data survives deregistration, npm uninstall, and same-artifact reinstall", {
  skip: supportedMachine ? false : "darwin-arm64 packaged Core integration only",
  timeout: 120_000,
}, async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-codex-retention-")));
  const clients = [];
  t.after(async () => {
    await Promise.all(clients.map((client) => client.dispose()));
    await rm(root, { recursive: true, force: true });
  });
  const startCore = async (runtimePath, dataPath, repositoryPath) => {
    const client = await CoreClient.start(runtimePath, dataPath, repositoryPath);
    clients.push(client);
    return client;
  };

  const artifactDirectory = join(root, "artifacts");
  const installPrefix = join(root, "install prefix-安装");
  const reinstallPrefix = join(root, "reinstall prefix-重装");
  const isolatedHome = join(root, "isolated home");
  const dataDirectory = join(root, "Core task data");
  const targetRepository = join(root, "target repository-仓库");
  const remoteRepository = join(root, "remote repository.git");
  const taskRepository = join(root, "task worktree-工作树");
  const fakeState = join(root, "fake Codex", "state.json");
  const fakeTrace = join(root, "fake Codex", "trace.jsonl");
  await Promise.all([
    mkdir(artifactDirectory, { recursive: true }),
    mkdir(installPrefix, { recursive: true }),
    mkdir(reinstallPrefix, { recursive: true }),
    mkdir(isolatedHome, { recursive: true }),
    mkdir(dataDirectory, { recursive: true }),
  ]);
  await initializeRepository(targetRepository, remoteRepository);

  const { stdout: buildOutput } = await execFile(buildScript, ["--output", artifactDirectory], {
    cwd: repositoryRoot,
    encoding: "utf8",
    maxBuffer: 20 * 1024 * 1024,
  });
  const build = JSON.parse(buildOutput);
  assert.equal(build.final_artifact, false);
  assert.match(build.artifact_sha256, /^[0-9a-f]{64}$/);

  await installArtifact(build.artifact_path, installPrefix);
  const installedPackage = await realpath(join(installPrefix, "node_modules", "taskbelay-codex"));
  const lifecycle = await importInstalledLifecycle(installedPackage, "initial");
  const admission = await importInstalledModule(installedPackage, "lib/task-admission.mjs", "admission-initial");
  const taskLaunch = await importInstalledModule(installedPackage, "lib/task-launch.mjs", "launch-initial");
  const paths = productPaths(installedPackage, isolatedHome, dataDirectory);
  const environment = fakeEnvironment(installPrefix, fakeState, fakeTrace);

  const setup = await lifecycle.setupRegistration({
    paths,
    packageVersion: build.package_version,
    codexExecutable: fakeCodexPath,
    environment,
    now: () => new Date("2026-08-15T00:00:00.000Z"),
  });
  assert.equal(setup.status, "installed");
  const taskRequest = "Prove graph task data survives bounded Codex deregistration";
  const assessmentAnchor = await admission.inspectAdmissionAnchor({
    request: taskRequest,
    repositories: [{ key: "primary", repository_path: targetRepository }],
  });
  const prepared = await taskLaunch.prepareTaskLaunch({
    launch_id: "retention-launch",
    request: taskRequest,
    handoff_file: await writeHandoffFixture(root, taskRequest),
    ...admissionFixture(assessmentAnchor),
    repository_key: "primary",
    repository_path: targetRepository,
    workspace_mode: "dedicated_worktree",
    source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "codex/retention-task",
    surface: "cli_worktree",
    worktree_path: taskRepository,
  }, { productSupportRoot: paths.productSupportRoot, enforcePrivateModes: true });
  const provisioned = await taskLaunch.provisionCliTask({
    launch_id: prepared.receipt.launch_id,
    repository_key: "primary",
    additional_worktree_paths: [],
  }, {
    productSupportRoot: paths.productSupportRoot,
    enforcePrivateModes: true,
    sourceRepositoryPath: targetRepository,
  });
  const repositoryBefore = await directoryManifest(targetRepository);
  const taskRepositoryBefore = await directoryManifest(taskRepository);
  const provisioningReceiptBefore = await readFile(prepared.receipt_path, "utf8");
  await mkdir(paths.configurationDirectory, { recursive: true, mode: 0o700 });
  await writeFile(paths.configurationPath, '{"codex":{"codebase_memory":true}}\n', { mode: 0o600 });
  const configurationBefore = await readFile(paths.configurationPath, "utf8");
  const adjacentFile = join(dirname(paths.receiptPath), "user-owned-adjacent.txt");
  const codexAdjacentFile = join(dirname(fakeState), "user-owned-codex-state.txt");
  const unrelatedMarketplaceRoot = join(root, "unrelated host marketplace");
  const unrelatedConfig = join(isolatedHome, ".codex", "config.toml");
  const unrelatedReceipt = join(isolatedHome, "other-product", "registrations", "codex.json");
  const unrelatedMarketplace = {
    name: "other-marketplace",
    root: unrelatedMarketplaceRoot,
    marketplaceSource: { sourceType: "local", source: unrelatedMarketplaceRoot },
  };
  const unrelatedPlugin = {
    pluginId: "other-plugin@other-marketplace",
    name: "other-plugin",
    marketplaceName: "other-marketplace",
    version: "9.9.9",
    installed: true,
    enabled: false,
    source: { source: "local", path: join(unrelatedMarketplaceRoot, "plugin") },
    marketplaceSource: { sourceType: "local", source: unrelatedMarketplaceRoot },
    installPolicy: "AVAILABLE",
    authPolicy: "ON_INSTALL",
  };
  const completeHostState = JSON.parse(await readFile(fakeState, "utf8"));
  completeHostState.marketplaces.push(unrelatedMarketplace);
  completeHostState.plugins.push(unrelatedPlugin);
  await Promise.all([
    mkdir(dirname(unrelatedConfig), { recursive: true }),
    mkdir(dirname(unrelatedReceipt), { recursive: true }),
    mkdir(join(unrelatedMarketplaceRoot, "plugin"), { recursive: true }),
  ]);
  await writeFile(fakeState, `${JSON.stringify(completeHostState, null, 2)}\n`);
  await writeFile(adjacentFile, "preserve adjacent data\n");
  await writeFile(codexAdjacentFile, "preserve Codex-adjacent data\n");
  await writeFile(unrelatedConfig, "model = \"unrelated-user-choice\"\n");
  await writeFile(unrelatedReceipt, "{\"owner\":\"other-product\"}\n");
  await writeFile(join(unrelatedMarketplaceRoot, "plugin", "marker.txt"), "unrelated plugin bytes\n");
  const unrelatedMarketplaceBefore = await directoryManifest(unrelatedMarketplaceRoot);

  const firstCore = await startCore(paths.runtimePath, dataDirectory, taskRepository);
  const info = await firstCore.callTool("taskbelay_server_info", {});
  assert.equal(info.result.product, "taskbelay");
  const opened = await firstCore.callTool("taskbelay_open_task", {
    operation_id: "retention-open", host: "codex",
    repositories: [{key:"primary",repository_path:taskRepository}],
    authorization: {user_ref:"fixture-retention-request",request:taskRequest,goals:[{id:"retention",statement:"The same task is readable after deregistration"}]},
    plan:planFor([{goal_ids:["retention"],id:"human",kind:"human",rationale:"Inspect retained task identity",conditions:"Same current artifact and original workspace"}]),
  });
  assert.equal(opened.ok, true);
  const taskBefore = taskIdentity(opened.result.task);
  assert.equal(opened.result.task.host, "codex");
  await firstCore.close();

  const dataBeforeRemoval = await directoryManifest(dataDirectory);
  assert.equal(dataBeforeRemoval.files.some((entry) => entry.path === "taskbelay.db"), true);
  const removed = await lifecycle.removeRegistration({
    paths,
    packageVersion: build.package_version,
    codexExecutable: fakeCodexPath,
    environment,
  });
  assert.deepEqual(removed, { status: "removed", changed: true });
  assert.equal(await optionalContents(paths.receiptPath), null);
  assert.equal(await readFile(prepared.receipt_path, "utf8"), provisioningReceiptBefore);
  assert.equal(await readFile(adjacentFile, "utf8"), "preserve adjacent data\n");
  assert.equal(await readFile(codexAdjacentFile, "utf8"), "preserve Codex-adjacent data\n");
  assert.equal(await readFile(unrelatedConfig, "utf8"), "model = \"unrelated-user-choice\"\n");
  assert.equal(await readFile(unrelatedReceipt, "utf8"), "{\"owner\":\"other-product\"}\n");
  assert.equal(await readFile(paths.configurationPath, "utf8"), configurationBefore);
  assert.deepEqual(await directoryManifest(unrelatedMarketplaceRoot), unrelatedMarketplaceBefore);
  assert.deepEqual(JSON.parse(await readFile(fakeState, "utf8")), {
    marketplaces: [unrelatedMarketplace],
    plugins: [unrelatedPlugin],
  });
  assert.deepEqual(await directoryManifest(dataDirectory), dataBeforeRemoval);
  assert.deepEqual(await directoryManifest(targetRepository), repositoryBefore);
  assert.deepEqual(await directoryManifest(taskRepository), taskRepositoryBefore);

  const reopenedCore = await startCore(paths.runtimePath, dataDirectory, taskRepository);
  await reopenedCore.callTool("taskbelay_server_info", {});
  const reopened = await reopenedCore.callTool("taskbelay_get_task", {
    refresh:false,
    task_id: taskBefore.task_id,
  });
  assert.equal(reopened.ok, true);
  assert.deepEqual(taskIdentity(reopened.result.task), taskBefore);
  await reopenedCore.close();
  const dataBeforeUninstall = await directoryManifest(dataDirectory);

  assert.deepEqual(await lifecycle.removeRegistration({
    paths,
    packageVersion: build.package_version,
    codexExecutable: fakeCodexPath,
    environment,
  }), { status: "already-absent", changed: false });

  await uninstallPackage(installPrefix);
  await assert.rejects(stat(installedPackage), { code: "ENOENT" });
  assert.deepEqual(await directoryManifest(dataDirectory), dataBeforeUninstall);
  assert.deepEqual(await directoryManifest(targetRepository), repositoryBefore);
  assert.equal(await readFile(prepared.receipt_path, "utf8"), provisioningReceiptBefore);
  assert.equal(await readFile(adjacentFile, "utf8"), "preserve adjacent data\n");
  assert.equal(await readFile(codexAdjacentFile, "utf8"), "preserve Codex-adjacent data\n");
  assert.equal(await readFile(unrelatedConfig, "utf8"), "model = \"unrelated-user-choice\"\n");
  assert.equal(await readFile(unrelatedReceipt, "utf8"), "{\"owner\":\"other-product\"}\n");
  assert.equal(await readFile(paths.configurationPath, "utf8"), configurationBefore);
  assert.deepEqual(await directoryManifest(unrelatedMarketplaceRoot), unrelatedMarketplaceBefore);
  assert.deepEqual(JSON.parse(await readFile(fakeState, "utf8")), {
    marketplaces: [unrelatedMarketplace],
    plugins: [unrelatedPlugin],
  });

  await installArtifact(build.artifact_path, reinstallPrefix);
  const reinstalledPackage = await realpath(join(reinstallPrefix, "node_modules", "taskbelay-codex"));
  assert.notEqual(reinstalledPackage, installedPackage);
  const reinstalledPaths = productPaths(reinstalledPackage, isolatedHome, dataDirectory);
  const finalCore = await startCore(reinstalledPaths.runtimePath, dataDirectory, taskRepository);
  await finalCore.callTool("taskbelay_server_info", {});
  const retained = await finalCore.callTool("taskbelay_get_task", {
    refresh:false,
    task_id: taskBefore.task_id,
  });
  assert.deepEqual(taskIdentity(retained.result.task), taskBefore);
  const repeatedRead = await finalCore.callTool("taskbelay_get_task", {
    refresh:false,
    task_id: taskBefore.task_id,
  });
  assert.deepEqual(taskIdentity(repeatedRead.result.task), taskBefore);
  await finalCore.close();
  assert.equal(retained.result.task.status, "active");

  await uninstallPackage(reinstallPrefix);
  await assert.rejects(stat(reinstalledPackage), { code: "ENOENT" });

  const fakeCalls = (await readFile(fakeTrace, "utf8")).trim().split("\n").filter(Boolean).map(JSON.parse);
  assert.equal(fakeCalls.length > 0, true);
  assert.equal(fakeCalls.every((entry) => entry.argv[0] === "--version" || entry.argv[0] === "plugin"), true);
  assert.deepEqual(await directoryManifest(targetRepository), repositoryBefore);
  assert.deepEqual(await directoryManifest(taskRepository), taskRepositoryBefore);
  assert.equal(await readFile(adjacentFile, "utf8"), "preserve adjacent data\n");
  assert.equal(await readFile(codexAdjacentFile, "utf8"), "preserve Codex-adjacent data\n");
  assert.equal(await readFile(unrelatedConfig, "utf8"), "model = \"unrelated-user-choice\"\n");
  assert.equal(await readFile(unrelatedReceipt, "utf8"), "{\"owner\":\"other-product\"}\n");
  assert.deepEqual(await directoryManifest(unrelatedMarketplaceRoot), unrelatedMarketplaceBefore);
  assert.deepEqual(JSON.parse(await readFile(fakeState, "utf8")), {
    marketplaces: [unrelatedMarketplace],
    plugins: [unrelatedPlugin],
  });
});

class CoreClient {
  static async start(runtimePath, dataDirectory, repositoryPath) {
    const client = new CoreClient(runtimePath, dataDirectory, repositoryPath);
    await client.initialize();
    return client;
  }

  constructor(runtimePath, dataDirectory, repositoryPath) {
    this.nextID = 1;
    this.pending = new Map();
    this.stderr = "";
    this.exited = false;
    this.child = spawn(runtimePath, ["mcp", "--stdio"], {
      cwd: repositoryPath,
      env: { ...process.env, TASKBELAY_DATA_DIR: dataDirectory },
      stdio: ["pipe", "pipe", "pipe"],
    });
    this.exitPromise = new Promise((resolve) => {
      this.child.once("exit", (code, signal) => {
        this.exited = true;
        const error = new Error(`packaged Core exited (${code ?? signal})${this.stderr ? `: ${this.stderr.trim()}` : ""}`);
        for (const { reject, timer } of this.pending.values()) {
          clearTimeout(timer);
          reject(error);
        }
        this.pending.clear();
        resolve({ code, signal });
      });
    });
    this.child.stderr.setEncoding("utf8");
    this.child.stderr.on("data", (chunk) => {
      this.stderr += chunk;
    });
    const lines = createInterface({ input: this.child.stdout, crlfDelay: Infinity });
    lines.on("line", (line) => {
      const response = JSON.parse(line);
      const pending = this.pending.get(response.id);
      if (!pending) return;
      clearTimeout(pending.timer);
      this.pending.delete(response.id);
      pending.resolve(response);
    });
  }

  async initialize() {
    const response = await this.request("initialize", {
      protocolVersion: "2025-06-18",
      capabilities: {},
      clientInfo: { name: "taskbelay-retention-test", version: "0.1.0" },
    });
    assert.equal(response.result.serverInfo.name, "taskbelay");
    this.notify("notifications/initialized", {});
  }

  async callTool(name, arguments_) {
    const response = await this.request("tools/call", { name, arguments: arguments_ });
    if (response.error) throw new Error(`Core RPC error ${response.error.code}: ${response.error.message}`);
    const result = response.result;
    assert.equal(result?.content?.[0]?.type, "text", `tool ${name} returned no complete text result`);
    const decoded = JSON.parse(result.content[0].text);
    if (result.structuredContent !== undefined) assert.deepEqual(decoded, result.structuredContent);
    return decoded;
  }

  request(method, params) {
    const id = this.nextID;
    this.nextID += 1;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`packaged Core request timed out: ${method}${this.stderr ? `: ${this.stderr.trim()}` : ""}`));
      }, 5_000);
      this.pending.set(id, { resolve, reject, timer });
      this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
    });
  }

  notify(method, params) {
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`);
  }

  async close() {
    if (!this.exited) this.child.stdin.end();
    const result = await Promise.race([
      this.exitPromise,
      new Promise((_, reject) => setTimeout(() => reject(new Error("packaged Core did not stop after EOF")), 5_000)),
    ]);
    assert.deepEqual(result, { code: 0, signal: null }, this.stderr);
  }

  async dispose() {
    if (this.exited) return;
    this.child.stdin.end();
    const stopped = await Promise.race([
      this.exitPromise.then(() => true),
      new Promise((resolve) => setTimeout(() => resolve(false), 1_000)),
    ]);
    if (!stopped) {
      this.child.kill("SIGTERM");
      await this.exitPromise;
    }
  }
}

async function initializeRepository(path, remotePath) {
  await execFile("git", ["init", "--bare", "--initial-branch=main", remotePath]);
  await mkdir(path, { recursive: true });
  await execFile("git", ["init", "--initial-branch=main"], { cwd: path });
  await execFile("git", ["config", "user.name", "TaskBelay Retention Test"], { cwd: path });
  await execFile("git", ["config", "user.email", "retention@example.invalid"], { cwd: path });
  await writeFile(join(path, "README.md"), "retention fixture\n");
  await execFile("git", ["add", "README.md"], { cwd: path });
  await execFile("git", ["commit", "-m", "retention fixture baseline"], { cwd: path });
  await execFile("git", ["remote", "add", "origin", remotePath], { cwd: path });
  await execFile("git", ["push", "-u", "origin", "main"], { cwd: path });
}

async function installArtifact(artifactPath, installPrefix) {
  await execFile("npm", [
    "install",
    "--ignore-scripts",
    "--no-audit",
    "--no-fund",
    "--prefix",
    installPrefix,
    artifactPath,
  ], { encoding: "utf8", maxBuffer: 20 * 1024 * 1024 });
}

async function uninstallPackage(installPrefix) {
  await execFile("npm", [
    "uninstall",
    "--ignore-scripts",
    "--no-audit",
    "--no-fund",
    "--prefix",
    installPrefix,
    "taskbelay-codex",
  ], { encoding: "utf8", maxBuffer: 20 * 1024 * 1024 });
}

async function importInstalledLifecycle(installedPackage, cacheKey) {
  return await importInstalledModule(installedPackage, "lib/lifecycle.mjs", cacheKey);
}

async function importInstalledModule(installedPackage, relativePath, cacheKey) {
  const url = pathToFileURL(join(installedPackage, relativePath));
  url.searchParams.set("retention", cacheKey);
  return import(url.href);
}

function productPaths(installedPackage, isolatedHome, dataDirectory) {
  const productSupportRoot = join(isolatedHome, "Library", "Application Support", "taskbelay");
  return {
    packageRoot: installedPackage,
    marketplaceRoot: installedPackage,
    pluginRoot: join(installedPackage, "plugin"),
    runtimePath: join(installedPackage, "runtime", "darwin-arm64", "taskbelay"),
    homeDirectory: isolatedHome,
    productSupportRoot,
    registrationsDirectory: join(productSupportRoot, "registrations"),
    receiptPath: join(productSupportRoot, "registrations", "codex.json"),
    configurationDirectory: join(isolatedHome, ".taskbelay"),
    configurationPath: join(isolatedHome, ".taskbelay", "config.json"),
    dataDirectory,
    usesDefaultDataDirectory: false,
    runtimeKey: "darwin-arm64",
  };
}

function taskIdentity(task) {
  const {task_id, revision, host, status, authorization, plan, checks, progress, evidence, acceptance} = task;
  return {task_id, revision, host, status, authorization, plan, checks, progress, evidence, acceptance};
}

function fakeEnvironment(installPrefix, statePath, tracePath) {
  return {
    ...process.env,
    PATH: `${join(installPrefix, "node_modules", ".bin")}:${process.env.PATH ?? ""}`,
    FAKE_CODEX_STATE: statePath,
    FAKE_CODEX_TRACE: tracePath,
    FAKE_CODEX_VERSION: "0.147.0",
  };
}

async function directoryManifest(root) {
  const files = [];
  async function visit(directory) {
    const entries = await readdir(directory, { withFileTypes: true });
    entries.sort((left, right) => Buffer.from(left.name).compare(Buffer.from(right.name)));
    for (const entry of entries) {
      const absolute = join(directory, entry.name);
      if (entry.isDirectory()) {
        await visit(absolute);
      } else {
        const contents = await readFile(absolute);
        files.push({
          path: relative(root, absolute).split("\\").join("/"),
          sha256: createHash("sha256").update(contents).digest("hex"),
        });
      }
    }
  }
  await visit(root);
  const canonical = files.map((entry) => `${entry.sha256}  ${entry.path}\n`).join("");
  return {
    files,
    sha256: createHash("sha256").update(canonical).digest("hex"),
  };
}

async function optionalContents(path) {
  try {
    return await readFile(path, "utf8");
  } catch (error) {
    if (error?.code === "ENOENT") return null;
    throw error;
  }
}
