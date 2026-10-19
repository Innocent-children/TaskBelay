import { BACK, CANCEL, createTerminalSession } from "./terminal.mjs";
import { resolveLanguage } from "./presentation.mjs";
export { promptForRequest } from "./menu.mjs";

export const AGENTS = Object.freeze(["codex", "deepseek", "claude", "zcode"]);
export const OPERATIONS = Object.freeze(["status", "doctor", "install", "upgrade", "repair", "reinstall", "uninstall", "delete-data", "delete-all"]);
export const isDataDeletion = operation => ["delete-data", "delete-all"].includes(operation);
const versioned = ["install", "upgrade", "repair", "reinstall"];
const stableVersion = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u;
const booleans = new Map([
  ["--all-known-profiles", "allKnownProfiles"], ["--adopt", "adopt"], ["--self", "self"],
  ["--confirm-zcode-closed", "zcodeClosed"], ["--permanent", "permanent"],
  ["--yes", "yes"], ["--plain", "plain"], ["--json", "json"],
]);
const values = new Map([
  ["--host", "hosts"], ["--profile", "profiles"], ["--version", "targetVersion"],
  ["--self-package", "selfPackage"], ["--confirm-delete", "confirmationToken"],
  ["--confirm-permanent", "permanentToken"], ["--confirm-downgrade", "downgradeToken"],
  ["--confirm-explicit-data", "confirmedExplicitData"],
]);
const repeatable = new Set(["hosts", "profiles", "confirmedExplicitData"]);

export function parseArguments(arguments_, { isTTY = Boolean(process.stdin.isTTY && process.stdout.isTTY), noColor = process.env.NO_COLOR !== undefined } = {}) {
  if (!Array.isArray(arguments_) || arguments_.some(value => typeof value !== "string" || value.includes("\0"))) throw new CLIError("arguments must be closed strings");
  if (!arguments_.length) {
    if (!isTTY) throw new CLIError("an operation is required outside a TTY");
    return Object.freeze({ interactive: true, outputMode: noColor ? "plain" : "rich" });
  }
  const [operation, ...rest] = arguments_;
  if (!OPERATIONS.includes(operation)) throw new CLIError(`unknown operation ${operation}`);
  if (rest.length === 1 && ["--help", "-h"].includes(rest[0])) return Object.freeze({ help: operation });
  const parsed = { operation, hosts: [], profiles: [], targetVersion: null, selfPackage: null,
    allKnownProfiles: false, adopt: false, self: false, zcodeClosed: false, permanent: false,
    yes: false, plain: false, json: false, confirmationToken: null, permanentToken: null,
    downgradeToken: null, confirmedExplicitData: [] };
  const seen = new Set();
  for (let index = 0; index < rest.length; index++) {
    const option = rest[index], field = booleans.get(option) ?? values.get(option);
    if (!field) throw new CLIError(`unknown option ${option}`);
    if (seen.has(field) && !repeatable.has(field)) throw new CLIError(`duplicate option ${option}`);
    seen.add(field);
    if (booleans.has(option)) { parsed[field] = true; continue; }
    const value = rest[++index];
    if (value === undefined || value.startsWith("--")) throw new CLIError(`${option} requires a value`);
    if (repeatable.has(field)) parsed[field].push(value); else parsed[field] = value;
  }
  if (parsed.hosts.some(host => ![...AGENTS, "all"].includes(host))) throw new CLIError("invalid Agent in --host");
  if (parsed.hosts.includes("all") && parsed.hosts.length !== 1) throw new CLIError("--host all cannot be combined with other Agents");
  if (isDataDeletion(operation)) {
    if (parsed.hosts.length && parsed.hosts[0] !== "all") throw new CLIError("shared data deletion requires all Agents; omit --host or use --host all");
    if (seen.has("profiles")) throw new CLIError("shared data deletion covers every known Profile");
    parsed.hosts = [...AGENTS]; parsed.allKnownProfiles = true;
  } else if (!parsed.hosts.length) {
    if (!["status", "doctor"].includes(operation) && !parsed.self && !isTTY) throw new CLIError("--host or --self is required for non-interactive mutation");
    if (!parsed.self) parsed.hosts = [...AGENTS];
  } else if (parsed.hosts[0] === "all") parsed.hosts = [...AGENTS];
  parsed.hosts = AGENTS.filter(host => parsed.hosts.includes(host));
  parsed.profiles = [...new Set(parsed.profiles.map(validateProfile))];
  parsed.confirmedExplicitData = [...new Set(parsed.confirmedExplicitData)];
  if (!parsed.hosts.includes("deepseek") && (parsed.profiles.length || parsed.allKnownProfiles || parsed.adopt)) throw new CLIError("DeepSeek Profile options require --host deepseek or all");
  if (parsed.hosts.includes("deepseek") && !parsed.profiles.length && !parsed.allKnownProfiles) parsed.profiles = ["web"];
  if (parsed.targetVersion !== null && parsed.targetVersion !== "latest" && !stableVersion.test(parsed.targetVersion)) throw new CLIError("--version must equal latest or a stable semantic version");
  if (seen.has("targetVersion") && !versioned.includes(operation)) throw new CLIError("--version is not supported for this operation");
  if (!isDataDeletion(operation) && (parsed.permanent || parsed.confirmationToken || parsed.permanentToken || parsed.confirmedExplicitData.length || parsed.zcodeClosed)) throw new CLIError("data deletion options require delete-data or delete-all");
  if (parsed.permanentToken && !parsed.permanent) throw new CLIError("--confirm-permanent requires --permanent");
  if (parsed.self && operation !== "upgrade") throw new CLIError("--self requires upgrade");
  if (parsed.selfPackage && !parsed.self) throw new CLIError("--self-package requires --self");
  if (parsed.adopt && !["install", "repair"].includes(operation)) throw new CLIError("--adopt requires install or repair");
  if (parsed.downgradeToken && !versioned.includes(operation)) throw new CLIError("--confirm-downgrade requires a versioned operation");
  if (parsed.json && parsed.plain) throw new CLIError("--json and --plain conflict");
  return Object.freeze({ ...parsed, interactive: false, outputMode: parsed.json ? "json" : parsed.plain || noColor || !isTTY ? "plain" : "rich" });
}

