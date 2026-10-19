import { describeInterface } from "./interface-reference.mjs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";


import { diagnoseInstallation } from "./diagnostics.mjs";
import { stopManagedCores } from "./core-maintenance.mjs";
import { CLIError, confirmPlan, parseArguments, promptForRequest, renderHelp, isDataDeletion, requestArguments, AGENTS } from "./cli.mjs";
import { CODEX_ACTIVATION_STEP } from "./hosts/codex.mjs";
import { createHostDrivers } from "./hosts/index.mjs";
import { createRun, recordRun } from "./journal.mjs";
import {
  inspectResource,
  moveTargetsToTrash,
  permanentlyRemoveTargets,
  resolveManagerPaths,
} from "./ownership.mjs";
import { supportsDesktopPet, loadPetInstaller, loadPetPlatform, loadMaintenancePlatform } from "./platform.mjs";

import { runPet, stopPetForCore } from "./pet.mjs";
import { createLifecyclePlan, isInstalled, targetKey, compareVersions } from "./plan.mjs";
import { renderPlan, renderProgress, renderResult, resolveLanguage } from "./presentation.mjs";
import { createTerminalSession, BACK, CANCEL } from "./terminal.mjs";
import { formatCommand } from "./command.mjs";
import { runTaskBelay } from "./runtime.mjs";
import { installedPackageRoot, readLocalPackages } from "./local-packages.mjs";
import { createLauncherDriver } from "./launcher.mjs";
import { observeCleanupResources, assertCleanupLocations, initializeEmptyData } from "./cleanup.mjs";
import { promptAfterResult } from "./menu.mjs";

export async function runMain(arguments_, dependencies = {}) {
  if (arguments_.length === 3 && arguments_[1] === "--help" && arguments_[2] === "--json") {
    try {
    const contract = await describeInterface(fileURLToPath(new URL("../", import.meta.url)), "taskbelay.lifecycle." + arguments_[0]);
    (dependencies.output ?? process.stdout).write(JSON.stringify(contract, null, 2) + "\n");
    return { code: 0 };
    } catch (error) { (dependencies.errorOutput ?? process.stderr).write(error.message + "\n");return {code:2}; }
  }
  const input = dependencies.input ?? process.stdin;
  const output = dependencies.output ?? process.stdout;
  const errorOutput = dependencies.errorOutput ?? process.stderr;
  let request;
  let session;
  const menuState = {};
  const cancellation = new AbortController();
  const signalSource = dependencies.signalSource ?? process;
  const interrupt = () => {
    cancellation.abort();
    session?.close();
    (arguments_.includes("--json") ? errorOutput : output).write(language === "zh-CN" ? "\n正在停止操作；已完成的步骤会保留，请先检查当前状态。\n" : "\nStopping; completed steps are retained. Inspect current state before retrying.\n");
  };
  const environment = dependencies.environment ?? process.env;
  const language = resolveLanguage(environment);
  const isTTY = dependencies.isTTY ?? Boolean(input.isTTY && output.isTTY);
  const interactive = arguments_.length === 0 && isTTY;
  signalSource.on("SIGINT", interrupt);
  try {
    if (!arguments_.length && !isTTY) { output.write(renderHelp(null, language)); return { code: 0 }; }
    request = parseArguments(arguments_, { isTTY, noColor: Object.hasOwn(environment, "NO_COLOR") });
    if (request.help) { output.write(renderHelp(request.help, language)); return { code: 0 }; }
    if (interactive) session = dependencies.session ?? createTerminalSession(input, output, { language, environment });
    do {
      try {
        if (interactive) {
          output.write(language === "zh-CN" ? "正在检查安装…\n" : "Checking installation…\n");
          let observation;
          try {
            observation = (await runLifecycle(parseArguments(["status", "--host", "all", "--all-known-profiles"], { isTTY: false }), { ...dependencies, signal: cancellation.signal })).result;
          } catch (error) { observation = failureResult("status", error); }
          if (cancellation.signal.aborted) return { code: 130 };
          request = await (dependencies.promptForRequest ?? promptForRequest)({ input, output, language, environment,
            platform: dependencies.platform, arch: dependencies.arch, observation, session, state: menuState, petState: observation?.pet_state ?? "unknown",
            inspectUpgrade: options => inspectUpgradeOptions(options, { ...dependencies, signal: cancellation.signal }) });
        }
        if (request.cancelled) return { code: cancellation.signal.aborted ? 130 : 0 };
        if (request.pet || request.webui) {
          const options = { ...dependencies, stdout: output, stderr: errorOutput, environment, language };
          const result = request.pet
            ? await (dependencies.runPet ?? runPet)(["pet", request.pet], options)
            : await (dependencies.runTaskBelay ?? runTaskBelay)(["webui", request.webui], options);
          if (!interactive || cancellation.signal.aborted) return { code: cancellation.signal.aborted ? 130 : result.code };
          if (!await promptAfterResult({ operation: request.pet ? "pet" : "webui", status: result.code ? "failed" : "ready" }, { session, state: menuState, language })) return { code: result.code };
          continue;
        }
        const onProgress = request.outputMode === "json" ? undefined : event => output.write(renderProgress(event, { language }));
        const result = await runLifecycle(request, { ...dependencies, input, output, environment, language, session, onProgress, signal: cancellation.signal });
        if (result.back) continue;
        output.write(renderResult(result.result, { mode: request.outputMode, language }));
        if (cancellation.signal.aborted) return { code: 130 };
        if (!interactive || result.result.completed_actions?.includes("taskbelay.upgrade")) return { code: result.code };
        if (!await promptAfterResult(result.result, { session, state: menuState, language })) return { code: result.code };
      } catch (error) {
        if (cancellation.signal.aborted) error.interrupted = true;
        const result = failureResult(request?.operation ?? "status", error);
        const mode = request?.outputMode === "json" || arguments_.includes("--json") ? "json" : "plain";
        (mode === "json" ? output : errorOutput).write(renderResult(result, { mode, language }));
        if (cancellation.signal.aborted) return { code: 130 };
        if (!interactive || error.failedAction === "taskbelay.upgrade" || (error.completedSteps?.includes("taskbelay.install_package") || error.completedSteps?.includes("taskbelay.upgrade"))) return { code: error.exitCode ?? 1 };
        if (!await promptAfterResult(result, { session, state: menuState, language })) return { code: error.exitCode ?? 1 };
      }
    } while (interactive && !session.cancelled);
    return { code: 0 };
  } catch (error) {
    const mode = arguments_.includes("--json") ? "json" : "plain";
    (mode === "json" ? output : errorOutput).write(renderResult(failureResult(arguments_[0] ?? "status", error), { mode, language }));
    return { code: error.exitCode ?? 1 };
  } finally { session?.close(); signalSource.off("SIGINT", interrupt); }
}

