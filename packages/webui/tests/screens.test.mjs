import assert from "node:assert/strict";
import { before, after, test } from "node:test";
import { resolve } from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";
import react from "@vitejs/plugin-react";
let server,
  TaskDetail,
  ChecksPanel,
  BudgetPanel,
  FailureAcceptance,
  LanguageProvider;
before(async () => {
  globalThis.localStorage = { getItem: () => "en", setItem() {} };
  server = await createServer({
    root: resolve(import.meta.dirname, ".."),
    configFile: false,
    plugins: [react()],
    server: { middlewareMode: true },
    appType: "custom",
  });
  ({ TaskDetail } = await server.ssrLoadModule(
    "/src/components/TaskDetail.tsx",
  ));
  ({ ChecksPanel } = await server.ssrLoadModule(
    "/src/components/ChecksPanel.tsx",
  ));
  ({ BudgetPanel } = await server.ssrLoadModule(
    "/src/components/BudgetPanel.tsx",
  ));
  ({ FailureAcceptance } = await server.ssrLoadModule(
    "/src/components/DecisionPanel.tsx",
  ));
  ({ LanguageProvider } = await server.ssrLoadModule("/src/lib/i18n.tsx"));
});
after(async () => {
  await server?.close();
  delete globalThis.localStorage;
});
const mutate = async () => ({ ok: true });
function view() {
  return {
    identity: { task_id: "fixture-task", revision: 1 },
    host: "codex",
    status: "done",
    archived: false,
    summary: "Delivered the requested change.",
    refreshed: false,
    context: {
      authorization: {
        request: "Improve the task interface.",
        goals: [{ id: "ui", statement: "A usable interface" }],
        limits: {},
        constraints: [],
      },
      repositories: [
        {
          key: "repo",
          repository_path: "/work/repo",
          branch: "main",
          changed_paths: [],
          upstream: null,
        },
      ],
      plan: {
        approach: "Keep Core decisions.",
        verification_basis: "Actual checks.",
        checks: [
          {
            id: "build",
            kind: "command",
            purpose: "acceptance",
            goal_ids: ["ui"],
            rationale: "Build the UI",
            conditions: "Local sources",
            inputs: ["."],
            argv: ["pnpm", "build"],
          },
        ],
      },
    },
    details: {
      created_at: "2026-10-18T01:00:00Z",
      updated_at: "2026-10-18T02:00:00Z",
      repositories: [],
      evidence: [],
      attempts: [],
      exceptions: [],
      changes: [],
    },
    checks: [
      {
        id: "build",
        kind: "command",
        purpose: "acceptance",
        goal_ids: ["ui"],
        source: "core_execution",
        status: "passed",
        evidence_id: "record",
        run_id: "record",
        applicable: true,
        summary: "actual exit 0",
        reason: "saved evidence",
      },
    ],
    progress: [],
    concerns: [],
    limitations: [],
    issue: null,
    digests: { authorization: "auth", content: "content", plan: "plan" },
    decision: {
      ready: false,
      advice: "stop",
      gaps: [],
      missing_checks: [],
      pending_runs: [],
    },
    usage: {
      runs: 1,
      execution_ms: 1234,
      core_ms: 1700,
      reserved_ms: 0,
      uncertain_ms: 0,
      host: {
        coverage: "unreported",
        observed_through: null,
        tokens: null,
        active_ms: null,
        wait_ms: null,
      },
    },
    budget: { limits: [], checks: [], warnings: [] },
  };
}
function render(Component, properties, language = "en") {
  globalThis.localStorage.getItem = () => language;
  return renderToStaticMarkup(
    createElement(LanguageProvider, null, createElement(Component, properties)),
  );
}
function props(record) {
  return {
    view: record,
    summary: { lifecycle: record.status },
    busy: false,
    saving: false,
    readAt: null,
    checkedAt: null,
    inspect() {},
    mutate,
  };
}

test("completed overview preserves results and all detail tabs without empty problems or action forms", () => {
  const html = render(TaskDetail, props(view()));
  assert.match(html, /Delivered the requested change/);
  assert.match(html, /Check records/);
  assert.match(html, /Requirements &amp; usage/);
  assert.match(html, /Saved at task end/);
  assert.doesNotMatch(html, /<h2>Remaining problems<\/h2>/);
  assert.doesNotMatch(html, /A decision is needed/);
  assert.match(html, /Not reported/);
  assert.match(html, /Archive task/);
});

test("Core blocked classification is retained independently from the active lifecycle", () => {
  const record = view();
  record.status = "active";
  const html = render(TaskDetail, {
    ...props(record),
    summary: { lifecycle: "blocked" },
  });
  assert.match(html, /Needs attention/);
  assert.match(html, /Check current code/);
  assert.match(html, /current code not inspected/);
});

test("a pending question keeps options, free text and authorization explanation together", () => {
  const record = view();
  record.status = "active";
  record.issue = {
    summary: "Keep the current scope?",
    options: ["Keep scope", "Adjust scope"],
  };
  record.issue_digest = "question";
  const html = render(TaskDetail, props(record));
  assert.match(html, /A decision is needed/);
  assert.match(html, /aria-pressed="false"/);
  assert.match(html, /Keep scope/);
  assert.match(html, /Your answer/);
  assert.match(html, /scope change must still be applied/);
});

