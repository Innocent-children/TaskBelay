import assert from "node:assert/strict";
import { PassThrough, Writable } from "node:stream";
import test from "node:test";
import { AGENTS, OPERATIONS, CLIError, confirmPlan, parseArguments, promptForRequest } from "../lib/cli.mjs";
import { BACK, CANCEL, createTerminalSession } from "../lib/terminal.mjs";
import { renderPlan, renderProgress, renderResult, resolveLanguage } from "../lib/presentation.mjs";

function output() { let text = ""; const stream = new Writable({ write(chunk, _encoding, done) { text += chunk; done(); } }); Object.defineProperties(stream, { text: { get: () => text }, columns: { value: 42 }, rows: { value: 20 } }); return stream; }
function scripted(answers, inspect = () => {}) {
  const ask = async (message, entries, options) => { inspect(message, entries, options); assert.ok(answers.length, `missing answer for ${message}`); return answers.shift(); };
  return { select: ask, multiselect: ask, question: ask, close() {} };
}

test("current operations normalize one or several Agents and reject retired commands", () => {
  for (const operation of OPERATIONS) {
    const request = parseArguments([operation, "--host", "all", "--json"], { isTTY: false });
    assert.deepEqual(request.hosts, AGENTS); assert.equal(request.outputMode, "json");
  }
  assert.deepEqual(parseArguments(["upgrade", "--host", "codex", "--host", "claude"]).hosts, ["codex", "claude"]);
  assert.deepEqual(parseArguments(["upgrade", "--self"], { isTTY: false }).hosts, []);
  for (const args of [["factory-reset"], ["delete-all", "--reinstall"], ["status", "--self"], ["install", "--self-package", "/a.tgz"], ["delete-data", "--host", "codex"], ["install", "--host", "all", "--host", "codex"], ["status", "--json", "--plain"], ["install", "--host", "deepseek", "--profile", "../web"], ["upgrade", "--host", "codex", "--version", "beta"]]) assert.throws(() => parseArguments(args), CLIError);
  assert.throws(() => parseArguments(["install"], { isTTY: false }), /--host or --self/);
  assert.deepEqual(parseArguments(["delete-data"], { isTTY: false }).hosts, AGENTS);
});

test("home exposes upgrade, Agent install and three distinct cleanup results", async () => {
  for (const operation of ["uninstall", "delete-data", "delete-all"]) {
    const answers = operation === "uninstall" ? ["cleanup", operation, ["codex"]] : ["cleanup", operation, "recoverable"];
    let home;
    const request = await promptForRequest({ output: output(), language: "zh-CN", session: scripted(answers, (_message, entries) => { home ??= entries; }), observation: { targets: [{ host: "codex", state: "ready", package_version: "1.0.0" }] } });
    assert.equal(request.operation, operation);
    assert.ok(home.some(entry => entry.label === "安装到 Agent"));
    assert.equal(home[0].value, "upgrade");
    assert.equal(request.allKnownProfiles, operation !== "uninstall");
  }
});

test("install supports arbitrary Agent and Profile subsets and retains choices on back", async () => {
  const state = {};
  const request = await promptForRequest({ output: output(), state, session: scripted(["install", ["codex", "deepseek"], BACK, ["codex", "deepseek"], ["web", "work"]], (message, _entries, options) => { if (message === "install" && state.selections.install) assert.deepEqual(options.initial, ["codex", "deepseek"]); }) });
  assert.deepEqual(request.hosts, ["codex", "deepseek"]);
  assert.deepEqual(request.profiles, ["web", "work"]);
  assert.equal(state.nextOperation, "install");
});

test("upgrade submits only selected installed components and scopes an explicit version to one independently released component", async () => {
  const choices = [{ value: "self", label: "CLI" }, { value: "codex", label: "Codex", host: "codex" }, { value: "deepseek:work", label: "work", host: "deepseek", profile: "work" }];
  const state = {};
  const request = await promptForRequest({ output: output(), state, session: scripted(["upgrade", { action: "v", selected: [] }, "deepseek:work", "1.2.3"]), inspectUpgrade: async options => { assert.ok(["latest", "1.2.3"].includes(options.targetVersion)); return choices; } });
  assert.deepEqual(request.hosts, ["deepseek"]); assert.deepEqual(request.profiles, ["work"]);
  assert.equal(request.self, false); assert.equal(request.targetVersion, "1.2.3");
});

