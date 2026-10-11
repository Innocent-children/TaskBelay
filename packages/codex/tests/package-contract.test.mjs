import assert from "node:assert/strict";
import { execFile as execFileCallback } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, readdir, stat } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import test from "node:test";
import { promisify } from "node:util";

import { sharedSkillReferences } from "../../../scripts/sync-skill-references.mjs";
import { hostCommandFiles } from "../../../scripts/sync-host-commands.mjs";
import { releaseOutputNames } from "../../../release/prepare.mjs";
import { execPortableCommand, findCommandPath } from "../lib/command.mjs";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const repositoryRoot = dirname(dirname(packageRoot));
const pluginRoot = join(packageRoot, "plugin");
const execFile = promisify(execFileCallback);
const currentVersion = (await readFile(join(repositoryRoot, "CORE_VERSION"), "utf8")).trim();

const expectedPackageFiles = JSON.parse(await readFile(join(packageRoot, "package.json"), "utf8")).files;

const expectedPackedFiles = ["README.md", "package.json", ...expectedPackageFiles].sort();


test("source package declares one public macOS arm64 and Windows x64 Codex product", async () => {
  const [coreVersion, manifest, plugin, marketplace, mcp] = await Promise.all([
    readFile(join(repositoryRoot, "CORE_VERSION"), "utf8").then((value) => value.trim()),
    readJSON(join(packageRoot, "package.json")),
    readJSON(join(pluginRoot, ".codex-plugin", "plugin.json")),
    readJSON(join(packageRoot, ".agents", "plugins", "marketplace.json")),
    readJSON(join(pluginRoot, ".mcp.json")),
  ]);

  assert.equal(manifest.name, "taskbelay-codex");
  assert.match(coreVersion, /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u);
  assert.equal(manifest.private, false);
  assert.equal(manifest.license, "Apache-2.0");
  assert.deepEqual(manifest.os, ["darwin", "win32"]);
  assert.deepEqual(manifest.cpu, ["arm64", "x64"]);
  assert.deepEqual(manifest.publishConfig, {
    access: "public",
    registry: "https://registry.npmjs.org/",
  });
  assert.deepEqual(manifest.repository, {
    type: "git",
    url: "git+https://github.com/Innocent-children/taskbelay.git",
    directory: "packages/codex",
  });
  assert.deepEqual(manifest.engines, { node: ">=24" });
  for (const field of [
    "dependencies",
    "optionalDependencies",
    "peerDependencies",
    "bundledDependencies",
    "bundleDependencies",
  ]) {
    assert.equal(field in manifest, false, field);
  }

  assert.equal(plugin.name, "taskbelay-codex");
  assert.equal(plugin.version, manifest.version);
  assert.equal(plugin.skills, "./skills/");
  assert.equal(plugin.mcpServers, "./.mcp.json");
  assert.equal(plugin.hooks, "./hooks/hooks.json");
  assert.equal("apps" in plugin, false);
  assert.deepEqual(marketplace.plugins.map((entry) => entry.name), ["taskbelay-codex"]);
  assert.deepEqual(mcp.mcpServers, {
    "taskbelay": {
      type: "stdio",
      command: "taskbelay-codex",
      args: ["mcp"],
      env_vars: ["TASKBELAY_DATA_DIR"],
    },
  });
  assert.equal(
    normalizeNewlines(await readFile(join(pluginRoot, "skills", "taskbelay", "agents", "openai.yaml"), "utf8")),
    "policy:\n  allow_implicit_invocation: true\n",
  );
});

