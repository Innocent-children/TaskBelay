import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, mkdir, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { bindTask, cleanup, inspect, prepare, provision, relocate, scope, session, status as launchStatus } from "../lib/workspace.mjs";
import { paths } from "../lib/runtime.mjs";
import { defaultRunGit as git } from "../lib/worktree-lifecycle.mjs";
import { examples } from "../../../tests/skills/resources.mjs";
async function fixture(t) {
  const parent = await realpath(await mkdtemp(join(tmpdir(), "zcode-workspace-")));
  const root = join(parent, "repo"); await mkdir(root);
  t.after(() => rm(parent, { recursive: true, force: true }));
  await git(["init", "-b", "main", root]); await git(["-C", root, "config", "user.name", "Test"]); await git(["-C", root, "config", "user.email", "test@example.invalid"]);
  await writeFile(join(root, "base.txt"), "base"); await git(["-C", root, "add", "."]); await git(["-C", root, "commit", "-m", "base"]);
  const options = { environment: { ...process.env, HOME: parent, USERPROFILE: parent, LOCALAPPDATA: join(parent, "appdata"), TASKBELAY_DATA_DIR: "" }, checkWorkspaceAvailable: async repository_path => ({ available: true, repository_path }) };
  return { root, options };
}

function example(markdown, kind, operation, name) {
  const found = examples(markdown, kind).find(entry => entry.operation === operation && entry.name === name);
  assert.ok(found, `${kind} ${operation} ${name}`);
  return found.value;
}

function shape(value) {
  if (value === null) return null;
  if (Array.isArray(value)) return value.map(shape);
  if (typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, entry]) => [key, shape(entry)]));
  return typeof value;
}

test("documented ZCode Host calls match the CLI and workspace result shapes", async t => {
  const { root, options } = await fixture(t);
  const admission = await readFile(new URL("../skills/taskbelay/references/admission.md", import.meta.url), "utf8");
  const lifecycle = await readFile(new URL("../skills/taskbelay/references/host-lifecycle.md", import.meta.url), "utf8");

  const inspectInput = structuredClone(example(admission, "host-launch", "inspect", "request"));
  inspectInput.repositories[0].repository_path = root;
  const command = spawnSync(process.execPath, [fileURLToPath(new URL("../bin/taskbelay-zcode.mjs", import.meta.url)), "host-launch", "inspect"], { input: JSON.stringify(inspectInput), encoding: "utf8" });
  assert.equal(command.status, 0, command.stderr);
  const anchor = JSON.parse(command.stdout);
  assert.deepEqual(shape(anchor), shape(example(admission, "host-launch-output", "inspect", "success")));
  assert.deepEqual(anchor, await inspect(inspectInput));

  const prepareInput = structuredClone(example(admission, "host-launch", "prepare", "request"));
  prepareInput.assessment.anchor = anchor;
  prepareInput.repositories[0].repository_path = root;
  prepareInput.repositories[0].worktree_path = root;
  const rejectedInput = structuredClone(prepareInput);
  rejectedInput.assessment.unknowns = ["Unresolved endpoint behavior"];
  const rejected = spawnSync(process.execPath, [fileURLToPath(new URL("../bin/taskbelay-zcode.mjs", import.meta.url)), "host-launch", "prepare"], { input: JSON.stringify(rejectedInput), env: options.environment, encoding: "utf8" });
  assert.equal(rejected.status, 1);
  assert.equal(rejected.stdout, "");
  assert.match(rejected.stderr, /Complete resolved assessment required/u);
  const receipt = await prepare(prepareInput, options);
  assert.deepEqual(shape(receipt), shape(example(admission, "host-launch-output", "prepare", "success")));
  assert.deepEqual(receipt.user_choice, prepareInput.user_choice);
  assert.equal(receipt.repositories[0].phase, "prepared");

  await provision(receipt.launch_id, options);
  const scoped = await scope(receipt.launch_id, options);
  assert.deepEqual(shape(scoped), shape(example(admission, "host-launch-output", "scope", "success")));
  const statusInput = structuredClone(example(lifecycle, "host-launch", "status", "request"));
  assert.deepEqual(Object.keys(statusInput), ["launch_id"]);
  statusInput.launch_id = receipt.launch_id;
  assert.equal((await launchStatus(statusInput.launch_id, options)).repositories[0].phase, "provisioned");

  const opened = await session(receipt.launch_id, "open", options);
  const documentedOpen = example(admission, "host-launch-output", "open", "success");
  assert.deepEqual(shape(opened), shape(documentedOpen));
  assert.deepEqual(opened.next_steps, documentedOpen.next_steps);
  assert.equal(opened.prompt, documentedOpen.prompt.replaceAll(documentedOpen.receipt_path, opened.receipt_path).replaceAll(documentedOpen.launch_id, receipt.launch_id));
  assert.equal(opened.status, "action_required");
  assert.equal(opened.task_id, null);
});

