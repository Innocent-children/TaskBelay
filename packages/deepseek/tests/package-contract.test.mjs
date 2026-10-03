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

const expectedPackageFiles = [
  "LICENSE",
  "README.md",
  "cordis.patch.yml",
  "lib/authorization.mjs",
  "lib/file-scope.mjs",
  "lib/index.mjs",
  "lib/paths.mjs",
  "lib/platform.mjs",
  "lib/platform/macos/policies.mjs",
  "lib/platform/windows/policies.mjs",
  "lib/provisioning-receipt.mjs",
  "lib/runtime.mjs",
  "lib/tool-names.mjs",
  "lib/workspace-coordinator.mjs",
  "lib/workspace-tool.mjs",
  "lib/worktree-snapshot.mjs",
  ...runtimePaths,
  "skills/taskbelay/SKILL.md",
  "skills/taskbelay/references/admission.md",
  "skills/taskbelay/references/artifact-contract.md",
  "skills/taskbelay/references/artifacts.md",
  "skills/taskbelay/references/connection-examples.md",
  "skills/taskbelay/references/connection.md",
  "skills/taskbelay/references/core-lifecycle-examples.md",
  "skills/taskbelay/references/core-lifecycle.md",
  "skills/taskbelay/references/host-lifecycle.md",
  "skills/taskbelay/references/method-profiles.md",
  "skills/taskbelay/references/node-payloads.md",
  "skills/taskbelay/references/nodes/comprehension-examples.md",
  "skills/taskbelay/references/nodes/comprehension.md",
  "skills/taskbelay/references/nodes/delivery-examples.md",
  "skills/taskbelay/references/nodes/delivery.md",
  "skills/taskbelay/references/nodes/design-examples.md",
  "skills/taskbelay/references/nodes/design.md",
  "skills/taskbelay/references/nodes/implementation-examples.md",
  "skills/taskbelay/references/nodes/implementation.md",
  "skills/taskbelay/references/nodes/refactor-examples.md",
  "skills/taskbelay/references/nodes/refactor.md",
  "skills/taskbelay/references/nodes/requirements-examples.md",
  "skills/taskbelay/references/nodes/requirements.md",
  "skills/taskbelay/references/nodes/tasks-examples.md",
  "skills/taskbelay/references/nodes/tasks.md",
  "skills/taskbelay/references/nodes/test-examples.md",
  "skills/taskbelay/references/nodes/test.md",
  "skills/taskbelay/references/recovery-examples.md",
  "skills/taskbelay/references/recovery.md",
  "skills/taskbelay/references/response-examples.md",
  "skills/taskbelay/references/tool-results.md",
  "skills/taskbelay/references/transport.md",
  "skills/taskbelay/references/verification.md",
  "skills/taskbelay/references/successes/taskbelay_abandon_task-abandon.md",
  "skills/taskbelay/references/successes/taskbelay_cancel_task-cancel.md",
  "skills/taskbelay/references/successes/taskbelay_get_next_action-guarded-read.md",
  "skills/taskbelay/references/successes/taskbelay_get_task-read.md",
  "skills/taskbelay/references/successes/taskbelay_open_task-create.md",
  "skills/taskbelay/references/successes/taskbelay_open_task-multiple.md",
  "skills/taskbelay/references/successes/taskbelay_open_task-resume.md",
  "skills/taskbelay/references/successes/taskbelay_prepare_task_branch_rename-prepare-branch-rename.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-rename-complete.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-rename-cancel.md",
  "skills/taskbelay/references/successes/taskbelay_get_task-history-page.md",
  "skills/taskbelay/references/successes/taskbelay_prepare_task_relocation-prepare.md",
  "skills/taskbelay/references/successes/taskbelay_recover_action-saved-operation.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-allow_once.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-expand_scope.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-history.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-reject.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-relocation.md",
  "skills/taskbelay/references/successes/taskbelay_resolve_blocker-verification-or-recovery.md",
  "skills/taskbelay/references/successes/taskbelay_server_info-handshake.md",
  "skills/taskbelay/references/successes/taskbelay_submit_comprehension-code_too_complex.md",
  "skills/taskbelay/references/successes/taskbelay_submit_comprehension-comprehension_passed.md",
  "skills/taskbelay/references/successes/taskbelay_submit_comprehension-design_too_complex.md",
  "skills/taskbelay/references/successes/taskbelay_submit_comprehension-evidence_insufficient.md",
  "skills/taskbelay/references/successes/taskbelay_submit_comprehension-implementation_defect.md",
  "skills/taskbelay/references/successes/taskbelay_submit_comprehension-requirement_unclear.md",
  "skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_complete.md",
  "skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_comprehension.md",
  "skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_design.md",
  "skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_implementation.md",
  "skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_requirements.md",
  "skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_test.md",
  "skills/taskbelay/references/successes/taskbelay_submit_design-design_ready.md",
  "skills/taskbelay/references/successes/taskbelay_submit_design-design_requires_requirements.md",
  "skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_needs_refactor.md",
  "skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_ready_for_test.md",
  "skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_requires_design.md",
  "skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_requires_requirements.md",
  "skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_ready_for_test.md",
  "skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_requires_design.md",
  "skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_requires_requirements.md",
  "skills/taskbelay/references/successes/taskbelay_submit_requirements-requirements_ready.md",
  "skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_plan_saved.md",
  "skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_ready.md",
  "skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_require_design.md",
  "skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_require_requirements.md",
  "skills/taskbelay/references/successes/taskbelay_submit_test-tests_accepted_with_known_failures.md",
  "skills/taskbelay/references/successes/taskbelay_submit_test-tests_expose_design_issue.md",
  "skills/taskbelay/references/successes/taskbelay_submit_test-tests_expose_requirement_issue.md",
  "skills/taskbelay/references/successes/taskbelay_submit_test-tests_failed_implementation.md",
  "skills/taskbelay/references/successes/taskbelay_submit_test-tests_passed.md",
  "skills/taskbelay/references/successes/taskbelay_submit_test-verification_budget_increased.md",
  "skills/taskbelay/references/successes/workspace-workspace_coordinator-cleanup_branch.md",
  "skills/taskbelay/references/successes/workspace-workspace_coordinator-cleanup_worktree.md",
  "skills/taskbelay/references/successes/workspace-workspace_coordinator-consume.md",
  "skills/taskbelay/references/successes/workspace-workspace_coordinator-prepare_cleanup.md",
  "skills/taskbelay/references/successes/workspace-workspace_coordinator-provision-local.md",
  "skills/taskbelay/references/successes/workspace-workspace_coordinator-provision-worktree.md",
  "skills/taskbelay/scripts/artifacts.mjs",
];
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

test("package README displays the public TaskBelay icon", async () => {
  const readme = await readFile(join(packageRoot, "README.md"), "utf8");
  assert.match(readme, new RegExp(`<img src="${packageIconUrl}" width="112" height="112" alt="TaskBelay" \\/>`));
});

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
  const helper = join(installedSkill, "scripts", "artifacts.mjs");
  const helperHelp = await execFile(process.execPath, [helper, "--help"], { encoding: "utf8" });
  assert.match(helperHelp.stdout, /collect\|prepare/u);


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
  const helperFailure = await runWithClosedInput(
    process.execPath, [helper, "collect"], { cwd: outputDirectory, env: environment },
    JSON.stringify({ host: "deepseek", task_id: "task-example", action_id: "action-example" }) + "\n", 1,
  );
  const envelope = JSON.parse(helperFailure.stdout);
  assert.equal(envelope.ok, false);
  assert.equal(envelope.error.code, "STORAGE_UNAVAILABLE");
  assert.equal(helperFailure.stderr, "");
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
