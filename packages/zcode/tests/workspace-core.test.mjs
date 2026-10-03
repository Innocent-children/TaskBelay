import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";
import test from "node:test";
import { execPortableCommand } from "../lib/command.mjs";
import { cleanup, bindTask, inspect, prepare, provision, relocate, scope } from "../lib/workspace.mjs";
import { defaultRunGit as git } from "../lib/worktree-lifecycle.mjs";

const testCore = process.env.TASKBELAY_ZCODE_TEST_CORE;
const supportedMachine = process.platform === "darwin" && process.arch === "arm64" || process.platform === "win32" && process.arch === "x64";

test("ZCode workspace keys survive real Core creation and relocation", {
  skip: !supportedMachine ? "ZCode supports darwin-arm64 and win32-x64" : !testCore ? "Set TASKBELAY_ZCODE_TEST_CORE to a native built Core for this integration check" : false,
  timeout: 120_000,
}, async t => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "zcode-workspace-core-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const runtime = await realpath(testCore);

  for (const keys of [["backend"], ["backend", "worker", "api"]]) {
    await t.test(keys.join(" + "), async t => {
      const home = join(root, String(keys.length));
      const data = join(home, "data");
      await mkdir(data, { recursive: true });
      const environment = { ...process.env, HOME: home, USERPROFILE: home, LOCALAPPDATA: join(home, "appdata"), TASKBELAY_DATA_DIR: data };
      const workspaceAvailable = async repository_path => JSON.parse((await execPortableCommand(runtime, ["host-check", "workspace-available"], {
        env: environment, input: JSON.stringify({ repository_path }), timeout: 10_000,
      })).stdout);
      const options = { environment, checkWorkspaceAvailable: workspaceAvailable };
      const repositories = [];
      for (const key of keys) {
        const source = join(home, "source-" + key);
        await mkdir(source);
        await git(["init", "-b", "main", source]);
        await git(["-C", source, "config", "user.name", "Test"]);
        await git(["-C", source, "config", "user.email", "test@example.invalid"]);
        await writeFile(join(source, "base.txt"), key);
        await git(["-C", source, "add", "."]);
        await git(["-C", source, "commit", "-m", "base"]);
        repositories.push({ key, repository_path: source, workspace_mode: "dedicated_worktree", source_type: "local", remote_name: "", base_branch: "main", target_branch: "task", carry_changes: false, worktree_path: join(home, "worktree-" + key) });
      }
      const request = "Preserve repository identities through relocation";
      const anchor = await inspect({ request, repositories: repositories.map(({ key, repository_path }) => ({ key, repository_path })) });
      const receipt = await prepare({ request, assessment: {
        change_level: "standard", candidate_components: ["workspace"], candidate_paths: ["base.txt"],
        public_contract_flags: ["repository identity"], persistence_or_state_flags: [], host_or_platform_flags: ["ZCode"],
        verification_shape: ["Core creation and relocation"], reasons: ["Preserve custom repository keys"], unknowns: [], anchor,
      }, user_choice: { source: "user", mode: "taskbelay", summary: "Confirmed workspaces" }, repositories,
      handoff: { discussion: [{ role: "user", text: request }] } }, options);
      await provision(receipt.launch_id, options);
      const creationScope = await scope(receipt.launch_id, options);
      assert.equal(creationScope.primary_repository_key, "backend");
      assert.deepEqual((creationScope.additional_repositories ?? []).map(repo => repo.key), keys.slice(1));

      const call = await coreClient(t, runtime, environment, home);
      const opened = await call("taskbelay_open_task", { host: "zcode", ...creationScope, new_task: {
        request, initial_scope: ["Workspace identity"], initial_out_of_scope: ["Application changes"],
        known_acceptance_criteria: ["Repository keys and files survive relocation"], method_profile: "plain",
      } });
      assert.equal(opened.created, true);
      assert.equal(opened.task.primary_repository_key, "backend");
      await bindTask(receipt.launch_id, { task_id: opened.task.task_id }, options);
      const renamed=await call("taskbelay_prepare_task_branch_rename",{host:"zcode",task_id:opened.task.task_id,revision:opened.task.revision,repository_key:"backend",target_branch:"task-renamed",reason:"Correct existing Task branch name"});
      await git(["-C",repositories[0].worktree_path,"branch","-m","task","task-renamed"]);
      opened.task=await call("taskbelay_resolve_blocker",{host:"zcode",task_id:opened.task.task_id,action_id:renamed.task.current_action.action_id,rename_id:renamed.rename_id,rename_choice:"complete"});
      assert.equal(opened.task.workspace_origin.task_branch,"task");
      assert.equal(opened.task.repository.current_branch,"task-renamed");
      await git(["-C",repositories[0].repository_path,"branch","task"]);
      assert.equal((await scope(receipt.launch_id,options)).workspace_origin.task_branch,"task");
      const prepared = await call("taskbelay_prepare_task_relocation", { host: "zcode", task_id: opened.task.task_id, revision: opened.task.revision });
      const destinations = repositories.map(repo => ({ repository_key: repo.key, repository_path: join(home, "relocated-" + repo.key) })).reverse();
      const moved = await relocate(receipt.launch_id, { relocation_id: prepared.relocation_id, destinations, authorized: true, core_preparation: prepared }, options);
      assert.deepEqual(moved.relocation_destinations.map(repo => repo.key), keys);
      const resolved = await call("taskbelay_resolve_blocker", { host: "zcode", task_id: opened.task.task_id,
        action_id: prepared.task.current_action.action_id, relocation_id: moved.relocation_id,
        relocation_destinations: moved.relocation_destinations });
      assert.equal(resolved.current_cursor, "REQUIREMENTS");
      assert.equal(resolved.blocker, null);
      assert.equal(resolved.primary_repository_key, "backend");
      assert.equal(resolved.workspace_origin.canonical_worktree_root, join(home, "relocated-backend"));
      assert.deepEqual((resolved.additional_repositories ?? []).map(repo => repo.key), keys.slice(1).sort());
      for (const destination of moved.relocation_destinations) {
        assert.equal(await readFile(join(destination.repository_path, "base.txt"), "utf8"), destination.key);
        assert.equal((await workspaceAvailable(destination.repository_path)).available, false);
      }
      const resumed = await call("taskbelay_open_task", { host: "zcode", repository_path: resolved.workspace_origin.canonical_worktree_root });
      assert.equal(resumed.created, false);
      assert.equal(resumed.task.task_id, opened.task.task_id);
      assert.equal(resumed.task.current_cursor, "REQUIREMENTS");
      const nextPreparation = await call("taskbelay_prepare_task_relocation", { host: "zcode", task_id: resumed.task.task_id, revision: resumed.task.revision });
      const nextDestinations = repositories.map(repo => ({ repository_key: repo.key, repository_path: join(home, "second-" + repo.key) }));
      const changedSource = structuredClone(nextPreparation);
      changedSource.task.workspace_origin.canonical_worktree_root = repositories[0].worktree_path;
      await assert.rejects(relocate(receipt.launch_id, { relocation_id: nextPreparation.relocation_id, destinations: nextDestinations,
        authorized: true, core_preparation: changedSource }, options), /source workspaces/);
      const nextMoveInput = { relocation_id: nextPreparation.relocation_id, destinations: nextDestinations, authorized: true, core_preparation: nextPreparation };
      const second = await relocate(receipt.launch_id, nextMoveInput, options);
      assert.deepEqual(await relocate(receipt.launch_id, nextMoveInput, options), second);
      const resolvedAgain = await call("taskbelay_resolve_blocker", { host: "zcode", task_id: resumed.task.task_id,
        action_id: nextPreparation.task.current_action.action_id, relocation_id: second.relocation_id,
        relocation_destinations: second.relocation_destinations });
      assert.equal(resolvedAgain.workspace_origin.canonical_worktree_root, join(home, "second-backend"));
      assert.equal(resolvedAgain.task_id, opened.task.task_id);
      assert.equal(resolvedAgain.blocker, null);
      const current=await call("taskbelay_get_task",{host:"zcode",task_id:opened.task.task_id});
      const terminal=await call("taskbelay_cancel_task",{request_id:"cancel-cleanup",host:"zcode",task_id:opened.task.task_id,revision:current.task.revision,reason:"End isolated cleanup verification"});
      const cleanupInput={repository_key:"backend",terminal:true,authorized:true,core_task:terminal};
      await cleanup(receipt.launch_id,"cleanup-worktree",cleanupInput,options);
      await cleanup(receipt.launch_id,"cleanup-branch",cleanupInput,options);
      assert.equal((await git(["-C",repositories[0].repository_path,"rev-parse","refs/heads/task"])).trim(),terminal.repository.current_head);
      await assert.rejects(git(["-C",repositories[0].repository_path,"rev-parse","--verify","refs/heads/task-renamed"]));
    });
  }
});