test("local branch prepares once and UI guidance preserves the Core identity", async t => {
  const { root, options } = await fixture(t);
  const receipt = await prepareSelections([{ ...dedicated(root), workspace_mode: "new_branch", worktree_path: root }], options);
  const ready = await provision(receipt.launch_id, options);
  assert.equal(ready.repositories[0].phase, "provisioned");
  assert.equal((await scope(receipt.launch_id, options)).workspace_origin.task_branch, "task");
  await provision(receipt.launch_id, options);
  const before = await launchStatus(receipt.launch_id, options);
  const opened = await session(receipt.launch_id, "open", options);
  assert.equal(opened.status, "action_required");
  assert.deepEqual(opened.workspace_paths, [root]);
  assert.equal(Object.hasOwn(opened, "executable"), false);
  assert.equal(Object.hasOwn(opened, "session_id"), false);
  assert.equal(opened.task_id, null);
  assert.match(opened.prompt, /unbound launch receipt does not prove/);
  assert.deepEqual(await launchStatus(receipt.launch_id, options), before);
  assert.deepEqual(await session(receipt.launch_id, "open", options), opened);
  await bindTask(receipt.launch_id, { task_id: "task-from-successful-core-response" }, options);
  const resumed = await session(receipt.launch_id, "resume", options);
  assert.equal(resumed.task_id, "task-from-successful-core-response");
  assert.match(resumed.prompt, /task-from-successful-core-response/);
  assert.equal(resumed.receipt_path, opened.receipt_path);
  await assert.rejects(bindTask(receipt.launch_id, { task_id: "task-other" }, options), /another Core Task/);
  await assert.rejects(session(receipt.launch_id, "launch", options), /UI guidance/);
});

test("unresolved assessment cannot write Git", async t => {
  const { root, options } = await fixture(t);
  await assert.rejects(prepare({ request: "x", assessment: { unknowns: ["target"] }, user_choice: { source: "user", mode: "taskbelay", summary: "yes" }, repositories: [], handoff: null }, options), /resolved assessment/);
  assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
});

test("uppercase repository keys are rejected before preparation changes Git", async t => {
  const { root, options } = await fixture(t);
  const before = await git(["-C", root, "show-ref"]);
  await assert.rejects(prepare({ request: "Keep repository keys consistent with Core", assessment: {
    change_level: "standard", candidate_components: ["workspace"], candidate_paths: ["base.txt"],
    public_contract_flags: [], persistence_or_state_flags: [], host_or_platform_flags: ["ZCode"],
    verification_shape: ["workspace checks"], reasons: ["Core repository identity"], unknowns: [],
  }, user_choice: { source: "user", mode: "taskbelay", summary: "Confirmed" },
  repositories: [dedicated(root, "Backend")], handoff: null }, options), /Repository key/);
  assert.equal(await git(["-C", root, "show-ref"]), before);
  assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
});

