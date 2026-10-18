import type { Mutate, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { dateTime, duration } from "../lib/presentation";
import { BudgetPanel } from "./BudgetPanel";

export function UsagePanel({
  view,
  busy,
  mutate,
}: {
  view: View;
  busy: boolean;
  mutate: Mutate;
}) {
  const { language, label } = useLanguage();
  const names: Record<string, string> = {
    runs: label("Executions", "执行次数"),
    execution_ms: label("Command time", "命令时间"),
    core_ms: label("Core work time", "Core 工作时间"),
    host_ms: label("Reported work and wait", "宿主报告工作与等待"),
    tokens: "Tokens",
  };
  const value = (name: string, number: number | null) =>
    number === null
      ? label("Unknown", "未知")
      : name.endsWith("_ms")
        ? duration(number, label)
        : number.toLocaleString();
  return (
    <section className="content-section">
      <div className="section-heading">
        <h2>{label("Cumulative usage", "累计投入")}</h2>
      </div>
      <div className="usage-metrics">
        <div>
          <strong>{view.usage.runs}</strong>
          <span>{label("Executions", "执行次数")}</span>
        </div>
        <div>
          <strong>{duration(view.usage.execution_ms, label)}</strong>
          <span>{label("Command time", "命令时间")}</span>
        </div>
        <div>
          <strong>{duration(view.usage.core_ms, label)}</strong>
          <span>
            {label(
              "Core calls, including execution",
              "Core 变更调用，包含执行",
            )}
          </span>
        </div>
        <div>
          <strong>
            {view.usage.host.tokens === null
              ? label("Not reported", "未上报")
              : view.usage.host.tokens.toLocaleString()}
          </strong>
          <span>{label("Host Tokens", "宿主 Tokens")}</span>
        </div>
      </div>
      <p className="help-text">
        {label("Host coverage: ", "宿主用量：")}
        {view.usage.host.coverage === "unreported"
          ? label("Not reported", "未上报")
          : view.usage.host.coverage === "partial"
            ? label("Partially reported", "部分上报")
            : label("Reported sources", "已报告来源")}
        {view.usage.host.observed_through &&
          " · " + dateTime(view.usage.host.observed_through, language)}
      </p>
      <p className="help-text">
        {label("Host activity / waiting: ", "宿主活动 / 等待：")}
        {duration(view.usage.host.active_ms, label)} /{" "}
        {duration(view.usage.host.wait_ms, label)}
      </p>
      <p className="help-text">
        {label(
          "Times overlap and must not be added. Saved-record reads do not write metering data.",
          "各类时间有重叠，不能直接相加。读取保存记录不写入计量。",
        )}
      </p>
      <div className="table-scroll">
        <table>
          <caption className="sr-only">
            {label("Actual usage and explicit limits", "实际投入与明确上限")}
          </caption>
          <thead>
            <tr>
              {[
                label("Usage", "投入"),
                label("Actual", "实际"),
                label("Reserved", "预留"),
                label("Uncertain", "未确定"),
                label("Limit", "上限"),
                label("Remaining", "剩余"),
              ].map((text) => (
                <th scope="col" key={text}>
                  {text}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {view.budget.limits.map((limit) => (
              <tr key={limit.name}>
                <th scope="row">{names[limit.name]}</th>
                <td>{value(limit.name, limit.actual)}</td>
                <td>{value(limit.name, limit.reserved)}</td>
                <td>{value(limit.name, limit.uncertain)}</td>
                <td>
                  {limit.limited
                    ? value(limit.name, limit.limit)
                    : label("Not set", "未设置")}
                </td>
                <td>
                  {limit.limited ? value(limit.name, limit.remaining) : "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {view.usage.uncertain_ms > 0 && (
        <p className="notice notice-warning">
          {label("Uncertain execution time: ", "未确定执行时间：")}
          {duration(view.usage.uncertain_ms, label)}
        </p>
      )}
      {view.budget.checks
        .filter((check) => check.required && !check.startable)
        .map((check) => (
          <p className="notice notice-warning" key={check.check_id}>
            {check.check_id} ·{" "}
            {label(
              "The current limits cannot reserve this check.",
              "当前上限无法预留这项检查。",
            )}
          </p>
        ))}
      {view.budget.warnings.length > 0 && (
        <details className="technical-details">
          <summary>{label("Usage warnings", "投入提醒")}</summary>
          {view.budget.warnings.map((warning) => (
            <p key={warning}>{warning}</p>
          ))}
        </details>
      )}
      {view.status === "active" && (
        <BudgetPanel view={view} busy={busy} mutate={mutate} />
      )}
    </section>
  );
}
