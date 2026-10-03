import { admissionFixture, receiptAdmissionFixture } from "./fixtures/task-handoff.mjs";
import assert from "node:assert/strict";
import { execFile as execFileCallback } from "node:child_process";
import { mkdir, mkdtemp, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import test from "node:test";
import { runCLI } from "../bin/taskbelay-codex.mjs";

import { handoffFixture, writeHandoffFixture } from "./fixtures/task-handoff.mjs";
import { readTaskHandoff, taskHandoffPaths } from "../lib/task-handoff.mjs";
import { inspectAdmissionAnchor } from "../lib/task-admission.mjs";
import {
  beginManagedTaskDispatch,
  claimManagedTaskDispatch,
  recoverUncalledManagedTaskDispatch,
  reconcileManagedTaskDispatch,
  beginTaskHandoff,
  bootstrapManagedTask,
  buildCliRelaunchDescriptor,
  buildOpenTaskRepositoryScope,
  cleanupCliTaskWorktree,
  cleanupTaskBranch,
  prepareTaskLaunch,
  readOpenTaskRepositoryScope,
  provisionCliTask,
  provisionLocalTask,
  recordManagedTaskDispatch,
  recordTaskHandoff,
  recordTaskHandoffStatus,
  validateWorkspaceOrigin,
} from "../lib/task-launch.mjs";
import { createProvisioningReceipt as rawCreateProvisioningReceipt, provisioningReceiptPath, readProvisioningReceipt, updateProvisioningReceipt, writeProvisioningReceiptAtomic } from "../lib/provisioning-receipt.mjs";
import { defaultRunGit, terminalCleanupDecision } from "../lib/worktree-lifecycle.mjs";

const execFile = promisify(execFileCallback);

test("scope reads one confirmed launch and refuses missing, pending or mixed-request records", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-scope-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const options = { productSupportRoot: root };
  const input = { launch_id: "launch-scope-test", repository_keys: ["web", "api"], primary_repository_key: "api" };
  const records = [];
  for (const key of input.repository_keys) {
    let receipt = createProvisioningReceipt({
      launchId: input.launch_id, repositoryKey: key, requestDigest: "a".repeat(64), handoffDigest: "b".repeat(64),
      sourceRepositoryIdentity: "c".repeat(64), sourceType: "remote", carryChanges: false, remoteName: "origin", baseBranch: "main", targetBranch: `codex/${key}`,
      worktreePath: join(root, key), surface: "cli_worktree",
    });
    receipt = updateProvisioningReceipt(receipt, { phase: "resolving", values: {} });
    receipt = updateProvisioningReceipt(receipt, { phase: "prepared", values: { base_commit: "d".repeat(40) } });
    receipt = updateProvisioningReceipt(receipt, { phase: "provisioning", values: {} });
    receipt = updateProvisioningReceipt(receipt, { phase: "provisioned", values: {} });
    await writeProvisioningReceiptAtomic(provisioningReceiptPath(root, input.launch_id, key), receipt, options);
    records.push(receipt);
  }
  const scope = await readOpenTaskRepositoryScope(input, options);
  assert.equal(scope.repository_path, join(root, "api"));
  assert.equal(scope.primary_repository_key, "api");
  assert.deepEqual(scope.additional_repositories.map((entry) => entry.key), ["web"]);
  let output = "";
  const command = await runCLI(["host-launch", "scope"], {
    resolvePaths: () => ({ ...options, enforcePrivateModes: true }),
    readInput: () => JSON.stringify(input),
    stdout: { write: (text) => { output += text; } },
    stderr: { write: (text) => assert.fail(text) },
    runGit: () => assert.fail("scope must not execute Git"),
  });
  assert.equal(command.code, 0);
  assert.deepEqual(JSON.parse(output), scope);
  assert.deepEqual(await readProvisioningReceipt(provisioningReceiptPath(root, input.launch_id, "web"), options), records[0]);
  await assert.rejects(readOpenTaskRepositoryScope({ ...input, repository_keys: ["api", "missing"] }, options), /missing/);
  await assert.rejects(readOpenTaskRepositoryScope({ ...input, repository_keys: ["api", "api"] }, options), /unique/);
  const webPath = provisioningReceiptPath(root, input.launch_id, "web");
  await writeProvisioningReceiptAtomic(webPath, { ...records[0], request_digest: "e".repeat(64), admission: {...records[0].admission, assessment: {...records[0].admission.assessment, anchor: {...records[0].admission.assessment.anchor, request_digest: "e".repeat(64)}}} }, options);
  await assert.rejects(readOpenTaskRepositoryScope(input, options), /one confirmed launch and request/);
  await writeProvisioningReceiptAtomic(webPath, { ...records[0], operation_status: { ...records[0].operation_status, phase: "provisioning" } }, options);
  await assert.rejects(readOpenTaskRepositoryScope(input, options), /every repository must be provisioned/);
  await writeFile(webPath, JSON.stringify({ ...records[0], repository_key: "other" }));
  await assert.rejects(readOpenTaskRepositoryScope(input, options), /does not match the request/);
});

test("available Codex CLI natively parses the relaunch -C and --add-dir options without starting a session", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-codex-parser-")));
  const primary = join(root, "primary worktree");
  const additional = join(root, "additional worktree");
  await Promise.all([mkdir(primary), mkdir(additional)]);
  t.after(() => rm(root, { recursive: true, force: true }));
  const descriptor = buildCliRelaunchDescriptor({
    worktreePath: primary,
    additionalWorktreePaths: [additional],
    prompt: "$taskbelay-codex:taskbelay receipt-backed bootstrap",
  });
  const parserArguments = descriptor.arguments.slice(0, -2).concat("--help");
  try {
    const { stdout } = await execFile(process.env.TASKBELAY_CODEX_EXECUTABLE ?? descriptor.executable, parserArguments, {
      encoding: "utf8",
      timeout: 10_000,
    });
    assert.match(stdout, /Codex CLI|Usage: codex/u);
  } catch (error) {
    if (error?.code === "ENOENT") {
      t.skip("Codex CLI is not available on PATH");
      return;
    }
    throw error;
  }
});

