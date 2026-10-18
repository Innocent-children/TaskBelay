import { useState } from "react";
import type { Mutate, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { useRun } from "../lib/useRun";
import { RunDetails } from "./RunOutput";
import { ActionFeedback, ErrorNotice, StatusBadge, useAction } from "./UI";

export function RecoveryPanel({
  view,
  busy,
  mutate,
}: {
  view: View;
  busy: boolean;
  mutate: Mutate;
}) {
  const { label } = useLanguage();
  const [selected, setSelected] = useState("");
  const ids = view.decision.pending_runs;
  if (!ids.length) return null;
  const id = ids.includes(selected) ? selected : ids[0];
  return (
    <section className="attention-panel recovery-panel">
      <div className="eyebrow">
        {label("EXECUTION TO INSPECT", "需要核对的原运行")}
      </div>
      <h2>
        {label("An execution has no confirmed result", "有运行尚未确定结果")}
      </h2>
      <p className="help-text">
        {label(
          "Recovery reads the original result without starting the command again. Core checks that the original process has stopped.",
          "恢复会读取原运行结果，不会重新启动命令。Core 会核对原进程是否已经停止。",
        )}
      </p>
      {ids.length > 1 && (
        <label>
          {label("Original run", "原运行")}
          <select
            disabled={busy}
            value={id}
            onChange={(e) => setSelected(e.target.value)}
          >
            {ids.map((value) => (
              <option key={value}>{value}</option>
            ))}
          </select>
        </label>
      )}
      <RecoveryRun key={id} id={id} busy={busy} mutate={mutate} />
    </section>
  );
}

function RecoveryRun({
  id,
  busy,
  mutate,
}: {
  id: string;
  busy: boolean;
  mutate: Mutate;
}) {
  const { label } = useLanguage();
  const result = useRun(id);
  const action = useAction(mutate);
  const [basis, setBasis] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [pendingAction, setPendingAction] = useState<
    "read" | "inspection" | null
  >(null);
  return (
    <div>
      <code className="run-id">{id}</code>
      <ErrorNotice error={result.error} />
      {result.loading && (
        <p role="status" className="muted">
          {label("Reading original run…", "正在读取原运行…")}
        </p>
      )}
      {result.error && (
        <button disabled={busy} onClick={result.reload}>
          {label("Read original run again", "重新读取原运行")}
        </button>
      )}
      {result.run && (
        <>
          <div className="recovery-run-heading">
            <strong>{result.run.check_id}</strong>
            <StatusBadge status={result.run.status} />
          </div>
          <details className="disclosure">
            <summary>
              {label("Saved execution details", "已保存的执行详情")}
            </summary>
            <RunDetails run={result.run} />
          </details>
          <button
            className="primary-button"
            disabled={busy || result.loading}
            onClick={async () => {
              setPendingAction("read");
              await action.submit("taskbelay_recover_run", {
                run_id: id,
                resolution_basis: "",
                user_ref: "",
              });
              setPendingAction(null);
            }}
          >
            {action.pending && pendingAction === "read"
              ? label("Reading result…", "正在读取结果…")
              : label("Recover saved result", "恢复已保存结果")}
          </button>
          <details className="disclosure">
            <summary>
              {label(
                "Process stopped, but the result is missing",
                "进程已停止，但没有运行结果",
              )}
            </summary>
            <form
              data-dirty={!!basis || confirmed}
              onSubmit={async (event) => {
                event.preventDefault();
                if (!basis.trim() || !confirmed) return;
                setPendingAction("inspection");
                await action.submit("taskbelay_recover_run", {
                  run_id: id,
                  resolution_basis: basis.trim(),
                  user_ref: "webui-recovery-" + crypto.randomUUID(),
                });
                setPendingAction(null);
              }}
            >
              <fieldset disabled={busy}>
                <label>
                  {label(
                    "Inspected side effects and remaining uncertainty",
                    "已核对的副作用和仍未知的内容",
                  )}
                  <textarea
                    required
                    value={basis}
                    onChange={(e) => setBasis(e.target.value)}
                    placeholder={label(
                      "Describe what the original command changed and what you could not confirm.",
                      "说明原命令实际改了什么，以及哪些内容还无法确认。",
                    )}
                  />
                </label>
                <label className="inline">
                  <input
                    type="checkbox"
                    required
                    checked={confirmed}
                    onChange={(e) => setConfirmed(e.target.checked)}
                  />
                  {label(
                    "I inspected the effects and want to save this account. The run result remains unknown.",
                    "我已实际核对副作用并保存上述说明，运行结果仍保留为未知。",
                  )}
                </label>
              </fieldset>
              <button disabled={busy || !basis.trim() || !confirmed}>
                {action.pending && pendingAction === "inspection"
                  ? label("Saving inspection…", "正在保存核对说明…")
                  : label(
                      "Save inspection as unknown",
                      "保存核对说明，结果保留未知",
                    )}
              </button>
            </form>
          </details>
          <ActionFeedback action={action} />
        </>
      )}
    </div>
  );
}
