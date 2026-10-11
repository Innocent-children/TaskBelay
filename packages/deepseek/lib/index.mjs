import * as mcpClientPlugin from "@deepseek-ai/dsh-mcp-client";
import { readFile } from "node:fs/promises";
import { join } from "node:path";

import { registerTaskBelayGuard } from "./authorization.mjs";
import { registerFileScopeGate } from "./file-scope.mjs";
import {
  ensureDefaultDataDirectory,
  packageRootFromModule,
  resolveDataDirectory,
} from "./paths.mjs";
import { preflightPackagedCore, selectPackagedRuntime } from "./runtime.mjs";
import { registerWorkspaceCoordinator } from "./workspace-tool.mjs";
import {
  TASKBELAY_SERVER_NAME,
  assertQualifiedToolCatalog,
} from "./tool-names.mjs";

export const name = "taskbelay-deepseek";
export const inject = ["skills", "tools"];

export async function apply(ctx, options = {}) {
  await activateDeepSeekIntegration(ctx, options);
}

export async function activateDeepSeekIntegration(ctx, {
  packageRoot = packageRootFromModule(import.meta.url),
  homeDirectory,
  environment = process.env,
  platform = process.platform,
  arch = process.arch,
  workspaceRoot = process.cwd(),
} = {}) {
  const manifest = await readPackageManifest(packageRoot);
  const runtimeSelection = await selectPackagedRuntime({ packageRoot, platform, arch });
  const dataSelection = await resolveDataDirectory({ homeDirectory, environment, platform, arch });
  if (dataSelection.usesDefaultDataDirectory) {
    await ensureDefaultDataDirectory(dataSelection);
  }
  const runtime = await preflightPackagedCore(runtimeSelection, {
    environment,
  });

  const skillDirectory = join(runtime.packageRoot, "skills", "taskbelay");
  const skillPath = join(skillDirectory, "SKILL.md");
  const skillContent = await readFile(skillPath, "utf8");

  ctx.skills.register(Object.freeze({
    name: "taskbelay",
    description: "Keep authorized outcomes, actual changes and verification evidence together.",
    whenToUse: "Use for authorized development or recovery of an existing Task; retain existing authorization.",
    invocation: Object.freeze({
      modelInvocable: true,
      userInvocable: true,
    }),
    source: "bundled",
    provider: name,
    resourceBase: Object.freeze({ kind: "directory", path: skillDirectory }),
    content: skillContent,
    path: skillPath,
  }));
  registerTaskBelayGuard(ctx, { workspaceRoot });
  registerFileScopeGate(ctx, {
    runtimePath: runtime.runtimePath,
    dataDirectory: dataSelection.dataDirectory,
    workspaceRoot,
  });
  registerWorkspaceCoordinator(ctx, {
    runtimePath: runtime.runtimePath,
    dataDirectory: dataSelection.dataDirectory,
    workspaceRoot,
  });

  let mcpFiber;
  let catalogCheckQueued = false;
  let catalogFailed = false;
  ctx.on("tools/change", () => {
    if (catalogCheckQueued || catalogFailed) return;
    catalogCheckQueued = true;
    queueMicrotask(() => {
      catalogCheckQueued = false;
      if (catalogFailed) return;
      try {
        verifyCurrentCatalog(ctx, { allowUnavailable: true });
      } catch (error) {
        catalogFailed = true;
        ctx.logger.error(`${name}: ${error.message}`);
        if (mcpFiber !== undefined) {
          void mcpFiber.dispose().catch((disposeError) => {
            ctx.logger.error(`${name}: failed to dispose incompatible MCP child: ${disposeError.message}`);
          });
        }
      }
    });
  });

  mcpFiber = await ctx.plugin(mcpClientPlugin, {
    transport: "stdio",
    serverName: TASKBELAY_SERVER_NAME,
    command: runtime.runtimePath,
    args: ["mcp", "--stdio"],
    env: { TASKBELAY_DATA_DIR: dataSelection.dataDirectory },
    cwd: runtime.packageRoot,
    toolCallTimeoutMs: 3_660_000,
    failOnStartupError: true,
    reconnect: {
      enabled: true,
      initialDelayMs: 500,
      maxDelayMs: 30_000,
      maxAttempts: 10,
    },
  });
  verifyCurrentCatalog(ctx, { allowUnavailable: true });
}

function verifyCurrentCatalog(ctx, options) {
  const toolNames = ctx.tools.schemas().map((schema) => schema.name);
  return assertQualifiedToolCatalog(toolNames, options);
}

async function readPackageManifest(packageRoot) {
  const manifest = JSON.parse(await readFile(join(packageRoot, "package.json"), "utf8"));
  if (manifest.name !== name || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/u.test(manifest.version ?? "")) {
    throw new Error("DeepSeek package manifest identity is invalid");
  }
  return manifest;
}
