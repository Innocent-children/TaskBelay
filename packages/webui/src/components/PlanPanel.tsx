import type { View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { dateTime } from "../lib/presentation";
import { StatusBadge } from "./UI";

export function PlanPanel({ view }: { view: View }) {
  const { label, language } = useLanguage();
  const { plan, authorization } = view.context;
  return (
    <>
      <section className="content-section">
        <h2>{label("Original request and goals", "原始需求与目标")}</h2>
        <p className="request-text">{authorization.request}</p>
        {authorization.goals.map((goal) => (
          <div className="goal-definition" key={goal.id}>
            <div className="section-heading">
              <strong>{goal.statement}</strong>
              <StatusBadge
                status={
                  view.progress.find((progress) => progress.goal_id === goal.id)
                    ?.status || "planned"
                }
              />
            </div>
            <span className="help-text">{goal.id}</span>
          </div>
        ))}
      </section>
      <section className="content-section">
        <h2>{label("Approach and verification", "实施思路与验证安排")}</h2>
        <p>{plan.approach}</p>
        <p className="muted">{plan.verification_basis}</p>
        {!!plan.expected_paths?.length && (
          <p className="help-text">
            {label("Predicted paths: ", "预计路径：")}
            <code>{plan.expected_paths.join(", ")}</code>
          </p>
        )}
      </section>
      <section className="content-section">
        <h2>{label("Workspace and boundaries", "工作位置与明确限制")}</h2>
        {view.context.repositories.map((repository) => (
          <div className="repository-detail" key={repository.key}>
            <div className="section-heading">
              <strong>{repository.key}</strong>
              <span className="repository-branch">{repository.branch}</span>
            </div>
            <code>{repository.repository_path}</code>
            {repository.upstream && (
              <p className="help-text">
                {label("Upstream: ", "上游：")}
                {repository.upstream}
              </p>
            )}
            {repository.changed_paths.length > 0 && (
              <details className="technical-details">
                <summary>
                  {label("Observed changed paths", "已观察到的改动路径")}
                </summary>
                {repository.changed_paths.map((path) => (
                  <code className="path-line" key={path}>
                    {path}
                  </code>
                ))}
              </details>
            )}
          </div>
        ))}
        <p>
          {label("Allowed paths: ", "允许改动范围：")}
          {authorization.allowed_paths?.join(", ") ||
            label("Bound repositories", "已绑定仓库")}
        </p>
        {!!authorization.forbidden_paths?.length && (
          <p>
            {label("Forbidden paths: ", "禁止改动范围：")}
            <code>{authorization.forbidden_paths.join(", ")}</code>
          </p>
        )}
        {authorization.constraints?.map((constraint) => (
          <p key={constraint.id}>{constraint.statement}</p>
        ))}
        {authorization.dependencies?.map((dependency) => (
          <p key={dependency.path}>
            {label("Dependency restriction: ", "依赖限制：")}
            <code>{dependency.path}</code> ·{" "}
            {dependency.allowed.join(", ") ||
              label("No changes allowed", "不允许变更")}
          </p>
        ))}
      </section>
      <details className="disclosure">
        <summary>
          {label("Check definitions", "完整检查定义")}
          <span className="disclosure-count">{plan.checks.length}</span>
        </summary>
        {plan.checks.map((check) => (
          <div className="check-definition" key={check.id}>
            <strong>{check.id}</strong>
            <p>{check.rationale}</p>
            <p className="muted">{check.conditions}</p>
            {check.argv?.length ? <pre>{check.argv.join(" ")}</pre> : null}
            <p className="help-text">
              {label("Inputs: ", "输入：")}
              <code>{check.inputs?.join(", ") || "."}</code>
            </p>
            <p className="help-text">
              {label("Goals: ", "对应目标：")}
              {check.goal_ids.join(", ")}
            </p>
          </div>
        ))}
      </details>
      {view.concerns.length > 0 && (
        <details className="disclosure">
          <summary>
            {label(
              "Engineering questions and resolutions",
              "工程问题与处理情况",
            )}
            <span className="disclosure-count">{view.concerns.length}</span>
          </summary>
          {view.concerns.map((concern) => (
            <div className="check-definition" key={concern.id}>
              <strong>{concern.summary}</strong>
              <p>
                {concern.resolution?.basis || label("Unresolved", "尚未解决")}
              </p>
              <p className="help-text">{concern.reason}</p>
            </div>
          ))}
        </details>
      )}
      <details className="disclosure">
        <summary>
          {label("Recent decisions and changes", "近期决定与调整")}
        </summary>
        {view.details.changes.length ? (
          <ol className="change-timeline">
            {view.details.changes.map((change) => (
              <li key={change.revision + ":" + change.at}>
                <span className="help-text">
                  #{change.revision} · {dateTime(change.at, language)}
                </span>
                <p>{change.summary}</p>
                <details className="technical-details">
                  <summary>{label("Record details", "记录详情")}</summary>
                  <code>{change.kind}</code>
                  {change.user_ref && (
                    <p className="help-text">{change.user_ref}</p>
                  )}
                </details>
              </li>
            ))}
          </ol>
        ) : (
          <p className="muted">
            {label("No recent changes recorded.", "还没有近期调整记录。")}
          </p>
        )}
      </details>
      <details className="disclosure">
        <summary>
          {label(
            "Raw requirements and Core observations",
            "原始需求数据与 Core 检查事项",
          )}
        </summary>
        <pre>
          {JSON.stringify(
            { authorization, checks: plan.checks, gaps: view.decision.gaps },
            null,
            2,
          )}
        </pre>
      </details>
    </>
  );
}
