import { readFile, realpath, stat, lstat } from "node:fs/promises";
import { createHash } from "node:crypto";
import { isAbsolute, join } from "node:path";
import { execPortableCommand } from "./command.mjs";
import { installedPackageRoot, readLocalPackages } from "./local-packages.mjs";

const version = value => {
  if (typeof value !== "string" || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u.test(value)) throw new Error("Invalid TaskBelay launcher version");
  return value;
};

/**
 * Launcher replacement stays in the installation's channel and npm prefix.
 * It is the last maintenance action; the running menu exits after replacement.
 */
export function createLauncherDriver({ packageRoot = installedPackageRoot, environment = process.env, signal, run = execPortableCommand } = {}) {
  const call = (executable, args, options = {}) => run(executable, args, { env: environment, encoding: "utf8", timeout: 120000, maxBuffer: 4 * 1024 * 1024, signal, ...options });
  async function observe() {
    const manifest = JSON.parse(await readFile(join(packageRoot, "package.json"), "utf8"));
    if (manifest.name !== "taskbelay") throw new Error("Unexpected launcher package identity");
    return { host: "taskbelay", profile: null, state: "ready", packageInstalled: true, packageVersion: version(manifest.version),
      packageRoot: await realpath(packageRoot), channel: Object.hasOwn(manifest, "taskBelayLocalPackages") ? "local" : "npm" };
  }
  async function resolveTarget({ targetVersion = "latest", selfPackage = null } = {}) {
    const current = await observe();
    const npmRoot = (await call("npm", ["root", "--global"])).stdout.trim();
    if (!isAbsolute(npmRoot) || (await lstat(join(npmRoot, "taskbelay")).catch(() => null))?.isSymbolicLink() || await realpath(join(npmRoot, "taskbelay")).catch(() => null) !== current.packageRoot) throw new Error("Self update requires this launcher to be the active global npm installation");
    if (current.channel === "local") {
      if (!selfPackage || !isAbsolute(selfPackage) || !selfPackage.endsWith(".tgz")) throw new Error("Local launcher update requires --self-package with an absolute complete local distribution .tgz path");
      const path = await realpath(selfPackage);
      if (!(await stat(path)).isFile()) throw new Error("Local distribution must be a file");
      const manifest = JSON.parse((await call("tar", ["-xOf", path, "package/package.json"])).stdout);
      if (manifest.name !== "taskbelay" || !manifest.taskBelayLocalPackages || Object.keys(manifest.taskBelayLocalPackages).sort().join(",") !== "claude,codex,deepseek,zcode") throw new Error("Local launcher update requires a complete local distribution; changing channels is not allowed");
      for (const product of ["codex", "deepseek", "claude", "zcode"]) {
        const entry = manifest.taskBelayLocalPackages[product];
        if (!entry || Object.keys(entry).sort().join(",") !== "path,sha256,version" || entry.path !== `local-packages/${product}.tgz` || !/^[a-f0-9]{64}$/u.test(entry.sha256)) throw new Error(`Invalid local ${product} metadata`);
        version(entry.version);
        const bytes = (await call("tar", ["-xOf", path, `package/${entry.path}`], { encoding: null, maxBuffer: 256 * 1024 * 1024 })).stdout;
        if (createHash("sha256").update(bytes).digest("hex") !== entry.sha256) throw new Error(`Local ${product} distribution digest mismatch`);
      }
      const next = version(manifest.version);
      if (targetVersion !== "latest" && targetVersion !== next) throw new Error("The local launcher package does not contain the requested version");
      return { version: next, artifact: path, digest: createHash("sha256").update(await readFile(path)).digest("hex"), channel: "local", packageRoot: current.packageRoot };
    }
    if (selfPackage) throw new Error("The npm launcher must be updated from npm, not a local distribution");
    const result = JSON.parse((await call("npm", ["view", `taskbelay@${targetVersion}`, "version", "--json"])).stdout);
    const next = version(Array.isArray(result) ? result.at(-1) : result);
    return { version: next, artifact: `taskbelay@${next}`, channel: "npm", packageRoot: current.packageRoot };
  }
  return {
    observe, resolveTarget,
    async execute(target) {
      const current = await observe();
      if (current.channel !== target.channel || current.packageRoot !== target.packageRoot) throw new Error("Launcher installation changed after confirmation");
      if (target.digest && createHash("sha256").update(await readFile(target.artifact)).digest("hex") !== target.digest) throw new Error("Local distribution changed after confirmation");
      let changed = false;
      try {
        await call("npm", ["install", "--global", "--ignore-scripts", "--no-audit", "--no-fund", target.artifact]);
        changed = true;
        const installed = await observe();
        if (installed.channel !== target.channel || installed.packageVersion !== target.version) throw new Error("Launcher version or channel did not match after update");
        if (installed.channel === "local") await readLocalPackages(packageRoot);
        return { changed: true, completedSteps: ["taskbelay.upgrade"], nextSteps: ["Run taskbelay again to use the updated launcher."] };
      } catch (error) {
        error.changed = changed;
        error.uncertain = !changed;
        error.nextStep = "taskbelay version";
        error.completedSteps = changed ? ["taskbelay.install_package"] : [];
        throw error;
      }
    },
  };
}
