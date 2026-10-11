import test from "node:test";
import assert from "node:assert/strict";
import { readFile, access, copyFile, lstat, mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { delimiter, dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";
import { execPortableCommand } from "../lib/command.mjs";
const root = new URL("../", import.meta.url);

test("npm-installed Claude bin returns status JSON with fixture Host and Core", {
  skip: process.platform === "win32" ? "npm bin symlink regression uses POSIX fixture executables" : false,
}, async t => {
  const isolated = await mkdtemp(join(tmpdir(), "taskbelay-claude-npm-"));
  t.after(() => rm(isolated, { recursive: true, force: true }));
  const prefix = join(isolated, "prefix");
  const hostBin = join(isolated, "host-bin");
  await mkdir(hostBin);
  await writeFile(join(hostBin, "claude"), `#!/usr/bin/env node
const args = JSON.stringify(process.argv.slice(2));
if (args !== '["plugin","list","--json"]' && args !== '["plugin","marketplace","list","--json"]') throw new Error("Unexpected fixture Claude invocation");
process.stdout.write("[]\\n");
`, { mode: 0o755 });
  const options = { env: { ...process.env, HOME: isolated, USERPROFILE: isolated, LOCALAPPDATA: join(isolated, "appdata"),
    CLAUDE_CONFIG_DIR: join(isolated, "claude-config"), TASKBELAY_DATA_DIR: "", PATH: hostBin + delimiter + process.env.PATH }, timeout: 60000 };
  await execPortableCommand("npm", ["install", "--global", "--prefix", prefix, "--cache", join(isolated, "cache"),
    "--install-links", "--ignore-scripts", "--offline", "--no-audit", "--no-fund", fileURLToPath(root)], options);
  const installed = join(await realpath(prefix), "lib/node_modules/taskbelay-claude");
  const runtime = join(installed, "runtime/darwin-arm64/taskbelay");
  await mkdir(dirname(runtime), { recursive: true });
  await writeFile(runtime, '#!/usr/bin/env node\nif (process.argv[2] !== "version") throw new Error("Unexpected fixture Core invocation");\nprocess.stdout.write("taskbelay 0.0.0\\n");\n', { mode: 0o755 });
  const command = join(prefix, "bin", "taskbelay-claude");
  assert.equal((await lstat(command)).isSymbolicLink(), true);
  assert.equal(await realpath(command), join(installed, "bin/taskbelay-claude.mjs"));
  const status = JSON.parse((await execPortableCommand(command, ["status", "--json"], options)).stdout);
  assert.equal(status.operation, "status");
  assert.equal(status.status, "partial");
  assert.equal(status.changed, false);
  assert.equal(status.core_version, "0.0.0");
  assert.deepEqual(status.registration, { receipt: false, marketplace: false, plugin: false, cache: false });
  assert.match((await execPortableCommand(command, ["--help"], options)).stdout, /^taskbelay-claude status\|setup\|remove/u);
  await assert.rejects(execPortableCommand(command, ["invalid-command"], options), error => {
    assert.equal(error.code, 1);
    assert.match(error.stderr, /Invalid command; use --help/u);
    return true;
  });
});

test("Claude plugin paths stay inside the independently cached package", async () => {
  const manifest = JSON.parse(await readFile(new URL(".claude-plugin/plugin.json", root), "utf8"));
  assert.equal(manifest.name, "taskbelay-claude");
  for (const file of [manifest.hooks, manifest.mcpServers]) { assert.equal(file.includes(".."), false); await access(new URL(file, root)); }
  const mcp = JSON.parse(await readFile(new URL(".mcp.json", root), "utf8"));
  assert.equal(mcp.mcpServers["taskbelay"].command, "node");
  assert.ok(mcp.mcpServers["taskbelay"].args[0].startsWith("${CLAUDE_PLUGIN_ROOT}/"));
  const pkg = JSON.parse(await readFile(new URL("package.json", root), "utf8"));
  assert.equal(pkg.version, manifest.version);
  assert.ok(pkg.files.includes("lib/worktree-snapshot.mjs"));
  assert.ok(pkg.files.includes("plugin/skills/taskbelay/references/protocol.md"));
});

test("Claude platform policies load from an independent package directory", async t => {
  const packageDirectory = await mkdtemp(join(tmpdir(), "taskbelay-claude-platform-"));
  t.after(() => rm(packageDirectory, { recursive: true, force: true }));
  const pkg = JSON.parse(await readFile(new URL("package.json", root), "utf8"));
  const platformFiles = ["lib/platform.mjs", "lib/platform/macos/policies.mjs", "lib/platform/windows/policies.mjs"];
  for (const relative of platformFiles) {
    assert.ok(pkg.files.includes(relative), relative);
    const target = join(packageDirectory, relative);
    await mkdir(dirname(target), { recursive: true });
    await copyFile(new URL(relative, root), target);
  }
  const { platformPolicy } = await import(pathToFileURL(join(packageDirectory, "lib/platform.mjs")).href);
  assert.equal(platformPolicy("win32", "x64").runtimeExecutable, "taskbelay.exe");
  assert.equal(platformPolicy("darwin", "arm64").runtimeExecutable, "taskbelay");
  assert.throws(() => platformPolicy("win32", "arm64"), /Unsupported Claude Adapter platform/);
  assert.throws(() => platformPolicy("linux", "x64"), /Unsupported Claude Adapter platform/);
});
