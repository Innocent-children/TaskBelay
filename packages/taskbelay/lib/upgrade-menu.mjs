import { BACK, CANCEL } from "./terminal.mjs";

/**
 * Latest updates may be selected together. A version belongs to one component,
 * since the launcher and Agent plugins have independent release numbers.
 */
export async function promptForUpgrade({ terminal, output, state, inspectUpgrade, language }) {
  const text = (zh, en) => language === "zh-CN" ? zh : en;
  const entry = (value, zh, en) => ({ value, label: text(zh, en) });
  let choices, refresh = false;
  const inspect = async options => {
    output.write(text("正在检查更新…\n", "Checking for updates…\n"));
    try { return await inspectUpgrade(options); }
    catch (error) {
      output.write(`${text("检查升级失败", "Update check failed")}: ${error.message}\n`);
      return [{ value: "failed", label: text("更新检查", "Update check"), status: "failed", disabled: true }];
    }
  };
  while (!terminal.cancelled) {
    choices ??= await inspect({ targetVersion: "latest", selfPackage: state.selfPackage ?? null, refresh });
    refresh = false;
    const local = choices.some(choice => choice.value === "self" && choice.channel === "local");
    const selectable = choices.filter(choice => !choice.disabled);
    const shortcuts = { r: text("重新检查", "Check again"), v: text("指定组件版本", "Choose a component version"),
      ...(local ? { l: text("选择本地分发包", "Choose local distribution") } : {}) };
    output.write("\n" + choices.map(choice => `  ${choice.label}: ${choice.hint ?? ""}`).join("\n") + "\n");
    let selected;
    if (selectable.length) {
      selected = await terminal.multiselect(text("升级：选择更新项", "Upgrade: select updates"), selectable, {
        initial: state.selections.upgrade ?? selectable.map(choice => choice.value),
        onChange: values => { state.selections.upgrade = values; }, shortcuts,
      });
    } else {
      const failed = choices.some(choice => choice.status === "failed");
      selected = await terminal.select(failed ? text("部分检查失败，尚不能确认是否有更新", "Some checks failed; update availability is unknown")
        : local ? text("本地安装：选择完整分发包更新", "Local installation: choose a complete distribution to update")
          : text("所有组件已是最新版本", "All components are up to date"), [
        entry("home", "返回首页", "Home"), entry("r", "重新检查", "Check again"), entry("v", "指定组件版本", "Choose a component version"),
        ...(local ? [entry("l", "选择本地分发包", "Choose local distribution")] : []),
      ], { shortcuts });
    }
    if (selected === CANCEL || selected === BACK || selected === "home") return selected === CANCEL ? CANCEL : BACK;
    if (Array.isArray(selected)) return { choices: choices.filter(choice => selected.includes(choice.value)), selfPackage: state.selfPackage };
    const action = selected.action ?? selected;
    if (action === "r") { choices = null; refresh = true; delete state.selections.upgrade; continue; }
    if (action === "l") {
      const value = await terminal.question(text("本地完整分发包的绝对路径（.tgz；留空清除）", "Absolute local distribution path (.tgz; empty clears)"), { initial: state.selfPackage ?? "" });
      if (value === CANCEL) return CANCEL;
      if (value !== BACK) { state.selfPackage = value || null; choices = null; delete state.selections.upgrade; }
      continue;
    }
    if (action !== "v") continue;
    const components = choices.filter(choice => choice.value !== "failed" && choice.channel !== "local");
    if (!components.length) { output.write(text("本地组件版本由完整分发包决定。\n", "Local component versions come from the complete distribution.\n")); continue; }
    const component = await terminal.select(text("选择要指定版本的组件", "Choose the component to version"), components.map(choice => ({ value: choice.value, label: choice.label })));
    if (component === CANCEL) return CANCEL;
    if (component === BACK) continue;
    state.versions ??= {};
    while (!terminal.cancelled) {
      const version = await terminal.question(text("目标版本（latest 或 x.y.z）", "Target version (latest or x.y.z)"), {
        initial: state.versions[component] ?? "latest", onChange: value => { state.versions[component] = value; },
        validate: value => /^(latest|(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*))$/u.test(value) ? null : text("请输入 latest 或 x.y.z。", "Enter latest or x.y.z."),
      });
      if (version === CANCEL) return CANCEL;
      if (version === BACK) break;
      state.versions[component] = version;
      const resolved = await inspect({ targetVersion: version, component });
      const target = resolved.find(choice => choice.value === component);
      if (target && !target.disabled) return { choices: [target], targetVersion: version };
      output.write(`${target?.hint ?? text("该组件无法升级。", "This component cannot be updated.")}\n`);
    }
  }
  return CANCEL;
}
