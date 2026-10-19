import assert from "node:assert/strict";
import { mkdir, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { createRun, readRun, recordRun } from "../lib/journal.mjs";
import { resolveManagerPaths } from "../lib/ownership.mjs";

test("operation journal is atomic, closed, and retains exact completed external effects", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-journal-"));
  const home = join(root, "home");
  await mkdir(home);
  const paths = await resolveManagerPaths({ homeDirectory: home, environment: {}, platform: "darwin", arch: "arm64" });
  const plan = { planId: "plan-fixture", digest: "a".repeat(64) };
  const run = await createRun(paths, plan, {
    operationId: "operation-fixture-0001", now: () => new Date("2026-08-25T00:00:00Z"),
  });
  const updated = await recordRun(paths, run, {
    completed_action_ids: ["codex.install"], next_step: "continue",
  }, { now: () => new Date("2026-08-25T00:01:00Z") });
  assert.deepEqual(await readRun(paths, run.operation_id), updated);
  assert.deepEqual(updated.completed_action_ids, ["codex.install"]);
  t.after(async () => { const { rm } = await import("node:fs/promises"); await rm(root, { recursive: true, force: true }); });
});
