import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { hookDecision, preparedWriteFromHook, runHook } from "../plugin/hooks/pre-tool-use.mjs";

test("Codex apply_patch hook extracts every target and keeps one stable intent", () => {
  const event = {
    hook_event_name: "PreToolUse",
    tool_name: "apply_patch",
    cwd: "/workspace/core",
    tool_input: { command: "*** Begin Patch\n*** Update File: src/a.go\n*** Move to: ../docs/a.go\n*** Add File: src/b.go\n*** End Patch" },
  };
  const first = preparedWriteFromHook(event);
  const second = preparedWriteFromHook(JSON.parse(JSON.stringify(event)));
  assert.deepEqual(first.paths, [
    resolve("/workspace/core/src/a.go"),
    resolve("/workspace/core/src/b.go"),
    resolve("/workspace/docs/a.go"),
  ].sort());
  assert.equal(first.path_parse_complete, true);
  assert.equal(first.intent_digest, second.intent_digest);
  event.tool_input.command += "\n";
  assert.notEqual(preparedWriteFromHook(event).intent_digest, first.intent_digest);
});

test("Codex hook decisions use the supported blocking output", () => {
  assert.equal(hookDecision({ decision: "allow" }), null);
  assert.deepEqual(hookDecision({ decision: "deny", reason: "Choose a file-scope decision." }), {
    systemMessage: "Choose a file-scope decision.",
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: "Choose a file-scope decision.",
    },
  });
  assert.equal(hookDecision({ decision: "future" }), undefined);
});

test("Codex hook permits absent Task data, uses the managed launcher, and fails closed for a check failure", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-hook-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const event = JSON.stringify({
    hook_event_name: "PreToolUse",
    tool_name: "apply_patch",
    cwd: "/workspace/core",
    tool_input: { command: "*** Begin Patch\n*** Update File: src/a.go\n*** End Patch" },
  });
  const writes = [];
  const absent = runHook({ readInput: () => event, output: { write: (value) => writes.push(value) }, error: { write: () => undefined }, environment: { HOME: root, PLUGIN_ROOT: join(root, "plugin") }, platform: "darwin", arch: "arm64" });
  assert.equal(absent, 0);
  assert.deepEqual(writes, []);

  let invocation;
  const allowed = runHook({
    readInput: () => event,
    output: { write: (value) => writes.push(value) },
    error: { write: () => undefined },
    environment: { TASKBELAY_DATA_DIR: root },
    platform: "darwin",
    arch: "arm64",
    spawn: (executable, arguments_, options) => {
      invocation = { executable, arguments_, options };
      return { status: 0, stdout: JSON.stringify({ ok:true, result:{ decision: "allow" } }), stderr: "" };
    },
  });
  assert.equal(allowed, 0);
  assert.equal(invocation.executable, process.execPath);
  assert.deepEqual(invocation.arguments_, [
    fileURLToPath(new URL("../bin/taskbelay-codex.mjs", import.meta.url)),
    "host-check",
    "pre-file-write",
  ]);
  assert.equal(invocation.options.cwd, "/workspace/core");
  assert.equal(JSON.parse(invocation.options.input).repository_path, "/workspace/core");

  const errors = [];
  const failed = runHook({
    readInput: () => event,
    output: { write: () => undefined },
    error: { write: (value) => errors.push(value) },
    environment: { TASKBELAY_DATA_DIR: root, PLUGIN_ROOT: join(root, "plugin") },
    platform: "darwin",
    arch: "arm64",
    spawn: () => ({ status: 1, stdout: "", stderr: "failed" }),
  });
  assert.equal(failed, 2);
  assert.match(errors.join(""), /write was stopped/u);
});

test("Windows hook fallback prefers USERPROFILE when HOME is also present", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-hook-windows-home-"));
  const userProfile = join(root, "ordinary-user");
  const gitHome = join(root, "git-home");
  const dataDirectory = join(userProfile, "AppData", "Local", "taskbelay", "data");
  await mkdir(dataDirectory, { recursive: true });
  t.after(() => rm(root, { recursive: true, force: true }));
  const event = JSON.stringify({
    hook_event_name: "PreToolUse",
    tool_name: "apply_patch",
    cwd: resolve(root),
    tool_input: { command: "*** Begin Patch\n*** Update File: src/a.go\n*** End Patch" },
  });
  let invocation;
  const code = runHook({
    readInput: () => event,
    output: { write: () => undefined },
    error: { write: () => undefined },
    environment: { USERPROFILE: userProfile, HOME: gitHome },
    platform: "win32",
    arch: "x64",
    spawn: (executable, arguments_, options) => {
      invocation = { executable, arguments_, options };
      return { status: 0, stdout: JSON.stringify({ ok:true, result:{ decision: "allow" } }), stderr: "" };
    },
  });
  assert.equal(code, 0);
  assert.equal(invocation.options.env.TASKBELAY_DATA_DIR, dataDirectory);
});