for (const surface of ["managed_worktree", "cli_worktree"]) {
  test(`${surface} prepare generates a launch ID and resumes the same receipt on retry`, async (t) => {
    const fixture = await makeRemoteFixture(t, "generated-launch-id");
    const request = "Prepare a launch without an explicit ID.";
    const input = Object.freeze({
      request,
      handoff_file: await writeHandoffFixture(fixture.root, request),
      ...admissionFixture(await assessmentAnchor(fixture, request)),
      repository_key: "primary",
      repository_path: fixture.source,
      workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
      base_branch: "main",
      target_branch: "codex/generated-launch-id",
      surface,
      worktree_path: surface === "managed_worktree" ? null : join(fixture.root, "CLI worktree"),
    });
    const launch = await prepareTaskLaunch(input, fixture.options);
    assert.match(launch.receipt.launch_id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u);
    assert.equal(Object.hasOwn(input, "launch_id"), false);
    assert.equal(launch.receipt.operation_status.phase, "prepared");
    assert.equal(launch.fetch_performed, true);
    assert.equal(launch.resumed, false);
    assert.equal(launch.receipt.base_commit, await gitOutput(fixture.source, "rev-parse", "refs/remotes/origin/main"));
    assert.deepEqual(await readProvisioningReceipt(launch.receipt_path, fixture.options), launch.receipt);

    const retryInput = { ...input, launch_id: launch.receipt.launch_id };
    const retryOptions = {
      ...fixture.options,
      createLaunchId: () => assert.fail("an explicit launch ID must be retained"),
      runGit: () => assert.fail("a fetched launch must resume without Git operations"),
    };
    const retry = await prepareTaskLaunch(retryInput, retryOptions);
    assert.equal(retry.receipt_path, launch.receipt_path);
    assert.deepEqual(retry.receipt, launch.receipt);
    assert.equal(retry.resumed, true);
    assert.equal(retry.fetch_performed, false);
    await assert.rejects(
      prepareTaskLaunch({ ...retryInput, target_branch: "codex/conflicting-target" }, retryOptions),
      /launch identity conflicts/u,
    );
    await writeFile(launch.receipt_path, JSON.stringify({ ...launch.receipt, launch_id: "different-launch" }));
    await assert.rejects(prepareTaskLaunch(retryInput, retryOptions), /launch identity conflicts/u);
  });
}

test("managed dispatch records complete Codex results and resumes without redispatch", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-dispatch-result-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const options = { productSupportRoot: root };
  const clientThreadId = "client-new-thread:1bb3adae-ad01-4210-909e-06fe333f69c2";
  const queued = { clientThreadId, hostId: "local" };
  const wrapped = { content: [{ type: "text", text: JSON.stringify(queued) }], isError: false };
  const cases = [
    ["text", wrapped, "queued"],
    ["direct", queued, "queued"],
    ["structured", { structuredContent: queued }, "queued"],
    ["structured-result", { structuredContent: { result: queued } }, "queued"],
    ["result", { result: queued }, "queued"],
    ["ready-text", { content: [{ type: "text", text: '{"threadId":"thread-1","hostId":"local"}' }] }, "dispatched"],
    ["tool-error", { ...wrapped, isError: true }, "uncertain"],
    ["structured-error", { structuredContent: queued, isError: true }, "uncertain"],
    ["invalid-json", { content: [{ type: "text", text: "creation timed out" }] }, "uncertain"],
    ["duplicate-id", { content: [{ type: "text", text: '{"clientThreadId":"a","clientThreadId":"b"}' }] }, "uncertain"],
    ["multiple-texts", { content: [...wrapped.content, ...wrapped.content] }, "uncertain"],
    ["missing-id", { content: [{ type: "text", text: '{"hostId":"local"}' }] }, "uncertain"],
    ["lost", null, "uncertain"],
  ];
  for (const [name, host_result, phase] of cases) {
    await t.test(name, async () => {
      const identity = { launch_id: `launch-result-${name}`, repository_key: "primary" };
      const path = provisioningReceiptPath(root, identity.launch_id, identity.repository_key);
      let receipt = createProvisioningReceipt({
        launchId: identity.launch_id, repositoryKey: identity.repository_key,
        requestDigest: "a".repeat(64), handoffDigest: "b".repeat(64), sourceRepositoryIdentity: "c".repeat(64),
        sourceType: "remote", carryChanges: false, remoteName: "origin", baseBranch: "main", targetBranch: "codex/result", surface: "managed_worktree",
      });
      receipt = updateProvisioningReceipt(receipt, { phase: "resolving", values: {} });
      receipt = updateProvisioningReceipt(receipt, { phase: "prepared", values: { base_commit: "d".repeat(40) } });
      receipt = updateProvisioningReceipt(receipt, { phase: "dispatch_prepared", values: { dispatch_attempt_id: "e".repeat(64), host_request: { prompt: "saved prompt", title: "saved title", target: { type: "project", projectId: "project-1", environment: { type: "worktree", startingState: { type: "branch", branchName: receipt.base_commit } } } } } });
      receipt = updateProvisioningReceipt(receipt, { phase: "dispatching", values: {} });
      await writeProvisioningReceiptAtomic(path, receipt, options);
      let output = "";
      const command = await runCLI(["host-launch", "dispatch-result"], {
        resolvePaths: () => ({ ...options, enforcePrivateModes: true }),
        readInput: () => JSON.stringify({ ...identity, host_result }),
        stdout: { write: (text) => { output += text; } },
        stderr: { write: (text) => assert.fail(text) },
      });
      assert.equal(command.code, 0);
      const saved = await readProvisioningReceipt(path, options);
      assert.deepEqual(JSON.parse(output).receipt, saved);
      assert.equal(saved.operation_status.phase, phase);
      assert.equal(saved.operation_status.host_client_thread_id, phase === "queued" ? clientThreadId : null);
      assert.equal(saved.operation_status.host_thread_id, phase === "dispatched" ? "thread-1" : null);
      assert.equal(saved.operation_status.dispatch_attempt_id, receipt.operation_status.dispatch_attempt_id);
      assert.equal((await beginManagedTaskDispatch({ ...identity, project_id: "project-1" }, options)).should_dispatch, false);

      if (phase === "uncertain") {
        const recovered = await recordManagedTaskDispatch({ ...identity, host_result: wrapped }, options);
        assert.equal(recovered.receipt.operation_status.phase, "queued");
        assert.equal(recovered.receipt.operation_status.host_client_thread_id, clientThreadId);
      }
      if (phase !== "dispatched") {
        const ready = await recordManagedTaskDispatch({
          ...identity, host_result: { content: [{ type: "text", text: '{"threadId":"thread-1"}' }], isError: false },
        }, options);
        assert.equal(ready.receipt.operation_status.phase, "dispatched");
        assert.equal(ready.receipt.operation_status.host_thread_id, "thread-1");
        assert.equal(ready.receipt.operation_status.host_client_thread_id, clientThreadId);
        assert.equal(ready.receipt.operation_status.dispatch_attempt_id, receipt.operation_status.dispatch_attempt_id);
        assert.deepEqual(await readProvisioningReceipt(path, options), ready.receipt);
        assert.equal((await beginManagedTaskDispatch({ ...identity, project_id: "project-1" }, options)).should_dispatch, false);
      }
    });
  }
});