export async function runLifecycle(request, dependencies = {}) {
  const environment = dependencies.environment ?? process.env;
  const packageRoot = dependencies.packageRoot ?? installedPackageRoot;
  const usesArtifacts = ["install", "upgrade", "repair", "reinstall"].includes(request.operation) && request.hosts.length > 0;
  const deletion = isDataDeletion(request.operation);
  const launcher = dependencies.launcherDriver ?? createLauncherDriver({ packageRoot, environment, signal: dependencies.signal });
  const packagedLocalPackages = usesArtifacts ? await readLocalPackages(packageRoot) : null;
  const localPackages = dependencies.localPackages ?? packagedLocalPackages;
  if (localPackages && request.targetVersion && request.targetVersion !== "latest" && ["install", "upgrade", "repair", "reinstall"].includes(request.operation)) {
    const products = request.hosts;
    if (products.some(product => localPackages[product].version !== request.targetVersion)) {
      throw new Error("the selected version is not contained in this local development package");
    }
  }
  let paths = await (dependencies.resolveManagerPaths ?? resolveManagerPaths)({
    homeDirectory: dependencies.homeDirectory,
    environment,
    platform: dependencies.platform,
    arch: dependencies.arch,
  });
  const makeDrivers = () => (dependencies.createHostDrivers ?? createHostDrivers)({ ...dependencies, paths, environment, localPackages });
  let drivers = makeDrivers();
  dependencies.onProgress?.({ type: "phase", message: (dependencies.language ?? resolveLanguage(environment)) === "zh-CN" ? "检查 Host、Adapter 与本地资源" : "Checking Hosts, Adapters and local resources" });
  const observed = await observeLifecycle(request, { paths, drivers });
  observed.launcher = await launcher.observe();
  if (["install", "upgrade", "repair", "reinstall"].includes(request.operation)) {
    const missing = [observed.codex, ...observed.deepseek, observed.claude, observed.zcode].filter(target => target?.hostAvailable === false && (request.operation !== "upgrade" || isInstalled(target)));
    if (missing.length) {
      const error = new Error(`Required Host unavailable: ${missing.map(target => target.host).join(", ")}`);
      error.nextStep = missing[0].issues?.find(issue => issue.code === "host_missing")?.command ?? `taskbelay doctor --host ${missing[0].host}`;
      throw error;
    }
    dependencies.onProgress?.({ type: "phase", message: (dependencies.language ?? resolveLanguage(environment)) === "zh-CN" ? "确认目标版本" : "Resolving target versions" });
  }
  const targetVersions = await resolveTargetVersions(request, observed, { drivers, localPackages });
  const launcherTarget = request.self ? await launcher.resolveTarget({ targetVersion: request.targetVersion ?? "latest", selfPackage: request.selfPackage }) : null;
  const petPlatform = usesArtifacts && supportsDesktopPet(paths.platform, paths.arch)
    ? await loadPetPlatform(paths.platform, paths.arch) : null;
  const installDesktopPet = petPlatform !== null && await petPlatform.isBundledPetApplicationAvailable(petPlatform.bundledPetExecutable(packageRoot));
  const plan = createLifecyclePlan(request, observed, {
    targetVersions, launcherTarget,
    planId: dependencies.planId,
    token: dependencies.token,
    now: dependencies.now,
    platformKey: paths.runtimeKey,
    recoverableCleanupDescription: paths.recoverableCleanupDescription,
    replaceLocalPackages: localPackages !== null && localPackages !== undefined,
    installDesktopPet,
  });

  if (["status", "doctor"].includes(request.operation)) {
    const result = resultFromObservation(request.operation, observed);
    result.launcher = observed.launcher;
    result.pet_state = (await inspectResource(join(paths.petDirectory, "runtime.json"), "pet-runtime")).exists ? "unknown" : "stopped";
    result.installation = { configuration: paths.configurationPath, data: paths.explicitDataDirectory ?? paths.defaultDataDirectory, pet: paths.petDirectory, manager: paths.managerRoot };
    if (request.operation === "doctor") {
      result.checks = await diagnoseInstallation(observed, { host: request.hosts.length === 1 ? request.hosts[0] : "all", paths, environment, validateConfiguration: dependencies.validateConfiguration });
      if (result.checks.some(check => check.status === "failed") && result.status === "ready") result.status = "partial";
    }
    return { code: request.operation === "doctor" && result.status !== "ready" ? 1 : 0, plan: null, result };
  }

  if (request.outputMode !== "json") (dependencies.output ?? process.stdout).write(renderPlan(plan, {
    mode: request.outputMode, language: dependencies.language ?? resolveLanguage(environment),
  }));
  const confirmed = await (dependencies.confirmPlan ?? confirmPlan)(plan, request, {
    input: dependencies.input ?? process.stdin,
    output: dependencies.output ?? process.stdout,
    language: dependencies.language ?? resolveLanguage(environment),
    session: dependencies.session,
  });
  if (confirmed === BACK) return { back: true };
  if (!confirmed) {
    return {
      code: 3,
      plan,
      result: confirmationResult(request, plan, paths.platform),
    };
  }

  if (plan.actions.length === 0) {
    const result = resultFromObservation(request.operation, observed);
    if (request.self) { result.launcher = observed.launcher; if (!request.hosts.length) result.status = "ready"; }
    return { code: 0, plan, result };
  }

  // Confirm all cleanup decisions before stopping services or removing plugins.
  if (deletion) {
    assertCleanupLocations(paths, plan.cleanupTargets, [packageRoot]);
    if (observed.zcode && isInstalled(observed.zcode) && !request.zcodeClosed) {
      if (request.fromMenu && dependencies.session) {
        const closed = await dependencies.session.select(dependencies.language === "zh-CN" ? "关闭 ZCode 的全部窗口和会话后继续" : "Close all ZCode windows and sessions before continuing", [
          { value: false, label: dependencies.language === "zh-CN" ? "返回，不删除" : "Back, do not delete" },
          { value: true, label: dependencies.language === "zh-CN" ? "我已关闭 ZCode 全部窗口和会话" : "I closed all ZCode windows and sessions" },
        ]);
        if (closed !== true) return { back: true };
        request = { ...request, zcodeClosed: true };
      } else {
        const error = new Error("Close all ZCode windows and sessions, then pass --confirm-zcode-closed; cached Core processes cannot be fully enumerated automatically");
        error.exitCode = 4; throw error;
      }
    }
  }
  for (const target of plan.cleanupTargets.filter(target => target.requiresExplicitConfirmation && !request.confirmedExplicitData.includes(target.path))) {
    const path = target.path;
    if (request.fromMenu && dependencies.session) {
      const answer = await dependencies.session.question(`${dependencies.language === "zh-CN" ? "输入完整路径确认清理" : "Type the full path to confirm cleanup"}: ${path}\n> `);
      if (answer === BACK) return { back: true };
      if (answer !== path) return { code: 3, plan, result: confirmationResult(request, plan, paths.platform) };
      request = { ...request, confirmedExplicitData: [...request.confirmedExplicitData, path] };
    } else {
      const error = new Error(`explicit Task data requires exact --confirm-explicit-data ${path}`);
      error.exitCode = 4;
      error.nextStep = confirmationResult(request, plan, paths.platform).next_step;
      throw error;
    }
  }

  if (usesArtifacts) {
    const maintenance = await (dependencies.loadMaintenancePlatform ?? loadMaintenancePlatform)(paths.platform, paths.arch);
    if (await maintenance.prepareInstallation(paths)) {
      paths = await (dependencies.resolveManagerPaths ?? resolveManagerPaths)({
        homeDirectory: dependencies.homeDirectory, environment,
        platform: dependencies.platform, arch: dependencies.arch,
      });
      drivers = makeDrivers();
    }
  }
  dependencies.onProgress?.({ type: "phase", message: (dependencies.language ?? resolveLanguage(environment)) === "zh-CN" ? "准备维护，停止受影响的桌面进程" : "Preparing maintenance and stopping affected desktop processes" });
  const launcherInstaller = request.self ? dependencies.petInstaller ?? await loadPetInstaller(paths.platform, paths.arch) : null;
  const resetCores = await prepareMaintenance(request, plan, { paths, environment, drivers, dependencies, replaceDesktop: installDesktopPet || request.self });

  let run = await (dependencies.createRun ?? createRun)(paths, plan, { now: dependencies.now, operationId: dependencies.operationId });
  const completedActions = [];
  let changed = false;
  let trashRoot = null;
  let currentAction = null;
  const nextSteps = [];
  try {
    for (const action of plan.actions) {
      currentAction = action;
      if (dependencies.signal?.aborted) throw Object.assign(new Error("Maintenance interrupted; inspect current state before retrying"), { interrupted: true });
      dependencies.onProgress?.({ type: "action_start", action });
      let effect;
      if (Object.hasOwn(drivers, action.owner)) {
        const driver = drivers[action.owner];
        const current = await driver.observe(action.profile);
        effect = await driver.execute(action.operation, {
          profile: action.profile,
          targetVersion: action.targetVersion,
          observed: current,
          adopt: request.adopt,
          onProgress: (stepId) => dependencies.onProgress?.({ type: "step_complete", action, stepId }),
          onStepStart: (stepId) => dependencies.onProgress?.({ type: "step_start", action, stepId }),
        });
      } else if (action.owner === "taskbelay") {
        effect = await launcher.execute(plan.launcherTarget);
        try {
          if (launcherInstaller) await launcherInstaller.ensurePetInstalled({ petDirectory: paths.petDirectory, sourcePackageRoots: [packageRoot], enforcePrivateModes: paths.enforcePrivateModes, replaceExisting: true });
        } catch (error) { error.changed = true; error.completedSteps = effect.completedSteps; throw error; }
      } else if (action.operation === "install_pet") {
        const installer = dependencies.petInstaller ?? await loadPetInstaller(paths.platform, paths.arch);
        const installation = await installer.ensurePetInstalled({
          petDirectory: paths.petDirectory, sourcePackageRoots: [packageRoot],
          enforcePrivateModes: paths.enforcePrivateModes, replaceExisting: true,
        });
        if (!installation.installed) throw new Error("the package did not install its desktop application");
        effect = { changed: installation.newlyInstalled === true };
      } else if (action.operation === "cleanup") {
        if (request.operation === "delete-all") {
          const remaining = await observeLifecycle(request, { paths, drivers, includeCleanup: false });
          const retained = [remaining.codex, ...remaining.deepseek, remaining.claude, remaining.zcode].filter(target => target && (isInstalled(target) || target.state !== "absent"));
          if (retained.length) {
            const error = new Error(`Plugins remain installed; finish their removal before deleting shared data: ${retained.map(target => target.host).join(", ")}`);
            error.nextStep = nextSteps.join("\n") || `taskbelay uninstall --host ${retained[0].host}`;
            throw error;
          }
        }
        await resetCores.verifyStopped();
        effect = await executeCleanup(request, plan.cleanupTargets, paths, dependencies);
        trashRoot = effect.trashRoot ?? null;
      } else if (action.operation === "initialize") {
        effect = await initializeEmptyData(paths, plan.cleanupTargets);
        await resetCores.verifyStopped();
      } else {
        throw new Error(`unsupported planned action ${action.actionId}`);
      }
      changed ||= effect.changed;
      if (effect.nextSteps) nextSteps.push(...effect.nextSteps);
      completedActions.push(action.actionId, ...(effect.completedSteps ?? []));
      if (action.operation === "initialize" || action.operation === "cleanup" && request.operation === "delete-all") await resetCores.completeCleanup();
      dependencies.onProgress?.({ type: "action_complete", action });
      run = await (dependencies.recordRun ?? recordRun)(paths, run, {
        completed_action_ids: [...completedActions],
        failed_action_id: null,
        temporary_roots: [...new Set([...run.temporary_roots, ...(effect.temporaryRoots ?? [])])],
        trash_root: trashRoot,
        next_step: "continue",
      }, { now: dependencies.now });
    }
  } catch (error) {
    if (dependencies.signal?.aborted) { error.interrupted = true; error.nextStep = "taskbelay doctor --host all --all-known-profiles"; }
    completedActions.push(...(error.completedSteps ?? []));
    await (dependencies.recordRun ?? recordRun)(paths, run, {
      completed_action_ids: [...new Set(completedActions)],
      failed_action_id: currentAction?.actionId ?? "unknown",
      trash_root: error.trashRoot ?? trashRoot,
      next_step: error.nextStep ?? retryCommand(request, currentAction, paths.platform),
    }, { now: dependencies.now }).catch(() => {});
    error.exitCode ??= completedActions.length > 0 ? 5 : 1;
    error.changed = changed || (error.changed ?? Boolean(error.completedSteps?.some(step => !step.endsWith("verify_artifact"))));
    error.trashRoot ??= trashRoot;
    error.completedSteps = completedActions;
    error.operationId = run.operation_id;
    error.failedAction = currentAction?.actionId ?? null;
    error.dataPolicy = deletion ? request.permanent ? "permanent_delete" : "recoverable_delete" : "preserve";
    error.nextStep ??= retryCommand(request, currentAction, paths.platform);
    throw error;
  }

  let finalObserved;
  try { finalObserved = await observeLifecycle(request, { paths, drivers, includeCleanup: false }); }
  catch (error) {
    error.completedSteps = completedActions;
    error.operationId = run.operation_id;
    error.failedAction = "verify_installation";
    error.exitCode = 5;
    error.changed = changed;
    error.trashRoot = trashRoot;
    error.dataPolicy = deletion ? request.permanent ? "permanent_delete" : "recoverable_delete" : "preserve";
    await (dependencies.recordRun ?? recordRun)(paths, run, { failed_action_id: "verify_installation", next_step: "taskbelay doctor --host all" }, { now: dependencies.now }).catch(() => {});
    error.nextStep = "taskbelay doctor --host all";
    throw error;
  }
  const result = resultFromObservation(request.operation, finalObserved, {
    operationId: run.operation_id,
    changed,
    completedActions: [...new Set(completedActions)],
    restartRequirements: plan.restartRequirements,
    dataPolicy: deletion ? request.permanent ? "permanent_delete" : "recoverable_delete" : "preserve",
    trashRoot,
  });
  result.next_steps = [...new Set([...result.next_steps, ...nextSteps, ...plan.restartRequirements])];
  const actionRequired = result.status === "action_required";
  const verified = request.operation === "delete-data" ? true : request.operation === "delete-all" ? result.status === "absent"
    : actionRequired || (request.operation === "uninstall" ? result.status === "absent" : request.self && request.hosts.length === 0 || result.status === "ready" || result.status === "restart_required" || request.operation === "upgrade" && result.status === "absent");
  if (request.self) { result.launcher = await launcher.observe(); if (!request.hosts.length) result.status = "ready"; }
  if (request.operation === "delete-data") {
    result.status = result.status === "partial" ? "partial" : plan.targets.some(target => target.packageInstalled) ? "restart_required" : "ready";
    result.next_step = result.status === "partial" ? "taskbelay doctor --host all --all-known-profiles" : plan.targets.some(target => target.packageInstalled) ? "Restart the affected Agent sessions before creating new tasks." : null;
  }
  if (!verified) { result.failed_action = "verify_installation"; result.next_step = "taskbelay doctor --host all"; }
  await (dependencies.recordRun ?? recordRun)(paths, run, {
    completed_action_ids: [...new Set(completedActions)],
    failed_action_id: verified ? null : "verify_installation",
    trash_root: trashRoot,
    next_step: actionRequired && request.operation !== "delete-data" ? "Complete the ZCode UI steps listed in next_steps" : verified ? "complete" : "taskbelay doctor --host all",
  }, { now: dependencies.now }).catch(() => {});
  return { code: verified ? 0 : 5, plan, result };
}

