import assert from "node:assert/strict";
import test from "node:test";
import { sameTaskVersion, SelectionRequests } from "../src/lib/requests.ts";
import {
  callTask,
  call,
  SavedStateRequired,
  RequestError,
} from "../src/lib/api.ts";

test("switching tasks or refreshing discards earlier read and mutation responses", () => {
  const requests = new SelectionRequests();
  const first = requests.select("first");
  const mutation = requests.capture();
  requests.select("second");
  assert.equal(requests.current(first, "first"), false);
  assert.equal(requests.current(mutation.sequence, mutation.id), false);
  const newer = requests.select("first");
  assert.equal(requests.current(first, "first"), false);
  assert.equal(requests.current(newer, "first"), true);
});

test("background polling can retain a Core inspection only for the exact same saved task and content", () => {
  const current = {
    identity: { task_id: "a", revision: 7 },
    digests: { authorization: "auth", content: "code", plan: "checks" },
    refreshed: true,
  };
  assert.equal(
    sameTaskVersion(current, { ...current, refreshed: false }),
    true,
  );
  assert.equal(
    sameTaskVersion(current, {
      ...current,
      identity: { task_id: "b", revision: 7 },
    }),
    false,
  );
  assert.equal(
    sameTaskVersion(current, {
      ...current,
      identity: { task_id: "a", revision: 8 },
    }),
    false,
  );
  assert.equal(
    sameTaskVersion(current, {
      ...current,
      digests: { ...current.digests, content: "changed" },
    }),
    false,
  );
  assert.equal(sameTaskVersion(null, current), false);
});

test("a lost mutation response is never retried or reported as saved", async () => {
  const original = globalThis.fetch;
  const document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: "fixture-csrf" }) };
  let calls = 0;
  globalThis.fetch = async () => {
    calls++;
    throw new Error("response lost");
  };
  try {
    await assert.rejects(
      callTask("taskbelay_resolve_issue", {
        operation_id: "stable-fixture-operation",
      }),
      /response was lost/,
    );
    assert.equal(calls, 1);
  } finally {
    globalThis.fetch = original;
    globalThis.document = document;
  }
});

test("failure refreshing a saved mutation preserves its outcome and never repeats it", async () => {
  const original = globalThis.fetch;
  const document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: "fixture-csrf" }) };
  const methods = [];
  globalThis.fetch = async (_, options) => {
    methods.push(options?.method ?? "GET");
    if (methods.length === 1)
      return {
        ok: true,
        json: async () => ({
          ok: true,
          result: { identity: { task_id: "fixture" } },
        }),
      };
    throw new Error("display refresh failed");
  };
  try {
    await assert.rejects(
      callTask("taskbelay_resolve_issue", {
        operation_id: "stable-fixture-operation",
      }),
      /Task saved/,
    );
    assert.deepEqual(methods, ["POST", "GET"]);
  } finally {
    globalThis.fetch = original;
    globalThis.document = document;
  }
});

test("run recovery refreshes the task identified by its view without repeating recovery", async () => {
  const original = globalThis.fetch,
    document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: "fixture-csrf" }) };
  const requests = [];
  globalThis.fetch = async (url, options) => {
    requests.push({ url, method: options?.method ?? "GET" });
    if (requests.length === 1)
      return {
        ok: true,
        json: async () => ({
          ok: true,
          result: {
            view: { identity: { task_id: "recovered-task" } },
            run: { run_id: "original-run", status: "interrupted_unknown" },
          },
        }),
      };
    return {
      ok: true,
      json: async () => ({
        ok: true,
        summary: { task_id: "recovered-task", lifecycle: "active" },
        result: {
          identity: { task_id: "recovered-task" },
          decision: { pending_runs: [] },
        },
      }),
    };
  };
  try {
    const result = await callTask("taskbelay_recover_run", {
      operation_id: "recover-once",
    });
    assert.equal(result.result.identity.task_id, "recovered-task");
    assert.equal(result.summary.lifecycle, "active");
    assert.deepEqual(requests, [
      { url: "/api/core/taskbelay_recover_run", method: "POST" },
      { url: "/api/tasks/recovered-task?refresh=1", method: "GET" },
    ]);
  } finally {
    globalThis.fetch = original;
    globalThis.document = document;
  }
});

test("Core reports of applied or unknown operations require a saved-state read and never trigger retries", async () => {
  const original = globalThis.fetch,
    document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: "fixture-csrf" }) };
  try {
    for (const outcome of ["applied", "unknown"]) {
      let calls = 0;
      globalThis.fetch = async () => {
        calls++;
        return {
          ok: false,
          json: async () => ({
            ok: false,
            outcome,
            error: {
              code: "TRANSPORT_FAILED",
              message: "Inspect saved records",
            },
            recovery: { operation: "read_before_retry" },
          }),
        };
      };
      await assert.rejects(
        callTask("taskbelay_archive_task", {}),
        (error) => error instanceof SavedStateRequired,
      );
      assert.equal(calls, 1);
    }
  } finally {
    globalThis.fetch = original;
    globalThis.document = document;
  }
});

test("a rejected operation keeps its Core error category for contextual form feedback", async () => {
  const original = globalThis.fetch,
    document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: "fixture-csrf" }) };
  globalThis.fetch = async () => ({
    ok: false,
    json: async () => ({
      ok: false,
      outcome: "not_applied",
      error: { code: "REVISION_CONFLICT", message: "Task changed" },
      recovery: { operation: "replace" },
    }),
  });
  try {
    await assert.rejects(
      call("taskbelay_resolve_issue", {}),
      (error) =>
        error instanceof RequestError &&
        !(error instanceof SavedStateRequired) &&
        error.code === "REVISION_CONFLICT",
    );
  } finally {
    globalThis.fetch = original;
    globalThis.document = document;
  }
});
