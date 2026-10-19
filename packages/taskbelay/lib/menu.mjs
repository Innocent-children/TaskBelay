import { AGENTS, parseArguments, validateProfile, isDataDeletion } from "./cli.mjs";
import { BACK, CANCEL, createTerminalSession } from "./terminal.mjs";
import { messagesForLanguage, resolveLanguage, selectInteractivePresentationMode } from "./presentation.mjs";
import { supportsDesktopPet } from "./platform.mjs";

/**
 * The menu keeps only unfinished user choices. Lifecycle observation and plans
 * remain authoritative when a request is returned for execution.
 */
export async function promptForRequest({ input = process.stdin, output = process.stdout,
  language = resolveLanguage(), environment = process.env, platform = process.platform, arch = process.arch,
  observation = null, session = null, state = {}, inspectUpgrade = async () => [], petState = "unknown",
} = {}) {
  const terminal = session ?? createTerminalSession(input, output, { language, environment });
  const zh = language === "zh-CN", messages = messagesForLanguage(language);
  const label = (cn, en) => zh ? cn : en;
  const entry = (value, cn, en, hint) => ({ value, label: label(cn, en), hint });
  state.selections ??= {};
  state.profiles ??= ["web"];
  const targets = observation?.targets ?? [];
  const installed = targets.filter(target => target.package_version || target.package_installed || target.state !== "absent");
  const makeRequest = (operation, args = []) => ({ ...parseArguments([operation, ...args], { isTTY: true }),
    fromMenu: true, outputMode: selectInteractivePresentationMode(output, environment) });
  try {
    if (observation?.error) output.write(`${label("安装检查失败", "Installation check failed")}: ${observation.error.message}\n`);
    while (!terminal.cancelled) {
      let operation = state.nextOperation;
      delete state.nextOperation;
      if (!operation) {
        output.write(`\n┌ TaskBelay\n${installed.map(target => `  ${target.host}${target.profile ? `/${target.profile}` : ""} · ${messages.statuses[target.state] ?? target.state}`).join("\n")}\n`);
        operation = await terminal.select(label("你想做什么？", "What would you like to do?"), [
          entry("upgrade", "升级", "Upgrade", label("命令入口与已安装插件", "Launcher and installed plugins")),
          entry("install", "安装到 Agent", "Install into Agents"), entry("doctor", "检查与修复", "Check and repair"),
          entry("webui", "打开控制中心", "Open Control Center"),
          ...(supportsDesktopPet(platform, arch) ? [entry("pet", "桌面宠物", "Desktop pet")] : []),
          entry("status", "安装信息", "Installation information"), entry("cleanup", "卸载与数据清理", "Uninstall and delete data"),
          entry("exit", "退出", "Exit"),
        ], { back: false, initialValue: installed.length ? "upgrade" : "install" });
      }
      if (operation === CANCEL || operation === "exit") return { cancelled: true };
      if (operation === "webui") return { webui: "start" };
      if (operation === "pet") {
        output.write(label(`桌面宠物：${petState === "unknown" ? "运行状态未知" : petState === "stopped" ? "未运行" : "运行中"}\n`, `Desktop pet: ${petState}\n`));
        const choice = await terminal.select(label("桌面宠物", "Desktop pet"), [
          entry("start", "开启或显示", "Start or show"), ...(petState === "stopped" ? [] : [entry("stop", "关闭", "Stop")]),
        ]);
        if (choice === BACK) continue;
        if (choice === CANCEL) return { cancelled: true };
        return { pet: choice };
      }
      if (["status", "doctor"].includes(operation)) return makeRequest(operation, ["--host", "all", "--all-known-profiles"]);
      if (operation === "cleanup") {
        operation = await terminal.select(label("卸载与数据清理", "Uninstall and delete data"), [
          entry("uninstall", "卸载插件（保留数据）", "Uninstall plugins (keep data)"),
          entry("delete-data", "删除数据（保留插件）", "Delete data (keep plugins)"),
          entry("delete-all", "删除所有数据（插件+数据）", "Delete everything (plugins + data)"),
        ]);
        if (operation === BACK) continue;
        if (operation === CANCEL) return { cancelled: true };
      }
      if (isDataDeletion(operation)) {
        const mode = await terminal.select(label("删除方式", "Deletion method"), [
          entry("recoverable", "可恢复删除", "Recoverable deletion", label("移到废纸篓或恢复目录", "Move to Trash or the recovery directory")),
          entry("permanent", "永久删除", "Permanent deletion"),
        ], { initialValue: state.deletionMode ?? "recoverable" });
        if (mode === BACK) { state.nextOperation = "cleanup"; continue; }
        if (mode === CANCEL) return { cancelled: true };
        state.deletionMode = mode;
        state.nextOperation = operation;
        return makeRequest(operation, mode === "permanent" ? ["--permanent"] : []);
      }
      if (operation === "upgrade") {
        let choices;
        try { choices = await inspectUpgrade({ targetVersion: state.targetVersion ?? "latest", selfPackage: state.selfPackage ?? null }); }
        catch (error) { output.write(`${label("检查升级失败", "Update check failed")}: ${error.message}\n`); choices = []; }
        const selection = await terminal.multiselect(label("升级：选择更新项", "Upgrade: select updates"), choices.length ? choices : [
          { value: "unavailable", label: label("没有可用的更新项", "No updates are available"), disabled: true },
        ], { initial: state.selections.upgrade ?? choices.filter(choice => !choice.disabled).map(choice => choice.value),
          onChange: values => { state.selections.upgrade = values; }, shortcuts: { v: label("指定版本 / latest", "Version / latest"), l: label("本地命令入口分发包", "Local launcher distribution") } });
        if (selection === BACK) continue;
        if (selection === CANCEL) return { cancelled: true };
        if (selection.action) {
          state.selections.upgrade = selection.selected;
          const version = selection.action === "v";
          const value = await terminal.question(version ? label("目标版本（latest 或 x.y.z）", "Target version (latest or x.y.z)") : label("本地完整分发包的绝对路径（.tgz）", "Absolute local distribution path (.tgz)"), { initial: version ? state.targetVersion ?? "latest" : state.selfPackage ?? "" });
          if (value === CANCEL) return { cancelled: true };
          if (value !== BACK) {
            if (version && !/^(latest|\d+\.\d+\.\d+)$/u.test(value)) output.write(label("版本格式无效。\n", "Invalid version.\n"));
            else { state[version ? "targetVersion" : "selfPackage"] = value; delete state.selections.upgrade; }
          }
          state.nextOperation = "upgrade"; continue;
        }
        state.selections.upgrade = selection;
        const chosen = choices.filter(choice => selection.includes(choice.value));
        const args = [];
        for (const host of [...new Set(chosen.map(choice => choice.host).filter(Boolean))]) args.push("--host", host);
        for (const item of chosen.filter(choice => choice.profile)) args.push("--profile", item.profile);
        if (selection.includes("self")) {
          args.push("--self"); if (state.selfPackage) args.push("--self-package", state.selfPackage);
        }
        if (state.targetVersion) args.push("--version", state.targetVersion);
        state.nextOperation = "upgrade";
        return makeRequest("upgrade", args);
      }
      const available = operation === "install" ? AGENTS.map(host => {
        const target = targets.find(target => target.host === host);
        return { value: host, label: messages.hosts[host], disabled: target?.host_available === false,
          hint: target?.host_available === false ? label("未检测到 Agent", "Agent unavailable") : target?.package_version ? label("已安装，保留当前版本", "Installed, keep version") : label("尚未安装", "Not installed") };
      }) : installed.map(target => ({ value: target.profile ? `deepseek:${target.profile}` : target.host,
        label: `${messages.hosts[target.host]}${target.profile ? ` / ${target.profile}` : ""}`, hint: target.package_version ?? messages.statuses[target.state] }));
      const selection = await terminal.multiselect(label(operation === "install" ? "安装到 Agent" : operation === "uninstall" ? "卸载插件（保留数据）" : operation === "repair" ? "修复当前安装" : "重新安装当前版本", operation), available.length ? available : [{ value: "empty", label: label("没有已安装插件", "No installed plugins"), disabled: true }], { initial: state.selections[operation] ?? [], onChange: values => { state.selections[operation] = values; } });
      if (selection === BACK) { if (operation === "uninstall") state.nextOperation = "cleanup"; continue; }
      if (selection === CANCEL) return { cancelled: true };
      state.selections[operation] = selection;
      const args = [];
      for (const host of [...new Set(selection.map(value => value.split(":")[0]))]) args.push("--host", host);
      if (operation === "install" && selection.includes("deepseek")) {
        let profiles;
        while (true) {
          const known = [...new Set(["web", ...targets.filter(target => target.host === "deepseek").map(target => target.profile), ...state.profiles])].filter(Boolean);
          profiles = await terminal.multiselect(label("选择 DeepSeek Profile", "Select DeepSeek Profiles"), known.map(value => ({ value, label: value })), { initial: state.profiles, onChange: values => { state.profiles = values; }, shortcuts: { n: label("输入其他 Profile", "Add a Profile") } });
          if (!profiles?.action) break;
          state.profiles = profiles.selected;
          const custom = await terminal.question(label("Profile 名称", "Profile name"), { validate: value => { try { validateProfile(value); return null; } catch { return label("使用字母、数字、点、下划线或连字符。", "Use letters, digits, dots, underscores or hyphens."); } } });
          if (custom === CANCEL) return { cancelled: true };
          if (custom !== BACK) state.profiles = [...new Set([...state.profiles, custom])];
        }
        if (profiles === CANCEL) return { cancelled: true };
        if (profiles === BACK) { state.nextOperation = operation; continue; }
        state.profiles = profiles;
        for (const profile of profiles) args.push("--profile", profile);
      } else for (const value of selection.filter(value => value.startsWith("deepseek:"))) args.push("--profile", value.slice(9));
      if (["install", "repair"].includes(operation) && selection.some(value => value === "deepseek" || value.startsWith("deepseek:")) && targets.some(target => target.issues?.some(issue => issue.code === "unmanaged_profile"))) {
        const adopt = await terminal.select(label("接管未受管的 DeepSeek 插件？", "Adopt the unmanaged DeepSeek plugin?"), [entry(false, "返回", "Back"), entry(true, "接管并重新安装", "Adopt and reinstall")]);
        if (adopt === CANCEL) return { cancelled: true };
        if (adopt !== true) { state.nextOperation = operation; continue; }
        args.push("--adopt");
      }
      state.nextOperation = operation;
      return makeRequest(operation, args);
    }
    return { cancelled: true };
  } finally { if (!session) terminal.close(); }
}

