import { useLanguage } from "../lib/i18n";
import type { View } from "../lib/api";

export function PlanPanel({ view }: { view: View }) {
  const { label } = useLanguage();
  const { plan, authorization } = view.context;
  const names: Record<string, string> = {
    runs: label("Executions", "执行次数"),
    execution_ms: label("Command time", "命令时间"),
    core_ms: label("Core work time", "Core 工作时间"),
    host_ms: label("Reported work and wait", "报告的工作与等待"),
    tokens: "Tokens",
  };
  const value = (name: string, n: number | null) =>
    n === null ? label("Unknown", "未知") :
    name.endsWith("_ms") ? `${Math.round(n / 1000)} s` : String(n);
  return (
    <details className="task-details">
      <summary>
        {label(
          "Approach, workspace and verification",
          "实施思路、工作位置与验证",
        )}
      </summary>
      <p>{plan.approach}</p>
      <p>{plan.verification_basis}</p>
      {view.details.repositories.map((r) => (
        <p key={r.key}>
          <strong>{r.key}</strong> ·{" "}
          <code>{r.workspace_origin.canonical_worktree_root}</code> ·{" "}
          {r.workspace_origin.task_branch}
        </p>
      ))}
      <p>
        {label("Allowed paths: ", "允许改动范围：")}
        {authorization.allowed_paths?.join(", ") ||
          label("Bound repositories", "已绑定仓库")}
      </p>
      {!!authorization.forbidden_paths?.length && (
        <p>
          {label("Forbidden paths: ", "禁止改动范围：")}
          {authorization.forbidden_paths.join(", ")}
        </p>
      )}
      {!!plan.expected_paths?.length && (
        <p>
          {label("Predicted paths: ", "预计路径：")}
          {plan.expected_paths.join(", ")}
        </p>
      )}
      <ul>
        {plan.checks.map((c) => (
          <li key={c.id}>
            <strong>{c.id}</strong> · {c.rationale} · {c.conditions}
          </li>
        ))}
      </ul>
      {authorization.dependencies?.map((p) => (
        <p key={p.path}>
          {label("Dependency restriction: ", "依赖限制：")}
          <code>{p.path}</code> ·{" "}
          {p.allowed.join(", ") || label("No changes allowed", "不允许变更")}
        </p>
      ))}
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>{label("Cost", "投入")}</th>
              <th>{label("Actual", "实际")}</th>
              <th>{label("Reserved", "预留")}</th>
              <th>{label("Uncertain", "未确定")}</th>
              <th>{label("Maximum", "上限")}</th>
              <th>{label("Remaining", "剩余")}</th>
            </tr>
          </thead>
          <tbody>
            {view.budget.limits.map((b) => (
              <tr key={b.name}>
                <td>{names[b.name]}</td>
                <td>{value(b.name, b.actual)}</td>
                <td>{value(b.name, b.reserved)}</td>
                <td>{value(b.name, b.uncertain)}</td>
                <td>
                  {b.limited
                    ? value(b.name, b.limit)
                    : label("Not set", "未设置")}
                </td>
                <td>{b.limited ? value(b.name, b.remaining) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {view.budget.checks
        .filter((c) => c.required && !c.startable)
        .map((c) => (
          <p key={c.check_id} className="finding">
            {c.check_id} ·{" "}
            {label(
              "Current limits cannot reserve this check.",
              "当前额度无法预留此检查。",
            )}
          </p>
        ))}
      {view.budget.warnings.map((w) => (
        <p className="finding" key={w}>
          {w}
        </p>
      ))}
      <p>
        {label(
          "Core and reported work time overlap; do not add them.",
          "Core 与报告工作时间可能重叠，不能直接相加。",
        )}
      </p>
      <details>
        <summary>
          {label(
            "Complete requirements and check definitions",
            "完整需求与检查定义",
          )}
        </summary>
        <pre>
          {JSON.stringify({ authorization, checks: plan.checks }, null, 2)}
        </pre>
      </details>
    </details>
  );
}
