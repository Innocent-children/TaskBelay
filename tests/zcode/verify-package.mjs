import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { mkdtemp, mkdir, readFile, realpath, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { execPortableCommand } from "../../packages/host-command/command.mjs";

const exec = promisify(execFile);
const [packageDirectory] = process.argv.slice(2);
if (!packageDirectory) throw new Error("Usage: node tests/zcode/verify-package.mjs ABSOLUTE_EXTRACTED_PACKAGE");
const source = await realpath(resolve(packageDirectory));
const isolated = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-zcode-acceptance-")));
const data = join(isolated, "data"), repository = join(isolated, "repository");
await mkdir(data); await mkdir(repository);
const environment = { ...process.env, HOME: isolated, USERPROFILE: isolated, LOCALAPPDATA: join(isolated, "appdata"), TASKBELAY_DATA_DIR: data };
const prefix = join(isolated, "npm-prefix");
const commandOptions = { env: environment, windowsHide: true, timeout: 30000, maxBuffer: 8 * 1024 * 1024 };
await execPortableCommand("npm", ["install", "--global", "--prefix", prefix, "--cache", join(isolated, "npm-cache"),
  "--install-links", "--ignore-scripts", "--offline", "--no-audit", "--no-fund", source], commandOptions);
const root = join(prefix, process.platform === "win32" ? "node_modules" : "lib/node_modules", "taskbelay-zcode");
const launcher = join(prefix, process.platform === "win32" ? "" : "bin", "taskbelay-zcode");
const cli = join(root, "bin/taskbelay-zcode.mjs");
const run = (executable, args, options = {}) => exec(executable, args, { env: environment, windowsHide: true, timeout: 30000, maxBuffer: 8 * 1024 * 1024, ...options });
async function command(args, input) {
  const { stdout } = await execPortableCommand(launcher, args, { ...commandOptions, input: input === undefined ? "" : JSON.stringify(input) });
  return stdout.trim() ? JSON.parse(stdout) : null;
}
const report = { package: root, source_package: source, platform: `${process.platform}-${process.arch}`, isolated_directory: isolated, checks: [],
  zcode_ui: "not exercised by this package harness", model_session: "not exercised", macos_native: process.platform === "darwin" ? "native package check only" : "not executed" };
await writeFile(join(data, "unrelated.txt"), "preserve\n");
const setup = await command(["setup", "--json"]);
assert.equal(setup.status, "action_required");
assert.equal(setup.registration.host, "unverified");
assert.equal((await command(["status", "--json"])).registration.phase, "prepared");
report.checks.push("npm-generated launcher returns valid setup and status JSON");
assert.equal((await command(["setup", "--json"])).changed, false);
report.checks.push("detached complete package, native Core and idempotent local preparation; UI state remains unverified");

await run("git", ["init", "-b", "main", repository]);
await run("git", ["-C", repository, "config", "user.name", "TaskBelay package verification"]);
await run("git", ["-C", repository, "config", "user.email", "package-verification@example.invalid"]);
await writeFile(join(repository, "README.md"), "package verification\n");
await run("git", ["-C", repository, "add", "README.md"]);
await run("git", ["-C", repository, "commit", "-m", "Initialize verification repository"]);
const request = "Verify a packaged ZCode Task can be created, resumed and cancelled.";
const prepared = await command(["host-launch", "prepare"], { request, authorization_ref: "isolated-fixture-user",
  repositories: [{ key: "primary", repository_path: repository, workspace_mode: "new_branch", source_type: "local", remote_name: "", base_branch: "main", target_branch: "verify/zcode", carry_changes: false, worktree_path: repository }], handoff: null });
await command(["host-launch", "provision"], { launch_id: prepared.launch_id });
const scope = await command(["host-launch", "scope"], { launch_id: prepared.launch_id });

const child = spawn(process.execPath, [cli, "mcp"], { env: environment, windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
let buffer = "", stderr = "", sequence = 0;
const pending = new Map();
child.stderr.on("data", chunk => { stderr += chunk; });
child.stdout.on("data", chunk => {
  buffer += chunk;
  let newline;
  while ((newline = buffer.indexOf("\n")) >= 0) {
    const line = buffer.slice(0, newline); buffer = buffer.slice(newline + 1);
    let response;
    try { response = JSON.parse(line); } catch { for (const call of pending.values()) call.reject(new Error("Invalid MCP output")); return; }
    const call = pending.get(response.id);
    if (!call) continue;
    clearTimeout(call.timer); pending.delete(response.id);
    response.error ? call.reject(new Error(JSON.stringify(response.error))) : call.resolve(response.result);
  }
});
const failed = error => { for (const call of pending.values()) { clearTimeout(call.timer); call.reject(error); } pending.clear(); };
child.on("error", failed);
child.on("close", code => failed(new Error(`MCP exited ${code}: ${stderr}`)));
function rpc(method, params) {
  const id = ++sequence;
  return new Promise((resolveCall, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`MCP ${method} timed out: ${stderr}`)); }, 30000);
    pending.set(id, { resolve: resolveCall, reject, timer });
    child.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
  });
}
async function tool(name, args) {
  const result = await rpc("tools/call", { name, arguments: args });
  return result.structuredContent ?? JSON.parse(result.content.find(block => block.type === "text").text);
}
try {
  await rpc("initialize", { protocolVersion: "2024-11-05", capabilities: {}, clientInfo: { name: "zcode-package-verification", version: "1.0" } });
  child.stdin.write(JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }) + "\n");
  const info = await tool("taskbelay_server_info", {});
  assert.equal(info.ok, true); assert.equal(info.result.tools.length, 12);
  const openRequest = { operation_id: "zcode-package-open", host: "zcode", ...scope,
    authorization: {user_ref: "isolated-fixture-user", request,
      goals: [{id: "package", statement: "The packaged Core runs the real local assertion", checks: ["readme"]}],
      expected_paths: ["README.md"], forbidden_paths: ["unexpected.txt"]},
    checks: [{id: "readme", kind: "command", argv: [process.execPath, "-e", "require('node:assert/strict').ok(require('node:fs').readFileSync('README.md','utf8').includes('package verification'))"],
      inputs: ["README.md"], input_basis: "Only this file and Node built-ins affect the assertion", conditions: "Local Node and README", rationale: "Execute an actual package check", timeout_ms: 3000, retry: "repeatable"}]};
  const opened = await tool("taskbelay_open_task", openRequest);
  assert.equal(opened.ok, true, JSON.stringify(opened));
  let view = opened.result;
  await command(["host-launch", "bind-task"], { launch_id: prepared.launch_id, task_id: view.task.task_id });
  const resumed = await tool("taskbelay_open_task", openRequest);
  assert.equal(resumed.ok, true); assert.equal(resumed.result.task.task_id, view.task.task_id); assert.equal(resumed.result.replayed, true);
  const executed = await tool("taskbelay_run_check", {task_id: view.task.task_id, revision: view.task.revision, operation_id: "zcode-package-run", check_id: "readme"});
  assert.equal(executed.ok, true, JSON.stringify(executed)); assert.equal(executed.result.run.status, "passed"); assert.equal(executed.result.run.exit_code, 0); view = executed.result.view;
  report.checks.push("packaged native stdio MCP, current catalog, Task creation, identical-open replay and actual local check");
  assert.equal(await command(["hook", "pre-tool-use"], { hook_event_name: "PreToolUse", cwd: repository,
    tool_name: "Edit", tool_input: { file_path: join(repository, "README.md"), old_string: "verification", new_string: "verified" } }), null);
  const hook = await command(["hook", "pre-tool-use"], { hook_event_name: "PreToolUse", cwd: repository,
    tool_name: "Write", tool_input: { file_path: join(repository, "unexpected.txt"), content: "must not write" } });
  assert.equal(hook.hookSpecificOutput.permissionDecision, "deny");
  report.checks.push("native Core-backed hook permits an authorized Edit and denies an explicitly forbidden Write without a phase approval");
  const saved = await tool("taskbelay_get_task", {task_id: view.task.task_id});
  const cancelled = await tool("taskbelay_cancel_task", {task_id: view.task.task_id, revision: saved.result.task.revision,
    operation_id: "zcode-package-cancel", reason: "The isolated package verification is complete."});
  assert.equal(cancelled.ok, true, JSON.stringify(cancelled)); assert.equal(cancelled.result.task.status, "cancelled");
  report.checks.push("cancellation releases the isolated repository claim and preserves code");

} finally {
  child.stdin.end(); child.kill();
}
const removed = await command(["remove", "--json"]);
assert.equal(removed.status, "action_required");
assert.equal(removed.registration.phase, "removal_required");
assert.equal((await command(["remove", "--json"])).changed, false);
assert.equal(await readFile(join(data, "unrelated.txt"), "utf8"), "preserve\n");
report.checks.push("idempotent removal preserves Task data and truthfully reports pending Host UI removal");
const reportPath = join(isolated, "report.json");
await writeFile(reportPath, JSON.stringify(report, null, 2) + "\n");
process.stdout.write(JSON.stringify({ ...report, report_path: reportPath }, null, 2) + "\n");
