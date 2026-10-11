import assert from "node:assert/strict";
import { chmod, copyFile, cp, link, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  activateDeepSeekIntegration,
  apply,
  inject,
  name,
} from "../lib/index.mjs";
import {
  TASKBELAY_QUALIFIED_TOOL_NAMES,
  TASKBELAY_TOOL_NAMESPACE_PREFIX,
  assertQualifiedToolCatalog,
} from "../lib/tool-names.mjs";
import { WORKSPACE_COORDINATOR_TOOL } from "../lib/workspace-coordinator.mjs";

const sourcePackageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const repositoryRoot = dirname(dirname(sourcePackageRoot));
const currentVersion = (await readFile(join(repositoryRoot, "CORE_VERSION"), "utf8")).trim();
const fixturePlatform = process.platform === "win32" ? "win32" : "darwin";
const fixtureArch = process.platform === "win32" ? "x64" : "arm64";
const fixtureRuntimeKey = `${fixturePlatform}-${fixtureArch}`;
const fixtureExecutable = fixturePlatform === "win32" ? "taskbelay.exe" : "taskbelay";

test("plugin identity and injection surface are fixed", () => {
  assert.equal(name, "taskbelay-deepseek");
  assert.deepEqual(inject, ["skills", "tools"]);
  assert.equal(typeof apply, "function");
  assert.equal(TASKBELAY_TOOL_NAMESPACE_PREFIX, "mcp__taskbelay__");
  assert.equal(TASKBELAY_QUALIFIED_TOOL_NAMES.length,12);
  assert.ok(TASKBELAY_QUALIFIED_TOOL_NAMES.includes("mcp__taskbelay__taskbelay_run_check"));
});

