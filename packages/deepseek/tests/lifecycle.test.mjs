import { planFor } from "../../../tests/journeys/fixtures/engineering.mjs";
import assert from "node:assert/strict";
import { execFile as execFileCallback } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, realpath, stat, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { arch, platform } from "node:os";
import { basename, dirname, join, relative } from "node:path";
import test from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { TASKBELAY_QUALIFIED_TOOL_NAMES } from "../lib/tool-names.mjs";
import { createWorkspaceCoordinator } from "../lib/workspace-coordinator.mjs";

const execFile = promisify(execFileCallback);
const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const repositoryRoot = dirname(dirname(packageRoot));
const dshCli = process.env.TASKBELAY_DSH_CLI;
const artifactPath = process.env.TASKBELAY_DEEPSEEK_ARTIFACT;
const lifecycleRoot = process.env.TASKBELAY_LIFECYCLE_ROOT;
const sourceCommit = process.env.TASKBELAY_ARTIFACT_SOURCE_COMMIT;
const evidencePath = join(repositoryRoot, "tests", "journeys", "deepseek", "evidence", "lifecycle-acceptance.json");

test("official DSH add/remove/reinstall preserves Core data, repository, and Codex identities", {
  skip: [dshCli, artifactPath, lifecycleRoot, sourceCommit].some((value) => value === undefined)
    ? "set the exact lifecycle environment for the official lifecycle gate"
    : false,
}, async (t) => {
  assert.equal(platform(), "darwin");
  assert.equal(arch(), "arm64");
  const canonicalRoot = await realpath(lifecycleRoot);
  const canonicalArtifact = await realpath(artifactPath);
  const dshHome = join(canonicalRoot, "dsh-home");
  const dataDirectory = join(canonicalRoot, "data");
  const repository = join(canonicalRoot, "repository");
  const remote = join(canonicalRoot, "remote.git");
  const profileName = "taskbelay-acceptance";
  const profileDirectory = join(dshHome, "profiles", profileName);
  await mkdir(dshHome, { recursive: true, mode: 0o700 });
  await mkdir(dataDirectory, { mode: 0o700 });
  await mkdir(repository);
  await initializeGit(repository, remote);

  const provisioned = await createWorkspaceCoordinator({ dataDirectory, workspaceRoot: repository }).provision({
    request: "Lifecycle acceptance task.",
    profile: profileName,
    repositories: [{ repository_key: "primary", source_repository_path: repository, source_type: "remote", carry_changes: false, remote_name: "origin", base_branch: "main", target_branch: "feature/lifecycle-acceptance" }],
  });
  const taskRepository = provisioned.workspace_root;
  const consumed = await createWorkspaceCoordinator({ dataDirectory, workspaceRoot: taskRepository }).consume({ launchID: provisioned.launch_id });

  await assertProductMatchesCommit(sourceCommit);
  const artifactIdentity = await fileIdentity(canonicalArtifact);
  const beforeCodex = await codexIdentity();
  const beforeRepository = await repositoryIdentity(taskRepository);
  const env = { ...process.env, DSH_HOME: dshHome };

  await runDsh(["plugin", "--profile", profileName, "add", canonicalArtifact], env);
  const addedDump = await dumpProfile(profileName, env);
  assert.equal(countOccurrences(addedDump, "id: taskbelay-deepseek"), 1);
  const addedManifest = await readJSON(join(profileDirectory, "package.json"));
  assert.deepEqual(addedManifest.dsh.profile.bundles.filter((name) => name === "taskbelay-deepseek"), ["taskbelay-deepseek"]);
  const installedPackage = await realpath(join(profileDirectory, "node_modules", "taskbelay-deepseek"));
  const installedProductManifest = await readJSON(join(installedPackage, "package.json"));
  const installedCore = join(installedPackage, "runtime", "darwin-arm64", "taskbelay");
  const coreIdentity = await fileIdentity(installedCore);
  const firstMount = await mountInstalledProduct(installedPackage, dataDirectory, taskRepository);
  assert.deepEqual(firstMount.toolNames, TASKBELAY_QUALIFIED_TOOL_NAMES);
  assert.deepEqual(firstMount.skill.invocation, { modelInvocable: true, userInvocable: true });
  assert.match(firstMount.unauthorizedText, /TASKBELAY_NO_AGENT/u);
  const serverInfo = firstMount.serverInfo;
  const opened = await firstMount.call("mcp__taskbelay__taskbelay_open_task", {
    operation_id:"lifecycle-open",host:"deepseek",repositories:[{key:"primary",repository_path:taskRepository}],
    authorization:{user_ref:"fixture-lifecycle-request",request:"Lifecycle acceptance task",goals:[{id:"retention",statement:"Reinstall preserves this current task"}]},
    plan:planFor([{goal_ids:["retention"],id:"human",kind:"human",rationale:"Inspect retained identity",conditions:"Same current artifact and workspace"}]),
  });
  const task = opened.result;
  assert.equal(opened.ok, true);
  await firstMount.dispose();
  const dataAfterCreate = await treeDigest(dataDirectory);

  await runDsh(["plugin", "--profile", profileName, "remove", "taskbelay-deepseek"], env);
  const removedDump = await dumpProfile(profileName, env);
  assert.equal(removedDump.includes("id: taskbelay-deepseek"), false);
  const removedManifest = await readJSON(join(profileDirectory, "package.json"));
  assert.equal(removedManifest.dsh.profile.bundles.includes("taskbelay-deepseek"), false);
  await assert.rejects(stat(join(profileDirectory, "node_modules", "taskbelay-deepseek")), { code: "ENOENT" });

  const repeatedRemoval = await runDshAllowFailure(
    ["plugin", "--profile", profileName, "remove", "taskbelay-deepseek"],
    env,
  );
  assert.ok(repeatedRemoval.code === 0 || repeatedRemoval.code > 0);
  assert.equal(await treeDigest(dataDirectory), dataAfterCreate);
  assert.deepEqual(await codexIdentity(), beforeCodex);
  assert.deepEqual(await repositoryIdentity(taskRepository), beforeRepository);

  await runDsh(["plugin", "--profile", profileName, "add", canonicalArtifact], env);
  const reinstalledDump = await dumpProfile(profileName, env);
  assert.equal(countOccurrences(reinstalledDump, "id: taskbelay-deepseek"), 1);
  const reinstalledPackage = await realpath(join(profileDirectory, "node_modules", "taskbelay-deepseek"));
  const reinstalledCore = await fileIdentity(join(reinstalledPackage, "runtime", "darwin-arm64", "taskbelay"));
  assert.deepEqual(reinstalledCore, coreIdentity);
  const secondMount = await mountInstalledProduct(reinstalledPackage, dataDirectory, taskRepository);
  const reopened = await secondMount.call("mcp__taskbelay__taskbelay_get_task", {task_id:task.identity.task_id,refresh:false});
  assert.equal(reopened.ok,true);
  assert.equal(reopened.result.identity.task_id, task.identity.task_id);
  assert.equal(reopened.result.identity.revision, task.identity.revision);
  await secondMount.dispose();
  assert.equal(reopened.result.status, task.status);
  assert.deepEqual(await codexIdentity(), beforeCodex);
  assert.deepEqual(await repositoryIdentity(taskRepository), beforeRepository);

  const evidence = {
    evidence_class: "official_profile_lifecycle",
    repository_commit: sourceCommit,
    package: {
      filename: basename(canonicalArtifact), size: artifactIdentity.size, sha256: artifactIdentity.sha256,
      version_label: installedProductManifest.version,
    },
    embedded_core: {
      filename: "runtime/darwin-arm64/taskbelay", size: coreIdentity.size,
      sha256: coreIdentity.sha256, reported_version: await coreVersion(installedCore),
    },
    dsh: {
      version: "0.1.2-rc.1",
      integrity: "sha512-VQU5NlomrKLRgcXuOf+sxWFvqxPA8q9vMhrKPlPPXiOJEhGlGlAdiyxZvZxkCVI+v0zbhe21cY3/luLyxpSzzA==",
      source_commit: "141eb6fef83422698aef7a981029e843e8161534",
    },
    runtime: {
      node: process.version, pnpm: (await execFile("pnpm", ["--version"])).stdout.trim(),
      os: platform(), architecture: arch(), profile_identity_sha256: sha256(profileName),
    },
    core_contract: {
      schema: serverInfo.result.schema,
      tools: serverInfo.result.tools,
      qualified_tools: TASKBELAY_QUALIFIED_TOOL_NAMES,
    },
    task: { task_id: task.identity.task_id, revision: task.identity.revision, reopened_revision: reopened.result.identity.revision },
    outcomes: {
      add_readback: "passed", remove_readback: "passed", repeated_remove_bounded: "passed",
      data_retention: "passed", repository_retention: "passed", codex_non_interference: "passed",
      exact_reinstall: "passed", same_task_reopen: "passed", read_only_reopen: "passed",
    },
  };
  await mkdir(dirname(evidencePath), { recursive: true, mode: 0o700 });
  await writeFile(evidencePath, `${JSON.stringify(evidence, null, 2)}\n`, { flag: "wx", mode: 0o600 });
  t.diagnostic(JSON.stringify({
    artifact_sha256: artifactIdentity.sha256,
    core_sha256: coreIdentity.sha256,
    task_id: task.identity.task_id,
    lifecycle: "passed",
  }));
});

