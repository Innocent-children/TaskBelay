const chinese = {
  title: "TaskBelay 管理",
  manage: "管理现有安装",
  choose: "请选择：",
  operationPrompt: "选择操作：",
  hostPrompt: "选择 Host：",
  profilePrompt: "DeepSeek Profile [web]：",
  installCodex: "安装 Codex Adapter",
  installDeepSeek: "安装 DeepSeek Adapter",
  installAll: "安装全部 Adapter",
  plan: "执行计划",
  next: "下一步",
  changed: "已变更",
  unchanged: "无变更",
  progress: {
    actionStart: "开始",
    actionComplete: "完成",
    steps: {
      "codex.install_package": "安装 Codex Adapter 全局 package",
      "codex.remove_registration": "移除 Codex 注册",
      "codex.uninstall_package": "卸载 Codex Adapter package",
      "codex.setup_registration": "配置 Codex marketplace、Plugin 和 MCP 注册",
      "codex.verify_ready": "回读并确认 Codex Adapter 已就绪",
      "zcode.install_package": "安装 ZCode Adapter 本地包",
      "zcode.setup_registration": "准备 ZCode 插件来源并校验本地运行时",
      "zcode.remove_registration": "记录待在 ZCode 中移除的插件",
      "zcode.uninstall_package": "卸载 ZCode Adapter package",
      "deepseek.verify_artifact": "下载并校验 DeepSeek Adapter 制品",
      "deepseek.verify_ready": "确认 DeepSeek Core 可用",
      "deepseek.remove": "移除 DeepSeek Profile 中的旧 Adapter",
      "deepseek.add": "把 DeepSeek Adapter 加入目标 Profile",
      "deepseek.write_receipt": "写入并保存 DeepSeek Profile 安装记录",
    },
  },
  continuePrompt: "继续？[y/N] ",
  operations: {
    status: "状态",
    doctor: "诊断",
    install: "安装",
    upgrade: "升级",
    repair: "修复",
    reinstall: "重装",
    uninstall: "卸载",
    "delete-data": "删除数据（保留插件）",
    "delete-all": "删除所有数据（插件+数据）",
    cleanup: "删除已确认的数据", initialize: "准备空数据目录",
  },
  hosts: { taskbelay: "TaskBelay 命令入口", codex: "Codex", deepseek: "DeepSeek", claude: "Claude Code", zcode: "ZCode", all: "全部" },
  statuses: {
    ready: "就绪",
    absent: "未安装",
    partial: "部分完成",
    incompatible: "不兼容",
    conflicted: "冲突",
    unknown: "未知",
    restart_required: "需要重启",
    action_required: "需要在 Host 中操作",
    confirmation_required: "等待确认",
    failed: "失败",
  },
  pet: {
    install: "更新桌面宠物程序",
    menuStart: "开启桌面宠物",
    menuStop: "关闭桌面宠物",
    started: "✓ 桌面宠物已开启",
    restored: "✓ 桌面宠物已在运行，窗口已恢复显示",
    stopped: "✓ 桌面宠物已关闭",
    invalidArguments: "参数无效；受支持的形式是 taskbelay pet start 和 taskbelay pet stop",
    unsupportedPlatform: "桌面宠物支持 Apple silicon macOS 与 Windows 10/11 x64",
    applicationUnavailable: "当前统一入口包内没有可用的桌面宠物应用",
    installAdapterFirst: "未找到可提供 Core 的 Adapter；先运行 taskbelay install",
    coreCommandFailed: "Core 命令执行失败",
    serviceUnavailable: "本地 Control Center 服务不可用；运行 taskbelay webui start 后重试",
    serviceStartFailed: "本地 Control Center 服务启动失败",
    identityMismatch: "本地 Control Center 服务身份与所选 Core 不一致",
    incompatibleRuntime: "当前数据目录与所选 Core 不兼容",
    launchFailed: "桌面宠物启动失败",
    stopFailed: "桌面宠物关闭失败",
  },
};
const english = {
  title: "TaskBelay Manager",
  manage: "Manage existing installation",
  choose: "Choose: ",
  operationPrompt: "Operation: ",
  hostPrompt: "Host: ",
  profilePrompt: "DeepSeek Profile [web]: ",
  installCodex: "Install Codex Adapter",
  installDeepSeek: "Install DeepSeek Adapter",
  installAll: "Install all Adapters",
  plan: "Plan",
  next: "Next step",
  changed: "Changed",
  unchanged: "No changes",
  progress: {
    actionStart: "Starting",
    actionComplete: "Completed",
    steps: {
      "codex.install_package": "Install the global Codex Adapter package",
      "codex.remove_registration": "Remove Codex registration",
      "codex.uninstall_package": "Uninstall the Codex Adapter package",
      "codex.setup_registration": "Configure the Codex marketplace, Plugin, and MCP registration",
      "codex.verify_ready": "Read back and verify that the Codex Adapter is ready",
      "zcode.install_package": "Install the ZCode Adapter package",
      "zcode.setup_registration": "Prepare the ZCode plugin source and verify its local runtime",
      "zcode.remove_registration": "Record the plugin that needs removal in ZCode",
      "zcode.uninstall_package": "Uninstall the ZCode Adapter package",
      "deepseek.verify_artifact": "Download and verify the DeepSeek Adapter artifact",
      "deepseek.verify_ready": "Verify the DeepSeek Core runtime",
      "deepseek.remove": "Remove the previous Adapter from the DeepSeek Profile",
      "deepseek.add": "Add the DeepSeek Adapter to the target Profile",
      "deepseek.write_receipt": "Write and save the DeepSeek Profile installation receipt",
    },
  },
  continuePrompt: "Continue? [y/N] ",
  operations: Object.fromEntries([
    "status", "doctor", "install", "upgrade", "repair", "reinstall", "uninstall", "delete-data", "delete-all", "cleanup", "initialize",
  ].map((operation) => [operation, operation])),
  hosts: { taskbelay: "TaskBelay launcher", codex: "codex", deepseek: "deepseek", claude: "Claude Code", zcode: "ZCode", all: "all" },
  statuses: Object.fromEntries([
    "ready", "absent", "partial", "incompatible", "conflicted", "unknown", "restart_required",
    "confirmation_required", "action_required", "failed",
  ].map((status) => [status, status])),
  pet: {
    install: "Update desktop pet application",
    menuStart: "Start the desktop pet",
    menuStop: "Stop the desktop pet",
    started: "✓ Desktop pet started",
    restored: "✓ Desktop pet is already running; its window was restored",
    stopped: "✓ Desktop pet stopped",
    invalidArguments: "invalid arguments; the supported forms are taskbelay pet start and taskbelay pet stop",
    unsupportedPlatform: "the desktop pet supports Apple silicon macOS and Windows 10/11 x64",
    applicationUnavailable: "this package does not provide a usable desktop pet application",
    installAdapterFirst: "no installed Codex or DeepSeek Adapter provides a Core runtime; run taskbelay install first",
    coreCommandFailed: "the Core command failed",
    serviceUnavailable: "the local Control Center service is unavailable; run taskbelay webui start and try again",
    serviceStartFailed: "the local Control Center service did not start",
    identityMismatch: "the local Control Center service identity differs from the selected Core",
    incompatibleRuntime: "the data directory is incompatible with the selected Core",
    launchFailed: "the desktop pet did not start",
    stopFailed: "the desktop pet did not stop",
  },
};