export async function observeLifecycle(request, { paths, drivers, includeCleanup = true }) {
  const { codex, deepseek, claude, zcode } = drivers;
  const knownDeepSeekProfiles = !request.hosts.includes("deepseek") ? [] : await deepseek.knownProfiles();
  let profiles = !request.hosts.includes("deepseek") ? [] : request.allKnownProfiles
    ? [...new Set([...knownDeepSeekProfiles, ...request.profiles])]
    : request.profiles;
  if (request.hosts.includes("deepseek") && profiles.length === 0 && ["install", "upgrade", "repair", "reinstall", "status", "doctor"].includes(request.operation)) profiles = ["web"];
  const [codexState, deepseekStates, configuration, defaultData, pet, explicitData] = await Promise.all([
    request.hosts.includes("codex") ? codex.observe() : Promise.resolve(null),
    Promise.all(profiles.map((profile) => deepseek.observe(profile))),
    inspectResource(paths.configurationPath, "configuration"),
    inspectResource(paths.defaultDataDirectory, "default-data"),
    inspectResource(paths.petDirectory, "pet"),
    paths.explicitDataDirectory ? inspectResource(paths.explicitDataDirectory, "explicit-data") : Promise.resolve(null),
  ]);
  const resources = { configuration, defaultData, pet, explicitData };
  return {
    codex: codexState,
    claude: request.hosts.includes("claude") ? await claude.observe() : null,
    zcode: request.hosts.includes("zcode") ? await zcode.observe() : null,
    deepseek: deepseekStates,
    knownDeepSeekProfiles,
    resources: isDataDeletion(request.operation) && includeCleanup ? await observeCleanupResources(paths, resources) : resources,
  };
}

