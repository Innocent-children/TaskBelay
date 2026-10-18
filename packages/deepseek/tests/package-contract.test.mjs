import assert from "node:assert/strict";
import { execFile as execFileCallback, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm } from "node:fs/promises";
import { arch, platform, tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { gunzipSync } from "node:zlib";

import { execPortableCommand } from "../../taskbelay/lib/command.mjs";
import { sharedSkillReferences } from "../../../scripts/sync-skill-references.mjs";
import { ustarEntryModes } from "../../../scripts/taskbelay-local.mjs";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const repositoryRoot = dirname(dirname(packageRoot));
const execFile = promisify(execFileCallback);
const currentCoreVersion = (await readFile(join(repositoryRoot, "CORE_VERSION"), "utf8")).trim();
const packageIconUrl = "https://raw.githubusercontent.com/Innocent-children/taskbelay/main/packages/webui/src/assets/taskbelay-app-icon-light.svg";
const runtimePaths = [
  "runtime/darwin-arm64/taskbelay",
  "runtime/win32-x64/taskbelay.exe",
];

const expectedPackageFiles = JSON.parse(await readFile(join(packageRoot,'package.json'),'utf8')).files;

const expectedSourcePackedFiles = [
  "package.json",
  ...expectedPackageFiles.filter((path) => !runtimePaths.includes(path)),
].sort();
const expectedFinalPackedFiles = ["package.json", ...expectedPackageFiles].sort();

const lifecycleHooks = [
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
  "postuninstall",
];


test("manifest declares one public macOS arm64 and Windows x64 ESM DeepSeek bundle", async () => {
  const manifest = await readJSON(join(packageRoot, "package.json"));

  assert.equal(manifest.name, "taskbelay-deepseek");
  assert.match(manifest.version, /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-beta\.(0|[1-9]\d*))?$/u);
  assert.equal(manifest.private, false);
  assert.equal(manifest.type, "module");
  assert.equal(manifest.main, "lib/index.mjs");
  assert.equal(manifest.license, "Apache-2.0");
  assert.deepEqual(manifest.repository, {
    type: "git",
    url: "git+https://github.com/Innocent-children/taskbelay.git",
    directory: "packages/deepseek",
  });
  assert.deepEqual(manifest.engines, { node: ">=24" });
  assert.deepEqual(manifest.os, ["darwin", "win32"]);
  assert.deepEqual(manifest.cpu, ["arm64", "x64"]);
  assert.deepEqual(manifest.publishConfig, { access: "public", registry: "https://registry.npmjs.org/" });
  assert.deepEqual(manifest.dsh, { bundle: { patch: "./cordis.patch.yml" } });
  assert.equal("bin" in manifest, false);
});

test("manifest closes final package, dependency, and lifecycle surfaces", async () => {
  const manifest = await readJSON(join(packageRoot, "package.json"));

  assert.deepEqual([...manifest.files].sort(), [...expectedPackageFiles].sort());
  assert.equal(manifest.files.some((path) => /[*?[\]{}]/u.test(path)), false);
  assert.deepEqual(manifest.dependencies, {
    "@deepseek-ai/dsh-mcp-client": ">=0.1.2-rc.1",
  });
  assert.deepEqual(manifest.peerDependencies, {
    "@deepseek-ai/cordis": ">=4.0.2 <5.0.0",
    "@deepseek-ai/dsh-skill": ">=0.1.2-rc.1",
    "@deepseek-ai/dsh-tools": ">=0.1.2-rc.1",
  });
  assert.equal(manifest.scripts["build:webui"], "node ../../scripts/build-webui.mjs");
  assert.equal(manifest.scripts["build:local"], "node ../../scripts/build-deepseek-local.mjs");

  for (const field of [
    "dependencies",
    "optionalDependencies",
    "peerDependencies",
    "devDependencies",
    "bundledDependencies",
    "bundleDependencies",
  ]) {
    assert.equal(
      Object.hasOwn(manifest, field) && Object.hasOwn(manifest[field], "taskbelay-codex"),
      false,
      field,
    );
  }
  for (const hook of lifecycleHooks) assert.equal(Object.hasOwn(manifest.scripts, hook), false, hook);
});


test("source checkout omits precompiled Core while preserving the final manifest contract", async () => {
  for (const runtimePath of runtimePaths) {
    await assert.rejects(lstat(join(packageRoot, runtimePath)), { code: "ENOENT" });
  }
  const { stdout } = await execPortableCommand(
    "pnpm",
    ["--config.ignore-scripts=true", "--dir", packageRoot, "pack", "--dry-run", "--json"],
    { encoding: "utf8" },
  );
  const report = JSON.parse(stdout);
  const packed = Array.isArray(report) ? report[0] : report;
  const packedFiles = (packed.files ?? [])
    .map((file) => (typeof file === "string" ? file : file.path ?? file.name))
    .sort();
  assert.equal(packed.name, "taskbelay-deepseek");
  assert.deepEqual(packedFiles, expectedSourcePackedFiles);
  assert.equal(packedFiles.some((path) => path.startsWith("runtime/")), false);
  assert.equal(packedFiles.some((path) => /(?:^|\/)(?:tests?|evidence|profiles?|data)(?:\/|$)/iu.test(path)), false);
});

for (const [name, ending] of [["LF", "\n"], ["CRLF", "\r\n"]]) {
  test(`tar listing parser preserves exact filenames with ${name} endings`, () => {
    const listing = [
      "package/", "package/lib/", "package/z.mjs", "package/lib/file name.mjs",
      "unexpected.txt", "package/ leading and trailing spaces ", "",
    ].join(ending);
    assert.deepEqual(tarListingFiles(listing), [
      " leading and trailing spaces ", "lib/file name.mjs", "unexpected.txt", "z.mjs",
    ]);
  });
}