export function resolveLanguage(environment = process.env) {
  const locale = environment.LC_ALL || environment.LC_MESSAGES || environment.LANG || "";
  return /^zh(?:[_-]|$)/iu.test(locale) ? "zh-CN" : "en";
}

export function messagesForLanguage(language) {
  return language === "zh-CN" ? chinese : english;
}

export function renderResult(result, { mode = "plain", language = resolveLanguage() } = {}) {
  if (mode === "json") return `${JSON.stringify(result)}\n`;
  const messages = messagesForLanguage(language);
  const zh = language === "zh-CN";
  const good = ["ready", "absent", "restart_required"].includes(result.status);
  const heading = `${good ? "✓" : "!"} ${messages.operations[result.operation] ?? result.operation}: ${messages.statuses[result.status] ?? result.status}`;
  const lines = ["", mode === "rich" ? `\u001b[1;${good ? "36" : "33"}m${heading}\u001b[0m` : heading,
    "─".repeat(36), result.changed === null ? (zh ? "变更结果待检查" : "Inspect changes before retrying") : result.changed ? messages.changed : messages.unchanged];
  for (const target of result.targets ?? []) {
    lines.push(`  ${messages.hosts[target.host] ?? target.host}${target.profile ? `/${target.profile}` : ""} · ${messages.statuses[target.state] ?? target.state}`);
    if (target.package_version) lines.push(`    Adapter ${target.package_version}${target.core_version ? ` · Core ${target.core_version}` : ""}`);
    if (target.host_available === false) lines.push(`    ! ${zh ? "未找到可用的 Host，请先安装 Host 并确认 PATH。" : "Host unavailable; install the Host and check PATH."}`);
    for (const issue of target.issues ?? []) {
      lines.push(`    ! ${issue.message}`);
      if (issue.detail) lines.push(`      ${issue.detail}`);
      if (issue.command) lines.push(`      ${issue.command}`);
    }
  }
  if (result.launcher) lines.push(`  TaskBelay ${result.launcher.packageVersion} · ${result.launcher.channel} · ${result.launcher.packageRoot}`);
  if (result.installation) for (const [name, path] of Object.entries(result.installation)) lines.push(`  ${name}: ${path}`);
  for (const check of result.checks ?? []) lines.push(`  ${check.status === "passed" ? "✓" : check.status === "not_installed" ? "–" : "!"} ${check.name}: ${check.message}`);
  if (result.error) {
    lines.push(`${zh ? "原因" : "Reason"}: ${result.error.message}`);
    if (result.error.detail) lines.push(result.error.detail);
  }
  if (result.operation_id) lines.push(`${zh ? "操作记录" : "Operation"}: ${result.operation_id}`);
  if (result.failed_action) lines.push(`${zh ? "失败步骤" : "Failed step"}: ${result.failed_action}`);
  if (result.error && result.completed_actions?.length) lines.push(`${zh ? "已完成" : "Completed"}: ${result.completed_actions.join(", ")}`);
  if (result.data?.trash_root) lines.push(`${zh ? "数据保存位置" : "Recovered data"}: ${result.data.trash_root}`);
  const steps = [...new Set([...(result.next_steps ?? []), ...(result.restart_requirements ?? [])])];
  for (const step of steps) {
    const restart = /^Restart DeepSeek Profile (.+)$/u.exec(step);
    const text = zh && restart ? `停止并重新启动 DeepSeek Profile ${restart[1]}，再使用 /taskbelay。`
      : zh && step.startsWith("Open Codex /hooks") ? "在 Codex /hooks 中审核并信任 TaskBelay hook，再开启新对话。" : step;
    lines.push(`${messages.next}: ${text}`);
  }
  if (result.changed && good && ["install", "reinstall", "repair", "upgrade"].includes(result.operation)) {
    if (result.targets?.some(t => t.host === "codex" && t.state === "ready")) lines.push("  $taskbelay-codex:taskbelay <task description>");
    if (result.targets?.some(t => t.host === "deepseek" && ["ready", "restart_required"].includes(t.state))) lines.push("  /taskbelay <task description>");
    lines.push("  taskbelay webui start");
  }
  if (["delete-data", "delete-all"].includes(result.operation)) lines.push(zh
    ? result.operation === "delete-data" ? "插件与注册保留；数据已清理时，请重启相关 Agent 会话。" : "命令入口与桌面程序保留；按结果确认插件及数据是否已移除。"
    : result.operation === "delete-data" ? "Plugins and registrations are retained; restart Agent sessions after data cleanup." : "Launcher and desktop application are retained; inspect the result for plugin and data removal.");
  if (result.next_step) lines.push(`${messages.next}: ${result.next_step}`);
  return `${lines.join("\n")}\n`;
}