async function resolveTargetVersions(request, observed, { drivers, localPackages }) {
  const result = {};
  if (!["install", "upgrade", "repair", "reinstall"].includes(request.operation)) return result;
  const targets = [observed.codex, ...observed.deepseek, observed.claude, observed.zcode].filter(Boolean).filter(target => request.operation !== "upgrade" || isInstalled(target));
  for (const target of targets) {
    const driver = drivers[target.host];
    const requested = request.targetVersion ?? (request.operation === "upgrade" ? "latest" : target.packageVersion ?? "latest");
    // A known installed version needs no registry access to produce an unchanged plan.
    result[`${target.host}:${target.profile ?? "default"}`] = !localPackages && requested === target.packageVersion
      ? requested : await driver.resolveTargetVersion(requested);
  }
  return result;
}

function retryCommand(request, action, platform) {
  if (isDataDeletion(request.operation)) {
    return formatCommand(["taskbelay", request.operation, ...(request.permanent ? ["--permanent"] : [])], platform);
  }
  if (action?.owner === "taskbelay") return "taskbelay version";
  const args = ["taskbelay", action?.operation === "uninstall" ? "uninstall" : "repair", "--host", action?.host ?? request.hosts[0] ?? "all"];
  if (action?.profile) args.push("--profile", action.profile);
  if (action?.targetVersion) args.push("--version", action.targetVersion);
  args.push("--yes");
  return formatCommand(args, platform);
}