async function prepareSelections(repositories, options) {
  const request = "Complete a multi-repository change";
  const anchor = await inspect({ request, repositories: repositories.map(({ key, repository_path }) => ({ key, repository_path })) });
  return prepare({ request, assessment: { change_level: "standard", candidate_components: ["workspace"], candidate_paths: ["base.txt"], public_contract_flags: [], persistence_or_state_flags: [], host_or_platform_flags: ["ZCode"], verification_shape: ["workspace checks"], reasons: ["multiple repositories"], unknowns: [], anchor },
    user_choice: { source: "user", mode: "taskbelay", summary: "Confirmed exact workspaces and carry choices" }, repositories,
    handoff: { discussion: [{ role: "user", text: request }] } }, options);
}
function dedicated(root, key = "primary", carry = false) {
  return { key, repository_path: root, workspace_mode: "dedicated_worktree", source_type: "local", remote_name: "", base_branch: "main", target_branch: "task", carry_changes: carry, worktree_path: join(root, "..", "worktree") };
}
test("dedicated worktree can exclude changes from an occupied source directory", async t => {
  for (const sourceType of ["local", "remote"]) await t.test(sourceType, async t => {
    const { root, options } = await fixture(t);
    const selection = { ...dedicated(root), source_type: sourceType };
    if (sourceType === "remote") {
      const remote = await fixture(t);
      await writeFile(join(remote.root, "base.txt"), "remote base");
      await git(["-C", remote.root, "commit", "-am", "remote base"]);
      await git(["-C", root, "remote", "add", "origin", remote.root]);
      selection.remote_name = "origin";
    }
    await writeFile(join(root, "base.txt"), "staged"); await git(["-C", root, "add", "base.txt"]);
    await writeFile(join(root, "base.txt"), "working"); await writeFile(join(root, "new.txt"), "untracked");
    const before = await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]);
    const index = await git(["-C", root, "write-tree"]);
    const head = await git(["-C", root, "rev-parse", "HEAD"]);
    const checks = [];
    const occupied = { ...options, checkWorkspaceAvailable: async repository_path => {
      checks.push(repository_path);
      return { available: false, repository_path, task_id: "existing-source-task" };
    } };
    const receipt = await prepareSelections([selection], occupied);
    assert.equal(receipt.repositories[0].snapshot, null);
    await provision(receipt.launch_id, occupied);
    const bound = await scope(receipt.launch_id, occupied);
    assert.equal(bound.repository_path, selection.worktree_path);
    assert.equal(bound.workspace_origin.source_type, sourceType);
    assert.equal(bound.workspace_origin.carry_changes, false);
    assert.equal(await readFile(join(selection.worktree_path, "base.txt"), "utf8"), sourceType === "remote" ? "remote base" : "base");
    assert.equal(await git(["-C", selection.worktree_path, "status", "--porcelain=v2"]), "");
    await assert.rejects(stat(join(selection.worktree_path, "new.txt")), { code: "ENOENT" });
    assert.deepEqual(checks, []);
    assert.equal(await readFile(join(root, "base.txt"), "utf8"), "working");
    assert.equal(await readFile(join(root, "new.txt"), "utf8"), "untracked");
    assert.equal(await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]), before);
    assert.equal(await git(["-C", root, "write-tree"]), index);
    assert.equal(await git(["-C", root, "rev-parse", "HEAD"]), head);
    assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
  });
});
test("local workspace modes retain initial-change acceptance and source occupancy checks", async t => {
  for (const mode of ["new_branch", "current_branch"]) await t.test(mode, async t => {
    const { root, options } = await fixture(t);
    const selection = { ...dedicated(root), workspace_mode: mode, worktree_path: root,
      target_branch: mode === "current_branch" ? "main" : "task" };
    await writeFile(join(root, "base.txt"), "initial work");
    await assert.rejects(prepareSelections([selection], options), /initial local changes require explicit acceptance/);
    selection.carry_changes = true;
    const checks = [];
    let available = false;
    const checked = { ...options, checkWorkspaceAvailable: async repository_path => {
      checks.push(repository_path);
      return { available, repository_path };
    } };
    await assert.rejects(prepareSelections([selection], checked), /claimed/);
    available = true;
    const receipt = await prepareSelections([selection], checked);
    available = false;
    await assert.rejects(provision(receipt.launch_id, checked), /claimed/);
    assert.equal((await launchStatus(receipt.launch_id, options)).repositories[0].phase, "prepared");
    assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
    assert.equal(await git(["-C", root, "branch", "--list", "task"]), "");
    available = true;
    await provision(receipt.launch_id, checked);
    assert.deepEqual(checks, [root, root, root, root]);
    assert.equal((await scope(receipt.launch_id, options)).workspace_origin.mode, mode);
    assert.equal(await readFile(join(root, "base.txt"), "utf8"), "initial work");
  });
});
test("dedicated worktree preserves staged, unstaged and untracked content with its source intact", async t => {
  const { root, options } = await fixture(t);
  await writeFile(join(root, "base.txt"), "staged"); await git(["-C", root, "add", "base.txt"]);
  await writeFile(join(root, "base.txt"), "working"); await writeFile(join(root, "new.txt"), "untracked");
  const before = await git(["-C", root, "status", "--porcelain=v2"]);
  const selection = dedicated(root, "primary", true);
  const receipt = await prepareSelections([selection], options); await provision(receipt.launch_id, options);
  assert.equal(await readFile(join(selection.worktree_path, "base.txt"), "utf8"), "working");
  assert.equal(String(await git(["-C", selection.worktree_path, "show", ":base.txt"])), "staged");
  assert.equal(await readFile(join(selection.worktree_path, "new.txt"), "utf8"), "untracked");
  assert.equal(await git(["-C", root, "status", "--porcelain=v2"]), before);
  assert.equal((await scope(receipt.launch_id, options)).workspace_origin.carry_changes, true);
});
test("snapshot rejects changed contents even when Git status is unchanged", async t => {
  const { root, options } = await fixture(t);
  await writeFile(join(root, "base.txt"), "staged\n"); await git(["-C", root, "add", "base.txt"]);
  await writeFile(join(root, "base.txt"), "tracked-A\n"); await writeFile(join(root, "new.txt"), "untracked-A\n");
  const before = await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]);
  const index = await git(["-C", root, "write-tree"]);
  const head = await git(["-C", root, "rev-parse", "HEAD"]);
  const stash = await git(["-C", root, "stash", "list"]);
  let captures = 0;
  const runGit = async (args, opts) => {
    const output = await git(args, opts);
    if (args.includes("stash") && args.includes("create") && ++captures === 1) {
      await writeFile(join(root, "base.txt"), "tracked-B\n");
      await writeFile(join(root, "new.txt"), "untracked-B\n");
      assert.equal(await git(["-C", root, "status", "--porcelain=v2", "--untracked-files=all"]), before);
    }
    return output;
  };
  const selection = dedicated(root, "primary", true);
  let launchID, receiptPath;
  await assert.rejects(prepareSelections([selection], { ...options, runGit }), error => {
    assert.match(error.message, /Source workspace changed while capturing/);
    assert.equal(typeof error.launch_id, "string");
    assert.equal(typeof error.receipt_path, "string");
    launchID = error.launch_id; receiptPath = error.receipt_path;
    return true;
  });
  assert.equal(captures, 2);
  assert.equal(receiptPath, join((await paths(options.environment)).productRoot, "provisioning", "zcode", launchID, "receipt.json"));
  const receipt = await launchStatus(launchID, options);
  assert.equal(receipt.repositories[0].phase, "uncertain");
  assert.equal(receipt.repositories[0].snapshot, null);
  await assert.rejects(stat(selection.worktree_path), { code: "ENOENT" });
  assert.equal(await git(["-C", root, "branch", "--list", selection.target_branch]), "");
  assert.equal(await readFile(join(root, "base.txt"), "utf8"), "tracked-B\n");
  assert.equal(await readFile(join(root, "new.txt"), "utf8"), "untracked-B\n");
  assert.equal(await git(["-C", root, "write-tree"]), index);
  assert.equal(await git(["-C", root, "rev-parse", "HEAD"]), head);
  assert.equal(await git(["-C", root, "stash", "list"]), stash);
});