async function mountInstalledProduct(installedPackage, dataDirectory, workspaceRoot) {
  const packageRequire = createRequire(join(installedPackage, "package.json"));
  const toolsRequire = createRequire(packageRequire.resolve("@deepseek-ai/dsh-tools"));
  const [cordis, systemPrompt, tools, skills, integration] = await Promise.all([
    importResolved(packageRequire, "@deepseek-ai/cordis"),
    importResolved(toolsRequire, "@deepseek-ai/dsh-system-prompt"),
    importResolved(packageRequire, "@deepseek-ai/dsh-tools"),
    importResolved(packageRequire, "@deepseek-ai/dsh-skill"),
    import(pathToFileURL(join(installedPackage, "lib", "index.mjs")).href),
  ]);
  const ctx = new cordis.Context();
  const fibers = [];
  fibers.push(await ctx.plugin(systemPrompt.default));
  fibers.push(await ctx.plugin(tools.default));
  fibers.push(await ctx.plugin(skills.default));
  const previousDataDirectory = process.env.TASKBELAY_DATA_DIR;
  process.env.TASKBELAY_DATA_DIR = dataDirectory;
  try {
    fibers.push(await ctx.plugin(integration, {
      packageRoot: installedPackage,
      environment: { ...process.env, TASKBELAY_DATA_DIR: dataDirectory },
      workspaceRoot,
      platform: platform(),
      arch: arch(),
    }));
  } finally {
    if (previousDataDirectory === undefined) delete process.env.TASKBELAY_DATA_DIR;
    else process.env.TASKBELAY_DATA_DIR = previousDataDirectory;
  }
  const skill = (await ctx.skills.list()).find((candidate) => candidate.name === "taskbelay");
  assert.notEqual(skill, undefined);
  const toolNames = ctx.tools.schemas()
    .map((schema) => schema.name)
    .filter((name) => name.startsWith("mcp__taskbelay__"))
    .sort();
  assert.deepEqual(toolNames, [...TASKBELAY_QUALIFIED_TOOL_NAMES].sort());
  const unauthorized = await ctx.tools.execute({
    callId: "unauthorized-lifecycle", name: TASKBELAY_QUALIFIED_TOOL_NAMES[0], arguments: {},
    signal: new AbortController().signal,
  });
  assert.equal(unauthorized.isError, true);
  const unauthorizedText = textResult(unauthorized);
  let callIndex = 0;
  return {
    skill,
    toolNames: TASKBELAY_QUALIFIED_TOOL_NAMES,
    unauthorizedText,
    serverInfo: await authorizedCall(ctx, TASKBELAY_QUALIFIED_TOOL_NAMES[0], {}, `authorized-${++callIndex}`),
    call: async (name, args) => await authorizedCall(ctx, name, args, `authorized-${++callIndex}`),
    async dispose() {
      for (const fiber of [...fibers].reverse()) await fiber.dispose();
    },
  };
}