/**
 * Drivers locate the managed executables before Host removal can erase registrations.
 * Data deletion keeps these locations for its final process check; ordinary maintenance uses
 * the platform replacement rules.
 */
async function prepareMaintenance(request, plan, { paths, environment, drivers, dependencies, replaceDesktop = false }) {
  const stop = dependencies.stopPetForCore ?? stopPetForCore;
  const shutdown = {
    environment,
    homeDirectory: dependencies.homeDirectory,
    platform: paths.platform,
    arch: paths.arch,
  };
  if (isDataDeletion(request.operation) || replaceDesktop) {
    // Both data operations stop the desktop before clearing its preferences.
    await stop({ ...shutdown, corePath: null });
  }
  const maintained = isDataDeletion(request.operation) ? plan.targets.filter(target => target.host !== "taskbelay" && (target.packageInstalled || target.state !== "absent")).map(target => ({ host: target.host, profile: target.profile })) : plan.actions.filter(action => Object.hasOwn(drivers, action.owner));
  if (maintained.length === 0 && !isDataDeletion(request.operation)) return null;
  const maintenance = await (dependencies.loadMaintenancePlatform ?? loadMaintenancePlatform)(paths.platform, paths.arch);
  const prepared = new Set();
  const resetTargets = [];
  for (const action of maintained) {
    const key = `${action.host}:${action.profile ?? "default"}`;
    if (prepared.has(key)) continue;
    prepared.add(key);
    const driver = drivers[action.host];
    const observed = await driver.observe(action.profile);
    const targets = await driver.maintenanceTargets({ profile: action.profile, observed, operation: request.operation, agentClosed: request.zcodeClosed });
    if (isDataDeletion(request.operation)) {
      assertCleanupLocations(paths, plan.cleanupTargets, [targets.installedRuntime?.packageRoot]);
      resetTargets.push({ ...targets, host: action.host });
      continue;
    }
    if (!replaceDesktop) {
      for (const corePath of targets.registeredCorePaths) await stop({ ...shutdown, corePath });
    }
    if (targets.installedRuntime) await maintenance.prepareReplacement({ runtime: targets.installedRuntime, paths, environment });
  }
  if (isDataDeletion(request.operation)) {
    return (dependencies.stopManagedCores ?? stopManagedCores)({
      targets: resetTargets,
      dataDirectories: plan.cleanupTargets.filter(target => target.label === "default-data" || target.requiresExplicitConfirmation).map(target => target.path),
      paths, environment, processes: maintenance,
      assertNoPendingRuns: dependencies.assertNoPendingRuns,
    });
  }
  return null;
}