test("package metadata closes source, artifact, and development command surfaces", async () => {
  const manifest = await readJSON(join(packageRoot, "package.json"));
  assert.ok(manifest.files.includes("plugin/skills/taskbelay/references/engineering.md"));
  assert.equal(manifest.files.includes("plugin/skills/taskbelay/references/engineering.md"), true);
  assert.equal(manifest.files.includes("plugin/skills/taskbelay/references/protocol.md"), true);
  assert.equal(manifest.files.some((path) => /[*?[\]{}]/u.test(path)), false);
  assert.deepEqual(manifest.bin, { "taskbelay-codex": "bin/taskbelay-codex.mjs" });
  assert.deepEqual(manifest.scripts, {
    test: "node --test tests/*.test.mjs",
    "test:package": "node --test tests/package-contract.test.mjs",
    "test:lifecycle": "node --test tests/lifecycle.test.mjs",
    "pack:dry": "pnpm pack --dry-run --json",
    "build:webui": "node ../../scripts/build-webui.mjs",
    "build:local": "../../scripts/build-codex-local.sh",
  });

  for (const name of [
    "preinstall",
    "install",
    "postinstall",
    "prepare",
    "prepack",
    "postpack",
    "prepublish",
    "prepublishOnly",
    "publish",
    "postpublish",
    "preuninstall",
    "uninstall",
  ]) {
    assert.equal(name in manifest.scripts, false, name);
  }

});



test("npm compatibility metadata rejects an unsupported OS and CPU", async () => {
  const manifest = await readJSON(join(packageRoot, "package.json"));
  const npmManifestPath = process.platform === "win32"
    ? join(dirname(await findCommandPath("npm")), "node_modules", "npm", "package.json")
    : join((await execPortableCommand("npm", ["root", "--global"], { encoding: "utf8" })).stdout.trim(), "npm", "package.json");
  const npmRequire = createRequire(npmManifestPath);
  const { checkPlatform } = npmRequire("npm-install-checks");

  assert.doesNotThrow(() => checkPlatform(manifest, false, { os: "darwin", cpu: "arm64" }));
  assert.doesNotThrow(() => checkPlatform(manifest, false, { os: "win32", cpu: "x64" }));
  assert.throws(
    () => checkPlatform(manifest, false, { os: "linux", cpu: "x64", libc: "glibc" }),
    (error) => {
      assert.equal(error.code, "EBADPLATFORM");
      assert.deepEqual(error.required.os, ["darwin", "win32"]);
      assert.deepEqual(error.required.cpu, ["arm64", "x64"]);
      return true;
    },
  );
});

test("packaged resources contain no tests or workflow engine", async () => {
  const manifest = await readJSON(join(packageRoot, "package.json"));

  for (const declaredPath of manifest.files) {
    assert.equal(/(?:^|\/)(?:tests?|fixtures?)(?:\/|$)/iu.test(declaredPath), false, declaredPath);
    assert.equal(/(?:^|\/)(?:cmd\/taskbelay|internal|protocol)(?:\/|$)/u.test(declaredPath), false, declaredPath);
  }

  for (const path of (await walkFiles(packageRoot, {
    skipDirectories: new Set(["node_modules", "tests", "runtime"]),
  }))) {
    const contents = await readFile(join(packageRoot, path));
    if (/^(?:bin|lib)\/.*\.mjs$/u.test(path)) {
      const source = contents.toString("utf8");
      assert.doesNotMatch(source, /transitionTable|taskStates?|nextState|persistTask|sqlite/iu, path);
      assert.doesNotMatch(source, /tests\/fixtures|fake-(?:codex|core)|protocol\/fixtures/iu, path);
    }
  }
});

test("release output names derive from Codex and Core versions", () => {
  assert.deepEqual(releaseOutputNames("codex", currentVersion, currentVersion), [
    "SHA256SUMS",
    `taskbelay-core-${currentVersion}-darwin-arm64`,
    `taskbelay-core-${currentVersion}-windows-amd64.exe`,
    `taskbelay-codex-${currentVersion}.tgz`,
    "release-manifest.json",
  ].sort());
});

