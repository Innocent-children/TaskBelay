import { createHash, randomBytes } from "node:crypto";
import { constants as fsConstants } from "node:fs";
import {
  access,
  chmod,
  lstat,
  mkdir,
  readFile,
  realpath,
  rename,
  rm,
  stat,
  unlink,
  writeFile,
} from "node:fs/promises";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";

import { commandResolvesToPackage, execPortableCommand, findCommandPath } from "./command.mjs";
import { containedPath } from "./paths.mjs";
import { SUPPORTED_RUNTIME_KEYS, ensureCodexPetInstalled, registrationPolicy } from "./platform.mjs";

export const CODEX_COMPATIBILITY_RANGE = ">=0.147.0";
export const MARKETPLACE_NAME = "taskbelay-local";
export const PLUGIN_NAME = "taskbelay-codex";
export const PLUGIN_SELECTOR = `${PLUGIN_NAME}@${MARKETPLACE_NAME}`;

const MCP_SCHEMA_URI = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json";
const IMPLICIT_SKILL_POLICY = "policy:\n  allow_implicit_invocation: true";

export const CODEX_MCP_INSTRUCTIONS = "Use TaskBelay for substantial development needing saved progress, verification reuse or recovery. Honor explicit invocation or opt-out; skip small unrelated work. Start from the actual user request in the selected/current workspace without repeat plan approval. Adjust methods within existing authority; ask only for real user-boundary changes. Core owns one Task, actual results and cumulative cost. Native permissions apply. After response loss read the original task/run before repeating effects. Default responses are summaries; get_task detail:true and get_run provide details. Structured hooks cover declared paths, not all shell effects. See the TaskBelay Skill when using these tools.";

const semverPattern = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/;
const digestPattern = /^[0-9a-f]{64}$/;

export async function runCodexJSON(
  arguments_,
  {
    codexExecutable = "codex",
    environment = process.env,
    currentDirectory = process.cwd(),
  } = {},
) {
  if (!Array.isArray(arguments_) || arguments_.some((argument) => typeof argument !== "string" || argument.includes("\0"))) {
    throw new Error("Codex arguments must be a closed string array");
  }
  let stdout;
  try {
    ({ stdout } = await execPortableCommand(codexExecutable, arguments_, {
      cwd: currentDirectory,
      env: environment,
      encoding: "utf8",
      maxBuffer: 1024 * 1024,
      windowsHide: true,
    }));
  } catch (error) {
    const detail = String(error?.stderr ?? "").trim();
    throw new Error(
      `Codex command failed (${arguments_.join(" ")})${detail ? `: ${detail}` : ""}`,
      { cause: error },
    );
  }

  try {
    return JSON.parse(stdout);
  } catch (error) {
    throw new Error(`Codex command did not return valid JSON (${arguments_.join(" ")})`, {
      cause: error,
    });
  }
}

export async function inspectCoreVersion(
  runtimePath,
  {
    environment = process.env,
    currentDirectory = dirname(runtimePath),
    requireExecutableMode = true,
  } = {},
) {
  await assertExecutableFile(runtimePath, "packaged Core", requireExecutableMode);
  let stdout;
  try {
    ({ stdout } = await execPortableCommand(runtimePath, ["version"], {
      cwd: currentDirectory,
      env: environment,
      encoding: "utf8",
      maxBuffer: 64 * 1024,
      windowsHide: true,
    }));
  } catch (error) {
    throw new Error("packaged Core version preflight failed", { cause: error });
  }
  const match = /^taskbelay (\S+)\n?$/.exec(stdout);
  if (!match) throw new Error("packaged Core returned an invalid version line");
  parseSemver(match[1], "packaged Core version");
  return match[1];
}


export async function setupRegistration({
  paths,
  packageVersion,
  codexExecutable = "codex",
  environment = process.env,
  now = () => new Date(),
} = {}) {
  const preflight = await preflightSetup({
    paths,
    packageVersion,
    codexExecutable,
    environment,
  });
  const commandOptions = {
    codexExecutable,
    environment,
    currentDirectory: paths.packageRoot,
  };
  const existingReceipt = await readReceipt(paths.receiptPath);
  const initialState = await readRegistrationState(commandOptions);
  const expectedReceipt = createReceipt({
    paths,
    packageVersion: preflight.packageVersion,
    coreVersion: preflight.coreVersion,
    codexVersion: preflight.codexVersion,
    resourceDigests: preflight.resourceDigests,
    installedAt: now().toISOString(),
  });

  if (existingReceipt) {
    if (receiptOwnershipMatches(existingReceipt, expectedReceipt)) {
      assertMatchingRegistrationState(initialState, paths, preflight.packageVersion);
      await ensureCodexPetInstalled(paths).catch(() => {});
      return {
        status: "already-installed",
        changed: false,
        receipt: existingReceipt,
        fileChanges: [],
      };
    }

    assertCompatibleReceiptUpgrade(existingReceipt, expectedReceipt);
    const registrationMatchesPrevious = registrationStateMatches(
      initialState,
      paths,
      existingReceipt.product.version,
    );
    const registrationMatchesCurrent = registrationStateMatches(
      initialState,
      paths,
      preflight.packageVersion,
    );
    if (!registrationMatchesPrevious && !registrationMatchesCurrent) {
      throw new Error("registration state conflicts with the owned upgrade; setup made no changes");
    }
    if (registrationMatchesPrevious) {
      const pluginAddResult = await runCodexJSON(
        ["plugin", "add", PLUGIN_SELECTOR, "--json"],
        commandOptions,
      );
      assertPluginAddResult(pluginAddResult, paths, preflight.packageVersion);
      const finalState = await readRegistrationState(commandOptions);
      assertMatchingRegistrationState(finalState, paths, preflight.packageVersion);
    }
    await writeReceiptAtomic(paths.receiptPath, expectedReceipt, {
      ownedRoot: paths.productSupportRoot,
      enforcePrivateModes: paths.enforcePrivateModes,
    });
    await ensureCodexPetInstalled(paths).catch(() => {});
    return {
      status: "installed",
      changed: true,
      receipt: expectedReceipt,
      fileChanges: [{ path: paths.receiptPath, change: "updated" }],
    };
  }

  assertRegistrationAbsent(initialState, paths);
  let marketplaceCreated = false;
  try {
    const marketplaceAddResult = await runCodexJSON(
      ["plugin", "marketplace", "add", paths.marketplaceRoot, "--json"],
      commandOptions,
    );
    assertMarketplaceAddResult(marketplaceAddResult, paths);
    marketplaceCreated = true;
    const pluginAddResult = await runCodexJSON(
      ["plugin", "add", PLUGIN_SELECTOR, "--json"],
      commandOptions,
    );
    assertPluginAddResult(pluginAddResult, paths, preflight.packageVersion);
    const finalState = await readRegistrationState(commandOptions);
    assertMatchingRegistrationState(finalState, paths, preflight.packageVersion);
    await writeReceiptAtomic(paths.receiptPath, expectedReceipt, {
      ownedRoot: paths.productSupportRoot,
      enforcePrivateModes: paths.enforcePrivateModes,
    });
    await ensureCodexPetInstalled(paths).catch(() => {});
    return {
      status: "installed",
      changed: true,
      receipt: expectedReceipt,
      fileChanges: [{ path: paths.receiptPath, change: "created" }],
    };
  } catch (error) {
    if (marketplaceCreated) {
      const rollback = await rollbackCreatedMarketplace(paths, commandOptions);
      if (rollback.error) {
        throw new Error(
          `setup failed and bounded marketplace rollback could not complete: ${rollback.error.message}`,
          { cause: error },
        );
      }
    }
    throw error;
  }
}