async function executeCleanup(request, targets, paths, dependencies) {
  for (const target of targets) {
    if (target.requiresExplicitConfirmation && !request.confirmedExplicitData.includes(target.path)) {
      const error = new Error("explicit Task data requires exact --confirm-explicit-data");
      error.exitCode = 4;
      throw error;
    }
  }
  targets = await Promise.all(targets.map(async target => target.label === "pet-runtime.json" ? await inspectResource(target.path, target.label) : target));
  if (request.permanent) {
    const removed = await (dependencies.permanentlyRemoveTargets ?? permanentlyRemoveTargets)(targets, { allowedPaths: targets.map((target) => target.path) });
    return { changed: removed.length > 0, completedSteps: removed.map((entry) => `manager.remove.${entry.label}`), trashRoot: null };
  }
  const moved = await (dependencies.moveTargetsToTrash ?? moveTargetsToTrash)(paths, targets, { now: dependencies.now, random: dependencies.random });
  return { changed: moved.moved.length > 0, completedSteps: moved.moved.map((entry) => `manager.trash.${entry.label}`), trashRoot: moved.trashRoot };
}

function resultFromObservation(operation, observed, {
  operationId = null,
  changed = false,
  completedActions = [],
  restartRequirements = [],
  dataPolicy = "preserve",
  trashRoot = null,
} = {}) {
  const targets = [observed.codex, ...observed.deepseek, observed.claude, observed.zcode]
    .filter(Boolean)
    .map((target) => ({ host: target.host, profile: target.profile, package_installed: isInstalled(target), package_version: target.packageVersion, core_version: target.coreVersion ?? null, state: target.state, host_available: target.hostAvailable ?? null, issues: target.issues ?? [] }));
  const states = targets.map((target) => target.state);
  const status = states.some((state) => ["partial", "incompatible", "conflicted", "unknown"].includes(state)) ? "partial"
    : states.length === 0 || states.every((state) => state === "absent") ? "absent"
      : states.some((state) => state === "action_required") ? "action_required"
        : states.some((state) => state === "restart_required") ? "restart_required" : "ready";
  return {
    operation_id: operationId,
    operation,
    status,
    changed,
    targets,
    data: {
      policy: dataPolicy,
      configuration: observed.resources.configuration.exists ? "present" : "absent",
      default_data: observed.resources.defaultData.exists ? "present" : "absent",
      pet: observed.resources.pet.exists ? "present" : "absent",
      explicit_data: observed.resources.explicitData ? [observed.resources.explicitData.path] : [],
      trash_root: trashRoot,
    },
    completed_actions: completedActions,
    failed_action: null,
    next_steps: [...new Set([
      ...(["install", "repair", "upgrade", "reinstall"].includes(operation) && targets.some(target => target.host === "codex" && target.state === "ready") ? [CODEX_ACTIVATION_STEP] : []),
      ...(observed.zcode?.nextSteps ?? []),
    ])],
    restart_requirements: restartRequirements,
    confirmation: null,
    next_step: (["uninstall", "delete-all"].includes(operation) && status === "absent" ? null : targets.filter(target => status === "absent" || target.state !== "absent").flatMap(target => target.issues).find(issue => issue.command)?.command) ??
      (status === "absent" && !["uninstall", "delete-all"].includes(operation) ? "taskbelay install --host codex --yes" : status === "partial" ? "taskbelay doctor --host all" : operation === "status" && status === "ready" ? "taskbelay webui start" : null),
  };
}