test("local package builder stages one exact non-final artifact in a temporary directory", {
  skip: process.platform === "win32" ? "the Unix builder is covered on Unix; Windows uses scripts/taskbelay-local.mjs" : false,
}, async () => {
  const outputDirectory = await mkdtemp(join(tmpdir(), "taskbelay-codex-package-contract-"));
  const { stdout } = await execFile(
    join(repositoryRoot, "scripts", "build-codex-local.sh"),
    ["--output", outputDirectory],
    { cwd: repositoryRoot, encoding: "utf8", maxBuffer: 4 * 1024 * 1024 },
  );
  const report = JSON.parse(stdout);
  assert.equal(report.final_artifact, false);
  assert.equal((await stat(report.artifact_path)).isFile(), true);
  assert.equal(await sha256(readFile(report.artifact_path)), report.artifact_sha256);

  const { stdout: manifestContents } = await execFile(
    "tar",
    ["-xOzf", report.artifact_path, "package/package.json"],
    { encoding: "utf8" },
  );
  const packedManifest = JSON.parse(manifestContents);
  assert.equal(packedManifest.private, false);
  assert.deepEqual(packedManifest.os, ["darwin", "win32"]);
  assert.deepEqual(packedManifest.cpu, ["arm64", "x64"]);

  const { stdout: listing } = await execFile("tar", ["-tzf", report.artifact_path], {
    encoding: "utf8",
  });
  const packedFiles = listing
    .trim()
    .split("\n")
    .filter((path) => path && !path.endsWith("/"))
    .map((path) => path.replace(/^package\//u, ""))
    .sort();
  assert.deepEqual(packedFiles, expectedPackedFiles);

  const extractDirectory = await mkdtemp(join(tmpdir(), "taskbelay-codex-package-extract-"));
  await execFile("tar", ["-xzf", report.artifact_path, "-C", extractDirectory]);
  for (const path of hostCommandFiles) {
    const source = (await readFile(join(repositoryRoot, "packages/host-command", path), "utf8")).replace(/\r\n?/gu, "\n");
    assert.equal(await readFile(join(extractDirectory, "package/lib", path), "utf8"), `// Generated from packages/host-command/${path}; edit the shared source.\n${source}`, path);
  }
  for (const [path, expected] of await sharedSkillReferences({ root: repositoryRoot, host: "codex" })) {
    assert.equal(await readFile(join(extractDirectory, "package", "plugin", "skills", "taskbelay", path), "utf8"), expected, path);
  }

  const handoff = await import(pathToFileURL(join(extractDirectory, "package", "lib", "task-handoff.mjs")));
  const reference = await readFile(join(extractDirectory, "package", "plugin", "skills", "taskbelay", "references", "task-handoff.md"), "utf8");
  const example = JSON.parse(reference.match(/<!-- task-handoff-example:start -->\n```json\n([\s\S]*?)\n```/u)[1]);
  assert.deepEqual(handoff.validateTaskHandoff(example), example);

  if (process.platform === "darwin" && process.arch === "arm64") {
    const runtime = join(extractDirectory, "package", "runtime", "darwin-arm64", "taskbelay");
    assert.notEqual((await stat(runtime)).mode & 0o111, 0);
    const { stdout: versionLine } = await execFile(runtime, ["version"], {
      cwd: extractDirectory,
      encoding: "utf8",
    });
    assert.equal(versionLine, `taskbelay ${currentVersion}\n`);
    const { stdout: help } = await execFile(runtime, ["help"], { cwd: extractDirectory, encoding: "utf8" });
    for (const command of ["webui start", "webui open", "webui status", "webui stop"]) {
      assert.match(help, new RegExp(command, "u"));
    }
  }
});

async function readJSON(path) {
  return JSON.parse(await readFile(path, "utf8"));
}

function normalizeNewlines(value) {
  return value.replaceAll("\r\n", "\n");
}

async function sha256(value) {
  const contents = await value;
  return createHash("sha256").update(contents).digest("hex");
}

async function walkFiles(root, { skipDirectories = new Set() } = {}) {
  const files = [];
  await visit(root);
  return files.sort();

  async function visit(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      if (entry.name === ".DS_Store") continue;
      const absolute = join(directory, entry.name);
      const relativePath = relative(root, absolute).split("\\").join("/");
      if (entry.isDirectory()) {
        if (!skipDirectories.has(entry.name)) await visit(absolute);
      } else if (entry.isFile()) {
        files.push(relativePath);
      }
    }
  }
}
