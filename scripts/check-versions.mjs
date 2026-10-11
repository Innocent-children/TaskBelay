#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const SEMVER = /^(0|[1-9][0-9]*)(?:\.(0|[1-9][0-9]*)){2}(?:-(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/u;

export async function checkVersions(root = repositoryRoot()) {
  const coreVersion = await readVersionFile(join(root, "CORE_VERSION"), "Core version");
  const [rootPackage, codexPackage, codexPlugin, deepseekPackage, taskBelayPackage] = await Promise.all([
    readJSON(join(root, "package.json")),
    readJSON(join(root, "packages/codex/package.json")),
    readJSON(join(root, "packages/codex/plugin/.codex-plugin/plugin.json")),
    readJSON(join(root, "packages/deepseek/package.json")),
    readJSON(join(root, "packages/taskbelay/package.json")),
  ]);

  if (rootPackage.name !== "taskbelay" || rootPackage.private !== true || Object.hasOwn(rootPackage, "version")) {
    throw new Error("root package must be private and must not declare a version");
  }
  const codexVersion = packageVersion(codexPackage, "taskbelay-codex", "Codex package");
  const pluginVersion = packageVersion(codexPlugin, "taskbelay-codex", "Codex plugin");
  const deepseekVersion = packageVersion(deepseekPackage, "taskbelay-deepseek", "DeepSeek package");
  const taskBelayVersion = packageVersion(taskBelayPackage, "taskbelay", "TaskBelay CLI package");
  const claudeVersion = packageVersion(await readJSON(join(root, "packages/claude/package.json")), "taskbelay-claude", "Claude package");
  const zcodeVersion = packageVersion(await readJSON(join(root, "packages/zcode/package.json")), "taskbelay-zcode", "ZCode package");
  if (packageVersion(await readJSON(join(root, "packages/zcode/.zcode-plugin/plugin.json")), "taskbelay-zcode", "ZCode plugin") !== zcodeVersion) throw new Error("ZCode plugin version must equal ZCode package version");
  const zcodeMarket = await readJSON(join(root, "packages/zcode/marketplace.json"));
  if (zcodeMarket.name !== "taskbelay-zcode-local" || zcodeMarket.plugins?.length !== 1 || zcodeMarket.plugins[0].name !== "taskbelay-zcode" || zcodeMarket.plugins[0].version !== zcodeVersion) throw new Error("ZCode marketplace version must equal ZCode package version");
  if (packageVersion(await readJSON(join(root, "packages/claude/.claude-plugin/plugin.json")), "taskbelay-claude", "Claude plugin") !== claudeVersion) throw new Error("Claude plugin version must equal Claude package version");
  if (pluginVersion !== codexVersion) throw new Error("Codex plugin version must equal Codex package version");
  return Object.freeze({ core: coreVersion, codex: codexVersion, deepseek: deepseekVersion, claude: claudeVersion, zcode: zcodeVersion, taskBelay: taskBelayVersion });
}

async function readVersionFile(path, label) {
  const raw = await readFile(path, "utf8");
  const value = raw.endsWith("\r\n")
    ? raw.slice(0, -2)
    : raw.endsWith("\n")
      ? raw.slice(0, -1)
      : raw;
  if (value.includes("\n") || value.includes("\r") || !SEMVER.test(value)) {
    throw new Error(`${label} must be valid SemVer`);
  }
  return value;
}

function packageVersion(manifest, expectedName, label) {
  if (manifest?.name !== expectedName || !SEMVER.test(manifest.version ?? "")) {
    throw new Error(`${label} identity/version is invalid`);
  }
  return manifest.version;
}

async function readJSON(path) {
  return JSON.parse(await readFile(path, "utf8"));
}

function repositoryRoot() {
  return dirname(dirname(fileURLToPath(import.meta.url)));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    const versions = await checkVersions();
    process.stdout.write(`Core ${versions.core}\nCodex ${versions.codex}\nDeepSeek ${versions.deepseek}\n`);
    process.stdout.write(`Claude ${versions.claude}\nZCode ${versions.zcode}\nTaskBelay CLI ${versions.taskBelay}\n`);
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
