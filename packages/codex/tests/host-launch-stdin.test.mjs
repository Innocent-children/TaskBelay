import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtemp, mkdir, readdir, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import test from "node:test";

const launcher = fileURLToPath(new URL("../bin/taskbelay-codex.mjs", import.meta.url));
const supported = process.platform === "darwin" || process.platform === "win32";

async function fixture(t) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-stdin-")));
  t.after(() => rm(root, { recursive: true, force: true }));
  const home = join(root, "home");
  const repository = join(root, "repository");
  await mkdir(home);
  await mkdir(repository);
  const env = { ...process.env, HOME: home, USERPROFILE: home, LOCALAPPDATA: home,
    TASKBELAY_DATA_DIR: "", GIT_CONFIG_GLOBAL: join(home, ".gitconfig"), GIT_CONFIG_NOSYSTEM: "1" };
  const git = (...args) => execFileSync("git", ["-C", repository, ...args], { env, encoding: "utf8" });
  git("init", "-b", "main");
  git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "fixture");
  return { root, home, repository, env, git };
}

async function run(f, operation, chunks, delay = 0) {
  const child = spawn(process.execPath, [launcher, "host-launch", operation], {
    env: { ...f.env, GIT_TRACE: join(f.home, "git-trace") },
    cwd: f.repository, stdio: ["pipe", "pipe", "pipe"],
  });
  let stdout = "";
  let stderr = "";
  child.stdout.setEncoding("utf8").on("data", (chunk) => { stdout += chunk; });
  child.stderr.setEncoding("utf8").on("data", (chunk) => { stderr += chunk; });
  // Oversized input may close the pipe before the writer finishes.
  child.stdin.on("error", (error) => { assert.ok(["EPIPE", "ECONNRESET"].includes(error.code)); });
  const result = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => resolve({ code, signal, stdout, stderr }));
  });
  for (const chunk of chunks) {
    child.stdin.write(chunk);
    if (delay) await setTimeout(delay);
  }
  child.stdin.end();
  return await result;
}

test("production host-launch inspect reads piped UTF-8 across writes", { skip: !supported, timeout: 15000 }, async (t) => {
  const f = await fixture(t);
  const request = "排查中文输入";
  const bytes = Buffer.from(JSON.stringify({ request, repositories: [{ key: "source", repository_path: f.repository }] }));
  const boundary = bytes.indexOf(Buffer.from("中"));
  for (const chunks of [[bytes], [bytes.subarray(0, boundary + 1), bytes.subarray(boundary + 1, boundary + 2), bytes.subarray(boundary + 2)]]) {
    const result = await run(f, "inspect", chunks, 50);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(result.signal, null);
    assert.equal(result.stderr, "");
    const value = JSON.parse(result.stdout);
    assert.equal(value.request_digest, createHash("sha256").update(request).digest("hex"));
    assert.equal(value.repositories[0].canonical_root, f.repository);
    assert.equal(value.repositories[0].head, f.git("rev-parse", "HEAD").trim());
    assert.deepEqual(value.repositories[0].dirty_paths, []);
  }
  assert.deepEqual(await readdir(f.home), ["git-trace"]);
});

test("production host-launch rejects invalid input before Git or receipt writes", { skip: !supported, timeout: 15000 }, async (t) => {
  const f = await fixture(t);
  const cases = [
    ["malformed", '{"request":', /one JSON object/],
    ["duplicate", '{"request":"a","request":"b"}', /duplicate field request/],
    ["nested duplicate", '{"nested":{"a":1,"\\u0061":2}}', /duplicate field a/],
    ["array", "[]", /one JSON object/],
    ["null", "null", /one JSON object/],
    ["empty", "", /one JSON object/],
    ["oversized", JSON.stringify({ request: "中".repeat(350000) }), /no larger than 1 MiB/],
    ["invalid UTF-8", Buffer.from([123, 34, 120, 34, 58, 34, 255, 34, 125]), /UTF-8/],
  ];
  for (const [name, input, error] of cases) {
    await t.test(name, async () => {
      const result = await run(f, "prepare", [input]);
      assert.equal(result.code, 1);
      assert.equal(result.stdout, "");
      assert.match(result.stderr, /^taskbelay-codex: /);
      assert.match(result.stderr, error);
      assert.deepEqual(await readdir(f.home), [], "no Git trace or product records");
    });
  }
});

test("production host-launch accepts exactly 1 MiB", { skip: !supported, timeout: 15000 }, async (t) => {
  const f = await fixture(t);
  const input = JSON.stringify({ lifecycle: "done", surface: "cli_worktree", clean: true, pushed: true, stateCertain: true });
  const result = await run(f, "cleanup-decision", [input, " ".repeat(1024 * 1024 - Buffer.byteLength(input))]);
  assert.equal(result.code, 0, result.stderr);
  assert.equal(result.stderr, "");
  assert.equal(JSON.parse(result.stdout).automatic_cleanup, false);
  assert.deepEqual(await readdir(f.home), []);
});
