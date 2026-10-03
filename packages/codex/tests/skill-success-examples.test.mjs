import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { runCLI } from "../bin/taskbelay-codex.mjs";
import { inspectAdmissionAnchor } from "../lib/task-admission.mjs";
import { handoffFixture } from "./fixtures/task-handoff.mjs";
import { readExamples, exampleNormalizer, verifySuccessExample } from "../../../tests/skills/executed-examples.mjs";

const root = fileURLToPath(new URL("../plugin/skills/taskbelay/", import.meta.url));
const examples = await readExamples(root, "host");
test("Skill response normalization maps native paths and quoted paths without changing other text", () => {
  const directory = join(tmpdir(), "taskbelay-normalizer");
  const path = join(directory, "handoffs", "material.md");
  const normalize = exampleNormalizer([[directory, "/example"]]);
  const unchanged = String.raw`Keep \n and "C:\unmapped\path" unchanged.`;
  assert.deepEqual(normalize({ path, prompt: `Read ${JSON.stringify(path)}. ${unchanged}`, unchanged }), {
    path: "/example/handoffs/material.md",
    prompt: `Read "/example/handoffs/material.md". ${unchanged}`,
    unchanged,
  });
});

for (const example of examples) {
  test(`complete Skill response ${example.tool}/${example.name}`, async (t) => {
    const dir = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-skill-codex-")));
    t.after(() => rm(dir, { recursive: true, force: true }));
    const source = join(dir, "source");
    const worktree = join(dir, "worktree");
    const relocated = join(dir, "relocated");
    const support = join(dir, "support");
    const handoff = join(dir, "handoff.json");
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

    const paths = new Map([
      ["/work/project", source], ["/work/tasks/endpoint-field", worktree],
      ["/work/tasks/relocated-endpoint", relocated], ["/private/tmp/taskbelay-handoff.json", handoff],
    ]);
    const substitute = (value) => {
      if (typeof value === "string") return paths.get(value) ?? value;
      if (Array.isArray(value)) return value.map(substitute);
      if (value && typeof value === "object") {
        return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, substitute(item)]));
      }
      return value;
    };
    const find = (tool, name) => examples.find((entry) => entry.tool === tool && (name === undefined || entry.name === name));
    const call = async (tool, input) => {
      let output = "", error = "";
      const result = await runCLI(["host-launch", tool], {
        resolvePaths: () => ({ productSupportRoot: support, enforcePrivateModes: true }),
        readInput: () => JSON.stringify(input),
        stdout: { write: (value) => output += value }, stderr: { write: (value) => error += value },
        createLaunchId: () => "launch-example", now: () => new Date("2026-09-10T00:00:00Z"),
        checkWorkspaceAvailable: async (repository_path) => ({ available: true, repository_path }),
      });
      assert.equal(result.code, 0, `${tool}: ${error}`);
      return JSON.parse(output);
    };
    const identity = { launch_id: "launch-example", repository_key: "primary" };
    let dispatch, claimed, provisioned;
    const prepare = async (name, key = "primary", path = source) => {
      const input = substitute(find("prepare", name).input);
      input.repository_key = key;
      input.repository_path = path;
      if (key !== "primary") {
        input.launch_id = "launch-example";
        input.worktree_path = join(dir, key);
        input.target_branch = `codex/${key}`;
      }
      await writeFile(handoff, JSON.stringify(handoffFixture(input.request)), { mode: 0o600 });
      input.assessment.observed_repositories = [path];
      input.assessment.anchor = await inspectAdmissionAnchor({ request: input.request, repositories: [{ key, repository_path: path }] });
      return { input, output: await call("prepare", input) };
    };
    const start = async () => {
      dispatch = await call("dispatch-start", { ...identity, project_id: "project-example" });
    };
    const claim = async () => {
      await start();
      claimed = await call("dispatch-call", { ...identity, dispatch_attempt_id: dispatch.receipt.operation_status.dispatch_attempt_id });
    };
    const provision = async () => {
      await prepare("remote-cli");
      provisioned = await call("cli-provision", { ...identity, source_repository_path: source, additional_worktree_paths: [] });
    };
    let input = substitute(example.input), output;
    if (example.tool === "prepare") {
      ({ input, output } = await prepare(example.name));
    } else if (example.tool === "inspect" || example.tool === "cleanup-decision") {
      output = await call(example.tool, input);
    } else {
      switch (example.tool) {
        case "supersede":
        case "supersede-resume": {
          const prepared = await prepare("local-managed");
          const status = await call("status", identity);
          const replacement = {...prepared.input, launch_id:"launch-successor", target_branch:"codex/revised-endpoint"};
          const supersedeInput = {...identity, expected_receipt_digest:status.receipt_digest, reason:"Use the corrected branch name before execution.", replacement};
          if (example.tool === "supersede") input = supersedeInput;
          else {
            const replaced = await call("supersede", supersedeInput);
            input = {...identity, seed_digest:replaced.predecessor.supersession.seed_digest};
          }
          break;
        }
        case "status":
        case "dispatch-start":
          await prepare("local-managed");
          break;
        case "local-provision":
          await prepare("local-branch");
          break;
        case "dispatch-call":
          await prepare("local-managed");
          await start();
          input.dispatch_attempt_id = dispatch.receipt.operation_status.dispatch_attempt_id;
          break;
        case "dispatch-result":
          await prepare("local-managed");
          await claim();
          break;
        case "dispatch-recover":
          await prepare("local-managed");
          await claim();
          input.dispatch_attempt_id = claimed.receipt.operation_status.dispatch_attempt_id;
          break;
        case "dispatch-reconcile":
          await prepare("local-managed");
          await claim();
          input.candidates[0].initial_prompt = claimed.host_request.prompt;
          break;
        case "bootstrap":
          await prepare("local-managed");
          await claim();
          await call("dispatch-result", { ...identity, host_result: { threadId: "thread-example", hostId: "local" } });
          git(source, "worktree", "add", "--detach", worktree, "main");
          break;
        case "cli-provision":
          await prepare("remote-cli");
          break;
        case "scope":
          if (example.name === "single") {
            await provision();
          } else {
            for (const key of ["api", "web"]) {
              const repo = join(dir, `source-${key}`);
              execFileSync("git", ["clone", join(dir, "remote.git"), repo], { stdio: "pipe" });
              await prepare("remote-cli", key, repo);
              await call("cli-provision", { launch_id: "launch-example", repository_key: key, source_repository_path: repo, additional_worktree_paths: [] });
            }
          }
          break;
        case "handoff-start":
        case "handoff-result":
        case "handoff-status":
          await provision();
          if (example.tool !== "handoff-start") {
            await call("handoff-start", { ...identity, relocation_id: "relocation-example", thread_id: "thread-example" });
          }
          if (example.tool === "handoff-status") {
            await call("handoff-result", substitute(find("handoff-result").input));
            if (example.name === "succeeded") git(source, "worktree", "move", worktree, relocated);
          }
          break;
        case "cleanup-worktree":
        case "cleanup-branch":
          await provision();
          git(worktree, "push", "-u", "origin", "codex/endpoint-field");
          input.core_task.workspace_origin = {...provisioned.workspace_origin, canonical_worktree_root:worktree};
          input.core_task.repository.current_head = git(worktree,"rev-parse","HEAD");
          if (example.tool === "cleanup-branch") {
            await call("cleanup-worktree", { ...identity, source_repository_path: source, terminal: true, authorized: true, core_task:input.core_task });
          }
          break;
        default:
          assert.fail(`no setup for ${example.tool}`);
      }
      output = await call(example.tool, input);
    }
    const normalize = exampleNormalizer([
      ...[...paths].map(([label, path]) => [path, label]), [support, "/example/support"], [dir, "/example"],
    ]);
    await verifySuccessExample(example, input, output, normalize);
  });
}
