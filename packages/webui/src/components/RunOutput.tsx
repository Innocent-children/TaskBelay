import type { Run } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { dateTime, duration, statusLabel } from "../lib/presentation";
import { useRun } from "../lib/useRun";
import { ErrorNotice } from "./UI";

export function RunDetails({ run }: { run: Run }) {
  const { label, language } = useLanguage();
  return (
    <div className="run-output">
      <div className="run-meta">
        <span>{statusLabel(run.status, label)}</span>
        <span>{duration(run.duration_ms, label)}</span>
        <span>
          {label("Exit code", "退出码")} {run.exit_code ?? "—"}
        </span>
        <time dateTime={run.started_at}>
          {dateTime(run.started_at, language)}
        </time>
      </div>
      <details className="technical-details">
        <summary>
          {label("Command and working directory", "命令与执行目录")}
        </summary>
        <code>{run.cwd}</code>
        <pre>{run.argv?.join(" ")}</pre>
        <code>{run.run_id}</code>
      </details>
      {!!run.output.stdout && (
        <div>
          <div className="output-label">
            {label("Standard output", "标准输出")}
          </div>
          <pre tabIndex={0}>{run.output.stdout}</pre>
        </div>
      )}
      {!!run.output.stderr && (
        <div>
          <div className="output-label">
            {label("Error output", "错误输出")}
          </div>
          <pre tabIndex={0}>{run.output.stderr}</pre>
        </div>
      )}
      {!run.output.stdout && !run.output.stderr && (
        <p className="muted">
          {label(
            "This run did not save any output.",
            "这次运行没有保存输出内容。",
          )}
        </p>
      )}
      {(run.truncated || run.output.truncated) && (
        <p className="notice notice-warning">
          {label(
            "Output is truncated. Saved prefix shown; total bytes: ",
            "输出已截断，当前展示已保存的前缀，总字节数：",
          )}
          {run.output.total_bytes}
        </p>
      )}
      {run.reconciliation && <p>{run.reconciliation}</p>}
    </div>
  );
}

export function RunOutput({ id }: { id: string }) {
  const { label } = useLanguage();
  const result = useRun(id);
  return (
    <div aria-busy={result.loading}>
      <ErrorNotice error={result.error} />
      {result.error && (
        <button onClick={result.reload}>
          {label("Read output again", "重新读取输出")}
        </button>
      )}
      {result.loading && (
        <p className="muted" role="status">
          {label("Reading saved output…", "正在读取保存的输出…")}
        </p>
      )}
      {result.run && <RunDetails run={result.run} />}
    </div>
  );
}