test("all repositories are required and replacement worktree instances are rejected", async t => {
  const one = await fixture(t), two = await fixture(t);
  const selected = [dedicated(one.root), dedicated(two.root, "secondary")];
  const receipt = await prepareSelections(selected, one.options); await provision(receipt.launch_id, one.options);
  const bound = await scope(receipt.launch_id, one.options);
  assert.equal(bound.additional_repositories[0].key, "secondary");
  await git(["-C", one.root, "worktree", "remove", selected[0].worktree_path]);
  await git(["-C", one.root, "worktree", "add", selected[0].worktree_path, "task"]);
  await assert.rejects(scope(receipt.launch_id, one.options), /identity/);
});
test("a partial provisioning failure retains the first workspace and cannot expose a partial Core scope", async t => {
  const one = await fixture(t), two = await fixture(t);
  const selected = [dedicated(one.root), dedicated(two.root, "secondary")];
  const receipt = await prepareSelections(selected, one.options);
  const failSecond = { ...one.options, runGit: async (args, opts) => {
    if (args.includes(two.root) && args.includes("worktree") && args.includes("add")) throw new Error("Injected second-workspace failure");
    return git(args, opts);
  } };
  await assert.rejects(provision(receipt.launch_id, failSecond), /second-workspace/);
  assert.equal(await readFile(join(selected[0].worktree_path, "base.txt"), "utf8"), "base");
  await assert.rejects(scope(receipt.launch_id, one.options));
  await assert.rejects(provision(receipt.launch_id, one.options), /uncertain/);
});