export function requestArguments(request) {
  const args = [request.operation];
  if (request.hosts.length === AGENTS.length) args.push("--host", "all");
  else for (const host of request.hosts) args.push("--host", host);
  for (const profile of request.profiles) args.push("--profile", profile);
  if (request.allKnownProfiles) args.push("--all-known-profiles");
  if (request.targetVersion) args.push("--version", request.targetVersion);
  if (request.selfPackage) args.push("--self-package", request.selfPackage);
  for (const [flag, field] of booleans) if (!["allKnownProfiles", "yes", "plain", "json"].includes(field) && request[field]) args.push(flag);
  return args;
}

export function operationDescription(operation, language = "en") {
  const zh = language === "zh-CN";
  return ({ status: zh ? "查看版本、路径和安装状态" : "Show versions, paths and installation state",
    doctor: zh ? "检查问题并给出对应处理步骤" : "Inspect failures and show relevant actions",
    install: zh ? "安装到 Agent，保留已有版本" : "Install into Agents, keeping existing versions",
    upgrade: zh ? "升级已安装插件；--self 更新命令入口" : "Upgrade installed plugins; --self updates the launcher",
    repair: zh ? "修复当前安装，保留版本和数据" : "Repair the current installation, retaining version and data",
    reinstall: zh ? "重新安装当前版本，保留数据" : "Reinstall the current version, retaining data",
    uninstall: zh ? "卸载插件（保留数据）" : "Uninstall plugins (keep data)",
    "delete-data": zh ? "删除数据（保留插件）" : "Delete data (keep plugins)",
    "delete-all": zh ? "删除所有数据（插件+数据）" : "Delete everything (plugins + data)",
  })[operation];
}