function confirmationResult(request, plan, platform = process.platform) {
  const args = ["taskbelay", ...requestArguments(request)];
  if (isDataDeletion(request.operation) && plan.targets.some(target => target.host === "zcode" && target.packageInstalled) && !request.zcodeClosed) args.push("--confirm-zcode-closed");
  if (plan.confirmationToken) args.push("--confirm-delete", plan.confirmationToken);
  if (plan.permanentToken) args.push("--confirm-permanent", plan.permanentToken);
  if (plan.downgradeToken) args.push("--confirm-downgrade", plan.downgradeToken);
  if (plan.confirmationClass === "mutation") args.push("--yes");
  for (const target of plan.cleanupTargets) if (target.requiresExplicitConfirmation) args.push("--confirm-explicit-data", target.path);
  return {
    operation_id: null,
    operation: request.operation,
    status: "confirmation_required",
    changed: false,
    targets: plan.targets.map((target) => ({ host: target.host, profile: target.profile, package_version: target.packageVersion, core_version: null, state: target.state })),
    data: { policy: "preserve", configuration: "preserved", default_data: "preserved", pet: "preserved", explicit_data: [], trash_root: null },
    completed_actions: [],
    failed_action: null,
    restart_requirements: plan.restartRequirements,
    confirmation: {
      class: plan.confirmationClass,
      plan_id: plan.planId,
      token: plan.confirmationToken ?? plan.downgradeToken,
      permanent_token: plan.permanentToken,
      impacts: plan.impacts,
      resources: plan.resources,
      actions: plan.actions,
    },
    next_step: formatCommand(args, platform),
  };
}