export function selectInteractivePresentationMode(output, environment = process.env) {
  if (!output?.isTTY || Object.hasOwn(environment ?? {}, "NO_COLOR")) return "plain";
  if ((environment?.TERM ?? "").toLowerCase() === "dumb") return "plain";
  if (!Number.isInteger(output.columns) || output.columns < 80) return "plain";
  return "rich";
}

export function renderPlan(plan, { mode = "plain", language = resolveLanguage() } = {}) {
  if (mode === "json") return "";
  const messages = messagesForLanguage(language);
  const lines = [`${messages.plan} ${messages.operations[plan.operation] ?? plan.operation} (${plan.planId})`];
  for (const action of plan.actions ?? []) {
    const target = plan.targets?.find(target => target.host === action.host && target.profile === action.profile);
    lines.push(`  ${actionLabel(action, messages)}${action.targetVersion ? ` · ${target?.packageVersion ?? "—"} → ${action.targetVersion}` : ""}`);
  }
  for (const resource of plan.resources ?? []) lines.push(`  ${resource.label}: ${resource.path}`);
  if (plan.actions?.length === 0) lines.push(language === "zh-CN" ? "  当前状态已满足要求，无需变更。" : "  Already satisfied; no changes needed.");
  for (const impact of plan.impacts) lines.push(`- ${translateImpact(impact, language, messages)}`);
  return `${lines.join("\n")}\n`;
}