test("current-branch and remote-base modes preserve their confirmed origins", async t => {
  const local = await fixture(t);
  const current = { ...dedicated(local.root), workspace_mode: "current_branch", target_branch: "main", worktree_path: local.root };
  const localReceipt = await prepareSelections([current], local.options);
  await provision(localReceipt.launch_id, local.options);
  assert.equal((await scope(localReceipt.launch_id, local.options)).workspace_origin.mode, "current_branch");
  const source = await fixture(t), remote = await fixture(t);
  await writeFile(join(remote.root, "remote.txt"), "remote-only");
  await git(["-C", remote.root, "add", "remote.txt"]); await git(["-C", remote.root, "commit", "-m", "remote content"]);
  await git(["-C", source.root, "remote", "add", "origin", remote.root]);
  const selection = { ...dedicated(source.root), source_type: "remote", remote_name: "origin" };
  const receipt = await prepareSelections([selection], source.options); await provision(receipt.launch_id, source.options);
  assert.equal(await readFile(join(selection.worktree_path, "remote.txt"), "utf8"), "remote-only");
  assert.equal((await scope(receipt.launch_id, source.options)).workspace_origin.source_type, "remote");
});

test("relocation and terminal cleanup retain Core authority and separate deletion decisions", async t => {
  const { root, options } = await fixture(t), selection = dedicated(root);
  const receipt = await prepareSelections([selection], options); await provision(receipt.launch_id, options);
  await bindTask(receipt.launch_id, { task_id: "task-test" }, options);
  await assert.rejects(cleanup(receipt.launch_id, "cleanup-worktree", { repository_key: "primary", terminal: false, authorized: true, core_task:null }, options), /Terminal/);
  const destination = join(root, "..", "relocated");
  const savedScope=await scope(receipt.launch_id,options);
  // Fixture Core facts; real Core interoperability is exercised in workspace-core.test.mjs.
  const core_task={task_id:"task-test",origin_host:"zcode",revision:3,current_cursor:"DONE",primary_repository_key:"primary",workspace_origin:{...savedScope.workspace_origin,canonical_worktree_root:selection.worktree_path},
    repository:{current_branch:"task",current_head:(await git(["-C",selection.worktree_path,"rev-parse","HEAD"])).trim(),detached:false,worktree_instance_digest:"a".repeat(64),binding_digest:"b".repeat(64)}};
  const core_preparation = { relocation_id: "relocation-confirmed-by-core", task: { task_id: "task-test", origin_host: "zcode", revision: 2,
    current_cursor: "BLOCKED", relocation: { relocation_id: "relocation-confirmed-by-core" },
    blocker: { cause: "task_relocation_pending", condition: { kind: "resolve_task_relocation", relocation_id: "relocation-confirmed-by-core" } },
    primary_repository_key: "primary", workspace_origin: core_task.workspace_origin, repository:core_task.repository } };
  const input = { relocation_id: core_preparation.relocation_id, core_preparation, destinations: [{ repository_key: "primary", repository_path: destination }], authorized: true };
  const moved = await relocate(receipt.launch_id, input, options);
  assert.equal(moved.core_resolution_required, true);
  assert.deepEqual(await relocate(receipt.launch_id, input, options), moved);
  await assert.rejects(relocate(receipt.launch_id, { ...input, destinations: [{ repository_key: "primary", repository_path: destination + "-other" }] }, options), /original Core preparation and destinations/);
  assert.equal((await scope(receipt.launch_id, options)).repository_path, destination);
  core_task.workspace_origin={...core_task.workspace_origin,canonical_worktree_root:destination};
  await cleanup(receipt.launch_id, "cleanup-worktree", { repository_key: "primary", terminal: true, authorized: true, core_task }, options);
  await cleanup(receipt.launch_id, "cleanup-worktree", { repository_key: "primary", terminal: true, authorized: true, core_task }, options);
  await assert.rejects(cleanup(receipt.launch_id, "cleanup-branch", { repository_key: "primary", terminal: true, authorized: false, core_task }, options), /authorization/);
  await cleanup(receipt.launch_id, "cleanup-branch", { repository_key: "primary", terminal: true, authorized: true, core_task }, options);
  await assert.rejects(git(["-C", root, "rev-parse", "--verify", "refs/heads/task"]));
});