async function coreClient(t, runtime, environment, cwd) {
  const child = spawn(runtime, ["mcp", "--stdio"], { cwd, env: environment, stdio: ["pipe", "pipe", "pipe"], windowsHide: true });
  const closed = once(child, "close");
  let nextID = 0, stderr = "";
  const pending = new Map();
  const fail = error => { for (const request of pending.values()) request.reject(error); pending.clear(); };
  child.on("error", fail);
  child.on("close", code => fail(new Error(`Core exited ${code}: ${stderr}`)));
  child.stderr.on("data", chunk => { stderr += chunk; });
  const lines = createInterface({ input: child.stdout });
  lines.on("line", line => {
    try {
      const response = JSON.parse(line);
      pending.get(response.id)?.resolve(response);
    } catch (error) { fail(error); }
  });
  t.after(async () => { child.stdin.end(); if (child.exitCode === null) child.kill(); await closed; lines.close(); });
  const request = (method, params) => {
    const id = ++nextID;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { pending.delete(id); reject(new Error(`Core request timed out: ${method}: ${stderr}`)); }, 10_000);
      const finish = callback => value => { clearTimeout(timer); pending.delete(id); callback(value); };
      pending.set(id, { resolve: finish(resolve), reject: finish(reject) });
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");

    });
  };
  const initialized = await request("initialize", { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "zcode-workspace-test", version: "0.1.0" } });
  assert.equal(initialized.result.serverInfo.name, "taskbelay");
  child.stdin.write(JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }) + "\n");
  return async (name, arguments_) => {
    const response = await request("tools/call", { name, arguments: arguments_ });
    assert.equal(response.error, undefined, JSON.stringify(response.error));
    const envelope = JSON.parse(response.result.content.find(item => item.type === "text").text);
    if (response.result.structuredContent) assert.deepEqual(envelope, response.result.structuredContent);
    assert.equal(envelope.ok, true, JSON.stringify(envelope));
    return envelope.result;
  };
}