export async function removeRegistration({
  paths,
  packageVersion,
  codexExecutable = "codex",
  environment = process.env,
} = {}) {
  const commandOptions = {
    codexExecutable,
    environment,
    currentDirectory: paths.packageRoot,
  };
  await assertNoSymbolicLinkComponents(paths.productSupportRoot, dirname(paths.receiptPath));
  await rejectSymbolicLink(paths.receiptPath);
  const receipt = await readReceipt(paths.receiptPath);
  let state = await readRegistrationState(commandOptions);

  if (!receipt) {
    assertRegistrationAbsent(state, paths);
    return { status: "already-absent", changed: false };
  }

  assertRemovalReceipt(receipt, paths);
  let owned = reconcileRemovalState(state, receipt);

  if (owned.plugin) {
    const pluginRemoveResult = await runCodexJSON(
      ["plugin", "remove", receipt.registration.plugin_selector, "--json"],
      commandOptions,
    );
    assertPluginRemoveResult(pluginRemoveResult, receipt.registration);
    state = await readRegistrationState(commandOptions);
    owned = reconcileRemovalState(state, receipt);
    if (owned.plugin) throw new Error("Codex plugin remains after removal readback");
  }

  if (owned.marketplace) {
    const marketplaceRemoveResult = await runCodexJSON(
      ["plugin", "marketplace", "remove", receipt.registration.marketplace_name, "--json"],
      commandOptions,
    );
    assertMarketplaceRemoveResult(
      marketplaceRemoveResult,
      receipt.registration.marketplace_name,
    );
    state = await readRegistrationState(commandOptions);
    owned = reconcileRemovalState(state, receipt);
    if (owned.marketplace) throw new Error("Codex marketplace remains after removal readback");
    if (owned.plugin) throw new Error("Codex plugin reappeared during marketplace removal");
  }

  try {
    await unlink(paths.receiptPath);
  } catch (error) {
    throw new Error(`delete exact registration receipt ${paths.receiptPath}: ${error.message}`, {
      cause: error,
    });
  }
  if (await readReceipt(paths.receiptPath)) {
    throw new Error("registration receipt remains after exact cleanup");
  }
  return { status: "removed", changed: true };
}

export async function inspectRegistrationStatus({
  paths,
  packageVersion,
  codexExecutable = "codex",
  environment = process.env,
} = {}) {
  const commandOptions = {
    codexExecutable,
    environment,
    currentDirectory: paths.packageRoot,
  };
  await assertNoSymbolicLinkComponents(paths.productSupportRoot, dirname(paths.receiptPath));
  await rejectSymbolicLink(paths.receiptPath);
  const receipt = await readReceipt(paths.receiptPath);
  const state = await readRegistrationState(commandOptions);

  if (!receipt) {
    assertRegistrationAbsent(state, paths);
    return {
      status: "absent",
      changed: false,
      receipt: false,
      marketplace: false,
      plugin: false,
    };
  }

  assertRemovalReceipt(receipt, paths);
  if (receipt.product.version !== packageVersion) {
    throw new Error("registration receipt version conflict; installed package differs from its receipt");
  }
  const owned = reconcileRemovalState(state, receipt);
  if (owned.marketplace && owned.plugin) {
    assertMatchingRegistrationState(state, paths, packageVersion);
    return {
      status: "ready",
      changed: false,
      receipt: true,
      marketplace: true,
      plugin: true,
    };
  }

  return {
    status: "partial",
    changed: false,
    receipt: true,
    marketplace: owned.marketplace !== null,
    plugin: owned.plugin !== null,
  };
}

