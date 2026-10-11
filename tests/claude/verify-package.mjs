import { mkdtemp, mkdir, readFile, realpath, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { execFile, spawn } from "node:child_process";
import { promisify } from "node:util";
import assert from "node:assert/strict";
import { execPortableCommand } from "../../packages/host-command/command.mjs";
const run = promisify(execFile);
const [packageDirectory, claudeExecutable] = process.argv.slice(2);
if (!packageDirectory || !claudeExecutable) throw new Error("Usage: node verify-package.mjs ABSOLUTE_EXTRACTED_PACKAGE ABSOLUTE_CLAUDE_EXECUTABLE");
const source = await realpath(resolve(packageDirectory)), isolated = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-claude-acceptance-")));
const config = join(isolated, "config"), data = join(isolated, "data");
await mkdir(config); await mkdir(data);
const environment = { ...process.env, HOME: isolated, USERPROFILE: isolated, LOCALAPPDATA: join(isolated, "appdata"), CLAUDE_CONFIG_DIR: config, TASKBELAY_DATA_DIR: data, PATH: dirname(claudeExecutable) + (process.platform === "win32" ? ";" : ":") + process.env.PATH };
const prefix = join(isolated, "npm-prefix");
const commandOptions = { env: environment, windowsHide: true, maxBuffer: 8 * 1024 * 1024, timeout: 60000 };
await execPortableCommand("npm", ["install", "--global", "--prefix", prefix, "--cache", join(isolated, "npm-cache"),
  "--install-links", "--ignore-scripts", "--offline", "--no-audit", "--no-fund", source], commandOptions);
const root = join(prefix, process.platform === "win32" ? "node_modules" : "lib/node_modules", "taskbelay-claude");
const launcher = join(prefix, process.platform === "win32" ? "" : "bin", "taskbelay-claude");
const command = async args => JSON.parse((await execPortableCommand(launcher, [...args, "--json"], commandOptions)).stdout);
const report = { package: root, source_package: source, host_version: (await run(claudeExecutable, ["--version"], { env: environment, windowsHide: true })).stdout.trim(), platform: process.platform + "-" + process.arch, isolated_directory: isolated, checks: [] };
await writeFile(join(config, "unrelated.txt"), "preserve");
const setup = await command(["setup"]); assert.equal(setup.status, "ready"); report.checks.push("native plugin installation and byte-for-byte cache closure");
assert.equal((await command(["status"])).status, "ready"); report.checks.push("npm-generated launcher returns valid setup and status JSON");
assert.equal((await command(["setup"])).changed, false); report.checks.push("repeat setup has no changes");
const info = await new Promise((done, fail) => {
  const child = spawn(process.execPath, [join(root, "bin/taskbelay-claude.mjs"), "mcp"], { env: environment, windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
  let buffer = "", errors = "", settled = false;
  const finish = (error, value) => { if (settled) return; settled = true; clearTimeout(timer); child.stdin.end(); child.kill(); error ? fail(error) : done(value); };
  const timer = setTimeout(() => finish(new Error("MCP handshake timeout: " + errors)), 15000);
  child.on("error", error => finish(error));
  child.stderr.on("data", chunk => { errors += chunk; });
  child.stdout.on("data", chunk => {
    buffer += chunk;
    let newline;
    while ((newline = buffer.indexOf("\n")) >= 0) {
      const line = buffer.slice(0,newline); buffer = buffer.slice(newline+1);
      let value; try { value=JSON.parse(line); } catch { return finish(new Error("Invalid MCP output")); }
      if (value.id === 1) {
        child.stdin.write(JSON.stringify({jsonrpc:"2.0",method:"notifications/initialized"})+"\n");
        child.stdin.write(JSON.stringify({jsonrpc:"2.0",id:2,method:"tools/call",params:{name:"taskbelay_server_info",arguments:{}}})+"\n");
      }
      if (value.id === 2) {
        if (value.error) return finish(new Error(JSON.stringify(value.error)));
        const envelope = value.result.structuredContent ?? JSON.parse(value.result.content.find(v=>v.type==="text").text);
        return finish(null,envelope);
      }
    }
  });
  child.on("close", code => { if (!settled) finish(new Error("MCP exited " + code + ": " + errors)); });
  child.stdin.write(JSON.stringify({jsonrpc:"2.0",id:1,method:"initialize",params:{protocolVersion:"2024-11-05",capabilities:{},clientInfo:{name:"claude-adapter-acceptance",version:"1.0"}}})+"\n");
});
assert.equal(info.ok,true); assert.equal(info.result.tools.length,12); assert.ok(info.result.coverage.claude);
report.checks.push("packaged stdio Core handshake exposes current tools and Claude coverage");
assert.equal((await command(["remove"])).status,"absent");
assert.equal(await readFile(join(config,"unrelated.txt"),"utf8"),"preserve");
assert.equal((await command(["remove"])).changed,false);
report.checks.push("native plugin removal, unrelated data retention and repeated removal");
report.model_session = "not checked by this harness";
const reportPath = join(isolated,"report.json"); await writeFile(reportPath,JSON.stringify(report,null,2)+"\n");
process.stdout.write(JSON.stringify({...report,report_path:reportPath},null,2)+"\n");