test("staged tarball contains and starts the current dual-platform Core", async (t) => {
  const outputDirectory = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-deepseek-package-contract-")));
  const extractDirectory = join(outputDirectory, "extract");
  await mkdir(extractDirectory);
  t.after(() => rm(outputDirectory, { recursive: true, force: true }));

  const { stdout } = await execFile(
    process.execPath,
    [join(repositoryRoot, "scripts", "build-deepseek-local.mjs"), "--output", outputDirectory],
    { cwd: repositoryRoot, encoding: "utf8", maxBuffer: 4 * 1024 * 1024 },
  );
  const report = JSON.parse(stdout);
  assert.equal(report.package_version, (await readJSON(join(packageRoot, "package.json"))).version);
  assert.equal(report.core_version, currentCoreVersion);
  assert.deepEqual(report.platforms, ["darwin-arm64", "win32-x64"]);
  assert.equal(await sha256(readFile(report.artifact_path)), report.artifact_sha256);

  const { stdout: listing } = await execFile("tar", ["-tzf", report.artifact_path], { encoding: "utf8" });
  const packedFiles = tarListingFiles(listing);
  assert.deepEqual(packedFiles, expectedFinalPackedFiles);

  const modes = ustarEntryModes(gunzipSync(await readFile(report.artifact_path)));
  assert.equal(modes.get("package/runtime/darwin-arm64/taskbelay"), 0o755);
  assert.equal(modes.get("package/runtime/win32-x64/taskbelay.exe"), 0o644);
  await execFile("tar", ["-xzf", report.artifact_path, "-C", extractDirectory]);
  const installedSkill = join(extractDirectory, "package", "skills", "taskbelay");
  for (const [path, expected] of await sharedSkillReferences({ root: repositoryRoot, host: "deepseek" })) {
    assert.equal(await readFile(join(installedSkill, path), "utf8"), expected, path);
  }
  assert.ok((await readFile(join(installedSkill, "references", "engineering.md"), "utf8")).includes("Core"));


  const runtimes = [
    { path: "runtime/darwin-arm64/taskbelay", goos: "darwin", goarch: "arm64" },
    { path: "runtime/win32-x64/taskbelay.exe", goos: "windows", goarch: "amd64" },
  ];
  for (const runtime of runtimes) {
    const runtimePath = join(extractDirectory, "package", runtime.path);
    assert.equal((await lstat(runtimePath)).isFile(), true);
    const { stdout: metadata } = await execFile("go", ["version", "-m", runtimePath], { encoding: "utf8" });
    for (const expected of [
      `\tbuild\tGOOS=${runtime.goos}`,
      `\tbuild\tGOARCH=${runtime.goarch}`,
      "\tbuild\tCGO_ENABLED=0",
    ]) {
      assert.equal(metadata.replaceAll("\r\n", "\n").split("\n").includes(expected), true, expected);
    }
  }

  const nativeRuntime = platform() === "darwin" && arch() === "arm64"
    ? "runtime/darwin-arm64/taskbelay"
    : platform() === "win32" && arch() === "x64"
      ? "runtime/win32-x64/taskbelay.exe"
      : null;
  assert.notEqual(nativeRuntime, null, "final artifact contract requires a supported native runner");
  const runtimePath = join(extractDirectory, "package", nativeRuntime);
  const dataDirectory = join(outputDirectory, "data");
  await mkdir(dataDirectory);
  assert.equal((await execFile(runtimePath, ["version"], { encoding: "utf8" })).stdout, `taskbelay ${currentCoreVersion}\n`);
  const environment = { ...process.env, TASKBELAY_DATA_DIR: dataDirectory };
  if (platform() === "win32") {
    environment.USERPROFILE = outputDirectory;
    delete environment.HOME;
  } else {
    environment.HOME = outputDirectory;
  }

  assert.deepEqual(await readdir(dataDirectory), []);
  const { stdout: mcpStdout, stderr: mcpStderr } = await runWithClosedInput(
    runtimePath,
    ["mcp", "--stdio"],
    { cwd: outputDirectory, env: environment },
  );
  assert.equal(mcpStdout, "");
  assert.equal(mcpStderr, "");
});

test("DeepSeek release preparation delegates to the shared Host builder", async () => {
  const release = await readFile(join(repositoryRoot, "scripts", "build-deepseek-release.sh"), "utf8");
  assert.match(release, /exec node "\$repository_root\/scripts\/build-host-release\.mjs" --product deepseek "\$@"/u);
});

async function runWithClosedInput(command, args, options, input = "", expectedExit = 0) {
  return await new Promise((resolve, reject) => {
    const child = spawn(command, args, { ...options, stdio: ["pipe", "pipe", "pipe"], windowsHide: true });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.once("error", reject);
    child.once("close", (code, signal) => {
      if (code === expectedExit) resolve({ stdout, stderr });
      else reject(new Error(`packaged Core exited with code ${code} and signal ${signal ?? "none"}: ${stderr}`));
    });
    child.stdin.end(input);
  });
}

function tarListingFiles(listing) {
  return listing.split(/\r?\n/u)
    .filter((path) => path !== "" && !path.endsWith("/"))
    .map((path) => path.replace(/^package\//u, ""))
    .sort();
}

async function readJSON(path) {
  return JSON.parse(await readFile(path, "utf8"));
}

async function sha256(value) {
  return createHash("sha256").update(await value).digest("hex");
}