function failureResult(operation, error) {
  return {
    operation_id: error.operationId ?? null,
    operation,
    error: { code: error.code ?? (error instanceof CLIError ? "INVALID_ARGUMENT" : "OPERATION_FAILED"), message: error.message, detail: String(error.stderr || error.cause?.stderr || error.cause?.message || "").trim().slice(0, 2048) },
    status: error.interrupted || error.uncertain || (error.changed ?? Boolean(error.completedSteps?.length)) ? "partial" : "failed",
    changed: error.interrupted || error.uncertain ? null : error.changed ?? Boolean(error.completedSteps?.length),
    targets: [],
    data: { policy: error.dataPolicy ?? "preserve", configuration: "unknown", default_data: "unknown", pet: "unknown", explicit_data: [], trash_root: error.trashRoot ?? null },
    completed_actions: error.completedSteps ?? [],
    failed_action: error.failedAction ?? null,
    restart_requirements: [],
    confirmation: null,
    next_step: error.nextStep ?? (error instanceof CLIError ? "taskbelay help" : "taskbelay doctor --host all"),
  };
}

/**
 * Resolve each update independently so an unavailable registry cannot turn a
 * failed lookup into "up to date" or prevent selecting other installed plugins.
 */
export async function inspectUpgradeOptions({ targetVersion = "latest", selfPackage = null } = {}, dependencies = {}) {
  const environment = dependencies.environment ?? process.env;
  const zh = resolveLanguage(environment) === "zh-CN";
  const packageRoot = dependencies.packageRoot ?? installedPackageRoot;
  const localPackages = dependencies.localPackages ?? await readLocalPackages(packageRoot);
  const paths = await (dependencies.resolveManagerPaths ?? resolveManagerPaths)({ homeDirectory: dependencies.homeDirectory, environment, platform: dependencies.platform, arch: dependencies.arch });
  const drivers = (dependencies.createHostDrivers ?? createHostDrivers)({ ...dependencies, paths, environment, localPackages });
  const request = parseArguments(["status", "--host", "all", "--all-known-profiles"], { isTTY: false });
  const observed = await observeLifecycle(request, { paths, drivers });
  const launcher = dependencies.launcherDriver ?? createLauncherDriver({ packageRoot, environment, signal: dependencies.signal });
  const currentLauncher = await launcher.observe();
  const choices = [];
  try {
    const next = await launcher.resolveTarget({ targetVersion, selfPackage });
    choices.push({ value: "self", label: zh ? "TaskBelay 命令入口" : "TaskBelay launcher", hint: `${currentLauncher.packageVersion} → ${next.version}${next.channel === "local" ? zh ? "（本地分发包）" : " (local distribution)" : ""}`,
      disabled: next.channel !== "local" && compareVersions(currentLauncher.packageVersion, next.version) === 0 });
  } catch (error) {
    choices.push({ value: "self", label: zh ? "TaskBelay 命令入口" : "TaskBelay launcher", hint: currentLauncher.channel === "local" && !selfPackage ? zh ? "本地安装：按 L 选择完整分发包" : "Local install: press L for a distribution" : `${zh ? "检查失败" : "Check failed"}: ${error.message}`, disabled: true });
  }
  for (const target of [observed.codex, ...observed.deepseek, observed.claude, observed.zcode].filter(target => target && (isInstalled(target) || ["partial", "unknown", "conflicted"].includes(target.state)))) {
    const option = { value: target.profile ? `deepseek:${target.profile}` : target.host, host: target.host, profile: target.profile,
      label: `${target.host}${target.profile ? ` / ${target.profile}` : ""}` };
    try {
      if (!isInstalled(target)) throw new Error(zh ? "无法确认安装状态，先检查安装" : "Installation state is uncertain; inspect it first");
      if (target.hostAvailable === false) throw new Error(zh ? "Agent 不可用，先检查安装" : "Agent unavailable; inspect installation first");
      const next = await drivers[target.host].resolveTargetVersion(targetVersion);
      const equal = target.packageVersion === next;
      option.hint = equal && !localPackages ? `${next} · ${zh ? "已是目标版本" : "Already at target version"}` : `${target.packageVersion ?? "?"} → ${next}${localPackages ? zh ? "（本地包）" : " (local package)" : ""}`;
      option.disabled = equal && !localPackages && ["ready", "action_required"].includes(target.state);
    } catch (error) { option.disabled = true; option.hint = `${zh ? "检查失败" : "Check failed"}: ${error.message}`; }
    choices.push(option);
  }
  return choices;
}
