import { randomUUID } from "node:crypto";
import { join } from "node:path";

import { ensureManagerDirectories, readOwnedJSON, writeOwnedJSON } from "./ownership.mjs";

export async function createRun(paths, plan, { now = () => new Date(), operationId = randomUUID() } = {}) {
  await ensureManagerDirectories(paths);
  const run = {
    operation_id: operationId,
    plan_id: plan.planId,
    plan_digest: plan.digest,
    completed_action_ids: [],
    failed_action_id: null,
    temporary_roots: [],
    trash_root: null,
    next_step: "resume",
    updated_at: now().toISOString(),
  };
  await writeRun(paths, run);
  return run;
}

export async function readRun(paths, operationId) {
  return await readOwnedJSON(runPath(paths, operationId), { root: paths.managerRoot, validate: validateRun });
}

export async function recordRun(paths, run, patch, { now = () => new Date() } = {}) {
  const updated = validateRun({ ...run, ...patch, updated_at: now().toISOString() });
  await writeRun(paths, updated);
  return updated;
}

export function validateRun(value) {
  const expected = ["operation_id", "plan_id", "plan_digest", "completed_action_ids", "failed_action_id", "temporary_roots", "trash_root", "next_step", "updated_at"];
  if (value === null || typeof value !== "object" || Array.isArray(value) || JSON.stringify(Object.keys(value).sort()) !== JSON.stringify(expected.sort())) {
    throw new Error("lifecycle run fields are invalid");
  }
  for (const field of ["operation_id", "plan_id", "plan_digest", "next_step"]) if (typeof value[field] !== "string" || value[field] === "") throw new Error(`lifecycle run ${field} is invalid`);
  if (!Array.isArray(value.completed_action_ids) || !Array.isArray(value.temporary_roots)) throw new Error("lifecycle run arrays are invalid");
  if (value.failed_action_id !== null && typeof value.failed_action_id !== "string") throw new Error("lifecycle run failed_action_id is invalid");
  if (value.trash_root !== null && typeof value.trash_root !== "string") throw new Error("lifecycle run trash_root is invalid");
  if (!Number.isFinite(Date.parse(value.updated_at))) throw new Error("lifecycle run updated_at is invalid");
  return structuredClone(value);
}

async function writeRun(paths, run) {
  validateRun(run);
  await writeOwnedJSON(runPath(paths, run.operation_id), run, {
    root: paths.managerRoot,
    enforcePrivateModes: paths.enforcePrivateModes,
  });
}

function runPath(paths, operationId) {
  if (!/^[0-9A-Za-z-]{8,80}$/u.test(operationId)) throw new Error("operation identity is invalid");
  return join(paths.runsDirectory, `${operationId}.json`);
}
