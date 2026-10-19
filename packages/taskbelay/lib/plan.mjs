import { planDigest } from "./ownership.mjs";
import { AGENTS, isDataDeletion } from "./cli.mjs";

const versionedOperations = new Set(["install", "upgrade", "repair", "reinstall"]);
export const targetKey = target => `${target.host}:${target.profile ?? "default"}`;
export const isInstalled = target => target && (target.packageInstalled === true || Boolean(target.packageVersion) || Boolean(target.receipt) || target.contribution === true);

export function createLifecyclePlan(request, observed, {
  targetVersions = {}, launcherTarget = null, planId = null, token = null, now = () => new Date(),
  platformKey = "darwin-arm64", recoverableCleanupDescription = "Move confirmed data to macOS Trash",
  replaceLocalPackages = false, installDesktopPet = false,
} = {}) {
  const deletion = isDataDeletion(request.operation);
  if (deletion && (request.hosts.length !== AGENTS.length || !request.allKnownProfiles)) throw planConflict("shared data deletion requires all Agents and every known Profile");
  const targets = [observed.codex, ...observed.deepseek, observed.claude, observed.zcode].filter(Boolean)
    .filter(target => request.hosts.includes(target.host) && (target.host !== "deepseek" || request.allKnownProfiles || request.profiles.includes(target.profile)))
    .filter(target => request.operation !== "upgrade" || isInstalled(target));
  const cleanupTargets = deletion ? cleanupResources(observed.resources) : [];
  const actions = [];
  let downgrade = false;
  if (deletion) {
    if (request.operation === "delete-all") for (const target of targets) if (target.state !== "absent" || isInstalled(target)) actions.push(actionFor(target, "uninstall", null));
    if (actions.length || cleanupTargets.length) {
      actions.push({ actionId: "manager.cleanup", owner: "manager", operation: "cleanup", host: "manager", profile: null, targetVersion: null });
      if (request.operation === "delete-data") actions.push({ actionId: "manager.initialize", owner: "manager", operation: "initialize", host: "manager", profile: null, targetVersion: null });
    }
  } else if (versionedOperations.has(request.operation)) {
    for (const target of targets) {
      const next = targetVersions[targetKey(target)];
      if (!next) throw new Error(`target version is missing for ${targetKey(target)}`);
      if (target.packageVersion && compareVersions(target.packageVersion, next) > 0) downgrade = true;
      const ready = (target.state === "ready" || target.state === "action_required" && target.localReady === true) && target.packageVersion === next;
      if (replaceLocalPackages || request.operation === "reinstall" || request.adopt || !ready) actions.push(actionFor(target, request.operation, next));
    }
  } else if (request.operation === "uninstall") {
    for (const target of targets) if (target.state !== "absent" || isInstalled(target)) actions.push(actionFor(target, "uninstall", null));
  }
  if (installDesktopPet && targets.length && versionedOperations.has(request.operation)) actions.push({ actionId: "pet.install", owner: "manager", operation: "install_pet", host: "manager", profile: null, targetVersion: null });
  if (launcherTarget) {
    if (compareVersions(observed.launcher.packageVersion, launcherTarget.version) > 0) downgrade = true;
    if (launcherTarget.channel === "local" || observed.launcher.packageVersion !== launcherTarget.version) actions.push({ actionId: "taskbelay.upgrade", owner: "taskbelay", operation: "upgrade", host: "taskbelay", profile: null, targetVersion: launcherTarget.version });
  }
  const selectedTargets = [...targets, ...(launcherTarget ? [observed.launcher] : [])];
  const observedDigest = planDigest({ targets: selectedTargets, resources: observed.resources });
  const stable = planDigest({ operation: request.operation, hosts: request.hosts, profiles: request.profiles, allKnownProfiles: request.allKnownProfiles,
    targetVersion: request.targetVersion, permanent: request.permanent, adopt: request.adopt, platformKey, observedDigest, actions, launcherTarget });
  const confirmationClass = !actions.length ? "none" : deletion ? request.permanent ? "permanent_delete" : "delete" : downgrade ? "downgrade" : "mutation";
  const suffix = token ? token() : stable.slice(0, 6).toUpperCase();
  const impacts = [];
  if (["status", "doctor"].includes(request.operation)) impacts.push("Read Host and Adapter state only");
  else if (deletion && actions.length) {
    impacts.push(request.operation === "delete-all" ? "Remove every installed Adapter before shared data cleanup" : "Preserve plugins, Agent registrations and installation records");
    if (observed.resources.configuration.exists) impacts.push("Clear TaskBelay user configuration");
    if (observed.resources.defaultData.exists) impacts.push("Clear current default Task data");
    if (cleanupTargets.some(target => target.label.startsWith("pet"))) impacts.push("Clear desktop pet records, preferences, and imported appearances");
    if (cleanupTargets.some(target => target.requiresExplicitConfirmation)) impacts.push("Clear the explicitly confirmed Task data directory");
    impacts.push("Clear completed maintenance records; retain the current operation result", "Preserve the launcher, desktop application, project code and Agent conversation history");
    impacts.push(request.permanent ? "Permanently remove confirmed data" : recoverableCleanupDescription);
    if (request.operation === "delete-data") impacts.push("Create empty data directories at the original paths and restore default preferences");
  } else if (deletion) impacts.push("No installed Adapter or active TaskBelay data was found");
  else impacts.push("Preserve TaskBelay user configuration and Task data");
  if (installDesktopPet && actions.some(action => action.operation === "install_pet")) impacts.push("Update desktop pet application; preserve settings and appearances");
  const restartRequirements = request.operation === "delete-data" && actions.length
    ? [...new Set(targets.filter(isInstalled).map(target => `Restart ${target.host}${target.profile ? ` Profile ${target.profile}` : " Agent sessions"}`))]
    : actions.filter(action => action.owner === "deepseek" && action.operation !== "uninstall").map(action => `Restart DeepSeek Profile ${action.profile}`);
  if (actions.some(action => action.owner === "taskbelay")) restartRequirements.push("Run taskbelay again to use the updated launcher.");
  const core = { planId: planId ?? `plan-${stable.slice(0, 20)}`, operation: request.operation, hosts: request.hosts,
    targets: selectedTargets.map(target => ({ host: target.host, profile: target.profile, state: target.state, packageInstalled: isInstalled(target), packageVersion: target.packageVersion })),
    actions, impacts, restartRequirements, resources: deletion ? cleanupTargets.map(({ label, path }) => ({ label, path })) : [],
    cleanupTargets, confirmationClass, observedDigest, launcherTarget };
  return Object.freeze({ ...core, digest: planDigest(core), createdAt: now().toISOString(),
    confirmationToken: deletion && actions.length ? `${request.operation.toUpperCase()}-${suffix}` : null,
    permanentToken: confirmationClass === "permanent_delete" ? `PERMANENT-${stable.slice(6, 12).toUpperCase()}` : null,
    downgradeToken: confirmationClass === "downgrade" ? `DOWNGRADE-${suffix}` : null });
}

function cleanupResources(resources) {
  const targets = new Map();
  for (const resource of [resources.configuration, resources.defaultData, ...(resources.petData ?? []), resources.explicitData, ...(resources.completedRuns ?? [])]) {
    if (!resource?.exists) continue;
    const existing = targets.get(resource.path);
    if (existing) {
      if (existing.identity !== resource.identity || existing.kind !== resource.kind) throw planConflict("cleanup target changed during observation");
      existing.requiresExplicitConfirmation ||= resource.label === "explicit-data";
    } else targets.set(resource.path, { ...resource, requiresExplicitConfirmation: resource.label === "explicit-data" });
  }
  return [...targets.values()];
}
function actionFor(target, operation, targetVersion) { return { actionId: `${target.host}.${target.profile ?? "default"}.${operation}`, owner: target.host, operation, host: target.host, profile: target.profile, targetVersion }; }
export function compareVersions(left, right) { const a = left.split(".").map(Number), b = right.split(".").map(Number); for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] - b[i]; return 0; }
function planConflict(message) { const error = new Error(message); error.exitCode = 4; return error; }