async function preflightSetup({ paths, packageVersion, codexExecutable, environment }) {
  assertObject(paths, "product paths");
  if (!SUPPORTED_RUNTIME_KEYS.includes(paths.runtimeKey)) {
    throw new Error(`unsupported platform ${paths.runtimeKey ?? "unknown"}; supported runtimes: ${SUPPORTED_RUNTIME_KEYS.join(", ")}`);
  }
  parseSemver(packageVersion, "package version");
  await assertPackageResources(paths, packageVersion);
  const launcherPath = await assertExecutableOnPath(
    "taskbelay-codex",
    environment,
    paths.platform,
    paths.requireExecutableMode,
  );
  const expectedLauncherPath = join(paths.packageRoot, "bin", "taskbelay-codex.mjs");
  let launcherOwnedByPackage;
  try {
    launcherOwnedByPackage = await commandResolvesToPackage(launcherPath, expectedLauncherPath, {
      platform: paths.platform,
      packageName: PLUGIN_NAME,
    });
  } catch (error) {
    throw new Error("resolve the package-owned taskbelay-codex launcher on PATH", { cause: error });
  }
  if (!launcherOwnedByPackage) {
    throw new Error("taskbelay-codex on PATH does not resolve to this installed package");
  }
  const coreVersion = await inspectCoreVersion(paths.runtimePath, {
    environment,
    currentDirectory: paths.packageRoot,
    requireExecutableMode: paths.requireExecutableMode,
  });
  const codexVersion = await inspectCodexVersion(codexExecutable, {
    environment,
    currentDirectory: paths.packageRoot,
  });
  if (!versionSatisfiesRange(codexVersion)) {
    throw new Error(`Codex version ${codexVersion} does not satisfy ${CODEX_COMPATIBILITY_RANGE}`);
  }
  return {
    packageVersion,
    coreVersion,
    codexVersion,
    resourceDigests: await digestResources(resourcePaths(paths)),
  };
}

async function inspectCodexVersion(codexExecutable, { environment, currentDirectory }) {
  let stdout;
  try {
    ({ stdout } = await execPortableCommand(codexExecutable, ["--version"], {
      cwd: currentDirectory,
      env: environment,
      encoding: "utf8",
      maxBuffer: 64 * 1024,
      windowsHide: true,
    }));
  } catch (error) {
    throw new Error("Codex version preflight failed", { cause: error });
  }
  const match = /^codex(?:-cli)? (\S+)\n?$/.exec(stdout);
  if (!match) throw new Error("Codex returned an invalid version line");
  parseSemver(match[1], "Codex version");
  return match[1];
}

async function assertPackageResources(paths, packageVersion) {
  const packageManifest = await readJSON(join(paths.packageRoot, "package.json"), "package manifest");
  const privateContract = !Object.hasOwn(packageManifest, "private") || packageManifest.private === false;
  const platformContract = stableJSON(packageManifest.os) === stableJSON(["darwin", "win32"]) &&
    stableJSON(packageManifest.cpu) === stableJSON(["arm64", "x64"]);
  const publishContract = packageManifest.publishConfig?.access === "public" &&
    packageManifest.publishConfig?.registry === "https://registry.npmjs.org/" &&
    stableJSON(Object.keys(packageManifest.publishConfig).sort()) === stableJSON(["access", "registry"]);
  if (
    packageManifest.name !== PLUGIN_NAME ||
    !privateContract ||
    !platformContract ||
    !publishContract ||
    packageManifest.license !== "Apache-2.0"
  ) {
    throw new Error("package manifest does not satisfy the fixed public package contract");
  }
  if (packageManifest.version !== packageVersion) {
    throw new Error("package manifest version does not match the requested package version");
  }

  const marketplace = await readJSON(
    join(paths.marketplaceRoot, ".agents", "plugins", "marketplace.json"),
    "marketplace catalog",
  );
  if (marketplace.name !== MARKETPLACE_NAME || !Array.isArray(marketplace.plugins) || marketplace.plugins.length !== 1) {
    throw new Error("marketplace catalog must contain exactly the TaskBelay marketplace entry");
  }
  const marketplacePlugin = marketplace.plugins[0];
  if (
    marketplacePlugin?.name !== PLUGIN_NAME ||
    marketplacePlugin?.source?.source !== "local" ||
    marketplacePlugin?.source?.path !== "./plugin"
  ) {
    throw new Error("marketplace catalog has an invalid local plugin source");
  }

  const pluginManifest = await readJSON(
    join(paths.pluginRoot, ".codex-plugin", "plugin.json"),
    "plugin manifest",
  );
  if (
    pluginManifest.name !== PLUGIN_NAME ||
    pluginManifest.version !== packageVersion ||
    pluginManifest.skills !== "./skills/" ||
    pluginManifest.mcpServers !== "./.mcp.json"
    || pluginManifest.hooks !== "./hooks/hooks.json"
  ) {
    throw new Error("plugin manifest identity or resource paths do not match the package");
  }

  const mcpConfiguration = await readJSON(
    join(paths.pluginRoot, ".mcp.json"),
    "MCP configuration",
  );
  assertExactKeys(mcpConfiguration, ["$schema", "mcpServers"], "MCP configuration");
  if (mcpConfiguration.$schema !== MCP_SCHEMA_URI) {
    throw new Error(`MCP configuration schema must equal ${MCP_SCHEMA_URI}`);
  }

  const hooksConfiguration = await readJSON(
    join(paths.pluginRoot, "hooks", "hooks.json"),
    "Plugin hooks configuration",
  );
  assertObject(hooksConfiguration.hooks, "Plugin hooks");
  const preToolUse = hooksConfiguration.hooks.PreToolUse;
  if (!Array.isArray(preToolUse) || preToolUse.length !== 1 || preToolUse[0]?.matcher !== "^apply_patch$" ||
      !Array.isArray(preToolUse[0]?.hooks) || preToolUse[0].hooks.length !== 1 ||
      preToolUse[0].hooks[0]?.type !== "command" ||
      preToolUse[0].hooks[0]?.command !== "taskbelay-codex hook pre-tool-use") {
    throw new Error("Plugin PreToolUse hook does not match the fixed apply_patch scope check");
  }
  await assertReadableFile(join(paths.pluginRoot, "hooks", "pre-tool-use.mjs"), "Plugin PreToolUse hook");
  assertObject(mcpConfiguration.mcpServers, "MCP servers");
  assertExactKeys(mcpConfiguration.mcpServers, ["taskbelay"], "MCP servers");
  const server = mcpConfiguration.mcpServers["taskbelay"];
  assertObject(server, "TaskBelay MCP server");
  assertExactKeys(server, ["type", "command", "args", "env_vars"], "TaskBelay MCP server");
  if (
    server.type !== "stdio" ||
    server.command !== "taskbelay-codex" ||
    stableJSON(server.args) !== stableJSON(["mcp"])
  ) {
    throw new Error("TaskBelay MCP server must invoke exactly taskbelay-codex mcp");
  }
  if (stableJSON(server.env_vars) !== stableJSON(["TASKBELAY_DATA_DIR"])) {
    throw new Error("TaskBelay MCP server must forward exactly TASKBELAY_DATA_DIR");
  }

  const skillPath = join(paths.pluginRoot, "skills", "taskbelay", "SKILL.md");
  let skill;
  try {
    skill = await readFile(skillPath, "utf8");
  } catch (error) {
    throw new Error("TaskBelay Skill is unavailable", { cause: error });
  }
  if (skill.trim() === "") throw new Error("TaskBelay Skill must be non-empty");
  const normalizedSkill = skill.replace(/\r\n?/gu, "\n");
  const skillFrontmatter = normalizedSkill.match(/^---\n([\s\S]*?)\n---\n/)?.[1] ?? "";
  if (/^allow_implicit_invocation\s*:/m.test(skillFrontmatter)) {
    throw new Error("TaskBelay Skill frontmatter must not carry Codex invocation policy");
  }
  const skillDescription = skillFrontmatter.match(/^description:\s*"([^"]+)"$/m)?.[1] ?? "";
  if (!/^name:\s*taskbelay\s*$/m.test(skillFrontmatter) || skillDescription.trim() === "") {
    throw new Error("TaskBelay Skill must have name taskbelay and a non-empty description");
  }
  await assertSkillReferences(dirname(skillPath), skillPath, normalizedSkill);

  let skillMetadata;
  try {
    skillMetadata = await readFile(
      join(paths.pluginRoot, "skills", "taskbelay", "agents", "openai.yaml"),
      "utf8",
    );
  } catch (error) {
    throw new Error("TaskBelay implicit Skill policy is unavailable", { cause: error });
  }
  if (skillMetadata.replace(/\r\n?/gu, "\n").trim() !== IMPLICIT_SKILL_POLICY) {
    throw new Error("TaskBelay implicit Skill policy must enable implicit invocation");
  }
}