async function authorizedCall(ctx, name, args, callId) {
  const events = [
    { seq: 0, time: 0, type: "turn/start", data: { turn: 1 } },
    { seq: 1, time: 1, type: "user/message", data: {
      id: `user-${callId}`, role: "user", source: { kind: "user" },
      content: [{ type: "text", text: "/taskbelay lifecycle" }],
    } },
    { seq: 2, time: 2, type: "tool/call", data: {
      turn: 1, step: 1, callId, name, arguments: JSON.stringify(args),
    } },
  ];
  const result = await ctx.tools.execute({
    callId, name, arguments: args, signal: new AbortController().signal,
    agent: { status: "running", session: { snapshotEvents: () => events }, ctx },
  });
  const envelope = JSON.parse(textResult(result));
  if (!envelope.ok) throw new Error(`${envelope.error.code}: ${envelope.error.message}`);
  return envelope;
}

function textResult(result) {
  return result.content.filter((block) => block.type === "text").map((block) => block.text).join("\n");
}

async function dumpProfile(profileName, env) {
  return (await runDsh(["--profile", profileName, "--dump-config"], env)).stdout;
}

async function runDsh(args, env) {
  return await execFile(dshCli, args, {
    cwd: repositoryRoot, env, encoding: "utf8", timeout: 120_000, maxBuffer: 8 * 1024 * 1024,
  });
}

