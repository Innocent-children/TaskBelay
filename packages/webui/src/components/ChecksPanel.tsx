import { useId, useState } from "react";
import type { Check, CheckSummary, Mutate, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { applicability, dateTime, sourceLabel } from "../lib/presentation";
import { RunOutput } from "./RunOutput";
import {
  ActionFeedback,
  Dialog,
  DialogRefresh,
  StatusBadge,
  useAction,
} from "./UI";

export function ChecksPanel({
  view,
  busy,
  mutate,
  inspect,
  compact = false,
  onAll,
}: {
  view: View;
  busy: boolean;
  mutate: Mutate;
  inspect: () => Promise<void>;
  compact?: boolean;
  onAll?: () => void;
}) {
  const { label } = useLanguage();
  const [human, setHuman] = useState<Check | null>(null);
  const checks = compact ? view.checks.slice(0, 4) : view.checks;
  const passed = view.checks.filter(
    (check) => check.status === "passed",
  ).length;
  const failed = view.checks.filter((check) =>
    ["failed", "timeout", "output_limit"].includes(check.status),
  ).length;
  return (
    <section className="content-section checks-section">
      <div className="section-heading">
        <h2>
          {compact
            ? label("Check summary", "检查摘要")
            : label("Check records", "检查记录")}
        </h2>
        <span className="muted count-summary">
          {passed} {label("passed", "通过")}
          {failed > 0 && (
            <>
              {" "}
              · {failed} {label("failed", "失败")}
            </>
          )}
        </span>
      </div>
      <p className="section-description">
        {view.status !== "active"
          ? label(
              "Results saved when the task ended.",
              "展示任务结束时保存的结果。",
            )
          : view.refreshed
            ? label(
                "Results and applicability for the last inspected conditions.",
                "展示上次核对条件的对应结果。",
              )
            : label(
                "Saved results. Inspect current code to assess whether they still apply.",
                "展示已保存结果；核对当前代码后可判断是否仍适用。",
              )}
      </p>
      {!checks.length && (
        <p className="empty-inline">
          {label("No checks defined.", "还没有检查定义。")}
        </p>
      )}
      <div className="check-list">
        {checks.map((check) => (
          <CheckRow
            key={check.id + ":" + (check.run_id || check.evidence_id || "")}
            check={check}
            view={view}
            compact={compact}
            busy={busy}
            human={(definition) => setHuman(definition)}
          />
        ))}
      </div>
      {compact && view.checks.length > 4 && (
        <button className="text-button section-link" onClick={onAll}>
          {label("View all ", "查看全部 ")}
          {view.checks.length}
          {label(" checks →", " 项检查 →")}
        </button>
      )}
      {!compact && view.status === "active" && (
        <p className="help-text">
          {label(
            "Command checks run through your coding agent's native process tools. Human checks record what you actually inspected.",
            "命令检查由编码代理通过宿主原生工具执行；人工检查只记录你实际完成的核对。",
          )}
        </p>
      )}
      {human && (
        <HumanCheckDialog
          view={view}
          check={human}
          busy={busy}
          mutate={mutate}
          inspect={inspect}
          close={() => setHuman(null)}
        />
      )}
    </section>
  );
}

function CheckRow({
  check,
  view,
  compact,
  busy,
  human,
}: {
  check: CheckSummary;
  view: View;
  compact: boolean;
  busy: boolean;
  human: (check: Check) => void;
}) {
  const { label, language } = useLanguage();
  const [expanded, setExpanded] = useState(false);
  const id = useId();
  const definition = view.context.plan.checks.find(
    (item) => item.id === check.id,
  );
  const evidence = view.details.evidence.find(
    (item) => item.id === check.evidence_id,
  );
  return (
    <div className="check-entry">
      <div className="check-row">
        <div className="check-name">
          <strong>{check.id}</strong>
          <span>
            {check.source
              ? sourceLabel(check.source, label)
              : check.kind === "human"
                ? label("Human check", "人工检查")
                : label("Command check", "命令检查")}
            {definition?.purpose === "acceptance"
              ? " · " + label("Acceptance", "需求验收")
              : definition?.purpose === "regression"
                ? " · " + label("Regression", "回归检查")
                : ""}
          </span>
        </div>
        <StatusBadge status={check.status} />
        <span className="check-applicability">
          {applicability(check, view, label)}
        </span>
        <button
          className="text-button"
          aria-expanded={expanded}
          aria-controls={id}
          onClick={() => setExpanded(!expanded)}
        >
          {expanded
            ? label("Close", "收起")
            : check.run_id
              ? label("Output", "日志")
              : label("Details", "详情")}
        </button>
      </div>
      {check.accepted_failure && (
        <div className="notice notice-warning">
          <strong>
            {label("Accepted existing failure", "已接受的已有回归失败")}
          </strong>
          <p>{check.accepted_failure.failures.join(" · ")}</p>
          <p>{check.accepted_failure.reason}</p>
        </div>
      )}
      {!compact &&
        check.kind === "human" &&
        definition &&
        view.status === "active" && (
          <div className="human-check-action">
            <button
              disabled={busy || !view.refreshed}
              onClick={() => human(definition)}
            >
              {label("Record my inspection", "记录我的检查结果")}
            </button>
            {!view.refreshed && (
              <span className="help-text">
                {label("Inspect current code first.", "请先核对当前代码。")}
              </span>
            )}
          </div>
        )}
      {expanded && (
        <div className="check-expanded" id={id}>
          {definition && (
            <>
              <p>{definition.rationale}</p>
              <p className="muted">{definition.conditions}</p>
            </>
          )}
          {evidence && (
            <p className="help-text">
              {label("Recorded", "记录于")}{" "}
              {dateTime(evidence.recorded_at, language)}
            </p>
          )}
          {check.run_id ? (
            <RunOutput id={check.run_id} />
          ) : (
            <p>{check.summary}</p>
          )}
          <details className="technical-details">
            <summary>
              {label("Result explanation from Core", "Core 返回的结果说明")}
            </summary>
            <p>{check.reason}</p>
            <p>{check.summary}</p>
          </details>
        </div>
      )}
    </div>
  );
}

function HumanCheckDialog({
  view,
  check,
  busy,
  mutate,
  inspect,
  close,
}: {
  view: View;
  check: Check;
  busy: boolean;
  mutate: Mutate;
  inspect: () => Promise<void>;
  close: () => void;
}) {
  const { label } = useLanguage();
  const [summary, setSummary] = useState("");
  const [result, setResult] = useState("");
  const action = useAction(mutate);
  return (
    <Dialog
      title={label("Record your inspection", "记录实际检查结果")}
      busy={action.pending}
      onClose={close}
    >
      <p className="dialog-context">
        <strong>{check.id}</strong>
        <br />
        {check.rationale}
      </p>
      <p className="help-text">{check.conditions}</p>
      <form
        data-dirty={!!(summary || result)}
        onSubmit={async (event) => {
          event.preventDefault();
          if (!summary.trim() || !result) return;
          if (
            await action.submit("taskbelay_record_evidence", {
              check_id: check.id,
              source: "human",
              status: result,
              summary: summary.trim(),
              user_ref: "webui-observation-" + crypto.randomUUID(),
              content_digest: view.digests.content,
            })
          )
            close();
        }}
      >
        <fieldset disabled={busy}>
          <legend>{label("What did you observe?", "实际检查结论")}</legend>
          <div className="choices">
            <label className="choice">
              <input
                type="radio"
                name="human-result"
                value="passed"
                checked={result === "passed"}
                onChange={(e) => setResult(e.target.value)}
                required
              />
              {label("Passed", "通过")}
            </label>
            <label className="choice">
              <input
                type="radio"
                name="human-result"
                value="failed"
                checked={result === "failed"}
                onChange={(e) => setResult(e.target.value)}
              />
              {label("Failed", "失败")}
            </label>
          </div>
          <label>
            {label("What you checked and observed", "实际检查内容与观察结果")}
            <textarea
              data-initial-focus
              required
              value={summary}
              onChange={(e) => setSummary(e.target.value)}
              placeholder={label(
                "Describe the actions you performed and the result you saw.",
                "说明你实际做了哪些操作，看到了什么结果。",
              )}
            />
          </label>
        </fieldset>
        <ActionFeedback action={action} />
        {(action.error || !view.refreshed) && (
          <DialogRefresh inspect={inspect} />
        )}
        <div className="form-actions">
          <button type="button" disabled={action.pending} onClick={close}>
            {label("Back", "返回")}
          </button>
          <button
            className="primary-button"
            disabled={busy || !view.refreshed || !summary.trim() || !result}
          >
            {action.pending
              ? label("Saving…", "正在保存…")
              : label("Save inspection", "保存检查结果")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
