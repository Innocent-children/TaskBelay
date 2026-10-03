import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { registerWorkspaceCoordinator } from "../lib/workspace-tool.mjs";
import { WORKSPACE_COORDINATOR_TOOL, workspaceConfirmationText, workspaceCleanupText, workspaceResumeText } from "../lib/workspace-coordinator.mjs";
import { readExamples, exampleNormalizer, verifySuccessExample } from "../../../tests/skills/executed-examples.mjs";

const root = fileURLToPath(new URL("../skills/taskbelay/", import.meta.url));
const examples = await readExamples(root, "workspace");
const launch = "11111111-1111-4111-8111-111111111111";
function execution(text, input) {
  const callId = "workspace-example";
  return {
    name: WORKSPACE_COORDINATOR_TOOL, callId, arguments: input, concludeTurn() {},
    agent: {
      status: "running", session: { snapshotEvents: () => [
        { seq: 0, type: "turn/start", data: { turn: 1 } },
        { seq: 1, type: "user/message", data: { id: "user", source: { kind: "user" }, content: [{ type: "text", text }] } },
        { seq: 2, type: "tool/call", data: { turn: 1, callId, name: WORKSPACE_COORDINATOR_TOOL } },
      ] },
    },
  };
}

for (const example of examples) {
  test(`complete Skill response workspace/${example.name}`, async (t) => {
    const dir = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-skill-deepseek-")));
    t.after(() => rm(dir, { recursive: true, force: true }));
    const source = join(dir, "source"), data = join(dir, "data");
    await mkdir(data, { mode: 0o700 });
    const git = (cwd, ...args) => execFileSync("git", ["-C", cwd, ...args], {
      encoding: "utf8", stdio: ["pipe", "pipe", "pipe"],
      env: { ...process.env, GIT_AUTHOR_DATE: "2026-09-10T00:00:00Z", GIT_COMMITTER_DATE: "2026-09-10T00:00:00Z" },
    }).trim();
    execFileSync("git", ["init", "--initial-branch=main", source], { stdio: "pipe" });
    git(source, "config", "user.name", "Skill Example");
    git(source, "config", "user.email", "skill@example.invalid");
    git(source, "config", "core.autocrlf", "false");
    await writeFile(join(source, "README.md"), "Example repository.\n");
    git(source, "add", ".");
    git(source, "commit", "-m", "Example starting point");
    execFileSync("git", ["clone", "--bare", source, join(dir, "remote.git")], { stdio: "pipe" });
    git(source, "remote", "add", "origin", join(dir, "remote.git"));

    let terminalTask;
    const call = async (input, workspaceRoot = source) => {
      let tool;
      registerWorkspaceCoordinator({ tools: {
        guard() { return () => {}; },
        register(value) { tool = value; return () => {}; },
      } }, {
        dataDirectory: data, workspaceRoot, launchID: () => launch,
        now: () => new Date("2026-09-10T00:00:00Z"),
        checkWorkspaceAvailable: async (repository_path) => ({ available: true, repository_path }),
        readTask: async () => terminalTask,
      });
      let text;
      if (input.operation === "provision") text = workspaceConfirmationText(input.repositories);
      else if (input.operation === "consume") text = workspaceResumeText(input.launch_id);
      else text = workspaceCleanupText(input.operation, {
        launchID: input.launch_id, repositoryKey: input.repository_key, taskID: input.task_id, revision: input.revision,
      });
      const result = await tool.execute(input, execution(text, input));
      const rendered = tool.output?.render?.(input, result);
      if (rendered) assert.deepEqual(JSON.parse(rendered[0].text), result);
      return result;
    };
    const input = structuredClone(example.input);
    if (input.source_repository_path) input.source_repository_path = source;
    if (input.repositories) input.repositories.forEach((repository) => repository.source_repository_path = source);
    let output;
    if (input.operation === "provision") {
      await writeFile(join(source, "notes.txt"), "Confirmed local notes.\n");
      output = await call(input);
    } else {
      const provision = structuredClone(examples.find((entry) => entry.name === "provision-worktree").input);
      provision.repositories[0].source_repository_path = source;
      const cleanup = input.operation !== "consume";
      if (cleanup) {
        Object.assign(provision.repositories[0], { source_type: "remote", carry_changes: false, remote_name: "origin" });
      }
      const opened = await call(provision);
      if (!cleanup) {
        output = await call(input, opened.workspace_root);
      } else {
        const consumed = await call({ operation: "consume", launch_id: launch }, opened.workspace_root);
        git(opened.workspace_root, "push", "-u", "origin", "feature/endpoint-field");
        terminalTask = {
          task_id: "task-example", revision: 9, current_cursor: "DONE", primary_repository_key: "primary",
          workspace_origin: { ...consumed.open_task.workspace_origin, canonical_worktree_root: opened.workspace_root },
          repository: {
            current_head: git(opened.workspace_root, "rev-parse", "HEAD"),
            current_branch: git(opened.workspace_root, "symbolic-ref", "--short", "HEAD"),
            detached: git(opened.workspace_root, "rev-parse", "--abbrev-ref", "HEAD") === "HEAD",
          }, additional_repositories: [],
        };
        if (input.operation === "cleanup_branch") {
          await call({ operation: "cleanup_worktree", launch_id: launch, repository_key: "primary", task_id: "task-example", revision: 9 }, source);
        }
        output = await call(input, input.operation === "prepare_cleanup" ? opened.workspace_root : source);
      }
    }
    const normalize = exampleNormalizer([[source, "/work/project"], [data, "/example/data"], [dir, "/example"]]);
    await verifySuccessExample(example, input, output, normalize);
  });
}
