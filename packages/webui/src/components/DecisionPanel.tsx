import { useState } from "react";
import type { View } from "../lib/api";
import { useLanguage } from "../lib/i18n";

type Props = {
  view: View;
  busy: boolean;
  mutate: (tool: string, fields: Record<string, unknown>) => Promise<void>;
};
export function DecisionPanel({ view, busy, mutate }: Props) {
  const { label } = useLanguage();
  const [decision, setDecision] = useState("");
  const issue = view.issue;
  if (!issue) return null;
  return (
    <section>
      <h2>{label("Your decision", "需要你的决定")}</h2>
      <p>{issue.summary}</p>
      <div className="choices">
        {issue.options.map((option) => (
          <button
            disabled={busy}
            key={option}
            onClick={() => setDecision(option)}
          >
            {option}
          </button>
        ))}
      </div>
      <label>
        {label("Answer", "答复")}
        <textarea
          value={decision}
          onChange={(event) => setDecision(event.target.value)}
        />
      </label>
      <p>
        {label(
          "This records your answer. A change to task scope still needs to be reflected in its authorization.",
          "这里保存你的答复；需要改变任务范围时，代理仍须按此答复更新授权。",
        )}
      </p>
      <button
        disabled={busy || !decision.trim()}
        onClick={() =>
          void mutate("taskbelay_resolve_issue", {
            issue_digest: view.issue_digest,
            decision,
            user_ref: "webui-decision-" + crypto.randomUUID(),
          })
        }
      >
        {label("Save decision", "保存决定")}
      </button>
    </section>
  );
}
export function FailureAcceptance({ view, busy, mutate }: Props) {
  const { label } = useLanguage();
  const [selected, setSelected] = useState("");
  const [comparison, setComparison] = useState("");
  const [failures, setFailures] = useState("");
  const [reason, setReason] = useState("");
  const applicable = (id: string) =>
    view.checks.some((f) => f.evidence_id === id && f.applicable);
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
  const comparisons = view.details.evidence.filter(
    (e) =>
      e.source === "core_execution" &&
      view.checks.some((f) => f.evidence_id === e.id && f.applicable && f.status === "passed") &&
      view.context.plan.checks.some(
        (c) => c.id === e.check_id && c.compares === chosen?.check_id,
      ),
  );
  if (!failed.length) return null;
  return (
    <details className="task-details">
      <summary>
        {label("Accept an existing regression failure", "接受已有回归失败")}
      </summary>
      <p>
        {label(
          "Acceptance checks still have to pass. Inspect the failed output and comparison before accepting an existing issue.",
          "需求验收检查仍须通过。请先查看失败输出和比较结果，再决定是否接受已有问题。",
        )}
      </p>
      <label>
        {label("Failed run", "失败运行")}
        <select
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setComparison("");
          }}
        >
          <option value="">—</option>
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
          value={comparison}
          onChange={(e) => setComparison(e.target.value)}
        >
          <option value="">—</option>
          {comparisons.map((e) => (
            <option key={e.id} value={e.id}>
              {e.check_id} · {e.id}
            </option>
          ))}
        </select>
      </label>
      <label>
        {label("Existing failures, one per line", "已有失败，每行一项")}
        <textarea
          value={failures}
          onChange={(e) => setFailures(e.target.value)}
        />
      </label>
      <label>
        {label("Reason to accept", "接受原因")}
        <textarea value={reason} onChange={(e) => setReason(e.target.value)} />
      </label>
      <button
        disabled={
          busy ||
          !chosen ||
          !comparisons.some((e) => e.id === comparison) ||
          !failures.trim() ||
          !reason.trim()
        }
        onClick={() =>
          void mutate("taskbelay_accept_known_failure", {
            check_id: chosen!.check_id,
            failed_evidence_id: selected,
            comparison_evidence_id: comparison,
            failures: failures
              .split("\n")
              .map((s) => s.trim())
              .filter(Boolean),
            reason,
            user_ref: "webui-failure-" + crypto.randomUUID(),
          })
        }
      >
        {label("Accept these existing failures", "接受这些已有失败")}
      </button>
    </details>
  );
}
