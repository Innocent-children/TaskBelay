import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import test from "node:test";

import * as codexPlatform from "../../packages/codex/lib/platform.mjs";
import { hookDecision, preparedWriteFromHook } from "../../packages/codex/plugin/hooks/pre-tool-use.mjs";
import { hasDirectUserSelector } from "../../packages/deepseek/lib/authorization.mjs";
import * as deepseekPlatform from "../../packages/deepseek/lib/platform.mjs";
import * as managerPlatform from "../../packages/taskbelay/lib/platform.mjs";

const repositoryRoot = join(import.meta.dirname, "../..");
const runtimeKeys = ["darwin-arm64", "win32-x64"];

const readJSON = async (relative) => JSON.parse(await readFile(join(repositoryRoot, relative), "utf8"));

test("all public packages select the same closed runtime pair set", () => {
  for (const implementation of [codexPlatform, deepseekPlatform, managerPlatform]) {
    assert.deepEqual(implementation.SUPPORTED_RUNTIME_KEYS, runtimeKeys);
    assert.equal(implementation.runtimeDescriptor("darwin", "arm64").runtimeExecutable, "taskbelay");
    assert.equal(implementation.runtimeDescriptor("win32", "x64").runtimeExecutable, "taskbelay.exe");
    for (const [platform, arch] of [["darwin", "x64"], ["win32", "arm64"], ["win32", "ia32"], ["linux", "x64"]]) {
      assert.throws(() => implementation.runtimeDescriptor(platform, arch), /unsupported platform/u);
    }
  }
});

test("package manifests expose complete test and runtime contracts", async () => {
  const codex = await readJSON("packages/codex/package.json");
  const deepseek = await readJSON("packages/deepseek/package.json");
  const manager = await readJSON("packages/taskbelay/package.json");
  const webui = await readJSON("packages/webui/package.json");

  for (const manifest of [codex, deepseek]) {
    assert.deepEqual(manifest.os, ["darwin", "win32"]);
    assert.deepEqual(manifest.cpu, ["arm64", "x64"]);
    assert.match(manifest.scripts.test, /node --test tests\/\*\.test\.mjs/u);
    for (const file of ["runtime/darwin-arm64/taskbelay", "runtime/win32-x64/taskbelay.exe"]) {
      assert.ok(manifest.files.includes(file), `${manifest.name} omits ${file}`);
    }
  }
  assert.match(manager.scripts.test, /node --test tests\/\*\.test\.mjs/u);
  assert.equal(webui.scripts.typecheck, "tsc --noEmit");
  assert.match(webui.scripts.build, /tsc --noEmit && vite build/u);
});

test("repository validation uses the current integrated engineering entrypoint", async () => {
  const validation = await readFile(join(repositoryRoot, "scripts/validate-repository.sh"), "utf8");
  assert.match(validation, /exec node scripts\/validate-engineering.mjs/u);
  const current = await readFile(join(repositoryRoot, "scripts/validate-engineering.mjs"), "utf8");
  for (const contract of ["'vet','./...'", "'test','-count=1'", "tests/journeys/engineering.test.mjs", "tests/host-engineering.test.mjs", "scripts/build-webui.mjs"])
    assert.ok(current.includes(contract), contract);
});

test("publication remains manual and separate from product qualification", async () => {
  const workflow = await readFile(join(repositoryRoot, ".github/workflows/publish-npm.yml"), "utf8");
  assert.match(workflow, /workflow_dispatch:/u);
  assert.doesNotMatch(workflow, /^  (?:pull_request|push|schedule):/mu);
  assert.match(workflow, /group: npm-release/u);
  assert.match(workflow, /cancel-in-progress: false/u);
  assert.doesNotMatch(workflow, /NODE_AUTH_TOKEN|NPM_TOKEN/u);
});

test("Codex and DeepSeek structured admission boundaries are conservative", () => {
  const directUserMessage = (text, kind = "user") => ({
    source: { kind },
    content: [{ type: "text", text }],
  });
  for (const text of ["/taskbelay", "please /taskbelay continue", "line one\n/taskbelay\nline three"]) {
    assert.equal(hasDirectUserSelector(directUserMessage(text)), true, text);
  }
  for (const text of ["/taskbelay,", "/taskbelayx", "path/taskbelay", "ordinary request"]) {
    assert.equal(hasDirectUserSelector(directUserMessage(text)), false, text);
  }
  assert.equal(hasDirectUserSelector(directUserMessage("/taskbelay", "plugin")), false);

  const patch = preparedWriteFromHook({
    hook_event_name: "PreToolUse",
    tool_name: "apply_patch",
    cwd: "/workspace/core",
    tool_input: { command: "*** Begin Patch\n*** Update File: src/a.go\n*** Add File: src/b.go\n*** End Patch" },
  });
  assert.equal(patch.path_parse_complete, true);
  assert.deepEqual(patch.paths, [resolve("/workspace/core/src/a.go"), resolve("/workspace/core/src/b.go")]);
  assert.match(patch.intent_digest, /^[0-9a-f]{64}$/u);
  assert.equal(preparedWriteFromHook({ hook_event_name: "PreToolUse", tool_name: "shell", cwd: "/workspace/core", tool_input: {} }), undefined);
  assert.equal(hookDecision({ decision: "allow" }), null);
  assert.equal(hookDecision({ decision: "future" }), undefined);
});