test("registers one model-visible assessment Skill, guarded workspace coordinator, and the official MCP child config", async (t) => {
  const dataDirectory = await temporaryDirectory(t, "data");
  const packageRoot = await temporaryPackage(t, "integration package-工具");
  const fake = createFakeContext({ packageRoot, initialToolNames: TASKBELAY_QUALIFIED_TOOL_NAMES });

  await activateDeepSeekIntegration(fake.ctx, {
    packageRoot,
    platform: fixturePlatform,
    arch: fixtureArch,
    environment: { TASKBELAY_DATA_DIR: dataDirectory },
  });

  assert.equal(fake.skills.length, 1);
  assert.deepEqual(fake.skills[0].invocation, {
    modelInvocable: true,
    userInvocable: true,
  });
  assert.equal(fake.skills[0].name, "taskbelay");
  assert.equal(fake.skills[0].provider, "taskbelay-deepseek");
  assert.equal(fake.skills[0].resourceBase.kind, "directory");
  assert.match(fake.skills[0].content, /# TaskBelay/u);
  assert.equal(fake.guards.length, 2);
  assert.equal(fake.children.length, 1);
  assert.equal(fake.children[0].plugin.name, "mcp-client");
  assert.deepEqual(fake.children[0].config, {
    transport: "stdio",
    serverName: "taskbelay",
    command: join(fake.packageRoot, "runtime", fixtureRuntimeKey, fixtureExecutable),
    args: ["mcp", "--stdio"],
    env: { TASKBELAY_DATA_DIR: dataDirectory },
    cwd: fake.packageRoot,
    toolCallTimeoutMs: 3_660_000,
    failOnStartupError: true,
    reconnect: {
      enabled: true,
      initialDelayMs: 500,
      maxDelayMs: 30_000,
      maxAttempts: 10,
    },
  });
  assert.deepEqual(assertQualifiedToolCatalog(fake.toolNames()), TASKBELAY_QUALIFIED_TOOL_NAMES);
  assert.equal(fake.toolNames().includes(WORKSPACE_COORDINATOR_TOOL), true);

  await fake.dispose();
  assert.equal(fake.skills.length, 0);
  assert.equal(fake.guards.length, 0);
  assert.equal(fake.toolNames().some((toolName) => toolName.startsWith("mcp__taskbelay__")), false);
  assert.equal(fake.children[0].disposed, true);
});

test("ordinary host tools remain executable and reconnect restores the exact catalog", async (t) => {
  const dataDirectory = await temporaryDirectory(t, "reconnect-data");
  const packageRoot = await temporaryPackage(t, "reconnect-package");
  const fake = createFakeContext({ packageRoot, initialToolNames: [], unrelatedToolNames: ["read_file"] });
  await activateDeepSeekIntegration(fake.ctx, {
    packageRoot,
    platform: fixturePlatform,
    arch: fixtureArch,
    environment: { TASKBELAY_DATA_DIR: dataDirectory },
  });

  let unrelatedDispatches = 0;
  const unrelatedDenial = fake.guards
    .map((guard) => guard({ name: "read_file" }))
    .find((reason) => reason !== undefined);
  if (unrelatedDenial === undefined) unrelatedDispatches += 1;
  assert.equal(unrelatedDispatches, 1);

  fake.replaceMcpCatalog(TASKBELAY_QUALIFIED_TOOL_NAMES);
  await nextMicrotask();
  assert.deepEqual(assertQualifiedToolCatalog(fake.toolNames()), TASKBELAY_QUALIFIED_TOOL_NAMES);
  assert.equal(fake.children[0].disposed, false);
});

test("missing or extra connected namespace tools fail compatibility and dispose the MCP child", async (t) => {
  const dataDirectory = await temporaryDirectory(t, "catalog-data");
  const packageRoot = await temporaryPackage(t, "catalog-package");
  const fake = createFakeContext({ packageRoot, initialToolNames: TASKBELAY_QUALIFIED_TOOL_NAMES });
  await activateDeepSeekIntegration(fake.ctx, {
    packageRoot,
    platform: fixturePlatform,
    arch: fixtureArch,
    environment: { TASKBELAY_DATA_DIR: dataDirectory },
  });

  fake.replaceMcpCatalog(TASKBELAY_QUALIFIED_TOOL_NAMES.slice(0, 5));
  await nextMicrotask();
  assert.equal(fake.children[0].disposed, true);
  assert.equal(fake.toolNames().some((toolName) => toolName.startsWith(TASKBELAY_TOOL_NAMESPACE_PREFIX)), false);
  assert.equal(fake.errors.some((message) => message.includes("catalog mismatch")), true);

  assert.throws(
    () => assertQualifiedToolCatalog([...TASKBELAY_QUALIFIED_TOOL_NAMES, "mcp__taskbelay__future_tool"]),
    /catalog mismatch/,
  );
});

test("preflight failure contributes no Skill, guard, or MCP child", async (t) => {
  const dataDirectory = await temporaryDirectory(t, "unsupported-data");
  const packageRoot = await temporaryPackage(t, "unsupported-package");
  const fake = createFakeContext({ packageRoot, initialToolNames: TASKBELAY_QUALIFIED_TOOL_NAMES });
  await assert.rejects(
    activateDeepSeekIntegration(fake.ctx, {
      packageRoot,
      environment: { TASKBELAY_DATA_DIR: dataDirectory },
      platform: "linux",
      arch: "arm64",
    }),
    /unsupported platform/,
  );
  assert.equal(fake.skills.length, 0);
  assert.equal(fake.guards.length, 0);
  assert.equal(fake.children.length, 0);
});

function createFakeContext({ packageRoot, initialToolNames, unrelatedToolNames = [] }) {
  const skills = [];
  const guards = [];
  const listeners = [];
  const preExecuteListeners = [];
  const effects = [];
  const children = [];
  const errors = [];
  const definitions = new Set(unrelatedToolNames);
  let mcpNames = new Set();
  const emitToolsChange = () => {
    for (const listener of [...listeners]) listener();
  };
  const replaceMcpCatalog = (names) => {
    for (const toolName of mcpNames) definitions.delete(toolName);
    mcpNames = new Set(names);
    for (const toolName of mcpNames) definitions.add(toolName);
    emitToolsChange();
  };

  const ctx = {
    skills: {
      register(skill) {
        skills.push(skill);
        const dispose = () => {
          const index = skills.indexOf(skill);
          if (index >= 0) skills.splice(index, 1);
        };
        effects.push(dispose);
        return dispose;
      },
    },
    tools: {
      register(definition) {
        assert.equal(typeof definition?.name, "string");
        definitions.add(definition.name);
        const dispose = () => definitions.delete(definition.name);
        effects.push(dispose);
        return dispose;
      },
      guard(guard) {
        guards.push(guard);
        const dispose = () => {
          const index = guards.indexOf(guard);
          if (index >= 0) guards.splice(index, 1);
        };
        effects.push(dispose);
        return dispose;
      },
      schemas() {
        return [...definitions].map((toolName) => ({ name: toolName }));
      },
    },
    on(eventName, listener) {
      assert.ok(eventName === "tools/change" || eventName === "tools/pre-execute");
      const target = eventName === "tools/change" ? listeners : preExecuteListeners;
      target.push(listener);
      const dispose = () => {
        const index = target.indexOf(listener);
        if (index >= 0) target.splice(index, 1);
      };
      effects.push(dispose);
      return dispose;
    },
    async plugin(plugin, config) {
      const child = {
        plugin,
        config,
        disposed: false,
        async dispose() {
          if (child.disposed) return;
          child.disposed = true;
          replaceMcpCatalog([]);
        },
      };
      children.push(child);
      effects.push(() => child.dispose());
      replaceMcpCatalog(initialToolNames);
      return child;
    },
    logger: {
      error(message) { errors.push(String(message)); },
    },
  };

  return {
    ctx,
    packageRoot,
    skills,
    guards,
    children,
    errors,
    replaceMcpCatalog,
    toolNames: () => [...definitions],
    async dispose() {
      for (const dispose of effects.reverse()) await dispose();
    },
  };
}

async function temporaryDirectory(t, name) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-integration-")));
  const directory = join(root, name);
  await mkdir(directory, { recursive: true });
  t.after(() => rm(root, { recursive: true, force: true }));
  return directory;
}

