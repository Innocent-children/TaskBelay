import { spawn } from "node:child_process";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { lstat, mkdir, realpath, stat } from "node:fs/promises";
import { platformPolicy } from "./platform.mjs";
export const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
export async function paths(environment = process.env) {
  const platform = platformPolicy();
  const home = platform.homeDirectory(environment, homedir());
  const productRoot = platform.productRoot(environment, home);
  const explicit = environment.TASKBELAY_DATA_DIR;
  const dataDirectory = explicit || join(productRoot, "data");
  if (!isAbsolute(dataDirectory)) throw new Error("TASKBELAY_DATA_DIR must be absolute");
  if (!explicit) {
    for (const directory of [productRoot, dataDirectory]) {
      try { const info = await lstat(directory); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Default TaskBelay data must use regular owned directories"); }
      catch (error) { if (error.code !== "ENOENT") throw error; }
    }
  }
  if (explicit && ((await realpath(explicit)) !== resolve(explicit) || !(await stat(explicit)).isDirectory())) throw new Error("TASKBELAY_DATA_DIR must be an existing canonical directory");
  return { packageRoot, productRoot, dataDirectory, explicitData: Boolean(explicit),
    configRoot: resolve(environment.CLAUDE_CONFIG_DIR || join(home, ".claude")),
    runtimePath: join(packageRoot, "runtime", platform.runtimeKey, platform.runtimeExecutable),
    receiptPath: join(productRoot, "registrations", "claude.json"), platform };
}
export async function core(arguments_, { input, environment = process.env, stream = false } = {}) {
  const p = await paths(environment);
  if (!p.explicitData && arguments_[0] === "mcp") await mkdir(p.dataDirectory, { recursive: true, mode: 0o700 });
  return await new Promise((resolveResult, reject) => {
    const child = spawn(p.runtimePath, arguments_, { env: { ...environment, TASKBELAY_DATA_DIR: p.dataDirectory }, stdio: stream ? "inherit" : ["pipe", "pipe", "pipe"], windowsHide: true });
    const signals = new Map();
    if (stream) for (const signal of p.platform.forwardedSignals) {
      const forward = () => child.kill(signal);
      signals.set(signal, forward); process.on(signal, forward);
    }
    const cleanup = () => { for (const [signal, forward] of signals) process.off(signal, forward); };
    child.once("error", cleanup); child.once("close", cleanup);
    let stdout = "", stderr = "";
    child.on("error", reject);
    if (!stream) {
      child.stdout.on("data", chunk => { stdout += chunk; if (Buffer.byteLength(stdout) > 8 * 1024 * 1024) child.kill(); });
      child.stderr.on("data", chunk => { stderr += chunk; });
      child.stdin.on("error", () => {});
      child.stdin.end(input === undefined ? "" : JSON.stringify(input));
    }
    child.on("close", code => {
      if (stream) { resolveResult({stdout, stderr, code: code ?? 1}); }
      else if (code !== 0) { const error = new Error(stderr || "Core exited with " + code); error.stdout = stdout; error.code = code; reject(error); }
      else resolveResult({ stdout, stderr, code });
    });
  });
}
export async function coreJSON(arguments_, input, options = {}) {
  const result = await core(arguments_, { ...options, input });
  return JSON.parse(result.stdout);
}