test("terminal arrows skip disabled rows, Space toggles, Enter submits and raw mode restores", async () => {
  const input = new PassThrough(), out = output();
  const raw = []; input.isTTY = true; input.setRawMode = value => { input.isRaw = value; raw.push(value); };
  const session = createTerminalSession(input, out, { language: "zh-CN", environment: { NO_COLOR: "1" } });
  const pending = session.multiselect("选择", [{ value: "a", label: "第一项" }, { value: "b", label: "不可选", disabled: true }, { value: "c", label: "第三项" }]);
  input.write(" \u001b[B \r");
  assert.deepEqual(await pending, ["a", "c"]);
  assert.equal(input.isRaw, false); assert.deepEqual(raw, [true, false]);
  const back = session.select("返回", [{ value: true, label: "执行" }]); input.write("\u001b[D"); assert.equal(await back, BACK);
  const cancel = session.select("退出", [{ value: true, label: "执行" }]); input.write("\u0003"); assert.equal(await cancel, CANCEL);
  session.close(); assert.equal(input.listenerCount("keypress"), 0);
  assert.match(out.text, /Space|空格/); assert.doesNotMatch(out.text, /\u001b\[[0-9;]+m/u);
});

test("empty multiselect does not submit; text keeps spaces and buffers no later approval", async () => {
  const input = new PassThrough(), out = output(), session = createTerminalSession(input, out);
  const selected = session.multiselect("Select", [{ value: "a", label: "A" }]);
  input.write("\r"); assert.match(out.text, /Select at least one/);
  input.write("a\rignored"); assert.deepEqual(await selected, ["a"]);
  const text = session.question("Path"); input.write("/tmp/my folder\r"); assert.equal(await text, "/tmp/my folder");
  session.close();
});

test("destructive, permanent and downgrade approvals cannot be supplied by --yes", async () => {
  const plan = { operation: "delete-all", confirmationClass: "permanent_delete", confirmationToken: "DELETE-ALL-123", permanentToken: "PERMANENT-123" };
  assert.equal(await confirmPlan(plan, { operation: "delete-all", yes: true, permanent: true, outputMode: "json" }, { input: { isTTY: true }, output: output() }), false);
  assert.equal(await confirmPlan(plan, { operation: "delete-all", permanent: true }, { input: { isTTY: true }, output: output(), session: scripted(["DELETE-ALL-123", "PERMANENT-123"]) }), true);
  assert.equal(await confirmPlan({ confirmationClass: "mutation" }, {}, { input: { isTTY: true }, output: output(), session: scripted([BACK]) }), BACK);
  assert.equal(await confirmPlan({ confirmationClass: "downgrade", downgradeToken: "DOWNGRADE-123" }, { yes: true }, { input: { isTTY: false }, output: output() }), false);
});

test("JSON output stays exactly one object while Chinese plans describe data retention", () => {
  const result = { operation: "delete-data", status: "restart_required", changed: true, targets: [], next_step: null };
  assert.deepEqual(JSON.parse(renderResult(result, { mode: "json", language: "zh-CN" })), result);
  const text = renderPlan({ operation: "delete-data", planId: "test", actions: [], impacts: ["Preserve plugins, Agent registrations and installation records", "Create empty data directories at the original paths and restore default preferences"] }, { language: "zh-CN" });
  assert.match(text, /删除数据（保留插件）/); assert.match(text, /在原位置准备空数据目录/);
  assert.equal(resolveLanguage({ LANG: "zh_CN.UTF-8" }), "zh-CN");
  assert.match(renderProgress({ type: "action_start", action: { operation: "cleanup", host: "manager" } }, { language: "zh-CN" }), /删除已确认的数据/);
});

test("all-current upgrade offers home and recheck without an empty multiselect or npm local archive", async () => {
  const out = output(); let checks = 0;
  const session = scripted(["upgrade", "r", "home", "exit"], (_message, entries, options) => {
    assert.ok(entries.every(entry => !entry.disabled));
    assert.equal(options?.shortcuts?.l, undefined);
  });
  session.multiselect = () => { throw new Error("no selectable updates"); };
  await promptForRequest({ output: out, language: "zh-CN", session, inspectUpgrade: async () => {
    checks++; return [{ value: "self", label: "CLI", channel: "npm", status: "current", disabled: true, hint: "0.3.8" }];
  } });
  assert.equal(checks, 2); assert.match(out.text, /正在检查更新/);
});

test("failed lookups are not labeled current and backing out of version input does not requery", async () => {
  const out = output(); let checks = 0; const titles = [];
  await promptForRequest({ output: out, language: "zh-CN", session: scripted(["upgrade", "v", "self", BACK, "home", "exit"], title => titles.push(title)),
    inspectUpgrade: async () => { checks++; return [{ value: "self", label: "CLI", channel: "npm", status: "failed", disabled: true, hint: "registry unavailable" }]; } });
  assert.equal(checks, 1); assert.ok(titles.some(title => title.includes("检查失败"))); assert.ok(titles.every(title => !title.includes("已是最新")));
});

test("terminal text editing and invalid input stay in the field", async () => {
  const input = new PassThrough(), out = output(), session = createTerminalSession(input, out, { language: "zh-CN" });
  const value = session.question("Version", { initial: "latest", validate: text => /^\d+\.\d+\.\d+$/.test(text) ? null : "invalid version" });
  input.write("\r"); assert.match(out.text, /invalid version/);
  input.write("\u00150.3.9\u001b[D\u001b[3~8\r");
  assert.equal(await value, "0.3.8");
  const words = session.question("Words", { initial: "first second" });
  input.write("\u0017third\u0001X\u0005!\r"); assert.equal(await words, "Xfirst third!");
  session.close();
});

test("diagnostics show optional Agents neutrally and ZCode instructions only once", () => {
  const step = "Start a new ZCode session. Check the TaskBelay Skill, plugin MCP server and Write/Edit Hook in ZCode before starting a Task. This CLI cannot observe their loaded state.";
  const result = { operation: "doctor", status: "action_required", changed: false, targets: [
    { host: "claude", state: "absent", host_available: false, issues: [{ code: "host_missing", message: "missing Claude" }] },
    { host: "zcode", state: "action_required", issues: [{ code: "host_action_required", message: step }] }],
    checks: [{ name: "zcode", status: "action_required", message: step }], next_steps: [step], installation: { data: "/private/data" } };
  const text = renderResult(result, { language: "zh-CN", detailed: false });
  assert.equal(text.split("开启新的 ZCode 会话").length, 2);
  assert.doesNotMatch(text, /Start a new|missing Claude|! 未检测到|private/);
});