export function renderProgress(event, { language = resolveLanguage() } = {}) {
  const messages = messagesForLanguage(language);
  if (event.type === "action_start" || event.type === "action_complete") {
    const state = event.type === "action_start" ? messages.progress.actionStart : messages.progress.actionComplete;
    return `${event.type === "action_start" ? "→" : "✓"} ${state}: ${actionLabel(event.action, messages)}\n`;
  }
  if (event.type === "step_complete") {
    return `  ✓ ${stepLabel(event.stepId, messages)}\n`;
  }
  if (event.type === "phase") return `… ${event.message}\n`;
  if (event.type === "step_start") return `  … ${stepLabel(event.stepId, messages)}\n`;
  throw new Error(`unsupported progress event ${event.type}`);
}

function actionLabel(action, messages) {
  if (action.operation === "install_pet") return messages.pet.install;
  if (["cleanup", "initialize"].includes(action.operation)) return messages.operations[action.operation];
  const host = messages.hosts[action.host] ?? action.host;
  const profile = action.profile ? ` Profile ${action.profile}` : "";
  return `${messages.operations[action.operation] ?? action.operation} ${host}${profile}`;
}

function stepLabel(stepId, messages) {
  const normalized = stepId.replace(/^deepseek\.[^.]+\./u, "deepseek.");
  return messages.progress.steps[normalized] ?? stepId;
}

function translateImpact(impact, language, messages) {
  if (language !== "zh-CN") return impact;
  const fixed = {
    "Update desktop pet application; preserve settings and appearances": "更新桌面宠物程序，保留设置与形象",
    "Read Host and Adapter state only": "仅读取 Agent 和插件状态",
    "Remove every installed Adapter before shared data cleanup": "在清理共享数据前移除所有已安装的插件",
    "Clear TaskBelay user configuration": "清理 TaskBelay 用户配置",
    "Clear current default Task data": "清理当前默认 Task 数据",
    "Clear desktop pet records, preferences, and imported appearances": "清理桌面宠物记录、偏好与导入形象",
    "Clear the explicitly confirmed Task data directory": "清理已明确确认的 Task 数据目录",
    "Permanently remove confirmed data": "永久删除已确认的数据",
    "Move confirmed data to the TaskBelay recovery directory": "将已确认的数据移入 TaskBelay 恢复目录",
    "Move confirmed data to macOS Trash": "将已确认的数据移入 macOS 废纸篓",
    "Preserve plugins, Agent registrations and installation records": "保留插件、Agent 注册和必要的安装记录",
    "Clear completed maintenance records; retain the current operation result": "清理已完成的维护记录，保留本次操作结果和未解决的记录",
    "Preserve the launcher, desktop application, project code and Agent conversation history": "保留命令入口、桌面程序、项目代码和 Agent 会话历史",
    "Create empty data directories at the original paths and restore default preferences": "在原位置准备空数据目录，恢复默认偏好",
    "No installed Adapter or active TaskBelay data was found": "未发现已安装的 Adapter 或有效 TaskBelay 数据",
    "Preserve TaskBelay user configuration and Task data": "保留 TaskBelay 用户配置和 Task 数据",
  };
  if (fixed[impact]) return fixed[impact];
  const target = /^(status|doctor|install|upgrade|repair|reinstall|uninstall|delete-data|delete-all) (codex|deepseek|claude|zcode)(?: Profile (.+)| Adapter)$/u.exec(impact);
  if (!target) return impact;
  const [, operation, host, profile] = target;
  return `${messages.operations[operation]} ${messages.hosts[host]}${profile ? ` Profile ${profile}` : " Adapter"}`;
}