// Registration checks that routed instructions are readable; contract tests own
// example validation and workflow semantics remain in Core.
async function assertSkillReferences(skillRoot, entryPath, entryText) {
  const pending = [{ path: entryPath, text: entryText }];
  const visited = new Set();
  const canonicalRoot = await realpath(skillRoot);
  while (pending.length > 0) {
    const current = pending.pop();
    if (visited.has(current.path)) continue;
    visited.add(current.path);
    for (const match of current.text.matchAll(/\[[^\]]*\]\(([^)\s]+\.md)(?:#[^)]*)?\)/gu)) {
      if (/^[a-z][a-z0-9+.-]*:/iu.test(match[1])) continue;
      const path = containedPath(skillRoot, resolve(dirname(current.path), match[1]), "Skill reference");
      await assertReadableFile(path, "TaskBelay Skill reference");
      containedPath(canonicalRoot, await realpath(path), "Skill reference");
      if (visited.has(path)) continue;
      const text = await readFile(path, "utf8");
      if (text.trim() === "") throw new Error("TaskBelay Skill reference must be non-empty");
      pending.push({ path, text });
    }
  }
}

async function readRegistrationState(commandOptions) {
  const marketplaceResponse = await runCodexJSON(
    ["plugin", "marketplace", "list", "--json"],
    commandOptions,
  );
  const pluginResponse = await runCodexJSON(["plugin", "list", "--json"], commandOptions);
  assertObject(marketplaceResponse, "Codex marketplace readback");
  assertExactKeys(marketplaceResponse, ["marketplaces"], "Codex marketplace readback");
  if (!Array.isArray(marketplaceResponse.marketplaces)) {
    throw new Error("Codex marketplace readback marketplaces must be an array");
  }
  assertObject(pluginResponse, "Codex plugin readback");
  assertExactKeys(pluginResponse, ["installed", "available"], "Codex plugin readback");
  if (!Array.isArray(pluginResponse.installed) || !Array.isArray(pluginResponse.available)) {
    throw new Error("Codex plugin readback installed and available must be arrays");
  }
  if (pluginResponse.available.length !== 0) {
    throw new Error("Codex plugin readback available must be empty without --available");
  }
  return { marketplaces: marketplaceResponse.marketplaces, plugins: pluginResponse.installed };
}

function assertRegistrationAbsent(state, paths) {
  const marketplaceCollision = state.marketplaces.find(
    (entry) =>
      entry?.name === MARKETPLACE_NAME ||
      entry?.root === paths.marketplaceRoot ||
      entry?.marketplaceSource?.source === paths.marketplaceRoot,
  );
  if (marketplaceCollision) {
    throw new Error("marketplace ownership conflict without a matching registration receipt");
  }
  const pluginCollision = state.plugins.find(
    (entry) =>
      entry?.name === PLUGIN_NAME ||
      entry?.pluginId === PLUGIN_SELECTOR ||
      entry?.source?.path === paths.pluginRoot,
  );
  if (pluginCollision) {
    throw new Error("plugin ownership conflict without a matching registration receipt");
  }
}

function assertRemovalReceipt(receipt, paths) {
  const matches =
    receipt.registration.marketplace_name === MARKETPLACE_NAME &&
    receipt.registration.marketplace_root === paths.marketplaceRoot &&
    receipt.registration.plugin_name === PLUGIN_NAME &&
    receipt.registration.plugin_selector === PLUGIN_SELECTOR &&
    receipt.registration.plugin_root === paths.pluginRoot &&
    receipt.paths.package_root === paths.packageRoot &&
    receipt.paths.runtime_path === paths.runtimePath &&
    receipt.paths.data_dir === paths.dataDirectory &&
    receipt.paths.receipt_path === paths.receiptPath;
  if (!matches) {
    throw new Error("registration receipt ownership conflict; removal made no changes");
  }
}

function reconcileRemovalState(state, receipt) {
  const matchingMarketplaces = state.marketplaces.filter(
    (entry) =>
      entry?.name === receipt.registration.marketplace_name ||
      entry?.root === receipt.registration.marketplace_root ||
      entry?.marketplaceSource?.source === receipt.registration.marketplace_root,
  );
  if (matchingMarketplaces.length > 1) {
    throw new Error("Codex marketplace ownership conflict during removal");
  }
  const marketplace = matchingMarketplaces[0] ?? null;
  if (marketplace) {
    assertMarketplaceReadback(
      marketplace,
      receipt.registration.marketplace_name,
      receipt.registration.marketplace_root,
      "marketplace removal readback",
    );
  }

  const matchingPlugins = state.plugins.filter(
    (entry) =>
      entry?.name === receipt.registration.plugin_name ||
      entry?.pluginId === receipt.registration.plugin_selector ||
      entry?.source?.path === receipt.registration.plugin_root,
  );
  if (matchingPlugins.length > 1) {
    throw new Error("Codex plugin ownership conflict during removal");
  }
  const plugin = matchingPlugins[0] ?? null;
  if (plugin) {
    assertPluginReadback(
      plugin,
      {
        pluginId: receipt.registration.plugin_selector,
        name: receipt.registration.plugin_name,
        marketplaceName: receipt.registration.marketplace_name,
        pluginRoot: receipt.registration.plugin_root,
        marketplaceRoot: receipt.registration.marketplace_root,
        requireEnabled: false,
      },
      "plugin removal readback",
    );
  }

  return { marketplace, plugin };
}

function assertMatchingRegistrationState(state, paths, packageVersion) {
  const matchingMarketplaces = state.marketplaces.filter(
    (entry) =>
      entry?.name === MARKETPLACE_NAME ||
      entry?.root === paths.marketplaceRoot ||
      entry?.marketplaceSource?.source === paths.marketplaceRoot,
  );
  if (matchingMarketplaces.length !== 1) {
    throw new Error("Codex readback must contain exactly one TaskBelay marketplace identity");
  }
  const marketplace = matchingMarketplaces[0];
  assertMarketplaceReadback(
    marketplace,
    MARKETPLACE_NAME,
    paths.marketplaceRoot,
    "marketplace readback",
  );

  const matchingPlugins = state.plugins.filter(
    (entry) =>
      entry?.name === PLUGIN_NAME ||
      entry?.pluginId === PLUGIN_SELECTOR ||
      entry?.source?.path === paths.pluginRoot,
  );
  if (matchingPlugins.length !== 1) {
    throw new Error("Codex readback must contain exactly one TaskBelay plugin identity");
  }
  const plugin = matchingPlugins[0];
  assertPluginReadback(
    plugin,
    {
      pluginId: PLUGIN_SELECTOR,
      name: PLUGIN_NAME,
      marketplaceName: MARKETPLACE_NAME,
      pluginRoot: paths.pluginRoot,
      marketplaceRoot: paths.marketplaceRoot,
      version: packageVersion,
      requireEnabled: true,
    },
    "plugin readback",
  );
}

function registrationStateMatches(state, paths, packageVersion) {
  try {
    assertMatchingRegistrationState(state, paths, packageVersion);
    return true;
  } catch {
    return false;
  }
}

async function rollbackCreatedMarketplace(paths, commandOptions) {
  try {
    const state = await readRegistrationState(commandOptions);
    if (state.plugins.some((entry) => entry?.name === PLUGIN_NAME || entry?.pluginId === PLUGIN_SELECTOR)) {
      return { preserved: true };
    }
    const marketplace = state.marketplaces.find((entry) => entry?.name === MARKETPLACE_NAME);
    if (!marketplace || marketplace.root !== paths.marketplaceRoot) return { preserved: true };
    assertMarketplaceReadback(marketplace, MARKETPLACE_NAME, paths.marketplaceRoot, "rollback marketplace readback");
    const removeResult = await runCodexJSON(
      ["plugin", "marketplace", "remove", MARKETPLACE_NAME, "--json"],
      commandOptions,
    );
    assertMarketplaceRemoveResult(removeResult, MARKETPLACE_NAME);
    const after = await readRegistrationState(commandOptions);
    if (after.marketplaces.some((entry) => entry?.name === MARKETPLACE_NAME)) {
      throw new Error("marketplace remains after rollback readback");
    }
    return { removed: true };
  } catch (error) {
    return { error };
  }
}

function assertMarketplaceReadback(marketplace, expectedName, expectedRoot, label) {
  assertObject(marketplace, label);
  const policy = registrationPolicy();
  assertExactKeys(marketplace, policy.marketplaceFields, label);
  if (policy.requiresMarketplaceSource) {
    assertObject(marketplace.marketplaceSource, `${label} source`);
    assertExactKeys(marketplace.marketplaceSource, ["sourceType", "source"], `${label} source`);
  }
  if (
    marketplace.name !== expectedName ||
    policy.path(marketplace.root) !== expectedRoot ||
    (policy.requiresMarketplaceSource && (marketplace.marketplaceSource.sourceType !== "local" ||
    policy.path(marketplace.marketplaceSource.source) !== expectedRoot))
  ) {
    throw new Error(`${label} conflicts with the expected local marketplace identity`);
  }
}

function assertPluginReadback(plugin, expected, label) {
  assertObject(plugin, label);
  parseSemver(plugin.version, `${label} version`);
  assertExactKeys(
    plugin,
    [
      "pluginId",
      "name",
      "marketplaceName",
      "version",
      "installed",
      "enabled",
      "source",
      "marketplaceSource",
      "installPolicy",
      "authPolicy",
    ],
    label,
  );
  assertObject(plugin.source, `${label} plugin source`);
  assertExactKeys(plugin.source, ["source", "path"], `${label} plugin source`);
  assertObject(plugin.marketplaceSource, `${label} marketplace source`);
  assertExactKeys(
    plugin.marketplaceSource,
    ["sourceType", "source"],
    `${label} marketplace source`,
  );
  if (
    plugin.pluginId !== expected.pluginId ||
    plugin.name !== expected.name ||
    plugin.marketplaceName !== expected.marketplaceName ||
    (expected.version !== undefined && plugin.version !== expected.version) ||
    plugin.installed !== true ||
    typeof plugin.enabled !== "boolean" ||
    (expected.requireEnabled && plugin.enabled !== true) ||
    plugin.source.source !== "local" ||
    plugin.source.path !== expected.pluginRoot ||
    plugin.marketplaceSource.sourceType !== "local" ||
    registrationPolicy().path(plugin.marketplaceSource.source) !== expected.marketplaceRoot ||
    plugin.installPolicy !== "AVAILABLE" ||
    plugin.authPolicy !== "ON_INSTALL"
  ) {
    throw new Error(`${label} conflicts with the expected installed plugin identity`);
  }
}

function assertMarketplaceAddResult(result, paths) {
  assertObject(result, "marketplace add result");
  assertExactKeys(result, ["marketplaceName", "installedRoot", "alreadyAdded"], "marketplace add result");
  if (
    result.marketplaceName !== MARKETPLACE_NAME ||
    result.installedRoot !== paths.marketplaceRoot ||
    typeof result.alreadyAdded !== "boolean"
  ) {
    throw new Error("marketplace add result conflicts with the requested local marketplace");
  }
  if (result.alreadyAdded) {
    throw new Error(
      "marketplace add reported alreadyAdded after absent readback; concurrent marketplace ownership is not rollback-owned",
    );
  }
}

function assertPluginAddResult(result, paths, packageVersion) {
  assertObject(result, "plugin add result");
  assertExactKeys(
    result,
    ["pluginId", "name", "marketplaceName", "version", "installedPath", "authPolicy"],
    "plugin add result",
  );
  assertCanonicalAbsolutePath(result.installedPath, "plugin add result installedPath");
  if (
    result.pluginId !== PLUGIN_SELECTOR ||
    result.name !== PLUGIN_NAME ||
    result.marketplaceName !== MARKETPLACE_NAME ||
    result.version !== packageVersion ||
    result.authPolicy !== "ON_INSTALL" ||
    result.installedPath === paths.pluginRoot
  ) {
    throw new Error("plugin add result conflicts with the requested plugin identity");
  }
}

function assertPluginRemoveResult(result, registration) {
  assertObject(result, "plugin remove result");
  assertExactKeys(result, ["pluginId", "name", "marketplaceName"], "plugin remove result");
  if (
    result.pluginId !== registration.plugin_selector ||
    result.name !== registration.plugin_name ||
    result.marketplaceName !== registration.marketplace_name
  ) {
    throw new Error("plugin remove result conflicts with the recorded plugin identity");
  }
}

function assertMarketplaceRemoveResult(result, marketplaceName) {
  assertObject(result, "marketplace remove result");
  assertExactKeys(result, ["marketplaceName", "installedRoot"], "marketplace remove result");
  if (result.marketplaceName !== marketplaceName || result.installedRoot !== null) {
    throw new Error("marketplace remove result conflicts with the recorded local marketplace");
  }
}

function createReceipt({
  paths,
  packageVersion,
  coreVersion,
  codexVersion,
  resourceDigests,
  installedAt,
}) {
  return validateReceipt({
    product: {
      name: PLUGIN_NAME,
      version: packageVersion,
      core_version: coreVersion,
      codex_compatibility: CODEX_COMPATIBILITY_RANGE,
    },
    host: {
      surface: "codex-cli",
      version: codexVersion,
      os: paths.platform ?? paths.runtimeKey.split("-")[0],
      arch: paths.arch ?? paths.runtimeKey.split("-")[1],
    },
    registration: {
      marketplace_name: MARKETPLACE_NAME,
      marketplace_root: paths.marketplaceRoot,
      plugin_name: PLUGIN_NAME,
      plugin_selector: PLUGIN_SELECTOR,
      plugin_root: paths.pluginRoot,
    },
    paths: {
      package_root: paths.packageRoot,
      runtime_path: paths.runtimePath,
      data_dir: paths.dataDirectory,
      receipt_path: paths.receiptPath,
    },
    resource_digests: resourceDigests,
    installed_at: installedAt,
  });
}

function resourcePaths(paths) {
  return {
    pluginManifest: join(paths.pluginRoot, ".codex-plugin", "plugin.json"),
    skill: join(paths.pluginRoot, "skills", "taskbelay", "SKILL.md"),
    skillMetadata: join(paths.pluginRoot, "skills", "taskbelay", "agents", "openai.yaml"),
    mcpConfiguration: join(paths.pluginRoot, ".mcp.json"),
  };
}

async function assertReadableFile(path, label) {
  try {
    const info = await stat(path);
    if (!info.isFile()) throw new Error("not a file");
    await access(path, fsConstants.R_OK);
  } catch (error) {
    throw new Error(`${label} must exist and be readable`, { cause: error });
  }
}

async function assertExecutableFile(path, label, requireExecutableMode = true) {
  let info;
  try {
    info = await stat(path);
    await access(path, requireExecutableMode ? fsConstants.X_OK : fsConstants.F_OK);
  } catch (error) {
    throw new Error(`${label} must exist and be executable`, { cause: error });
  }
  if (!info.isFile() || requireExecutableMode && (info.mode & 0o111) === 0) {
    throw new Error(`${label} must exist and be executable`);
  }
}

async function assertExecutableOnPath(name, environment, platform, requireExecutableMode) {
  const candidate = await findCommandPath(name, { environment, platform });
  await assertExecutableFile(candidate, name, requireExecutableMode);
  return candidate;
}

async function readJSON(path, label) {
  let contents;
  try {
    contents = await readFile(path, "utf8");
  } catch (error) {
    throw new Error(`${label} is unavailable at ${path}`, { cause: error });
  }
  try {
    const parsed = JSON.parse(contents);
    assertObject(parsed, label);
    return parsed;
  } catch (error) {
    throw new Error(`${label} is not valid JSON`, { cause: error });
  }
}

export function versionSatisfiesRange(version, range = CODEX_COMPATIBILITY_RANGE) {
  const match = /^>=(\S+)$/.exec(range);
  if (!match) throw new Error(`unsupported compatibility range ${JSON.stringify(range)}`);
  const candidate = parseSemver(version, "Codex version");
  const minimum = parseSemver(match[1], "compatibility minimum");
  return compareSemver(candidate, minimum) >= 0;
}

export function validateReceipt(receipt, { compatibilityRange = CODEX_COMPATIBILITY_RANGE } = {}) {
  assertObject(receipt, "registration receipt");
  assertExactKeys(
    receipt,
    ["product", "host", "registration", "paths", "resource_digests", "installed_at"],
    "registration receipt",
  );

  assertObject(receipt.product, "product");
  assertExactKeys(receipt.product, ["name", "version", "core_version", "codex_compatibility"], "product");
  assertEqual(receipt.product.name, "taskbelay-codex", "product.name");
  parseSemver(receipt.product.version, "product.version");
  parseSemver(receipt.product.core_version, "product.core_version");
  assertEqual(receipt.product.codex_compatibility, compatibilityRange, "product compatibility range");

  assertObject(receipt.host, "host");
  assertExactKeys(receipt.host, ["surface", "version", "os", "arch"], "host");
  assertEqual(receipt.host.surface, "codex-cli", "host.surface");
  const runtimeKey = `${receipt.host.os}-${receipt.host.arch}`;
  if (!SUPPORTED_RUNTIME_KEYS.includes(runtimeKey)) {
    throw new Error(`host platform ${runtimeKey} is unsupported`);
  }
  if (!versionSatisfiesRange(receipt.host.version, compatibilityRange)) {
    throw new Error(`host.version ${receipt.host.version} does not satisfy ${compatibilityRange}`);
  }

  assertObject(receipt.registration, "registration");
  assertExactKeys(
    receipt.registration,
    ["marketplace_name", "marketplace_root", "plugin_name", "plugin_selector", "plugin_root"],
    "registration",
  );
  assertEqual(receipt.registration.marketplace_name, "taskbelay-local", "registration.marketplace_name");
  assertEqual(receipt.registration.plugin_name, "taskbelay-codex", "registration.plugin_name");
  assertEqual(
    receipt.registration.plugin_selector,
    "taskbelay-codex@taskbelay-local",
    "registration.plugin_selector",
  );
  assertCanonicalAbsolutePath(receipt.registration.marketplace_root, "registration.marketplace_root");
  assertCanonicalAbsolutePath(receipt.registration.plugin_root, "registration.plugin_root");

  assertObject(receipt.paths, "paths");
  assertExactKeys(receipt.paths, ["package_root", "runtime_path", "data_dir", "receipt_path"], "paths");
  for (const field of ["package_root", "runtime_path", "data_dir", "receipt_path"]) {
    assertCanonicalAbsolutePath(receipt.paths[field], `paths.${field}`);
  }

  assertObject(receipt.resource_digests, "resource_digests");
  assertExactKeys(
    receipt.resource_digests,
    ["plugin_manifest", "skill", "skill_metadata", "mcp_configuration"],
    "resource_digests",
  );
  for (const field of ["plugin_manifest", "skill", "skill_metadata", "mcp_configuration"]) {
    if (!digestPattern.test(receipt.resource_digests[field])) {
      throw new Error(`resource_digests.${field} must be a lowercase SHA-256 digest`);
    }
  }

  if (typeof receipt.installed_at !== "string" || !Number.isFinite(Date.parse(receipt.installed_at))) {
    throw new Error("installed_at must be an RFC 3339 date-time string");
  }
  return structuredClone(receipt);
}

export async function readReceipt(receiptPath, options) {
  let contents;
  try {
    contents = await readFile(receiptPath, "utf8");
  } catch (error) {
    if (error?.code === "ENOENT") return null;
    throw new Error(`read registration receipt ${receiptPath}: ${error.message}`, { cause: error });
  }

  let receipt;
  try {
    receipt = JSON.parse(contents);
  } catch (error) {
    throw new Error(`parse registration receipt ${receiptPath}: invalid JSON`, { cause: error });
  }
  try {
    return validateReceipt(receipt, options);
  } catch (error) {
    throw new Error(`registration receipt ${receiptPath} is invalid: ${error.message}`, { cause: error });
  }
}

export async function writeReceiptAtomic(receiptPath, receipt, {
  ownedRoot,
  compatibilityRange,
  enforcePrivateModes = true,
} = {}) {
  if (!ownedRoot) throw new Error("receipt owned root is required");
  assertCanonicalAbsolutePath(receiptPath, "receipt path");
  assertCanonicalAbsolutePath(ownedRoot, "receipt owned root");
  const expectedPath = join(ownedRoot, "registrations", "codex.json");
  if (receiptPath !== expectedPath) {
    throw new Error(`receipt path must equal the product-owned path ${expectedPath}`);
  }
  containedPath(ownedRoot, receiptPath, "receipt path");

  const validated = validateReceipt(receipt, { compatibilityRange });
  if (validated.paths.receipt_path !== receiptPath) {
    throw new Error("receipt payload path does not match the write target");
  }

  await mkdir(ownedRoot, { recursive: true, mode: 0o700 });
  const canonicalOwnedRoot = await realpath(ownedRoot);
  if (canonicalOwnedRoot !== ownedRoot) {
    throw new Error("receipt owned root must be canonical and may not use a symbolic link");
  }
  const parent = dirname(receiptPath);
  await assertNoSymbolicLinkComponents(ownedRoot, parent);
  await mkdir(parent, { recursive: true, mode: 0o700 });
  await assertNoSymbolicLinkComponents(ownedRoot, parent);
  if (enforcePrivateModes) await chmod(parent, 0o700);
  await rejectSymbolicLink(receiptPath);

  const temporaryPath = join(parent, `.${basename(receiptPath)}.tmp-${process.pid}-${randomBytes(8).toString("hex")}`);
  try {
    await writeFile(temporaryPath, `${JSON.stringify(validated, null, 2)}\n`, {
      encoding: "utf8",
      mode: 0o600,
      flag: "wx",
    });
    await rename(temporaryPath, receiptPath);
    if (enforcePrivateModes) await chmod(receiptPath, 0o600);
  } catch (error) {
    await unlink(temporaryPath).catch(() => {});
    throw error;
  }
}

export async function digestResources({ pluginManifest, skill, skillMetadata, mcpConfiguration }) {
  const entries = {
    plugin_manifest: pluginManifest,
    skill,
    skill_metadata: skillMetadata,
    mcp_configuration: mcpConfiguration,
  };
  const result = {};
  for (const [name, path] of Object.entries(entries)) {
    if (!path) throw new Error(`resource path ${name} is required`);
    result[name] = createHash("sha256").update(await readFile(path)).digest("hex");
  }
  return result;
}

export function receiptOwnershipMatches(left, right) {
  try {
    const first = validateReceipt(left);
    const second = validateReceipt(right);
    return stableJSON(ownershipProjection(first)) === stableJSON(ownershipProjection(second));
  } catch {
    return false;
  }
}

function assertCompatibleReceiptUpgrade(previousReceipt, currentReceipt) {
  const previous = validateReceipt(previousReceipt);
  const current = validateReceipt(currentReceipt);
  const order = compareSemver(
    parseSemver(current.product.version, "current product version"),
    parseSemver(previous.product.version, "previous product version"),
  );
  if (order < 0) {
    throw new Error("package downgrade is not allowed; setup made no changes");
  }
  if (order === 0) {
    throw new Error("registration receipt ownership conflict; setup made no changes");
  }
  if (stableJSON(upgradeOwnershipProjection(previous)) !== stableJSON(upgradeOwnershipProjection(current))) {
    throw new Error("registration receipt ownership conflict; setup made no changes");
  }
}

function upgradeOwnershipProjection(receipt) {
  return {
    product: {
      name: receipt.product.name,
      codex_compatibility: receipt.product.codex_compatibility,
    },
    host: {
      surface: receipt.host.surface,
      os: receipt.host.os,
      arch: receipt.host.arch,
    },
    registration: receipt.registration,
    paths: receipt.paths,
  };
}

function ownershipProjection(receipt) {
  return {
    product: receipt.product,
    host: receipt.host,
    registration: receipt.registration,
    paths: receipt.paths,
    resource_digests: receipt.resource_digests,
  };
}

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

function assertObject(value, label) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} must be an object`);
  }
}

function assertExactKeys(value, requiredKeys, label) {
  for (const field of requiredKeys) {
    if (!Object.hasOwn(value, field)) throw new Error(`${label} missing field ${field}`);
  }
  const allowed = new Set(requiredKeys);
  for (const field of Object.keys(value)) {
    if (!allowed.has(field)) throw new Error(`${label} has unexpected field ${field}`);
  }
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`${label} must equal ${JSON.stringify(expected)}; got ${JSON.stringify(actual)}`);
  }
}

function assertCanonicalAbsolutePath(value, label) {
  if (typeof value !== "string" || !isAbsolute(value)) {
    throw new Error(`${label} must be an absolute path`);
  }
  if (resolve(value) !== value) throw new Error(`${label} must be canonical`);
}

function parseSemver(value, label) {
  if (typeof value !== "string") throw new Error(`${label} must be a SemVer string`);
  const match = semverPattern.exec(value);
  if (!match) throw new Error(`${label} must be valid SemVer`);
  const prerelease = match[4] ? match[4].split(".") : [];
  for (const identifier of prerelease) {
    if (/^[0-9]+$/.test(identifier) && identifier.length > 1 && identifier.startsWith("0")) {
      throw new Error(`${label} has an invalid numeric prerelease identifier`);
    }
  }
  return { major: Number(match[1]), minor: Number(match[2]), patch: Number(match[3]), prerelease };
}

function compareSemver(left, right) {
  for (const field of ["major", "minor", "patch"]) {
    if (left[field] !== right[field]) return left[field] < right[field] ? -1 : 1;
  }
  if (left.prerelease.length === 0 || right.prerelease.length === 0) {
    if (left.prerelease.length === right.prerelease.length) return 0;
    return left.prerelease.length === 0 ? 1 : -1;
  }
  const count = Math.max(left.prerelease.length, right.prerelease.length);
  for (let index = 0; index < count; index += 1) {
    const a = left.prerelease[index];
    const b = right.prerelease[index];
    if (a === undefined || b === undefined) return a === undefined ? -1 : 1;
    if (a === b) continue;
    const aNumeric = /^[0-9]+$/.test(a);
    const bNumeric = /^[0-9]+$/.test(b);
    if (aNumeric && bNumeric) return Number(a) < Number(b) ? -1 : 1;
    if (aNumeric !== bNumeric) return aNumeric ? -1 : 1;
    return a < b ? -1 : 1;
  }
  return 0;
}

async function assertNoSymbolicLinkComponents(root, candidate) {
  const offset = relative(root, candidate);
  if (offset === ".." || offset.startsWith(`..${sep}`) || isAbsolute(offset)) {
    throw new Error("receipt path escapes its owned root");
  }
  let current = root;
  for (const component of offset.split(sep).filter(Boolean)) {
    current = join(current, component);
    try {
      const info = await lstat(current);
      if (info.isSymbolicLink()) throw new Error(`receipt path contains a symbolic link: ${current}`);
    } catch (error) {
      if (error?.code === "ENOENT") return;
      throw error;
    }
  }
}

async function rejectSymbolicLink(path) {
  try {
    if ((await lstat(path)).isSymbolicLink()) {
      throw new Error(`receipt target is a symbolic link: ${path}`);
    }
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
  }
}

export { ensureCodexPetInstalled } from "./platform.mjs";