export async function promptAfterResult(result, { session, state, language = "en" }) {
  delete state.nextOperation;
  const zh = language === "zh-CN";
  const choices = [{ value: "home", label: zh ? "返回首页" : "Home" }];
  if (result.operation === "doctor") {
    const needsRepair = result.targets?.some(target => target.issues?.some(issue => ["registration_incomplete", "adapter_check_failed", "core_check_failed", "unmanaged_profile"].includes(issue.code)));
    if (needsRepair) choices.unshift({ value: "repair", label: zh ? "修复当前安装" : "Repair installation" });
    choices.push({ value: "doctor", label: zh ? "重新检查" : "Check again" }, { value: "reinstall", label: zh ? "更多修复：重新安装当前版本" : "More repairs: reinstall current version" });
  } else if (["failed", "partial", "action_required", "restart_required"].includes(result.status)) choices.push({ value: "doctor", label: zh ? "检查当前状态" : "Check current state" });
  if (result.operation === "status") choices.push({ value: "upgrade", label: zh ? "前往升级" : "Upgrade" });
  if (result.operation === "delete-all" && result.status === "absent") choices.push({ value: "install", label: zh ? "安装到 Agent" : "Install into Agents" });
  choices.push({ value: "exit", label: zh ? "退出" : "Exit" });
  const value = await session.select(zh ? "下一步" : "Next", choices, { back: false });
  if (value === CANCEL || value === "exit") return false;
  if (value !== "home" && value !== BACK) state.nextOperation = value;
  return true;
}
