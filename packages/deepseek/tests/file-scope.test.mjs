import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { PassThrough } from "node:stream";
import test from "node:test";

import { preparedWrite, registerFileScopeGate } from "../lib/file-scope.mjs";

test("DeepSeek structured write paths and intent digests are stable", (t) => {
  const workspaceRoot = workspaceDirectory(t);
  mkdirSync(join(workspaceRoot, "docs"));
  const execution = { name: "edit", arguments: { file_path: "docs/guide.md", old_string: "a", new_string: "b" } };
  const first = preparedWrite(execution, workspaceRoot);
  const reordered = preparedWrite({ name: "edit", arguments: { new_string: "b", old_string: "a", file_path: "docs/guide.md" } }, workspaceRoot);
  const expectedPath = resolve(workspaceRoot, "docs", "guide.md");
  assert.deepEqual(first.paths, [expectedPath]);
  assert.equal(first.repository_path, join(workspaceRoot, "docs"));
  assert.equal(first.path_parse_complete, true);
  assert.equal(first.intent_digest, reordered.intent_digest);
});

test("DeepSeek new-file lookup stays in the target repository under a shared Workspace Root", (t) => {
  const workspaceRoot = workspaceDirectory(t);
  for (const name of ["api", "web"]) {
    const repositoryRoot = join(workspaceRoot, name);
    mkdirSync(repositoryRoot);
    const request = preparedWrite({ name: "write", arguments: { file_path: `${name}/new/nested/file.txt`, content: "new" } }, workspaceRoot);
    assert.equal(request.repository_path, repositoryRoot);
    assert.deepEqual(request.paths, [join(repositoryRoot, "new", "nested", "file.txt")]);
    assert.equal(request.path_parse_complete, true);
  }
});

test("DeepSeek stops invalid parent paths before dispatch or Core lookup", async (t) => {
  const workspaceRoot = workspaceDirectory(t);
  writeFileSync(join(workspaceRoot, "file"), "not a directory");
  let listener;
  let spawns = 0;
  let dispatched = 0;
  const ctx = { on: (_event, callback) => { listener = callback; return () => undefined; } };
  registerFileScopeGate(ctx, { runtimePath: "/runtime/taskbelay", dataDirectory: "/data", workspaceRoot, spawnImpl: () => { spawns += 1; } });
  const result = await listener(selectedExecution("write", { file_path: "file/new/target.txt", content: "x" }), async () => { dispatched += 1; });
  assert.equal(result.kind, "deny");
  assert.equal(spawns, 0);
  assert.equal(dispatched, 0);
});

test("DeepSeek pre-execute gate blocks a selected TaskBelay write before dispatch", async () => {
  let listener;
  const ctx = { on: (event, callback) => { assert.equal(event, "tools/pre-execute"); listener = callback; return () => undefined; } };
  registerFileScopeGate(ctx, { runtimePath: "/runtime/taskbelay", dataDirectory: "/data", workspaceRoot: "/workspace", spawnImpl: fakeSpawn({ decision: "deny", reason: "Choose allow_once, expand_scope or reject." }) });
  let dispatched = 0;
  const result = await listener(selectedExecution("write", { file_path: "/workspace/config.yml", content: "x" }), async () => { dispatched += 1; return { kind: "allow" }; });
  assert.deepEqual(result, { kind: "deny", reason: "Choose allow_once, expand_scope or reject." });
  assert.equal(dispatched, 0);
});

test("DeepSeek gate retains scope checks across ordinary follow-ups and bypasses read-only calls", async () => {
  let listener;
  let spawns = 0;
  const ctx = { on: (_event, callback) => { listener = callback; return () => undefined; } };
  registerFileScopeGate(ctx, { runtimePath: "/runtime/taskbelay", dataDirectory: "/data", workspaceRoot: "/workspace", spawnImpl: (...args) => { spawns += 1; return fakeSpawn({ decision: "allow" })(...args); } });
  let dispatched = 0;
  await listener(selectedExecution("str_replace_editor", { command: "view", path: "/workspace/file" }), async () => { dispatched += 1; return { kind: "allow" }; });
  await listener(ordinaryExecution("write", { file_path: "/workspace/file", content: "x" }), async () => { dispatched += 1; return { kind: "allow" }; });
  assert.equal(dispatched, 2);
  assert.equal(spawns, 1);
});

function selectedExecution(name, argumentsValue, text = "/taskbelay continue") {
  const callId = "scope-call";
  return {
    name,
    arguments: argumentsValue,
    callId,
    signal: new AbortController().signal,
    agent: { status: "running", session: { snapshotEvents: () => [
      { seq: 0, type: "turn/start", data: { turn: 1 } },
      { seq: 1, type: "user/message", data: { id: "user", source: { kind: "user" }, content: [{ type: "text", text }] } },
      { seq: 2, type: "tool/call", data: { turn: 1, callId, name } },
    ] } },
  };
}

function ordinaryExecution(name, argumentsValue) {
  return selectedExecution(name, argumentsValue, "ordinary turn");
}

function workspaceDirectory(t) {
  const directory = mkdtempSync(join(tmpdir(), "deepseek-file-scope-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function fakeSpawn(result) {
  return () => {
    const child = new EventEmitter();
    child.stdout = new PassThrough();
    child.stderr = new PassThrough();
    child.kill = () => undefined;
    child.stdin = { end: () => queueMicrotask(() => {
      child.stdout.write(`${JSON.stringify(result)}\n`);
      child.stdout.end();
      child.emit("exit", 0, null);
    }) };
    return child;
  };
}
