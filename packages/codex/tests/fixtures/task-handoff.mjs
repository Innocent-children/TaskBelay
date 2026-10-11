import { randomUUID } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";

export function handoffFixture(request) {
  return {
    request,
    goal: request,
    confirmed_requirements: [{ text: request, source_ids: ["m1"] }],
    terminology: [],
    scope_and_constraints: [],
    investigation: [],
    work_requirements: [],
    unconfirmed_suggestions: [],
    assumptions: [],
    open_questions: [],
    discussion: [{ id: "m1", role: "user", text: request }],
  };
}

export async function writeHandoffFixture(directory, request, changes = {}) {
  const path = join(directory, `handoff-${randomUUID()}.json`);
  await writeFile(path, JSON.stringify({ ...handoffFixture(request), ...changes }), { mode: 0o600 });
  return path;
}

export function admissionFixture() { return {authorization_ref:"fixture-user-workspace-authorization"}; }
export function receiptAdmissionFixture() { return "fixture-user-workspace-authorization"; }