test("interrupted dispatch preserves its request, fences stale claims, and reconciles actual Host tasks", async (t) => {
  const fixture = await makeRemoteFixture(t, "dispatch-recovery");
  const request = "Recover a dispatch after truncated output.";
  const identity = { launch_id: "dispatch-recovery", repository_key: "primary" };
  const launch = await prepareTaskLaunch({
    ...identity, request, handoff_file: await writeHandoffFixture(fixture.root, request),
    ...admissionFixture(await assessmentAnchor(fixture, request)), repository_path: fixture.source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin", base_branch: "main", target_branch: "codex/recovery",
    surface: "managed_worktree", worktree_path: null,
  }, fixture.options);
  const invoke = async (operation, input) => {
    let output = "";
    const result = await runCLI(["host-launch", operation], {
      resolvePaths: () => ({ ...fixture.options, enforcePrivateModes: true }),
      readInput: () => JSON.stringify(input), stdout: { write: (text) => { output += text; } },
      stderr: { write: (text) => assert.fail(text) },
    });
    assert.equal(result.code, 0);
    // The caller parses the complete file, independently of the displayed output limit.
    const outputPath = join(fixture.root, "command-result.json");
    await writeFile(outputPath, output);
    assert.throws(() => JSON.parse(output.slice(0, 40)));
    return JSON.parse(await readFile(outputPath, "utf8"));
  };
  const prepared = await invoke("dispatch-start", { ...identity, project_id: "project-1" });
  assert.equal(prepared.should_dispatch, false);
  assert.equal(prepared.receipt.operation_status.phase, "dispatch_prepared");
  const status = await invoke("status", identity);
  assert.deepEqual(status.receipt.operation_status.host_request, prepared.host_request);
  await assert.rejects(beginManagedTaskDispatch({ ...identity, project_id: "other" }, fixture.options), /conflicts/);
  // Saved request reads survive removal of the original rendering material.
  await rm(taskHandoffPaths(launch.receipt_path).markdown_path);
  assert.deepEqual((await invoke("dispatch-start", { ...identity, project_id: "project-1" })).host_request, prepared.host_request);
  const firstClaim = { ...identity, dispatch_attempt_id: status.receipt.operation_status.dispatch_attempt_id };
  assert.equal((await invoke("dispatch-call", firstClaim)).should_dispatch, true);
  assert.equal((await invoke("dispatch-call", firstClaim)).should_dispatch, false);
  const recovery = { ...firstClaim, host_call_not_made: true, previous_caller_stopped: true, reason: "JSON parsing failed before create_thread; original execution ended." };
  await assert.rejects(recoverUncalledManagedTaskDispatch({ ...recovery, host_call_not_made: false }, fixture.options), /uncalled/);
  await assert.rejects(recoverUncalledManagedTaskDispatch({ ...recovery, previous_caller_stopped: false }, fixture.options), /stopped/);
  const recovered = await invoke("dispatch-recover", recovery);
  assert.equal(recovered.should_dispatch, false);
  assert.deepEqual(recovered.host_request, prepared.host_request);
  assert.equal(recovered.receipt.operation_status.dispatch_recovery_reason, recovery.reason);
  await assert.rejects(claimManagedTaskDispatch(firstClaim, fixture.options), /does not match/);
  const newClaim = { ...identity, dispatch_attempt_id: recovered.receipt.operation_status.dispatch_attempt_id };
  assert.equal((await invoke("dispatch-call", newClaim)).should_dispatch, true);
  await invoke("dispatch-result", { ...identity, host_result: null });
  await assert.rejects(recoverUncalledManagedTaskDispatch({ ...recovery, ...newClaim }, fixture.options), /uncalled dispatching/);
  assert.equal((await invoke("dispatch-call", newClaim)).should_dispatch, false);
  assert.equal((await invoke("dispatch-reconcile", { ...identity, candidates: [] })).matched, false);
  assert.equal((await invoke("dispatch-reconcile", { ...identity, candidates: [{ thread_id: "unrelated", initial_prompt: "different launch" }] })).matched, false);
  const candidate = { thread_id: "actual-host-task", initial_prompt: prepared.host_request.prompt };
  await assert.rejects(reconcileManagedTaskDispatch({ ...identity, candidates: [candidate, { ...candidate, thread_id: "duplicate" }] }, fixture.options), /multiple/);
  const matched = await invoke("dispatch-reconcile", { ...identity, candidates: [candidate] });
  assert.equal(matched.matched, true);
  assert.equal(matched.should_dispatch, false);
  assert.equal(matched.receipt.operation_status.host_thread_id, candidate.thread_id);
  assert.equal((await invoke("dispatch-call", newClaim)).should_dispatch, false);
  assert.equal(await gitOutput(fixture.source, "branch", "--list", "codex/recovery"), "");
});