async function temporaryPackage(t, name) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-package-")));
  const packageRoot = join(root, name);
  const runtimePath = join(packageRoot, "runtime", fixtureRuntimeKey, fixtureExecutable);
  const skillRoot = join(packageRoot, "skills", "taskbelay");
  await mkdir(dirname(runtimePath), { recursive: true });
  await mkdir(join(skillRoot, "references"), { recursive: true });
  await writeFile(join(packageRoot, "package.json"), `${JSON.stringify({
    name: "taskbelay-deepseek",
    version: currentVersion,
  })}\n`);
  await copyFile(join(sourcePackageRoot, "skills", "taskbelay", "SKILL.md"), join(skillRoot, "SKILL.md"));
  for (const directory of ["references", "scripts"]) {
    await cp(join(sourcePackageRoot, "skills", "taskbelay", directory), join(skillRoot, directory), { recursive: true });
  }
  if (fixturePlatform === "win32") {
    try {
      await link(process.execPath, runtimePath);
    } catch {
      await copyFile(process.execPath, runtimePath);
    }
    await writeFile(
      join(dirname(runtimePath), "version"),
      `process.stdout.write('taskbelay ${currentVersion}\\n');\n`,
      "utf8",
    );
  } else {
    await writeFile(runtimePath, [
      "#!/bin/sh",
      "if [ \"$1\" = \"version\" ]; then",
      `  printf 'taskbelay ${currentVersion}\\n'`,
      "  exit 0",
      "fi",
      "exit 1",
      "",
    ].join("\n"));
    await chmod(runtimePath, 0o755);
  }
  t.after(() => rm(root, { recursive: true, force: true }));
  return packageRoot;
}

async function nextMicrotask() {
  await new Promise((resolve) => setImmediate(resolve));
}
