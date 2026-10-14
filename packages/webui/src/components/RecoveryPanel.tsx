import { useState } from "react";
import type { Run, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";

export function RecoveryPanel({
  view,
  run,
  busy,
  readRun,
  mutate,
}: {
  view: View;
  run: Run | null;
  busy: boolean;
  readRun: (id: string) => Promise<void>;
  mutate: (tool: string, fields: Record<string, unknown>) => Promise<void>;
}) {
  const { label } = useLanguage();
  const [basis, setBasis] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  if (!view.decision.pending_runs.length) return null;
  const pending = run && view.decision.pending_runs.includes(run.run_id);
  return (
    <section>
      <h2>
        {label("Interrupted or unfinished execution", "中断或尚未结束的执行")}
      </h2>
      <p>
        {label(
          "Recovering reads the original result. It never starts the command again. Core refuses recovery while the original process is still running or cannot be observed.",
          "恢复会读取原运行结果，不会重新启动命令。原进程仍在运行或无法确认时，Core 会拒绝恢复。",
        )}
      </p>
      {view.decision.pending_runs.map((id) => (
        <button key={id} disabled={busy} onClick={() => void readRun(id)}>
          {label("Inspect ", "查看 ")}
          {id}
        </button>
      ))}
      {pending && (
        <>
          <p>
            {run.status} · {run.check_id}
          </p>
          <button
            disabled={busy}
            onClick={() =>
              void mutate("taskbelay_recover_run", {
                run_id: run.run_id,
                resolution_basis: "",
                user_ref: "",
              })
            }
          >
            {label("Recover saved result", "恢复已保存结果")}
          </button>
          <details>
            <summary>
              {label(
                "The process stopped without a result",
                "进程已停止但没有结果",
              )}
            </summary>
            <label>
              {label(
                "Describe the side effects you checked and what remains unknown",
                "说明已核对的副作用和仍未知的内容",
              )}
              <textarea
                value={basis}
                onChange={(e) => setBasis(e.target.value)}
              />
            </label>
            <label className="inline">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
              />
              {label(
                "I authorize recording this inspection. The result remains unknown.",
                "我确认保存这次核对，运行结果仍保留为未知。",
              )}
            </label>
            <button
              disabled={busy || !basis.trim() || !confirmed}
              onClick={() =>
                void mutate("taskbelay_recover_run", {
                  run_id: run.run_id,
                  resolution_basis: basis,
                  user_ref: "webui-recovery-" + crypto.randomUUID(),
                })
              }
            >
              {label(
                "Record stopped execution as unknown",
                "将已停止执行记录为未知",
              )}
            </button>
          </details>
        </>
      )}
    </section>
  );
}