test("managed launch freezes the confirmed remote ref, dispatches once, and bootstraps a named clean branch", async (t) => {
  const fixture = await makeRemoteFixture(t, "managed");
  await writeFile(join(fixture.source, "source-only.txt"), "do not copy\n");
  await writeFile(join(fixture.source, "staged-only.txt"), "do not copy staged content\n");
  await git(fixture.source, "add", "staged-only.txt");
  await writeFile(join(fixture.source, "base.txt"), "do not copy unstaged content\n");
  const request = "Implement the confirmed managed task.";
  const assessment_anchor = await assessmentAnchor(fixture, request);
  const launch = await prepareTaskLaunch({
    launch_id: "launch-managed-0001",
    request,
    handoff_file: await writeHandoffFixture(fixture.root, request),
    ...admissionFixture(assessment_anchor),
    repository_key: "primary",
    repository_path: fixture.source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "codex/managed-task",
    surface: "managed_worktree",
    worktree_path: null,
  }, fixture.options);
  assert.equal(launch.receipt.operation_status.phase, "prepared");
  assert.throws(() => buildOpenTaskRepositoryScope([launch.receipt]), /every repository must be provisioned/u);
  assert.equal(launch.source_dirty, true);
  if (fixture.options.enforcePrivateModes) assert.equal((await stat(launch.receipt_path)).mode & 0o077, 0);
  assert.deepEqual(Object.keys(launch.receipt).sort(), [
    "admission", "base_branch", "created_at", "base_commit", "source_type", "carry_changes", "snapshot_commit", "host", "launch_id", "operation_status",
    "remote_name", "repository_key", "workspace_mode", "request_digest", "handoff_digest", "source_repository_identity", "target_branch",
    "worktree_path",
  ].sort());
  const retainedReceipt = await readFile(launch.receipt_path, "utf8");
  assert.equal(retainedReceipt.includes("Implement the confirmed managed task"), false);
  assert.equal(retainedReceipt.includes("do not copy staged content"), false);
  assert.equal(retainedReceipt.includes(fixture.remote), false);

  const dispatched = await beginManagedTaskDispatch({
    launch_id: "launch-managed-0001",
    repository_key: "primary",
    project_id: "project-1",
  }, fixture.options);
  assert.equal(dispatched.should_dispatch, false);
  const claim = await claimManagedTaskDispatch({ launch_id: "launch-managed-0001", repository_key: "primary", dispatch_attempt_id: dispatched.receipt.operation_status.dispatch_attempt_id }, fixture.options);
  assert.equal(claim.should_dispatch, true);
  assert.equal(dispatched.host_request.title, "TaskBelay launch-managed-0001 primary");
  assert.deepEqual(dispatched.host_request.target.environment.startingState, {
    type: "branch",
    branchName: launch.receipt.base_commit,
  });
  assert.equal(Object.hasOwn(dispatched.host_request.target.environment.startingState, "onMissing"), false);
  assert.equal((await beginManagedTaskDispatch({
    launch_id: "launch-managed-0001",
    repository_key: "primary",
    project_id: "project-1",
  }, fixture.options)).should_dispatch, false);

  await recordManagedTaskDispatch({
    launch_id: "launch-managed-0001",
    repository_key: "primary",
    host_result: { clientThreadId: "client-thread-1" },
  }, fixture.options);
  await recordManagedTaskDispatch({
    launch_id: "launch-managed-0001",
    repository_key: "primary",
    host_result: { threadId: "thread-1", hostId: "local" },
  }, fixture.options);

  const managedWorktree = join(fixture.root, "managed worktree");
  await git(fixture.source, "worktree", "add", "--detach", managedWorktree, launch.receipt.base_commit);
  const bootstrapped = await bootstrapManagedTask({
    launch_id: "launch-managed-0001",
    repository_key: "primary",
    worktree_path: managedWorktree,
  }, fixture.options);
  assert.deepEqual(validateWorkspaceOrigin(bootstrapped.workspace_origin), {
    mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    base_commit: launch.receipt.base_commit,
    task_branch: "codex/managed-task",
    provisioning_receipt_id: bootstrapped.workspace_origin.provisioning_receipt_id,
  });
  assert.match(bootstrapped.workspace_origin.provisioning_receipt_id, /^codex-[0-9a-f]{64}$/u);
  assert.deepEqual(buildOpenTaskRepositoryScope([bootstrapped.receipt]), {
    repository_path: managedWorktree,
    workspace_origin: bootstrapped.workspace_origin,
  });
  assert.equal((await gitOutput(managedWorktree, "branch", "--show-current")), "codex/managed-task");
  await assert.rejects(readFile(join(managedWorktree, "source-only.txt")), { code: "ENOENT" });
  await assert.rejects(readFile(join(managedWorktree, "staged-only.txt")), { code: "ENOENT" });
  assert.equal(await readFile(join(managedWorktree, "base.txt"), "utf8"), "base\n");
});

