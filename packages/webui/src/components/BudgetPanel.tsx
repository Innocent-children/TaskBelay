import { useEffect, useState } from "react";
import type { Mutate, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { duration } from "../lib/presentation";
import { ActionFeedback, useAction } from "./UI";

export interface BudgetDecision {
  runs: number;
  execution_ms: number;
  work_ms: number;
  tokens: number;
}

export function BudgetPanel({
  view,
  busy,
  mutate,
}: {
  view: View;
  busy: boolean;
  mutate: Mutate;
}) {
  const { label } = useLanguage();
  const limits = {
    runs: 0,
    execution_ms: 0,
    core_ms: 0,
    host_ms: 0,
    tokens: 0,
    ...view.context.authorization.limits,
  };
  const initial = () => ({
    runs: String(limits.runs),
    command: String(limits.execution_ms / 60000),
    work: String(Math.max(limits.core_ms, limits.host_ms) / 60000),
    tokens: String(limits.tokens),
  });
  const [fields, setFields] = useState(initial);
  const [touched, setTouched] = useState(false);
  const action = useAction(mutate);
  useEffect(() => {
    if (!touched) setFields(initial());
  }, [view.digests.authorization, touched]);
  const budget: BudgetDecision = {
    runs: Number(fields.runs),
    execution_ms: Math.round(Number(fields.command) * 60000),
    work_ms: Math.round(Number(fields.work) * 60000),
    tokens: Number(fields.tokens),
  };
  const next = {
    runs: budget.runs,
    execution_ms: budget.execution_ms,
    core_ms: budget.work_ms,
    host_ms: budget.work_ms,
    tokens: budget.tokens,
  };
  const names = {
    runs: label("Executions", "执行次数"),
    execution_ms: label("Command time", "累计命令时间"),
    core_ms: label("Core work time", "Core 工作时间"),
    host_ms: label("Reported work time", "宿主报告工作时间"),
    tokens: "Tokens",
  };
  const value = (name: string, n: number) =>
    n === 0
      ? label("No limit", "不设上限")
      : name.endsWith("_ms")
        ? duration(n, label)
        : n.toLocaleString();
  const changes = (Object.keys(next) as (keyof typeof next)[]).filter(
    (key) => next[key] !== limits[key],
  );
  const valid =
    Object.values(fields).every((n) => n.trim() !== "") &&
    Object.values(budget).every((n) => Number.isSafeInteger(n) && n >= 0) &&
    budget.runs <= 10000 &&
    budget.execution_ms <= 86400000;
  const changeText = changes.map(
    (key) =>
      names[key] +
      ": " +
      value(key, limits[key]) +
      " → " +
      value(key, next[key]),
  );
  return (
    <details className="disclosure">
      <summary>{label("Adjust budget limits", "调整投入上限")}</summary>
      <p className="help-text">
        {label(
          "Zero means no limit. Existing usage stays recorded. Work minutes change both overlapping Core and Host time limits.",
          "0 表示不设上限，已有消耗保留。工作分钟会同时调整 Core 和宿主报告时间上限，两者有重叠。",
        )}
      </p>
      <form
        data-dirty={touched}
        onSubmit={async (event) => {
          event.preventDefault();
          if (!valid || !changes.length) return;
          if (
            await action.submit("taskbelay_update_budget", {
              authorization_digest: view.digests.authorization,
              budget,
              response:
                label(
                  "I approve these exact budget changes: ",
                  "我批准这些具体预算变化：",
                ) + changeText.join("; "),
              user_ref: "webui-budget-" + crypto.randomUUID(),
            })
          )
            setTouched(false);
        }}
      >
        <fieldset disabled={busy}>
          <div className="form-grid">
            {(
              [
                ["runs", label("Execution limit", "累计执行次数"), "1"],
                ["command", label("Command minutes", "累计命令分钟"), "any"],
                ["work", label("Work minutes", "累计工作分钟"), "any"],
                ["tokens", label("Token limit", "累计 Tokens"), "1"],
              ] as const
            ).map(([key, title, step]) => (
              <label key={key}>
                {title}
                <input
                  type="number"
                  required
                  min="0"
                  step={step}
                  value={fields[key]}
                  onChange={(event) => {
                    setTouched(true);
                    setFields({ ...fields, [key]: event.target.value });
                    action.reset();
                  }}
                />
              </label>
            ))}
          </div>
        </fieldset>
        {changes.length > 0 && valid && (
          <div className="budget-diff">
            <strong>{label("Changes to approve", "本次批准的变化")}</strong>
            {changeText.map((text) => (
              <p key={text}>{text}</p>
            ))}
          </div>
        )}
        {!valid && (
          <p className="help-text" role="status">
            {label(
              "Fill every limit with a non-negative number. Executions must be an integer up to 10,000 and command time cannot exceed 1,440 minutes.",
              "每个上限都须填写非负数；执行次数最多 10000 次，命令时间最多 1440 分钟，次数和 Tokens 须为整数。",
            )}
          </p>
        )}
        <ActionFeedback action={action} />
        <div className="form-actions">
          <button
            type="button"
            disabled={busy || !touched}
            onClick={() => {
              setTouched(false);
              action.reset();
            }}
          >
            {label("Reset changes", "撤销填写")}
          </button>
          <button
            className="primary-button"
            disabled={busy || !valid || !changes.length}
          >
            {action.pending
              ? label("Saving…", "正在保存…")
              : label("Approve these limit changes", "批准这些上限变化")}
          </button>
        </div>
      </form>
    </details>
  );
}
