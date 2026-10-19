import { execPortableCommand } from "../command.mjs";
import { createCodexDriver } from "./codex.mjs";
import { createDeepSeekDriver } from "./deepseek.mjs";
import { createClaudeDriver } from "./claude.mjs";
import { createZCodeDriver } from "./zcode.mjs";

export function createHostDrivers({ paths, environment, localPackages = null, signal,
  codexDriver, deepseekDriver, claudeDriver, zcodeDriver, runCodexChild, runDeepSeekChild, runClaudeChild, runZCodeChild } = {}) {
  const cancellableRun = signal ? (exe, args, { environment: env = environment, ...options } = {}) => execPortableCommand(exe, args, { encoding: "utf8", windowsHide: true, timeout: 120000, maxBuffer: 8 * 1024 * 1024, ...options, env, signal }) : undefined;
  return Object.freeze({
    codex: codexDriver ?? createCodexDriver({ paths, environment, run: runCodexChild ?? cancellableRun, localPackage: localPackages?.codex ?? null }),
    claude: claudeDriver ?? createClaudeDriver({ paths, environment, run: runClaudeChild ?? cancellableRun, localPackage: localPackages?.claude ?? null }),
    deepseek: deepseekDriver ?? createDeepSeekDriver({ paths, environment, run: runDeepSeekChild ?? cancellableRun, localPackage: localPackages?.deepseek ?? null }),
    zcode: zcodeDriver ?? createZCodeDriver({ paths, environment, run: runZCodeChild ?? cancellableRun, localPackage: localPackages?.zcode ?? null }),
  });
}
