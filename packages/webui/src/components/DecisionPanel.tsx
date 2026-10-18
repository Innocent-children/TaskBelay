import { useState } from "react";
import type { Mutate, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { ActionFeedback, useAction } from "./UI";

type Props = { view: View; busy: boolean; mutate: Mutate };

export function DecisionPanel({ view, busy, mutate }: Props) {
  const { label } = useLanguage();
  const [decision, setDecision] = useState("");
  const action = useAction(mutate);
  if (!view.issue) return null;
  return (
    <section className="attention-panel">
      <div className="eyebrow">{label("YOUR INPUT", "需要你处理")}</div>
      <h2>{label("A decision is needed", "需要你的决定")}</h2>
      <p>{view.issue.summary}</p>
      <form
        data-dirty={!!decision}
        onSubmit={async (event) => {
          event.preventDefault();
          if (!decision.trim()) return;
          await action.submit("taskbelay_resolve_issue", {
            issue_digest: view.issue_digest,
            decision: decision.trim(),
            user_ref: "webui-decision-" + crypto.randomUUID(),
          });
        }}
      >
        <fieldset disabled={busy}>
          <div className="choices">
            {view.issue.options?.map((option) => (
              <button
                type="button"
                className={
                  "choice-button" + (decision === option ? " is-selected" : "")
                }
                aria-pressed={decision === option}
                key={option}
                onClick={() => {
                  setDecision(option);
                  action.reset();
                }}
              >
                {option}
              </button>
            ))}
          </div>
          <label>
            {label("Your answer", "你的答复")}
            <textarea
              value={decision}
              onChange={(e) => {
                setDecision(e.target.value);
                action.reset();
              }}
              placeholder={label(
                "Choose an option or write your answer…",
                "可以选择上面的选项，也可以直接填写…",
              )}
              required
            />
          </label>
        </fieldset>
        <p className="help-text">
          {label(
            "Your answer is saved for the agent. Any task scope change must still be applied to its authorization.",
            "答复会保存给代理；涉及任务范围的变化，代理仍须据此更新授权。",
          )}
        </p>
        <ActionFeedback action={action} />
        <button className="primary-button" disabled={busy || !decision.trim()}>
          {action.pending
            ? label("Saving…", "正在保存…")
            : label("Save answer", "保存答复")}
        </button>
      </form>
    </section>
  );
}

export function FailureAcceptance({ view, busy, mutate }: Props) {
  const { label } = useLanguage();
  const [selected, setSelected] = useState("");
  const [comparison, setComparison] = useState("");
  const [failures, setFailures] = useState("");
  const [reason, setReason] = useState("");
  const action = useAction(mutate);
  const applicable = (id: string) =>
    view.checks.some((check) => check.evidence_id === id && check.applicable);
  const failed = view.details.evidence.filter(
    (e) =>
      e.source === "core_execution" &&
      e.status === "failed" &&
      applicable(e.id) &&
      view.context.plan.checks.some(
        (c) => c.id === e.check_id && c.purpose === "regression",
      ),
  );
  const chosen = failed.find((e) => e.id === selected);
  const comparisons = chosen
    ? view.details.evidence.filter(
        (e) =>
          e.source === "core_execution" &&
          view.checks.some(
            (f) =>
              f.evidence_id === e.id && f.applicable && f.status === "passed",
          ) &&
          view.context.plan.checks.some(
            (c) => c.id === e.check_id && c.compares === chosen.check_id,
          ),
      )
    : [];
  if (!failed.length) return null;
  return (
    <details className="disclosure">
      <summary>
        {label("Accept an existing regression failure", "接受已有回归失败")}
      </summary>
      <p className="help-text">
        {label(
          "Inspect the failed output and passed comparison first. Checks for the requested outcome must still pass.",
          "先查看失败输出和通过的比较结果。本次需求的验收检查仍须通过。",
        )}
      </p>
      <form
        data-dirty={!!(selected || comparison || failures || reason)}
        onSubmit={async (event) => {
          event.preventDefault();
          if (
            !chosen ||
            !comparisons.some((e) => e.id === comparison) ||
            !failures.trim() ||
            !reason.trim()
          )
            return;
          if (
            await action.submit("taskbelay_accept_known_failure", {
              check_id: chosen.check_id,
              failed_evidence_id: selected,
              comparison_evidence_id: comparison,
              failures: failures
                .split("\n")
                .map((s) => s.trim())
                .filter(Boolean),
              reason: reason.trim(),
              user_ref: "webui-failure-" + crypto.randomUUID(),
            })
          ) {
            setSelected("");
            setComparison("");
            setFailures("");
            setReason("");
          }
        }}
      >
        <fieldset disabled={busy || !view.refreshed}>
          <div className="form-grid">
            <label>
              {label("Failed run", "失败运行")}
              <select
                required
                value={selected}
                onChange={(e) => {
                  setSelected(e.target.value);
                  setComparison("");
                }}
              >
                <option value="">
                  {label("Select a failed run", "选择失败运行")}
                </option>
                {failed.map((e) => (
                  <option key={e.id} value={e.id}>
                    {e.check_id} · {e.id}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {label("Passed comparison", "通过的比较运行")}
              <select
                required
                value={comparison}
                onChange={(e) => setComparison(e.target.value)}
              >
                <option value="">
                  {label("Select a comparison", "选择比较运行")}
                </option>
                {comparisons.map((e) => (
                  <option key={e.id} value={e.id}>
                    {e.check_id} · {e.id}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label>
            {label("Existing failures, one per line", "已有失败，每行一项")}
            <textarea
              required
              value={failures}
              onChange={(e) => setFailures(e.target.value)}
            />
          </label>
          <label>
            {label("Reason to accept", "接受原因")}
            <textarea
              required
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </label>
        </fieldset>
        {!view.refreshed && (
          <p className="help-text">
            {label(
              "Inspect current code before accepting a failure.",
              "请先核对当前代码，再接受已有失败。",
            )}
          </p>
        )}
        {selected && !comparisons.length && (
          <p className="help-text">
            {label(
              "A passed, applicable comparison is required.",
              "还需要一项适用且通过的比较检查结果。",
            )}
          </p>
        )}
        <ActionFeedback action={action} />
        <button
          disabled={
            busy ||
            !view.refreshed ||
            !chosen ||
            !comparisons.some((e) => e.id === comparison) ||
            !failures.trim() ||
            !reason.trim()
          }
        >
          {action.pending
            ? label("Saving…", "正在保存…")
            : label("Accept these existing failures", "接受这些已有失败")}
        </button>
      </form>
    </details>
  );
}
