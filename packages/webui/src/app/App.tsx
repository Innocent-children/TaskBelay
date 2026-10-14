import { useEffect, useRef, useState } from "react";
import { BudgetPanel } from "../components/BudgetPanel";
import { DecisionPanel, FailureAcceptance } from "../components/DecisionPanel";
import { PlanPanel } from "../components/PlanPanel";
import { RecoveryPanel } from "../components/RecoveryPanel";
import { read, type Summary } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { useTask } from "../lib/useTask";

const duration = (ms: number) =>
  ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;

export function App() {
  const { language, setLanguage, label } = useLanguage();
  const task = useTask();
  const { view, busy, mutationBlocked, run, mutate, readRun } = task;
  const [tasks, setTasks] = useState<Summary[]>([]);
  const [listError, setListError] = useState("");
  const [search, setSearch] = useState("");
  const [host, setHost] = useState("");
  const [lifecycle, setLifecycle] = useState("");
  const [page, setPage] = useState(1);
  const [hasNext, setHasNext] = useState(false);
  const [includeArchived, setIncludeArchived] = useState(false);
  const [listRevision, refreshList] = useState(0);
  const listRequest = useRef(0);
  const statusLabel = (status: string) =>
    ({
      active: label("Active", "进行中"),
      blocked: label("Blocked", "需处理"),
      done: label("Done", "完成"),
      cancelled: label("Cancelled", "取消"),
      planned: label("Planned", "待执行"),
      doing: label("In progress", "进行中"),
    })[status] ?? status;

  useEffect(() => {
    const request = ++listRequest.current;
    const query = new URLSearchParams({
      page: String(page),
      include_archived: String(includeArchived),
      search,
      host,
      lifecycle,
    });
    setListError("");
    void read<{ items: Summary[]; has_next: boolean }>("/api/tasks?" + query)
      .then((result) => {
        if (request === listRequest.current) {
          setTasks(result.items);
          setHasNext(result.has_next);
        }
      })
      .catch((error) => {
        if (request === listRequest.current) setListError(String(error));
      });
    return () => {
      listRequest.current++;
    };
  }, [page, includeArchived, search, host, lifecycle, listRevision]);

  useEffect(() => {
    function navigate() {
      const match = /^\/tasks\/([^/]+)\/?$/.exec(window.location.pathname);
      if (match) void task.select(decodeURIComponent(match[1]));
      else task.clear();
    }
    navigate();
    window.addEventListener("popstate", navigate);
    return () => window.removeEventListener("popstate", navigate);
  }, []);

  function select(id: string) {
    window.history.pushState(null, "", "/tasks/" + encodeURIComponent(id));
    void task.select(id);
  }
  async function change(tool: string, fields: Record<string, unknown>) {
    await mutate(tool, fields);
    refreshList((value) => value + 1);
  }
  async function human(check: string, passed: boolean) {
    const summary = window.prompt(
      label(
        "Describe what you actually checked and observed.",
        "说明你实际检查的内容与观察结果。",
      ),
    );
    if (summary?.trim() && view)
      await change("taskbelay_record_evidence", {
        check_id: check,
        source: "human",
        status: passed ? "passed" : "failed",
        summary,
        user_ref: "webui-observation-" + crypto.randomUUID(),
        content_digest: view.content_digest,
      });
  }
  async function cancel() {
    const reason = window.prompt(
      label(
        "Reason to cancel this task (files remain).",
        "取消此任务的原因（保留文件）。",
      ),
    );
    if (reason?.trim()) await change("taskbelay_cancel_task", { reason });
  }

  return (
    <div className="shell">
      <header>
        <a href="/">TaskBelay</a>
        <span>
          {label("Development progress and results", "开发进展与检查结果")}
        </span>
        <select
          aria-label={label("Language", "语言")}
          value={language}
          onChange={(e) => setLanguage(e.target.value as "en" | "zh")}
        >
          <option value="en">English</option>
          <option value="zh">中文</option>
        </select>
        <button
          disabled={busy}
          onClick={() => {
            refreshList((value) => value + 1);
            if (view) void task.select(view.task.task_id, true);
          }}
        >
          {label("Refresh actual state", "刷新实际状态")}
        </button>
      </header>
      <main>
        <aside>
          <h2>{label("Your tasks", "你的任务")}</h2>
          <div className="filters">
            <label>
              {label("Search request or task ID", "搜索需求或任务 ID")}
              <input
                type="search"
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setPage(1);
                }}
              />
            </label>
            <label>
              {label("Host", "宿主")}
              <select
                value={host}
                onChange={(e) => {
                  setHost(e.target.value);
                  setPage(1);
                }}
              >
                <option value="">{label("All hosts", "全部宿主")}</option>
                {["codex", "deepseek", "claude", "zcode", "cli"].map(
                  (value) => (
                    <option key={value}>{value}</option>
                  ),
                )}
              </select>
            </label>
            <label>
              {label("Status", "状态")}
              <select
                value={lifecycle}
                onChange={(e) => {
                  setLifecycle(e.target.value);
                  setPage(1);
                }}
              >
                <option value="">{label("All states", "全部状态")}</option>
                {["active", "blocked", "done", "cancelled"].map((value) => (
                  <option key={value} value={value}>
                    {statusLabel(value)}
                  </option>
                ))}
              </select>
            </label>
            <label className="inline">
              <input
                type="checkbox"
                checked={includeArchived}
                onChange={(e) => {
                  setIncludeArchived(e.target.checked);
                  setPage(1);
                }}
              />
              {label("Include archived", "包含归档任务")}
            </label>
          </div>
          {listError && <p role="alert">{listError}</p>}
          {tasks.map((item) => (
            <button
              disabled={busy}
              className={
                "task " +
                (view?.task.task_id === item.task_id ? "selected" : "")
              }
              key={item.task_id}
              onClick={() => select(item.task_id)}
            >
              <strong>{item.request_summary}</strong>
              <span>{statusLabel(item.lifecycle)}</span>
              <small>{item.worktree_path}</small>
            </button>
          ))}
          {!tasks.length && (
            <p>
              {label(
                "No matching tasks. Start a task in your coding agent.",
                "没有匹配的任务。可以在编码代理中开始任务。",
              )}
            </p>
          )}
          <div className="pagination">
            <button disabled={page <= 1} onClick={() => setPage(page - 1)}>
              {label("Previous", "上一页")}
            </button>
            <span>{page}</span>
            <button disabled={!hasNext} onClick={() => setPage(page + 1)}>
              {label("Next", "下一页")}
            </button>
          </div>
        </aside>
        <article aria-busy={mutationBlocked}>
          {task.error && (
            <div role="alert" className="error">
              {task.error}
            </div>
          )}
          {!view ? (
            <section>
              <h1>{label("Results and remaining work", "结果与剩余工作")}</h1>
              <p>
                {busy
                  ? label("Loading saved task…", "正在读取任务…")
                  : label(
                      "Select a task to inspect its results and next steps.",
                      "选择任务，查看检查结果与下一步。",
                    )}
              </p>
            </section>
          ) : (
            <>
              <section>
                <div className="row">
                  <span className="badge">{statusLabel(view.task.status)}</span>
                  <small>
                    {view.task.status !== "active" ? label("Results saved at completion", "结束时保存的结果") : view.refreshed
                      ? label("Actual conditions refreshed", "已刷新实际条件")
                      : label(
                          "Saved state · refresh to assess results",
                          "保存状态 · 刷新后判断结果是否仍适用",
                        )}
                  </small>
                </div>
                <h1>{view.task.authorization.request}</h1>
                {view.task.summary && <p>{view.task.summary}</p>}
                {view.task.authorization.goals.map((goal) => {
                  const progress = view.task.progress.find(
                    (p) => p.goal_id === goal.id,
                  );
                  return (
                    <div className="goal" key={goal.id}>
                      <strong>{goal.statement}</strong>
                      <span>{statusLabel(progress?.status ?? "planned")}</span>
                      <p>{progress?.summary}</p>
                      {progress?.next_step && (
                        <p>
                          {label("Next: ", "下一步：")}
                          {progress.next_step}
                        </p>
                      )}
                    </div>
                  );
                })}
              </section>
              <section>
                <h2>{label("Remaining problems", "剩余问题")}</h2>
                {view.decision.findings
                  .filter((f) => f.blocking)
                  .map((f) => (
                    <p className="finding" key={f.id}>
                      {f.path && <code>{f.path}</code>}
                      {f.detail}
                    </p>
                  ))}
                {view.decision.missing_checks.length > 0 && (
                  <p>
                    {label("Required checks: ", "仍需检查：")}
                    {view.decision.missing_checks.join(", ")}
                  </p>
                )}
              </section>
              {view.task.status === "active" && (
                <DecisionPanel
                  key={view.task.task_id + ":" + view.issue_digest}
                  view={view}
                  busy={mutationBlocked}
                  mutate={change}
                />
              )}
              <RecoveryPanel
                key={run?.run_id}
                view={view}
                run={run}
                busy={mutationBlocked}
                readRun={readRun}
                mutate={change}
              />
              <PlanPanel view={view} />
              <section>
                <h2>{label("Checks and actual results", "检查与实际结果")}</h2>
                {view.decision.accepted_failures.map((f) => (
                  <div className="finding" key={f.check_id}>
                    <strong>
                      {label(
                        "Accepted existing failures: ",
                        "已接受的已有失败：",
                      )}
                      {f.check_id}
                    </strong>
                    <p>{f.failures.join(", ")}</p>
                    <p>{f.reason}</p>
                  </div>
                ))}
                {view.task.evidence.map((e) => (
                  <div className="evidence" key={e.id}>
                    <strong>{e.check_id}</strong>
                    <span>
                      {e.source} · {e.status}
                    </span>
                    <p>{e.summary}</p>
                    <small>
                      {
                        view.decision.evidence.find(
                          (f) => f.evidence_id === e.id,
                        )?.reason
                      }
                    </small>
                    {e.source === "core_execution" && (
                      <button onClick={() => void readRun(e.id)}>
                        {label("Read actual output", "查看实际输出")}
                      </button>
                    )}
                  </div>
                ))}
                {view.task.status === "active" &&
                  view.task.plan.checks
                    .filter((c) => c.kind === "human")
                    .map((c) => (
                      <div key={c.id}>
                        <p>
                          {c.rationale} · {c.conditions}
                        </p>
                        <button
                          disabled={mutationBlocked || !view.refreshed}
                          onClick={() => void human(c.id, true)}
                        >
                          {label("I checked: passed", "我已检查：通过")}
                        </button>{" "}
                        <button
                          disabled={mutationBlocked || !view.refreshed}
                          onClick={() => void human(c.id, false)}
                        >
                          {label("I checked: failed", "我已检查：失败")}
                        </button>
                      </div>
                    ))}
                {run && (
                  <pre>
                    {run.run_id} · {run.status}
                    {run.truncated && " · " + label("truncated", "已截断")}
                    {"\n"}
                    {run.stdout}
                    {"\n"}
                    {run.stderr}
                  </pre>
                )}
              </section>
              {view.task.status === "active" && (
                <FailureAcceptance
            key={view.task.task_id}
                  view={view}
                  busy={mutationBlocked || !view.refreshed}
                  mutate={change}
                />
              )}
              <section>
                <h2>{label("Cumulative work", "累计投入")}</h2>
                <div className="metrics">
                  <div>
                    <strong>{view.task.usage.runs}</strong>
                    <span>{label("Executions", "执行次数")}</span>
                  </div>
                  <div>
                    <strong>{duration(view.task.usage.execution_ms)}</strong>
                    <span>{label("Execution time", "执行时间")}</span>
                  </div>
                  <div>
                    <strong>{duration(view.task.usage.core_ms)}</strong>
                    <span>
                      {label(
                        "Mutating Core calls, including execution",
                        "Core 变更调用时间，包含执行",
                      )}
                    </span>
                  </div>
                  <div>
                    <strong>
                      {view.task.usage.host_reported_tokens} tokens
                    </strong>
                    <span>
                      {label(
                        "Host reported · may be incomplete",
                        "宿主报告 · 可能不完整",
                      )}
                    </span>
                  </div>
                </div>
                <p>
                  {label(
                    "Saved-state reads do not write metering data. Timings overlap and must not be added.",
                    "读取保存状态不写入计量。各类时间有重叠，不能直接相加。",
                  )}
                </p>
                <p>
                  {label("Uncertain execution time: ", "未确定执行时间：")}
                  {duration(view.task.usage.uncertain_ms)}
                </p>
              </section>
              {view.task.status === "active" && (
                <BudgetPanel
                  key={view.task.task_id + ":" + view.authorization_digest}
                  view={view}
                  busy={mutationBlocked}
                  update={(budget, response) =>
                    void change("taskbelay_update_budget", {
                      authorization_digest: view.authorization_digest,
                      budget,
                      response,
                      user_ref: "webui-budget-" + crypto.randomUUID(),
                    })
                  }
                />
              )}
              <details className="task-details">
                <summary>
                  {label("Recent decisions and changes", "近期决定与调整")}
                </summary>
                {view.task.changes?.map((c, i) => (
                  <div key={i}>
                    <small>
                      #{c.revision} · {c.at} · {c.kind}
                    </small>
                    <p>{c.summary}</p>
                    {c.user_ref && <small>{c.user_ref}</small>}
                  </div>
                ))}
              </details>
              <section>
                <h2>
                  {label("Constraints and remaining risks", "约束与剩余问题")}
                </h2>
                {view.task.authorization.constraints?.map((c) => (
                  <p key={c.id}>{c.statement}</p>
                ))}
                {view.task.risks.map((r) => (
                  <p key={r}>{r}</p>
                ))}
                {view.task.status === "active" ? (
                  <button
                    disabled={mutationBlocked}
                    onClick={() => void cancel()}
                  >
                    {label("Cancel task", "取消任务")}
                  </button>
                ) : (
                  <button
                    disabled={mutationBlocked}
                    onClick={() =>
                      void change("taskbelay_archive_task", {
                        archived: !view.task.archived,
                      })
                    }
                  >
                    {view.task.archived
                      ? label("Restore from archive", "取消归档")
                      : label("Archive task", "归档任务")}
                  </button>
                )}
              </section>
            </>
          )}
        </article>
      </main>
    </div>
  );
}
