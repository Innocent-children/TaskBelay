import { join, resolve, isAbsolute } from "node:path";
import { adapterCoreRuntimePath, installedCoreRuntime, readRuntimeJSON } from "../core-runtime.mjs";
import { execPortableCommand } from "../command.mjs";

const packageName = "taskbelay-zcode";
const stable = value => {
  if (typeof value !== "string" || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u.test(value)) throw new Error("Invalid ZCode Adapter version");
  return value;
};
const defaultRun = (executable, args, { environment = process.env, ...options } = {}) =>
  execPortableCommand(executable, args, { ...options, env: environment, encoding: "utf8", windowsHide: true, timeout: 120000, maxBuffer: 8 * 1024 * 1024 });

export function createZCodeDriver({ paths, environment = process.env, run = defaultRun, localPackage = null } = {}) {
  const call = (executable, args) => run(executable, args, { environment });
  const adapterReport = async operation => {
    const command = `${packageName} ${operation} --json`;
    const { stdout } = await call(packageName, [operation, "--json"]);
    if (typeof stdout !== "string" || !stdout.trim()) throw new Error(`${command} returned empty JSON output`);
    let report;
    try { report = JSON.parse(stdout); }
    catch (cause) { throw new Error(`${command} returned invalid JSON`, { cause }); }
    if (report === null || typeof report !== "object" || Array.isArray(report)) throw new Error(`${command} must return a JSON object`);
    return report;
  };
  const receiptPath = join(paths.productRoot, "registrations", "zcode.json");
  const removalSteps = ["In ZCode Settings > Plugins, uninstall taskbelay-zcode and remove the taskbelay-zcode-local marketplace. Start a new session to unload its tools and hooks."];
  async function readReceipt() {
    const receipt = await readRuntimeJSON(receiptPath, "ZCode receipt");
    if (receipt === null) return null;
    if (receipt.product?.name !== packageName || receipt.host?.surface !== "zcode-ui" ||
        `${receipt.host.os}-${receipt.host.arch}` !== paths.runtimeKey ||
        !["prepared", "removal_required"].includes(receipt.phase) ||
        !isAbsolute(receipt.paths?.package_root ?? "") || !isAbsolute(receipt.paths?.runtime_path ?? "") ||
        !isAbsolute(receipt.paths?.data_dir ?? "") ||
        resolve(receipt.paths?.receipt_path ?? "") !== resolve(receiptPath) ||
        resolve(receipt.paths?.marketplace_path ?? "") !== resolve(receipt.paths.package_root, "marketplace.json") ||
        resolve(receipt.paths.runtime_path) !== resolve(adapterCoreRuntimePath(receipt.paths.package_root, paths))) {
      throw new Error("ZCode receipt identity or owned paths are invalid");
    }
    stable(receipt.product.version);
    if (receipt.phase === "prepared") {
      stable(receipt.product.core_version);
      if (!/^[a-f0-9]{64}$/u.test(receipt.package_digest ?? "")) throw new Error("ZCode package digest is invalid");
    } else if (receipt.product.core_version !== null) stable(receipt.product.core_version);
    return receipt;
  }
  return {
    async runtimeCandidates() {
      const receipt = await readReceipt();
      if (receipt === null || receipt.phase !== "prepared") return [];
      return [{ source: "zcode", packageName, packageVersion: receipt.product.version,
        expectedCoreVersion: receipt.product.core_version, packageRoot: receipt.paths.package_root, runtimePath: receipt.paths.runtime_path }];
    },
    async maintenanceTargets({ observed, operation, agentClosed = false }) {
      const receipt = await readReceipt();
      // ZCode copies plugins into a private cache. There is no supported cache/process
      // inventory API, so a data reset cannot prove that every cached Core is stopped.
      if (["delete-data", "delete-all"].includes(operation) && (observed.packageInstalled || receipt !== null) && !agentClosed) {
        throw new Error("ZCode plugin caches cannot be verified automatically. Close every ZCode window and session, then explicitly confirm --confirm-zcode-closed before deleting shared Task data.");
      }
      let installedRuntime = null;
      if (observed.packageInstalled) {
        const npmRoot = (await call("npm", ["root", "--global"])).stdout.trim();
        if (!isAbsolute(npmRoot)) throw new Error("npm did not return an absolute global package directory");
        installedRuntime = await installedCoreRuntime(join(npmRoot, packageName), paths, { host: "zcode", profile: null, packageName });
      }
      return { registeredCorePaths: receipt?.phase === "prepared" ? [receipt.paths.runtime_path] : [], installedRuntime };
    },
    async observe() {
      const issues = [];
      let report = null, packageVersion = null, receipt = null;
      try {
        report = await adapterReport("status");
        packageVersion = stable(report.package_version);
        if (!["partial", "action_required"].includes(report.status) || !Array.isArray(report.next_steps)) throw new Error("Invalid ZCode installation status");
      } catch (error) {
        report = null;
        try {
          let listing;
          try { listing = await call("npm", ["list", "--global", "--depth=0", "--json", packageName]); }
          catch (failure) { if (!failure.stdout) throw failure; listing = failure; }
          const installed = JSON.parse(listing.stdout).dependencies?.[packageName];
          if (installed) packageVersion = stable(installed.version);
        } catch (failure) { issues.push({ code: "package_check_failed", message: failure.message, command: "taskbelay doctor --host zcode" }); }
        if (packageVersion) issues.push({ code: "adapter_check_failed", message: error.message, command: "taskbelay repair --host zcode --yes" });
      }
      try { receipt = await readReceipt(); }
      catch (error) { issues.push({ code: "registration_check_failed", message: error.message, command: "taskbelay doctor --host zcode" }); }
      if (report?.status === "partial") issues.push({ code: "adapter_check_failed",
        message: report.issues?.join("; ") || "The local ZCode package needs preparation or repair.", command: "taskbelay repair --host zcode --yes" });
      const pendingRemoval = receipt?.phase === "removal_required";
      const nextSteps = report?.next_steps ?? (receipt ? removalSteps : []);
      if (receipt || report?.status === "action_required") {
        issues.push({ code: pendingRemoval || !packageVersion ? "host_removal_required" : "host_action_required",
          message: nextSteps.join(" "), command: null });
      } else if (!packageVersion && !issues.length) {
        issues.push({ code: "adapter_missing", message: "TaskBelay ZCode Adapter is not installed.", command: "taskbelay install --host zcode --yes" });
      }
      const failed = issues.some(issue => ["package_check_failed", "adapter_check_failed", "registration_check_failed"].includes(issue.code));
      const state = failed ? "partial" : report?.status === "action_required" || receipt ? "action_required" : packageVersion ? "partial" : "absent";
      return { host: "zcode", profile: null, hostAvailable: null, hostVersion: null, state,
        packageInstalled: Boolean(packageVersion), packageVersion, coreVersion: report?.core_version ?? receipt?.product.core_version ?? null,
        receipt: receipt !== null, localReady: report?.status === "action_required" && report.registration?.phase === "prepared" && !failed,
        orphanedRegistration: !packageVersion && receipt !== null, nextSteps, issues };
    },
    async resolveTargetVersion(target) {
      if (localPackage) return stable(localPackage.version);
      const versions = JSON.parse((await call("npm", ["view", `${packageName}@${target}`, "version", "--json"])).stdout);
      return stable(Array.isArray(versions) ? versions.at(-1) : versions);
    },
    async execute(operation, { targetVersion, observed, onProgress = () => {}, onStepStart = () => {} }) {
      const completedSteps = [];
      const step = async (name, executable, args) => {
        onStepStart(name);
        const result = await call(executable, args);
        completedSteps.push(name); onProgress(name);
        return result;
      };
      try {
        if (operation === "uninstall") {
          if (!observed.packageInstalled) return { changed: false, completedSteps, nextSteps: observed.receipt ? removalSteps : [] };
          if (observed.receipt) {
            const removed = JSON.parse((await step("zcode.remove_registration", packageName, ["remove", "--json"])).stdout);
            if (removed.status !== "action_required" || removed.registration?.phase !== "removal_required") throw new Error("ZCode removal readback failed");
            // Keep the confirmation command installed until the user has removed
            // the cached plugin and closed its sessions in ZCode.
            return { changed: removed.changed, completedSteps, nextSteps: [
              ...removed.next_steps,
              "After confirming Host removal, run taskbelay uninstall --host zcode --yes to uninstall the Adapter package.",
            ] };
          }
          await step("zcode.uninstall_package", "npm", ["uninstall", "--global", packageName]);
          return { changed: true, completedSteps, nextSteps: [] };
        }
        await step("zcode.install_package", "npm", ["install", "--global", localPackage?.path ?? `${packageName}@${stable(targetVersion)}`]);
        onStepStart("zcode.setup_registration");
        const prepared = await adapterReport("setup");
        if (prepared.status !== "action_required" || prepared.package_version !== targetVersion ||
            prepared.registration?.phase !== "prepared" || !Array.isArray(prepared.next_steps)) {
          throw new Error("taskbelay-zcode setup --json did not confirm the requested local preparation");
        }
        completedSteps.push("zcode.setup_registration"); onProgress("zcode.setup_registration");
        const readback = await adapterReport("status");
        if (readback.status !== "action_required" ||
            readback.package_version !== targetVersion || readback.registration?.phase !== "prepared" || !Array.isArray(readback.next_steps)) {
          throw new Error("taskbelay-zcode status --json local preparation readback failed");
        }
        return { changed: true, completedSteps, nextSteps: readback.next_steps };
      } catch (error) {
        error.completedSteps = completedSteps;
        error.changed = error.changed === true || completedSteps.length > 0;
        error.nextStep = `taskbelay ${operation === "uninstall" ? "uninstall" : "repair"} --host zcode --yes`;
        throw error;
      }
    },
  };
}