test("manual evidence is disabled for saved conditions and enabled after an actual Core refresh", () => {
  const record = view();
  record.status = "active";
  record.context.plan.checks[0].kind = "human";
  record.checks[0].kind = "human";
  record.checks[0].source = "human";
  const saved = render(ChecksPanel, { view: record, busy: false, mutate });
  assert.match(saved, /<button disabled="">Record my inspection/);
  assert.match(saved, /Inspect current code first/);
  record.refreshed = true;
  const refreshed = render(ChecksPanel, { view: record, busy: false, mutate });
  assert.match(refreshed, /<button>Record my inspection/);
});

test("saved applicability gaps and the current question are shown in their owning sections without duplicate raw problems", () => {
  const record = view();
  record.status = "active";
  record.issue = {
    summary: "A real pending choice",
    options: ["Keep scope", "Expand scope"],
  };
  record.issue_digest = "opaque-digest";
  record.decision.gaps = [
    {
      code: "check_stale",
      subject: "build",
      message:
        "saved evidence; refresh actual conditions to assess applicability",
      path: "checks",
    },
    {
      code: "user_decision",
      subject: "opaque-digest",
      message: "A real pending choice",
      path: "issue",
    },
  ];
  const html = render(TaskDetail, props(record));
  assert.doesNotMatch(html, /<h2>Remaining problems<\/h2>/);
  assert.match(html, /A real pending choice/);
  record.refreshed = true;
  record.checks[0].applicable = false;
  const refreshed = render(TaskDetail, props(record));
  assert.match(refreshed, /<h2>Remaining problems<\/h2>/);
  assert.match(refreshed, /no longer applies to the inspected conditions/);
});

test("an accepted existing regression stays failed with its precise reason", () => {
  const record = view();
  record.checks[0].status = "failed";
  record.checks[0].purpose = "regression";
  record.checks[0].accepted_failure = {
    check_id: "build",
    failures: ["Existing snapshot mismatch"],
    reason: "Compared with unchanged sources.",
  };
  const html = render(ChecksPanel, { view: record, busy: false, mutate });
  assert.match(html, /Accepted existing failure/);
  assert.match(html, /Existing snapshot mismatch/);
  assert.match(html, /Compared with unchanged sources/);
  assert.match(html, /status-badge danger/);
});

test("budget displays both Core and Host changes in readable units before approval", () => {
  const record = view();
  record.status = "active";
  record.context.authorization.limits = { core_ms: 60000, host_ms: 120000 };
  const html = render(BudgetPanel, { view: record, busy: false, mutate });
  assert.match(html, /Core work time: 1m 0s → 2m 0s/);
  assert.match(html, /Changes to approve/);
  assert.match(html, /Zero means no limit/);
  assert.match(html, /Approve these limit changes/);
});

test("Chinese UI localizes states and source names while escaping request content", () => {
  const record = view();
  record.context.authorization.request = "<img src=x onerror=alert(1)>";
  const html = render(TaskDetail, props(record), "zh");
  assert.match(html, /已完成/);
  assert.match(html, /本机命令执行/);
  assert.match(html, /结束时保存的记录/);
  assert.match(html, /&lt;img src=x onerror=alert\(1\)&gt;/);
  assert.doesNotMatch(html, /<img src=x/);
});

test("an existing-failure form does not offer unrelated passing runs before a failed run is chosen", () => {
  const record = view();
  record.status = "active";
  record.refreshed = true;
  record.context.plan.checks = [
    {
      id: "old-failure",
      kind: "command",
      purpose: "regression",
      goal_ids: ["ui"],
    },
    {
      id: "unrelated-pass",
      kind: "command",
      purpose: "acceptance",
      goal_ids: ["ui"],
    },
  ];
  record.details.evidence = [
    {
      id: "failed-record",
      check_id: "old-failure",
      source: "core_execution",
      status: "failed",
    },
    {
      id: "unrelated-record",
      check_id: "unrelated-pass",
      source: "core_execution",
      status: "passed",
    },
  ];
  record.checks = [
    {
      id: "old-failure",
      evidence_id: "failed-record",
      applicable: true,
      status: "failed",
    },
    {
      id: "unrelated-pass",
      evidence_id: "unrelated-record",
      applicable: true,
      status: "passed",
    },
  ];
  const html = render(FailureAcceptance, { view: record, busy: false, mutate });
  assert.match(html, /failed-record/);
  assert.doesNotMatch(html, /unrelated-record/);
});

test("active progress records do not claim that no summary exists when the terminal summary is empty", () => {
  const record = view();
  record.status = "active";
  record.summary = "";
  record.progress = [
    {
      goal_id: "ui",
      status: "doing",
      summary: "Implemented draft retention.",
      next_step: "Inspect the actual interface.",
    },
  ];
  const html = render(TaskDetail, props(record));
  assert.match(html, /Implemented draft retention/);
  assert.doesNotMatch(html, /No progress summary has been recorded/);
});