export function renderHelp(operation = null, language = "en") {
  const zh = language === "zh-CN";
  return ["TaskBelay", "", ...(operation ? [operation] : OPERATIONS).map(value => `  taskbelay ${value} — ${operationDescription(value, language)}`),
    "", "  taskbelay webui start|open|status|stop [--plain|--json]", "  taskbelay pet start|stop", "  taskbelay version", "",
    "  --host codex|deepseek|claude|zcode|all  (repeatable)", "  --profile <name>  --all-known-profiles",
    "  --version latest|<x.y.z>  --confirm-downgrade <token>", "  upgrade: --self [--self-package <absolute-local-distribution.tgz>]",
    "  --yes  --plain  --json  --help  --adopt (install/repair, DeepSeek)",
    "  delete-data / delete-all: --confirm-delete <token>", "    --confirm-explicit-data <absolute-path>  --confirm-zcode-closed",
    "    --permanent --confirm-permanent <token>", "",
    zh ? "方向键选择，回车确认，空格多选，A 全选，Esc 返回，Ctrl+C 退出。" : "Arrows select, Enter confirms, Space toggles, A selects all, Esc goes back, Ctrl+C exits.",
    zh ? "升级只处理已安装项，默认最新稳定版；安装/修复/重装保留已有版本。" : "Upgrade selects installed items only and defaults to latest; install/repair/reinstall keep current versions.",
    zh ? "数据删除作用于全部 Agent 共用的确认目录；保留命令入口、桌面程序及项目代码。" : "Data deletion covers confirmed shared directories; it keeps the launcher, desktop application and project code.",
    zh ? "--yes 不确认数据删除、永久删除或降级；JSON 模式不询问，返回准确确认命令。" : "--yes does not approve data deletion, permanent deletion or downgrade; JSON never prompts and returns the exact confirmation command.",
    zh ? "本地开发安装保持原渠道；更新命令入口须提供新的本地完整分发包。" : "Local development installations retain their channel; updating the launcher requires a new complete local distribution.", "",
    "  taskbelay install --host codex --host claude --yes", "  taskbelay upgrade --host all --all-known-profiles --self --yes",
    "  taskbelay delete-data --json", "  taskbelay delete-all --json", "",
  ].join("\n");
}

export async function confirmPlan(plan, request, { input = process.stdin, output = process.stdout, language = resolveLanguage(), session = null } = {}) {
  if (plan.confirmationClass === "none") return true;
  if (plan.confirmationClass === "mutation" && request.yes) return true;
  if (plan.confirmationClass === "downgrade" && request.downgradeToken === plan.downgradeToken) return true;
  const deletion = ["delete", "permanent_delete"].includes(plan.confirmationClass);
  if (deletion && request.confirmationToken === plan.confirmationToken && (!request.permanent || request.permanentToken === plan.permanentToken)) return true;
  if (!input.isTTY || request.outputMode === "json") return false;
  const terminal = session ?? createTerminalSession(input, output, { language });
  const zh = language === "zh-CN";
  try {
    if (deletion || plan.confirmationClass === "downgrade") {
      const tokens = deletion ? [plan.confirmationToken, ...(request.permanent ? [plan.permanentToken] : [])] : [plan.downgradeToken];
      for (const token of tokens) {
        const answer = await terminal.question(zh ? `输入 ${token} 确认${deletion ? operationDescription(request.operation, language) : "降级"}：` : `Type ${token} to confirm ${deletion ? request.operation : "downgrade"}:`, {
          validate: value => value === token ? null : zh ? "确认文字不匹配；重新输入，或按 Esc 返回。" : "Confirmation does not match; try again or press Esc to return.",
        });
        if (answer === BACK) return BACK;
        if (answer !== token) return CANCEL;
      }
      return true;
    }
    const answer = await terminal.select(zh ? "确认本次操作" : "Confirm this operation", [
      { value: true, label: zh ? "开始执行" : "Start" }, { value: BACK, label: zh ? "返回修改选择" : "Back to selection" },
      { value: CANCEL, label: zh ? "取消" : "Cancel" },
    ]);
    return answer;
  } finally { if (!session) terminal.close(); }
}

export class CLIError extends Error {
  constructor(message) { super(message); this.name = "CLIError"; this.exitCode = 2; }
}

export function validateProfile(value) {
  if (!value || value === "." || value === ".." || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/u.test(value)) throw new CLIError(`invalid DeepSeek Profile ${JSON.stringify(value)}`);
  return value;
}
