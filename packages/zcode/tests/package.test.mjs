import test from "node:test";
import assert from "node:assert/strict";
import { readFile, access, copyFile, lstat, mkdir, mkdtemp, realpath, rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";
import { operations } from "../bin/taskbelay-zcode.mjs";
import { execPortableCommand } from "../lib/command.mjs";

const root = new URL("../", import.meta.url);

test("npm-installed ZCode bin returns status JSON before local preparation", async t => {
  const isolated = await mkdtemp(join(tmpdir(), "taskbelay-zcode-npm-"));
  t.after(() => rm(isolated, { recursive: true, force: true }));
  const prefix = join(isolated, "prefix");
  const options = { env: { ...process.env, HOME: isolated, USERPROFILE: isolated, LOCALAPPDATA: join(isolated, "appdata"), TASKBELAY_DATA_DIR: "" }, timeout: 60000 };
  await execPortableCommand("npm", ["install", "--global", "--prefix", prefix, "--cache", join(isolated, "cache"),
    "--install-links", "--ignore-scripts", "--offline", "--no-audit", "--no-fund", fileURLToPath(root)], options);
  const command = join(prefix, process.platform === "win32" ? "" : "bin", "taskbelay-zcode");
  if (process.platform !== "win32") {
    assert.equal((await lstat(command)).isSymbolicLink(), true);
    assert.equal(await realpath(command), join(await realpath(prefix), "lib/node_modules/taskbelay-zcode/bin/taskbelay-zcode.mjs"));
  }
  const status = JSON.parse((await execPortableCommand(command, ["status", "--json"], options)).stdout);
  assert.equal(status.operation, "status");
  assert.equal(status.status, "partial");
  assert.equal(status.changed, false);
  assert.equal(status.registration.receipt, false);
  assert.equal(status.registration.phase, null);
  assert.deepEqual(status.next_steps.slice(0, 1), ["Run taskbelay-zcode setup --json to prepare and verify the local plugin source."]);
  assert.match((await execPortableCommand(command, ["--help"], options)).stdout, /^taskbelay-zcode status\|setup\|remove/u);
  await assert.rejects(execPortableCommand(command, ["invalid-command"], options), error => {
    assert.equal(error.code, 1);
    assert.match(error.stderr, /Invalid command; use --help/u);
    return true;
  });
});

test("native plugin, marketplace, MCP and process Hook share a self-contained package", async () => {
  const pkg = JSON.parse(await readFile(new URL("package.json", root), "utf8"));
  const plugin = JSON.parse(await readFile(new URL(".zcode-plugin/plugin.json", root), "utf8"));
  const market = JSON.parse(await readFile(new URL("marketplace.json", root), "utf8"));
  const mcp = JSON.parse(await readFile(new URL(".mcp.json", root), "utf8"));
  const hooks = JSON.parse(await readFile(new URL("hooks/hooks.json", root), "utf8"));
  assert.equal(plugin.name, pkg.name);
  assert.equal(plugin.version, pkg.version);
  assert.equal(market.plugins[0].version, pkg.version);
  assert.equal(market.plugins[0].source, "./");
  assert.equal(Object.hasOwn(plugin, "hooks"), false, "standard hooks must be discovered exactly once");
  assert.deepEqual(mcp.mcpServers["taskbelay"].args, ["${ZCODE_PLUGIN_ROOT}/bin/taskbelay-zcode.mjs", "mcp"]);
  const group = hooks.hooks.PreToolUse[0];
  assert.equal(group.matcher, "Write|Edit");
  assert.equal(group.hooks[0].type, "process");
  assert.equal(group.hooks[0].command, "node");
  assert.deepEqual(group.hooks[0].args, ["${ZCODE_PLUGIN_ROOT}/bin/taskbelay-zcode.mjs", "hook", "pre-tool-use"]);
  for (const file of pkg.files) {
    assert.equal(file.includes(".."), false);
    if (file === "LICENSE" || file.startsWith("runtime/")) continue; // Staging supplies these from the release/build inputs.
    await access(new URL(file, root));
  }
  assert.ok(pkg.files.includes("runtime/win32-x64/taskbelay.exe"));
  assert.ok(pkg.files.includes("runtime/darwin-arm64/taskbelay"));
  assert.ok(pkg.files.includes("skills/taskbelay/references/engineering.md"));
  assert.ok(pkg.files.includes("lib/worktree-snapshot.mjs"));
  assert.ok(operations.includes("open"));
  assert.equal(operations.includes("launch"), false);
  assert.equal(operations.includes("record-session"), false);
});

test("platform policies resolve from an independent package without another Host installation", async t => {
  const directory = await mkdtemp(join(tmpdir(), "zcode-platform-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const pkg = JSON.parse(await readFile(new URL("package.json", root), "utf8"));
  for (const file of ["lib/platform.mjs", "lib/platform/macos/policies.mjs", "lib/platform/windows/policies.mjs"]) {
    assert.ok(pkg.files.includes(file));
    const target = join(directory, file);
    await mkdir(dirname(target), { recursive: true });
    await copyFile(new URL(file, root), target);
  }
  const { platformPolicy } = await import(pathToFileURL(join(directory, "lib/platform.mjs")).href);
  assert.equal(platformPolicy("win32", "x64").runtimeExecutable, "taskbelay.exe");
  assert.equal(platformPolicy("darwin", "arm64").runtimeExecutable, "taskbelay");
  assert.throws(() => platformPolicy("win32", "arm64"), /Unsupported ZCode/);
  assert.throws(() => platformPolicy("linux", "x64"), /Unsupported ZCode/);
});