test("concurrent callers never reclaim a stale operation lock", async t => {
  const { root, options } = await fixture(t);
  const receipt = await prepareSelections([{ ...dedicated(root), workspace_mode: "new_branch", worktree_path: root }], options);
  const lockPath = join((await paths(options.environment)).productRoot, "provisioning", "zcode", receipt.launch_id, "operation.lock");
  const retained = JSON.stringify({ pid: 2147483647, token: "stopped-owner" });
  await writeFile(lockPath, retained);
  const results = await Promise.allSettled([provision(receipt.launch_id, options), provision(receipt.launch_id, options)]);
  for (const result of results) {
    assert.equal(result.status, "rejected");
    assert.match(result.reason.message, /lock remains/);
  }
  assert.equal(await readFile(lockPath, "utf8"), retained);
  assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
  assert.equal((await launchStatus(receipt.launch_id, options)).repositories[0].phase, "prepared");
});

test("a replaced operation lock is retained and cannot authorize receipt writes", async t => {
  const { root, options } = await fixture(t);
  const receipt = await prepareSelections([{ ...dedicated(root), workspace_mode: "new_branch", worktree_path: root }], options);
  const lockPath = join((await paths(options.environment)).productRoot, "provisioning", "zcode", receipt.launch_id, "operation.lock");
  const replacement = JSON.stringify({ pid: process.pid, token: "replacement-owner" });
  let changed = false;
  const runGit = async (args, opts) => {
    if (!changed) {
      changed = true;
      await writeFile(lockPath, replacement);
      throw new Error("The operation lost its lock");
    }
    return git(args, opts);
  };
  await assert.rejects(provision(receipt.launch_id, { ...options, runGit }), /lock changed/);
  assert.equal(await readFile(lockPath, "utf8"), replacement);
  assert.equal((await launchStatus(receipt.launch_id, options)).repositories[0].phase, "provisioning");
  assert.equal(String(await git(["-C", root, "branch", "--show-current"])).trim(), "main");
});

test("prepare CLI exposes its saved launch identity after a failed remote fetch", async t => {
  const { root, options } = await fixture(t);
  const remote = join(root, "..", "empty-remote");
  await git(["init", "--bare", remote]);
  await git(["-C", root, "remote", "add", "origin", remote]);
  const request = "Prepare from the explicitly selected remote base";
  const anchor = await inspect({ request, repositories: [{ key: "primary", repository_path: root }] });
  const input = { request, assessment: { change_level: "standard", candidate_components: ["workspace"], candidate_paths: ["base.txt"],
    public_contract_flags: [], persistence_or_state_flags: [], host_or_platform_flags: ["ZCode"], verification_shape: ["workspace checks"],
    reasons: ["remote source"], unknowns: [], anchor },
    user_choice: { source: "user", mode: "taskbelay", summary: "Confirmed remote base and workspace" },
    repositories: [{ ...dedicated(root), source_type: "remote", remote_name: "origin" }],
    handoff: { discussion: [{ role: "user", text: request }] } };
  const result = spawnSync(process.execPath, [fileURLToPath(new URL("../bin/taskbelay-zcode.mjs", import.meta.url)), "host-launch", "prepare"], {
    input: JSON.stringify(input), env: options.environment, encoding: "utf8", windowsHide: true, timeout: 15000,
  });
  assert.equal(result.error, undefined);
  assert.equal(result.status, 1);
  assert.equal(result.stdout, "");
  const launchID = /Saved launch_id: ([a-z0-9-]+)/.exec(result.stderr)?.[1];
  assert.ok(launchID, result.stderr);
  const receipt = await launchStatus(launchID, options);
  assert.equal(receipt.repositories[0].phase, "uncertain");
  assert.ok(result.stderr.includes(join((await paths(options.environment)).productRoot, "provisioning", "zcode", launchID, "receipt.json")));
  assert.equal(await git(["-C", root, "branch", "--list", "task"]), "");
});
