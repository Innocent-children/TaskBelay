import { spawn } from "node:child_process";
import { lstat, mkdir, realpath, stat } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { platformPolicy } from "./platform.mjs";

export const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export async function paths(environment = process.env, options = {}) {
  const platform = platformPolicy(options.platform, options.arch);
  const root = resolve(options.packageRoot ?? packageRoot);
  const home = platform.homeDirectory(environment, homedir());
  const productRoot = platform.productRoot(environment, home);
  const explicit = environment.TASKBELAY_DATA_DIR;
  const dataDirectory = explicit || join(productRoot, "data");
  if (!isAbsolute(dataDirectory)) throw new Error("TASKBELAY_DATA_DIR must be absolute");
  if (!explicit) {
    for (const directory of [productRoot, dataDirectory]) {
      try {
        const info = await lstat(directory);
        if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Default TaskBelay data must use regular owned directories");
      } catch (error) { if (error.code !== "ENOENT") throw error; }
    }
  } else if (await realpath(explicit) !== resolve(explicit) || !(await stat(explicit)).isDirectory()) {
    throw new Error("TASKBELAY_DATA_DIR must be an existing canonical directory");
  }
  return {
    packageRoot: root, productRoot, dataDirectory, explicitData: Boolean(explicit), platform,
    runtimePath: join(root, "runtime", platform.runtimeKey, platform.runtimeExecutable),
    marketplacePath: join(root, "marketplace.json"),
    receiptPath: join(productRoot, "registrations", "zcode.json"),
  };
}

export async function core(arguments_, { input, environment = process.env, stream = false, timeoutMs = 25000 } = {}) {
  const p = await paths(environment);
  if (!p.explicitData && arguments_[0] === "mcp") await mkdir(p.dataDirectory, { recursive: true, mode: 0o700 });
  return await new Promise((resolveResult, reject) => {
    const child = spawn(p.runtimePath, arguments_, {
      env: { ...environment, TASKBELAY_DATA_DIR: p.dataDirectory },
      stdio: stream ? "inherit" : ["pipe", "pipe", "pipe"], windowsHide: true,
    });
    const signals = new Map();
    let failure = null, stdout = "", stderr = "", outputBytes = 0;
    const stop = message => { failure ??= new Error(message); child.kill(); };
    const timer = stream ? null : setTimeout(() => stop("Core command timed out"), timeoutMs);
    if (stream) for (const signal of p.platform.forwardedSignals) {
      const forward = () => child.kill(signal);
      signals.set(signal, forward);
      process.on(signal, forward);
    }
    const cleanup = () => {
      if (timer) clearTimeout(timer);
      for (const [signal, forward] of signals) process.off(signal, forward);
    };
    child.once("error", error => { cleanup(); reject(error); });
    if (!stream) {
      const collect = (kind, chunk) => {
        outputBytes += Buffer.byteLength(chunk);
        if (outputBytes > 8 * 1024 * 1024) return stop("Core output exceeds 8 MiB");
        if (kind === "stdout") stdout += chunk; else stderr += chunk;
      };
      child.stdout.setEncoding("utf8");
      child.stderr.setEncoding("utf8");
      child.stdout.on("data", chunk => collect("stdout", chunk));
      child.stderr.on("data", chunk => collect("stderr", chunk));
      child.stdin.on("error", () => {});
      child.stdin.end(input === undefined ? "" : JSON.stringify(input));
    }
    child.once("close", code => {
      cleanup();
      if (stream && !failure && Number.isInteger(code)) { resolveResult({ stdout, stderr, code }); return; }
      if (failure || code !== 0) {
        const error = failure ?? new Error(stderr || `Core exited with ${code}`);
        error.stdout = stdout; error.code = Number.isInteger(code) && code !== 0 ? code : 1;
        reject(error);
      } else resolveResult({ stdout, stderr, code });
    });
  });
}

export async function coreJSON(arguments_, input, options = {}) {
  return JSON.parse((await core(arguments_, { ...options, input })).stdout);
}