test("CLI launch returns parser-ready argv and retains separate worktree and branch cleanup decisions", async (t) => {
  const fixture = await makeRemoteFixture(t, "cli");
  const worktree = join(fixture.root, "CLI worktree");
  const request = "Implement the CLI task.";
  await prepareTaskLaunch({
    launch_id: "launch-cli-0001",
    request,
    handoff_file: await writeHandoffFixture(fixture.root, request),
    ...admissionFixture(await assessmentAnchor(fixture, request)),
    repository_key: "primary",
    repository_path: fixture.source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "codex/cli-task",
    surface: "cli_worktree",
    worktree_path: worktree,
  }, fixture.options);
  const provisioned = await provisionCliTask({
    launch_id: "launch-cli-0001",
    repository_key: "primary",
    additional_worktree_paths: [join(fixture.root, "additional worktree")],
  }, { ...fixture.options, sourceRepositoryPath: fixture.source });
  assert.equal(provisioned.relaunch.executable, "codex");
  assert.deepEqual(provisioned.relaunch.arguments.slice(0, -1), [
    "-C", worktree, "--add-dir", join(fixture.root, "additional worktree"), "--",
  ]);
  assert.match(provisioned.relaunch.arguments.at(-1), /## Confirmed requirements/u);
  assert.ok(provisioned.relaunch.arguments.at(-1).includes(request));
  assert.deepEqual(terminalCleanupDecision({
    lifecycle: "DONE", surface: "cli_worktree", clean: true, pushed: false, stateCertain: true,
  }), {
    automatic_cleanup: false,
    worktree_cleanup: "separate_authorization_required",
    branch_cleanup: "requires_unpushed_review",
  });

  // Core facts are a fixture here; Git rename/ref preservation is real and isolated.
  await execFile("git",["-C",worktree,"branch","-m","codex/cli-task","codex/renamed-task"]);
  await execFile("git",["-C",fixture.source,"branch","codex/cli-task"]);
  const core_task={task_id:"task-cleanup",origin_host:"codex",revision:10,current_cursor:"DONE",primary_repository_key:"primary",workspace_origin:{...provisioned.workspace_origin,canonical_worktree_root:worktree},repository:{current_branch:"codex/renamed-task",current_head:await gitOutput(worktree,"rev-parse","HEAD"),detached:false,worktree_instance_digest:"a".repeat(64),binding_digest:"b".repeat(64)}};
  await cleanupCliTaskWorktree({
    launch_id: "launch-cli-0001", repository_key: "primary", terminal: true, authorized: true, core_task,
  }, { ...fixture.options, sourceRepositoryPath: fixture.source });
  await assert.rejects(stat(worktree), { code: "ENOENT" });
  await cleanupTaskBranch({
    launch_id: "launch-cli-0001", repository_key: "primary", terminal: true, authorized: true, core_task,
  }, { ...fixture.options, sourceRepositoryPath: fixture.source });
  await assert.rejects(
    execFile("git", ["-C", fixture.source, "show-ref", "--verify", "refs/heads/codex/renamed-task"]),
  );
  assert.equal(await gitOutput(fixture.source,"rev-parse","refs/heads/codex/cli-task"),core_task.repository.current_head);
});

test("queued dispatch and Handoff persist one-shot state for read-before-retry", async (t) => {
  const fixture = await makeRemoteFixture(t, "handoff");
  const worktree = join(fixture.root, "handoff worktree");
  const request = "Implement then relocate the task.";
  await prepareTaskLaunch({
    launch_id: "launch-handoff-0001",
    request,
    handoff_file: await writeHandoffFixture(fixture.root, request),
    ...admissionFixture(await assessmentAnchor(fixture, request)),
    repository_key: "primary",
    repository_path: fixture.source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "codex/handoff-task",
    surface: "cli_worktree",
    worktree_path: worktree,
  }, fixture.options);
  await provisionCliTask({
    launch_id: "launch-handoff-0001", repository_key: "primary", additional_worktree_paths: [],
  }, { ...fixture.options, sourceRepositoryPath: fixture.source });
  const handoff = await beginTaskHandoff({
    launch_id: "launch-handoff-0001",
    repository_key: "primary",
    relocation_id: "relocation-1",
    thread_id: "thread-1",
  }, fixture.options);
  assert.equal(handoff.should_dispatch, true);
  assert.equal((await beginTaskHandoff({
    launch_id: "launch-handoff-0001",
    repository_key: "primary",
    relocation_id: "relocation-1",
    thread_id: "thread-1",
  }, fixture.options)).should_dispatch, false);
  await recordTaskHandoff({
    launch_id: "launch-handoff-0001",
    repository_key: "primary",
    host_result: { operationId: "host-operation-1", revision: 3 },
  }, fixture.options);
  const completed = await recordTaskHandoffStatus({
    launch_id: "launch-handoff-0001",
    repository_key: "primary",
    status: "succeeded",
    revision: 4,
    worktree_path: worktree,
  }, fixture.options);
  assert.equal(completed.relocation_id, "relocation-1");
  assert.equal(completed.receipt.operation_status.host_operation_id, "host-operation-1");
});

test("a failed fetch leaves a failed receipt and no target branch or worktree", async (t) => {
  const fixture = await makeRemoteFixture(t, "fetch-failure");
  const worktree = join(fixture.root, "missing worktree");
  const request = "Use a missing base.";
  await assert.rejects(prepareTaskLaunch({
    launch_id: "launch-failed-0001",
    request,
    handoff_file: await writeHandoffFixture(fixture.root, request),
    ...admissionFixture(await assessmentAnchor(fixture, request)),
    repository_key: "primary",
    repository_path: fixture.source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "missing-base",
    target_branch: "codex/not-created",
    surface: "cli_worktree",
    worktree_path: worktree,
  }, fixture.options));
  const receiptPath = provisioningReceiptPath(fixture.productSupportRoot, "launch-failed-0001", "primary");
  const receipt = await readProvisioningReceipt(receiptPath, { productSupportRoot: fixture.productSupportRoot });
  assert.equal(receipt.operation_status.phase, "failed");
  await assert.rejects(stat(worktree), { code: "ENOENT" });
  await assert.rejects(execFile("git", ["-C", fixture.source, "show-ref", "--verify", "refs/heads/codex/not-created"]));
});

test("provisioning refuses a request, HEAD, or status that changed after assessment", async (t) => {
  const fixture = await makeRemoteFixture(t, "stale-assessment");
  const request = "Implement only the assessed change.";
  const assessment_anchor = await assessmentAnchor(fixture, request);
  await writeFile(join(fixture.source, "late-change.txt"), "changed while waiting\n");
  await assert.rejects(prepareTaskLaunch({
    launch_id: "launch-stale-0001",
    request,
    handoff_file: await writeHandoffFixture(fixture.root, request),
    ...admissionFixture(assessment_anchor),
    repository_key: "primary",
    repository_path: fixture.source,
    workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
    base_branch: "main",
    target_branch: "codex/stale-task",
    surface: "managed_worktree",
    worktree_path: null,
  }, fixture.options), /assessment is stale/u);
  const receiptPath = provisioningReceiptPath(fixture.productSupportRoot, "launch-stale-0001", "primary");
  assert.equal(await readProvisioningReceipt(receiptPath, { productSupportRoot: fixture.productSupportRoot }), null);
});

test("managed and CLI send the same saved requirements after the source draft is removed", async (t) => {
  const fixture = await makeRemoteFixture(t, "complete-handoff");
  const request = "Build the confirmed preview.";
  const material = {
    ...handoffFixture(request),
    confirmed_requirements: [{ text: "Preserve original whitespace, including the final correction.", source_ids: ["m2"] }],
    work_requirements: ["Use the requested method profile and keep the agreed verification scope."],
    unconfirmed_suggestions: ["Automatic saving was suggested, not approved."],
    discussion: [
      { id: "m1", role: "user", text: request },
      { id: "m2", role: "user", text: "Final correction: preserve original whitespace." },
    ],
  };
  const handoffFile = await writeHandoffFixture(fixture.root, request, material);
  const anchor = await assessmentAnchor(fixture, request);
  const launches = [];
  for (const surface of ["managed_worktree", "cli_worktree"]) {
    launches.push(await prepareTaskLaunch({
      launch_id: `launch-${surface}`,
      request,
      handoff_file: handoffFile,
      ...admissionFixture(anchor),
      repository_key: "primary",
      repository_path: fixture.source,
      workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin",
      base_branch: "main",
      target_branch: `codex/${surface}`,
      surface,
      worktree_path: surface === "managed_worktree" ? null : join(fixture.root, "CLI worktree"),
    }, fixture.options));
  }
  await rm(handoffFile);
  const managed = await beginManagedTaskDispatch({
    launch_id: launches[0].receipt.launch_id, repository_key: "primary", project_id: "project-1",
  }, fixture.options);
  const cli = await provisionCliTask({
    launch_id: launches[1].receipt.launch_id, repository_key: "primary", additional_worktree_paths: [],
  }, { ...fixture.options, sourceRepositoryPath: fixture.source });
  const body = (prompt) => prompt.slice(prompt.indexOf("## Goal and expected result"));
  assert.equal(body(managed.host_request.prompt), body(cli.relaunch.arguments.at(-1)));
  assert.ok(body(managed.host_request.prompt).includes(material.confirmed_requirements[0].text));
  for (const launch of launches) {
    const saved = await readTaskHandoff(launch.receipt_path, launch.receipt.handoff_digest);
    assert.deepEqual(saved.material, material);
  }
  assert.equal(await gitOutput(fixture.source, "status", "--porcelain"), "");
});

test("sender refuses changed handoff files before managed dispatch or CLI worktree creation", async (t) => {
  const fixture = await makeRemoteFixture(t, "changed-handoff");
  const request = "Keep all confirmed requirements.";
  for (const surface of ["managed_worktree", "cli_worktree"]) {
    const worktree = join(fixture.root, surface);
    const launch = await prepareTaskLaunch({
      launch_id: `changed-${surface}`, request,
      handoff_file: await writeHandoffFixture(fixture.root, request),
      ...admissionFixture(await assessmentAnchor(fixture, request)),
      repository_key: "primary", repository_path: fixture.source,
      workspace_mode: "dedicated_worktree", source_type: "remote", carry_changes: false, remote_name: "origin", base_branch: "main", target_branch: `codex/changed-${surface}`,
      surface, worktree_path: surface === "managed_worktree" ? null : worktree,
    }, fixture.options);
    await writeFile(taskHandoffPaths(launch.receipt_path).markdown_path, "Only a partial summary");
    const input = { launch_id: launch.receipt.launch_id, repository_key: "primary" };
    await assert.rejects(surface === "managed_worktree"
      ? beginManagedTaskDispatch({ ...input, project_id: "project-1" }, fixture.options)
      : provisionCliTask({ ...input, additional_worktree_paths: [] }, { ...fixture.options, sourceRepositoryPath: fixture.source }), /has changed/u);
    const retained = await readProvisioningReceipt(launch.receipt_path, fixture.options);
    assert.equal(retained.operation_status.phase, "prepared");
    await assert.rejects(stat(worktree), { code: "ENOENT" });
  }
});

async function makeRemoteFixture(t, name) {
  const root = await realpath(await mkdtemp(join(tmpdir(), `taskbelay-codex-${name}-`)));
  const remote = join(root, "remote.git");
  const source = join(root, "中文 source checkout");
  const productSupportRoot = join(root, "product support");
  await mkdir(productSupportRoot);
  t.after(() => rm(root, { recursive: true, force: true }));
  await execFile("git", ["init", "--bare", "--initial-branch=main", remote], { encoding: "utf8" });
  await execFile("git", ["clone", remote, source], { encoding: "utf8" });
  await git(source, "config", "user.email", "codex@example.invalid");
  await git(source, "config", "user.name", "Codex Test");
  await git(source, "config", "core.autocrlf", "false");
  await writeFile(join(source, "base.txt"), "base\n");
  await git(source, "add", "base.txt");
  await git(source, "commit", "-m", "base");
  await git(source, "push", "-u", "origin", "main");
  return {
    root,
    remote,
    source,
    productSupportRoot,
    options: { productSupportRoot, enforcePrivateModes: process.platform !== "win32" },
  };
}

async function assessmentAnchor(fixture, request) {
  return await inspectAdmissionAnchor({
    request,
    repositories: [{ key: "primary", repository_path: fixture.source }],
  });
}

async function git(cwd, ...arguments_) {
  return await execFile("git", arguments_, { cwd, encoding: "utf8" });
}

async function gitOutput(cwd, ...arguments_) {
  return (await git(cwd, ...arguments_)).stdout.trim();
}

for (const carry of [false, true]) test(`local CLI launch selects an offline branch and carry_changes=${carry}`, async (t) => {
  const fixture = await makeRemoteFixture(t, `local-${carry}`);
  await git(fixture.source, "remote", "remove", "origin");
  await git(fixture.source, "branch", "local-base");
  await writeFile(join(fixture.source, ".gitignore"), "ignored.txt\n");
  await git(fixture.source, "add", ".gitignore");
  await writeFile(join(fixture.source, "base.txt"), "staged\n");
  await git(fixture.source, "add", "base.txt");
  await writeFile(join(fixture.source, "base.txt"), "unstaged\n");
  await writeFile(join(fixture.source, "新增 file.txt"), "new file\n");
  await writeFile(join(fixture.source, "ignored.txt"), "ignored\n");
  const before = await gitOutput(fixture.source, "status", "--porcelain=v2", "--untracked-files=all");
  const beforeIndex = await gitOutput(fixture.source, "write-tree");
  const request = "Create local worktree.";
  const input = {
    request, ...admissionFixture(await assessmentAnchor(fixture, request)), launch_id: `local-${carry}`,
    repository_key: "primary", repository_path: fixture.source, workspace_mode: "dedicated_worktree", source_type: "local", carry_changes: carry,
    remote_name: "", base_branch: "local-base", target_branch: "codex/local", surface: "cli_worktree",
    worktree_path: join(fixture.root, "local-task"), handoff_file: await writeHandoffFixture(fixture.root, request),
  };
  const launch = await prepareTaskLaunch(input, fixture.options);
  assert.equal(launch.fetch_performed, false);
  // The captured contents stay frozen even when source bytes subsequently change.
  if (carry) await writeFile(join(fixture.source, "base.txt"), "later edit\n");
  const result = await provisionCliTask({ launch_id: input.launch_id, repository_key: "primary", additional_worktree_paths: [] }, { ...fixture.options, sourceRepositoryPath: fixture.source });
  assert.equal(result.workspace_origin.source_type, "local");
  assert.equal(result.workspace_origin.carry_changes, carry);
  assert.equal(await gitOutput(fixture.source, "write-tree"), beforeIndex);
  assert.equal(await gitOutput(fixture.source, "status", "--porcelain=v2", "--untracked-files=all"), before);
  assert.equal(await readFile(join(input.worktree_path, "base.txt"), "utf8"), carry ? "unstaged\n" : "base\n");
  if (carry) {
    assert.equal(await gitOutput(input.worktree_path, "show", ":base.txt"), "staged");
    assert.equal(await readFile(join(input.worktree_path, "新增 file.txt"), "utf8"), "new file\n");
  } else await assert.rejects(readFile(join(input.worktree_path, "新增 file.txt")), { code: "ENOENT" });
  await assert.rejects(readFile(join(input.worktree_path, "ignored.txt")), { code: "ENOENT" });
  assert.equal(await gitOutput(fixture.source, "stash", "list"), "");
});

test("snapshot rejects changed contents even when Git status is unchanged", async (t) => {
  const fixture = await makeRemoteFixture(t, "changing-snapshot");
  await writeFile(join(fixture.source, "base.txt"), "staged\n");
  await git(fixture.source, "add", "base.txt");
  await writeFile(join(fixture.source, "base.txt"), "tracked-A\n");
  await writeFile(join(fixture.source, "new.txt"), "untracked-A\n");
  const before = await gitOutput(fixture.source, "status", "--porcelain=v2", "--untracked-files=all");
  const index = await gitOutput(fixture.source, "write-tree");
  const head = await gitOutput(fixture.source, "rev-parse", "HEAD");
  const stash = await gitOutput(fixture.source, "stash", "list");
  let captures = 0;
  const runGit = async (args, options) => {
    const output = await defaultRunGit(args, options);
    if (args.includes("stash") && args.includes("create") && ++captures === 1) {
      await writeFile(join(fixture.source, "base.txt"), "tracked-B\n");
      await writeFile(join(fixture.source, "new.txt"), "untracked-B\n");
      assert.equal(await gitOutput(fixture.source, "status", "--porcelain=v2", "--untracked-files=all"), before);
    }
    return output;
  };
  const request = "Carry one stable source snapshot.";
  const input = {
    request, ...admissionFixture(await assessmentAnchor(fixture, request)), launch_id: "changing-snapshot",
    repository_key: "primary", repository_path: fixture.source, workspace_mode: "dedicated_worktree",
    source_type: "local", carry_changes: true, remote_name: "", base_branch: "main", target_branch: "codex/snapshot",
    surface: "cli_worktree", worktree_path: join(fixture.root, "task"),
    handoff_file: await writeHandoffFixture(fixture.root, request),
  };
  await assert.rejects(prepareTaskLaunch(input, { ...fixture.options, runGit }), /Source workspace changed while capturing/);
  assert.equal(captures, 2);
  const receipt = await readProvisioningReceipt(provisioningReceiptPath(fixture.productSupportRoot, input.launch_id, "primary"), fixture.options);
  assert.equal(receipt.operation_status.phase, "failed");
  assert.equal(receipt.snapshot_commit, null);
  await assert.rejects(stat(input.worktree_path), { code: "ENOENT" });
  assert.equal(await gitOutput(fixture.source, "branch", "--list", input.target_branch), "");
  assert.equal(await readFile(join(fixture.source, "base.txt"), "utf8"), "tracked-B\n");
  assert.equal(await readFile(join(fixture.source, "new.txt"), "utf8"), "untracked-B\n");
  assert.equal(await gitOutput(fixture.source, "write-tree"), index);
  assert.equal(await gitOutput(fixture.source, "rev-parse", "HEAD"), head);
  assert.equal(await gitOutput(fixture.source, "stash", "list"), stash);
});

test("snapshot helpers in all Host packages match their shared source", async () => {
  const source = (await readFile(new URL("../../host-workspace/worktree-snapshot.mjs", import.meta.url), "utf8")).replaceAll("\r\n", "\n");
  for (const host of ["codex", "claude", "deepseek"]) {
    const generated = (await readFile(new URL(`../../${host}/lib/worktree-snapshot.mjs`, import.meta.url), "utf8")).replaceAll("\r\n", "\n");
    assert.equal(generated, "// Generated from packages/host-workspace/worktree-snapshot.mjs; edit the shared source.\n" + source, host);
  }
});

for (const conflict of [false, true]) test(`local managed bootstrap applies the frozen snapshot to another branch; conflict=${conflict}`, async (t) => {
  const fixture = await makeRemoteFixture(t, `managed-local-${conflict}`);
  await git(fixture.source, "switch", "-c", "selected");
  await writeFile(join(fixture.source, conflict ? "base.txt" : "branch.txt"), "selected branch\n");
  await git(fixture.source, "add", ".");
  await git(fixture.source, "commit", "-m", "selected branch");
  await git(fixture.source, "switch", "main");
  await writeFile(join(fixture.source, "base.txt"), "carried\n");
  const request = "Managed local work.";
  const input = { request, ...admissionFixture(await assessmentAnchor(fixture, request)), launch_id: `managed-local-${conflict}`,
    repository_key: "primary", repository_path: fixture.source, workspace_mode: "dedicated_worktree", source_type: "local", carry_changes: true,
    remote_name: "", base_branch: "selected", target_branch: "codex/local", surface: "managed_worktree", worktree_path: null,
    handoff_file: await writeHandoffFixture(fixture.root, request) };
  const launch = await prepareTaskLaunch(input, fixture.options);
  const dispatch = await beginManagedTaskDispatch({ launch_id: input.launch_id, repository_key: "primary", project_id: "project" }, fixture.options);
  assert.equal(dispatch.host_request.target.environment.startingState.branchName, launch.receipt.base_commit);
  await claimManagedTaskDispatch({ launch_id: input.launch_id, repository_key: "primary", dispatch_attempt_id: dispatch.receipt.operation_status.dispatch_attempt_id }, fixture.options);
  const worktree = join(fixture.root, "managed-local");
  await git(fixture.source, "worktree", "add", "--detach", worktree, launch.receipt.base_commit);
  const bootstrap = bootstrapManagedTask({ launch_id: input.launch_id, repository_key: "primary", worktree_path: worktree }, fixture.options);
  if (conflict) {
    await assert.rejects(bootstrap, /could not be applied/);
    const receipt = await readProvisioningReceipt(launch.receipt_path, fixture.options);
    assert.equal(receipt.operation_status.phase, "failed");
    assert.match(await gitOutput(worktree, "status", "--porcelain"), /UU/);
  } else {
    assert.equal((await bootstrap).workspace_origin.carry_changes, true);
    assert.equal(await readFile(join(worktree, "base.txt"), "utf8"), "carried\n");
    assert.equal(await readFile(join(worktree, "branch.txt"), "utf8"), "selected branch\n");
  }
  assert.equal(await readFile(join(fixture.source, "base.txt"), "utf8"), "carried\n");
});

function createProvisioningReceipt(input) { return rawCreateProvisioningReceipt({workspaceMode:"dedicated_worktree", ...input, admission:receiptAdmissionFixture(input)}); }

test("prepare rejects missing, unresolved or unchosen assessments before any provisioning operation", async (t) => {
 const fixture = await makeRemoteFixture(t, "admission-guard");
 const request = "Validate complete admission";
 const admission = admissionFixture(await assessmentAnchor(fixture, request));
 const input = {request, ...admission, repository_key:"primary", repository_path:fixture.source,
 workspace_mode: "dedicated_worktree", source_type:"local", carry_changes:false, remote_name:"", base_branch:"main", target_branch:"codex/admission-guard",
 surface:"managed_worktree", worktree_path:null, handoff_file:await writeHandoffFixture(fixture.root, request)};
 const mutations = [
 value => { delete value.assessment; }, value => { delete value.user_choice; },
 value => { value.user_choice.mode = "direct"; }, value => { value.user_choice.source = "assistant"; },
 value => { value.assessment.change_level="uncertain"; value.assessment.recommendation="clarify"; value.assessment.unknowns=["unknown caller"]; },
 value => { value.assessment.candidate_paths=[]; }, value => { value.assessment.verification_shape=[]; },
 value => { value.assessment.observed_repositories=[]; },
 ];
 for (const mutate of mutations) {
   const invalid = structuredClone(input); mutate(invalid);
   await assert.rejects(prepareTaskLaunch(invalid, {...fixture.options, runGit:()=>assert.fail("invalid admission reached Git"), createLaunchId:()=>"invalid-admission"}));
   assert.equal(await readProvisioningReceipt(provisioningReceiptPath(fixture.productSupportRoot,"invalid-admission","primary"), fixture.options), null);
 }
 const prepared = await prepareTaskLaunch(input, {...fixture.options, createLaunchId:()=>"valid-admission"});
 assert.deepEqual(prepared.receipt.admission, admission);
 const resumed = await prepareTaskLaunch({...input,launch_id:prepared.receipt.launch_id}, {...fixture.options,runGit:()=>assert.fail("confirmed continuation repeated Git")});
 assert.equal(resumed.resumed, true);
 const changedChoice = structuredClone(input); changedChoice.user_choice.summary="A different decision";
 await assert.rejects(prepareTaskLaunch({...changedChoice,launch_id:prepared.receipt.launch_id}, fixture.options), /conflicts/);
});

for (const mode of ["new_branch", "current_branch"]) for (const carry of [false, true]) {
  test(`${mode} keeps the current directory and confirmed initial changes (${carry})`, async (t) => {
    const fixture = await makeRemoteFixture(t, `in-place-${mode}-${carry}`);
    await git(fixture.source, "remote", "remove", "origin");
    await writeFile(join(fixture.source, ".git", "info", "exclude"), "ignored.txt\n");
    await writeFile(join(fixture.source, "ignored.txt"), "local environment\n");
    if (carry) {
      await writeFile(join(fixture.source, "base.txt"), "staged\n");
      await git(fixture.source, "add", "base.txt");
      await writeFile(join(fixture.source, "base.txt"), "unstaged\n");
      await writeFile(join(fixture.source, "new.txt"), "untracked\n");
    }
    const before = await gitOutput(fixture.source, "status", "--porcelain=v2", "--untracked-files=all");
    const index = await gitOutput(fixture.source, "write-tree");
    const request = "Work in the existing local directory.";
    const input = {
      ...admissionFixture(await assessmentAnchor(fixture, request)), request,
      repository_key: "primary", repository_path: fixture.source, workspace_mode: mode,
      source_type: "local", carry_changes: carry, remote_name: "", base_branch: "main",
      target_branch: mode === "new_branch" ? "codex/local-task" : "main",
      surface: "current_session", worktree_path: fixture.source, handoff_file: null,
    };
    let checks = 0;
    const options = { ...fixture.options, checkWorkspaceAvailable: async (root) => { checks++; return { available: true, repository_path: root }; } };
    const prepared = await prepareTaskLaunch(input, options);
    assert.equal(prepared.receipt.handoff_digest, null);
    assert.equal(prepared.fetch_performed, false);
    const identity = { launch_id: prepared.receipt.launch_id, repository_key: "primary" };
    const ready = await provisionLocalTask(identity, options);
    assert.equal(checks, 2);
    assert.equal(ready.workspace_origin.mode, mode);
    assert.equal(ready.receipt.worktree_path, fixture.source);
    assert.equal(await gitOutput(fixture.source, "branch", "--show-current"), input.target_branch);
    assert.equal(await gitOutput(fixture.source, "status", "--porcelain=v2", "--untracked-files=all"), before);
    assert.equal(await gitOutput(fixture.source, "write-tree"), index);
    assert.equal(await readFile(join(fixture.source, "ignored.txt"), "utf8"), "local environment\n");
    assert.equal((await gitOutput(fixture.source, "worktree", "list", "--porcelain")).split("worktree ").length - 1, 1);
    assert.equal((await readOpenTaskRepositoryScope({ launch_id: identity.launch_id, repository_keys: ["primary"], primary_repository_key: "primary" }, options)).workspace_origin.mode, mode);
    await writeFile(join(fixture.source, "later.txt"), "later work\n");
    const retry = await provisionLocalTask(identity, { ...options, runGit: () => assert.fail("provisioned retry changed Git") });
    assert.deepEqual(retry.receipt, ready.receipt);
    assert.equal(await readFile(join(fixture.source, "later.txt"), "utf8"), "later work\n");
    assert.equal(terminalCleanupDecision({ lifecycle: "DONE", surface: "current_session", clean: false, pushed: false, stateCertain: true }).worktree_cleanup, "not_applicable");
    await assert.rejects(beginTaskHandoff({ ...identity, relocation_id: "move", thread_id: "thread" }, options), /do not support worktree handoff/);
    await assert.rejects(cleanupTaskBranch({ ...identity, terminal: true, authorized: true, core_task:null }, options), /terminal Core Task|local Task directories and branches are retained/);
  });
}

test("local launch rejects dirty content and active Tasks before branch changes", async (t) => {
  const fixture = await makeRemoteFixture(t, "local-conflict");
  const request = "Create a local task safely.";
  await writeFile(join(fixture.source, "base.txt"), "existing work\n");
  const input = {
    ...admissionFixture(await assessmentAnchor(fixture, request)), request,
    repository_key: "primary", repository_path: fixture.source, workspace_mode: "new_branch",
    source_type: "local", carry_changes: false, remote_name: "", base_branch: "main", target_branch: "codex/local-conflict",
    surface: "current_session", worktree_path: fixture.source, handoff_file: null,
  };
  const available = async (root) => ({ available: true, repository_path: root });
  await assert.rejects(prepareTaskLaunch(input, { ...fixture.options, checkWorkspaceAvailable: available }), /initial local changes/);
  input.carry_changes = true;
  await assert.rejects(prepareTaskLaunch(input, { ...fixture.options, checkWorkspaceAvailable: async (root) => ({ available: false, repository_path: root, task_id: "active" }) }), /active TaskBelay Task/);
  const prepared = await prepareTaskLaunch(input, { ...fixture.options, checkWorkspaceAvailable: available });
  await assert.rejects(provisionLocalTask({ launch_id: prepared.receipt.launch_id, repository_key: "primary" }, { ...fixture.options, checkWorkspaceAvailable: async (root) => ({ available: false, repository_path: root }) }), /active TaskBelay Task/);
  assert.equal(await gitOutput(fixture.source, "branch", "--show-current"), "main");
  assert.equal(await readFile(join(fixture.source, "base.txt"), "utf8"), "existing work\n");
});