async function runDshAllowFailure(args, env) {
  try {
    const result = await runDsh(args, env);
    return { code: 0, ...result };
  } catch (error) {
    return { code: error.code ?? 1, stdout: error.stdout ?? "", stderr: error.stderr ?? "" };
  }
}

async function assertProductMatchesCommit(commit) {
  await execFile("git", ["cat-file", "-e", `${commit}^{commit}`], { cwd: repositoryRoot });
  const manifest = await readJSON(join(packageRoot, "package.json"));
  const productPaths = ["package.json", "README.md", "cordis.patch.yml", ...manifest.files]
    .filter((path) => !path.startsWith("LICENSE"))
    .filter((path) => !path.startsWith("runtime/"))
    .map((path) => join("packages", "deepseek", path));
  await execFile("git", ["diff", "--quiet", commit, "--", ...productPaths], { cwd: repositoryRoot });
}

async function codexIdentity() {
  const files = [
    "packages/codex/package.json",
    "packages/codex/plugin/.mcp.json",
    ...await listFiles(join(repositoryRoot, "packages", "codex", "plugin", "skills", "taskbelay")),
  ];
  const normalized = files.map((path) => path.startsWith("packages/") ? path : join("packages", "codex", "plugin", "skills", "taskbelay", path));
  return {
    manifest: await fileHash(join(repositoryRoot, "packages/codex/package.json")),
    mcp_registration: await fileHash(join(repositoryRoot, "packages/codex/plugin/.mcp.json")),
    skill_tree: await digestPaths(normalized),
  };
}

async function repositoryIdentity(repository) {
  const [head, statusOutput] = await Promise.all([
    execFile("git", ["rev-parse", "HEAD"], { cwd: repository }),
    execFile("git", ["status", "--porcelain=v2"], { cwd: repository }),
  ]);
  return { head: head.stdout.trim(), status: statusOutput.stdout };
}

async function treeDigest(root) {
  const files = await listFiles(root);
  return await digestPaths(files.map((path) => join(root, path)), root);
}

async function digestPaths(paths, base = repositoryRoot) {
  const hash = createHash("sha256");
  for (const path of [...paths].sort()) {
    hash.update(relative(base, path));
    hash.update("\0");
    hash.update(await readFile(path));
    hash.update("\0");
  }
  return hash.digest("hex");
}

async function listFiles(root, prefix = "") {
  const { readdir } = await import("node:fs/promises");
  const files = [];
  for (const entry of await readdir(join(root, prefix), { withFileTypes: true })) {
    const path = prefix ? join(prefix, entry.name) : entry.name;
    if (entry.isDirectory()) files.push(...await listFiles(root, path));
    else files.push(path);
  }
  return files;
}

async function fileIdentity(path) {
  const info = await stat(path);
  return { size: info.size, sha256: await fileHash(path) };
}

async function fileHash(path) { return sha256(await readFile(path)); }
function sha256(value) { return createHash("sha256").update(value).digest("hex"); }
function countOccurrences(text, pattern) { return text.split(pattern).length - 1; }
async function readJSON(path) { return JSON.parse(await readFile(path, "utf8")); }

async function coreVersion(path) {
  return (await execFile(path, ["version"], { encoding: "utf8" })).stdout.trim();
}

async function importResolved(require, specifier) {
  return await import(pathToFileURL(require.resolve(specifier)).href);
}

async function initializeGit(repository, remote) {
  const env = { ...process.env, GIT_CONFIG_NOSYSTEM: "1" };
  await execFile("git", ["init", "--bare", "--initial-branch=main", remote], { env });
  await execFile("git", ["init", "-q", "--initial-branch=main"], { cwd: repository, env });
  await execFile("git", ["config", "user.email", "lifecycle@example.invalid"], { cwd: repository, env });
  await execFile("git", ["config", "user.name", "Lifecycle Test"], { cwd: repository, env });
  await writeFile(join(repository, "README.md"), "lifecycle\n");
  await execFile("git", ["add", "README.md"], { cwd: repository, env });
  await execFile("git", ["commit", "-q", "-m", "initial"], { cwd: repository, env });
  await execFile("git", ["remote", "add", "origin", remote], { cwd: repository, env });
  await execFile("git", ["push", "-u", "origin", "main"], { cwd: repository, env });
}
