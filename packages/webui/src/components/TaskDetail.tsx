import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import type { Mutate, Summary, View } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import {
  dateTime,
  duration,
  repositoryName,
  taskTitle,
} from "../lib/presentation";
import { ChecksPanel } from "./ChecksPanel";
import { DecisionPanel, FailureAcceptance } from "./DecisionPanel";
import { PlanPanel } from "./PlanPanel";
import { RecoveryPanel } from "./RecoveryPanel";
import {
  ActionFeedback,
  Dialog,
  DialogRefresh,
  StatusBadge,
  useAction,
} from "./UI";
import { UsagePanel } from "./UsagePanel";

type Tab = "overview" | "checks" | "requirements";
type Props = {
  view: View;
  summary: Summary | null;
  busy: boolean;
  saving: boolean;
  readAt: string | null;
  checkedAt: string | null;
  inspect: () => Promise<void>;
  mutate: Mutate;
};

export function TaskDetail({
  view,
  summary,
  busy,
  saving,
  readAt,
  checkedAt,
  inspect,
  mutate,
}: Props) {
  const { label, language } = useLanguage();
  const [tab, setTab] = useState<Tab>("overview");
  const [cancelOpen, setCancelOpen] = useState(false);
  const archive = useAction(mutate);
  const menu = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const close = (event: PointerEvent) => {
      if (
        event.target instanceof Node &&
        !menu.current?.contains(event.target) &&
        menu.current
      )
        menu.current.open = false;
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, []);
  const tabs: { id: Tab; title: string }[] = [
    { id: "overview", title: label("Overview", "概览") },
    { id: "checks", title: label("Check records", "检查记录") },
    { id: "requirements", title: label("Requirements & usage", "需求与投入") },
  ];
  const title = taskTitle(view.context.authorization.request);
  function tabKey(event: KeyboardEvent<HTMLDivElement>) {
    const buttons = Array.from(
      event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]'),
    );
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % buttons.length;
    else if (event.key === "ArrowLeft")
      next = (index - 1 + buttons.length) % buttons.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = buttons.length - 1;
    else return;
    event.preventDefault();
    setTab(tabs[next].id);
    buttons[next].focus();
  }
  return (
    <>
      <div className="task-heading">
        <div className="task-breadcrumb">
          <span>
            {view.context.repositories
              .map((repository) => repositoryName(repository.repository_path))
              .join(" / ")}
          </span>
          <span className="breadcrumb-separator">/</span>
          <span>{view.host}</span>
          <StatusBadge status={summary?.lifecycle || view.status} />
          {view.archived && (
            <span className="archive-label">{label("Archived", "已归档")}</span>
          )}
        </div>
        <h1>{title}</h1>
        <div className="task-heading-bottom">
          <div className="record-times">
            <span>
              {label("Saved update", "保存更新")}{" "}
              <time dateTime={view.details.updated_at}>
                {dateTime(view.details.updated_at, language)}
              </time>
            </span>
            <span>
              {label("Read", "读取于")} {dateTime(readAt, language)}
            </span>
          </div>
          <div className="task-toolbar">
            {view.status === "active" && (
              <button disabled={busy} onClick={inspect}>
                {label("Check current code", "核对当前代码")}
              </button>
            )}
            <details
              className="task-menu"
              ref={menu}
              onKeyDown={(event) => {
                if (event.key === "Escape" && menu.current)
                  menu.current.open = false;
              }}
            >
              <summary aria-label={label("Task actions", "任务操作")}>
                ···
              </summary>
              <div className="task-menu-content">
                {view.status === "active" ? (
                  <>
                    <button
                      className="danger-text"
                      disabled={busy || view.decision.pending_runs.length > 0}
                      aria-label={label("Cancel task", "取消任务")}
                      onClick={() => {
                        if (menu.current) menu.current.open = false;
                        setCancelOpen(true);
                      }}
                    >
                      {label("Cancel task", "取消任务")}
                    </button>
                    {view.decision.pending_runs.length > 0 && (
                      <p className="help-text">
                        {label(
                          "Handle uncertain executions first.",
                          "请先处理未确定执行。",
                        )}
                      </p>
                    )}
                  </>
                ) : (
                  <button
                    disabled={busy}
                    aria-label={
                      view.archived
                        ? label("Restore from archive", "取消归档")
                        : label("Archive task", "归档任务")
                    }
                    onClick={() => {
                      if (menu.current) menu.current.open = false;
                      void archive.submit("taskbelay_archive_task", {
                        archived: !view.archived,
                      });
                    }}
                  >
                    {view.archived
                      ? label("Restore from archive", "取消归档")
                      : label("Archive task", "归档任务")}
                  </button>
                )}
                <code>{view.identity.task_id}</code>
                <span className="help-text">
                  {label("Revision", "修订")} {view.identity.revision}
                </span>
              </div>
            </details>
          </div>
        </div>
        {title !== view.context.authorization.request && (
          <details className="request-disclosure">
            <summary>{label("Original request", "原始需求")}</summary>
            <p>{view.context.authorization.request}</p>
          </details>
        )}
        <div className="freshness-note">
          <span
            className={
              view.refreshed && view.status === "active"
                ? "inspected-dot"
                : "saved-dot"
            }
            aria-hidden="true"
          />
          <span>
            {view.status !== "active"
              ? label("Results saved at task end", "任务结束时保存的结果")
              : view.refreshed
                ? label(
                    "Code and execution conditions inspected",
                    "已核对代码与执行条件",
                  )
                : label(
                    "Saved records · current code not inspected",
                    "保存记录 · 尚未核对当前代码",
                  )}
          </span>
          {checkedAt && (
            <span>
              {label("Last inspected in this page", "本页上次核对")}{" "}
              {dateTime(checkedAt, language)}
            </span>
          )}
          {saving && <span role="status">{label("Saving…", "正在保存…")}</span>}
        </div>
        <ActionFeedback action={archive} />
      </div>
      <div
        className="detail-tabs"
        role="tablist"
        aria-label={label("Task details", "任务详情")}
        onKeyDown={tabKey}
      >
        {tabs.map((item) => (
          <button
            key={item.id}
            id={"tab-" + item.id}
            role="tab"
            aria-selected={tab === item.id}
            aria-controls={"panel-" + item.id}
            tabIndex={tab === item.id ? 0 : -1}
            onClick={() => setTab(item.id)}
          >
            {item.title}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id="panel-overview"
        aria-labelledby="tab-overview"
        hidden={tab !== "overview"}
      >
        {view.status === "active" && (
          <DecisionPanel
            key={view.issue_digest}
            view={view}
            busy={busy}
            mutate={mutate}
          />
        )}
        <RecoveryPanel view={view} busy={busy} mutate={mutate} />
        <section className="content-section outcome-section">
          <div className="section-heading">
            <h2>
              {view.status === "done"
                ? label("Delivered result", "交付结果")
                : view.status === "cancelled"
                  ? label("Saved result", "已保存结果")
                  : label("Current progress", "当前进展")}
            </h2>
            {view.status === "active" && (
              <span className="eyebrow">
                {label("SAVED PROGRESS", "已保存进展")}
              </span>
            )}
          </div>
          {view.summary ? (
            <p className="outcome-text">{view.summary}</p>
          ) : (
            !view.progress.some((progress) => progress.summary.trim()) && (
              <p className="outcome-text">
                {label(
                  "The task is saved. No progress summary has been recorded yet.",
                  "任务已保存，还没有进展总结。",
                )}
              </p>
            )
          )}
          {view.status === "active" && (
            <div className="goal-progress">
              {view.context.authorization.goals.map((goal) => {
                const progress = view.progress.find(
                  (p) => p.goal_id === goal.id,
                );
                return (
                  <div key={goal.id}>
                    <div className="goal-title">
                      <span>{goal.statement}</span>
                      <StatusBadge status={progress?.status || "planned"} />
                    </div>
                    {progress?.summary && progress.summary !== view.summary && (
                      <p>{progress.summary}</p>
                    )}
                    {progress?.next_step && (
                      <p className="next-step">
                        <span>{label("Next", "下一步")}</span>
                        {progress.next_step}
                      </p>
                    )}
                  </div>
                );
              })}
            </div>
          )}
          {view.status === "active" && view.decision.ready && (
            <p className="notice notice-success">
              {label(
                "Core reports that completion requirements are satisfied. The agent can record the final delivery.",
                "Core 判断当前已满足结束条件，代理可以记录最终交付结论。",
              )}
            </p>
          )}
        </section>
        <RemainingProblems view={view} />
        {view.limitations.length > 0 && (
          <section className="content-section">
            <h2>{label("Delivery limitations", "交付限制")}</h2>
            {view.limitations.map((limit) => (
              <div className="limitation" key={limit.id}>
                <strong>{limit.summary}</strong>
                <p>{limit.basis}</p>
              </div>
            ))}
          </section>
        )}
        <ChecksPanel
          view={view}
          busy={busy}
          mutate={mutate}
          inspect={inspect}
          compact
          onAll={() => setTab("checks")}
        />
        <div className="usage-strip">
          <span>
            {label("Executions", "累计执行")} <strong>{view.usage.runs}</strong>
          </span>
          <span>
            {label("Command time", "命令时间")}{" "}
            <strong>{duration(view.usage.execution_ms, label)}</strong>
          </span>
          <span>
            Tokens{" "}
            <strong>
              {view.usage.host.tokens === null
                ? label("Not reported", "未上报")
                : view.usage.host.tokens.toLocaleString()}
            </strong>
          </span>
          <button
            className="text-button"
            onClick={() => setTab("requirements")}
          >
            {label("Usage details →", "投入详情 →")}
          </button>
        </div>
      </div>
      <div
        role="tabpanel"
        id="panel-checks"
        aria-labelledby="tab-checks"
        hidden={tab !== "checks"}
      >
        <ChecksPanel
          view={view}
          busy={busy}
          mutate={mutate}
          inspect={inspect}
        />
        {view.status === "active" && (
          <FailureAcceptance view={view} busy={busy} mutate={mutate} />
        )}
      </div>
      <div
        role="tabpanel"
        id="panel-requirements"
        aria-labelledby="tab-requirements"
        hidden={tab !== "requirements"}
      >
        <PlanPanel view={view} />
        <UsagePanel view={view} busy={busy} mutate={mutate} />
      </div>
      {cancelOpen && (
        <CancelDialog
          view={view}
          busy={busy}
          mutate={mutate}
          inspect={inspect}
          close={() => setCancelOpen(false)}
        />
      )}
    </>
  );
}

function RemainingProblems({ view }: { view: View }) {
  const { label } = useLanguage();
  const routine = [
    "goal_not_done",
    "check_missing",
    "assessment_missing",
    "review_missing",
    "issue_pending",
    "execution_pending",
  ];
  const gaps = view.decision.gaps.filter(
    (gap) =>
      !routine.includes(gap.code) &&
      !(gap.code === "user_decision" && view.issue) &&
      !(gap.code === "check_stale" && !view.refreshed) &&
      !(
        gap.action === "recover_run" &&
        view.decision.pending_runs.includes(gap.subject)
      ),
  );
  if (!gaps.length) return null;
  const descriptions: Record<string, string> = {
    check_stale: label(
      "This check no longer applies to the inspected conditions. Review its inputs and run it again through the agent.",
      "这项结果已不适用于核对后的条件，请由代理检查输入变化并重新验证。",
    ),
    check_failed: label(
      "This check did not pass. Inspect its actual output in Check records.",
      "这项检查未通过，请在检查记录中查看实际输出。",
    ),
  };
  return (
    <section className="content-section">
      <h2>{label("Remaining problems", "剩余问题")}</h2>
      {gaps.map((gap) => (
        <div className="problem-row" key={gap.code + ":" + gap.subject}>
          <span className="problem-marker" aria-hidden="true">
            !
          </span>
          <div>
            <strong>{gap.subject}</strong>
            <p>{descriptions[gap.code] || gap.message}</p>
            {descriptions[gap.code] && (
              <details className="technical-details">
                <summary>{label("Core explanation", "Core 详细说明")}</summary>
                <p>{gap.message}</p>
                {gap.path && <code>{gap.path}</code>}
              </details>
            )}
          </div>
        </div>
      ))}
    </section>
  );
}

function CancelDialog({
  view,
  busy,
  mutate,
  inspect,
  close,
}: {
  view: View;
  busy: boolean;
  mutate: Mutate;
  inspect: () => Promise<void>;
  close: () => void;
}) {
  const { label } = useLanguage();
  const [reason, setReason] = useState("");
  const action = useAction(mutate);
  return (
    <Dialog
      title={label("Cancel this task", "取消此任务")}
      busy={action.pending}
      onClose={close}
    >
      <p>{taskTitle(view.context.authorization.request)}</p>
      <p className="help-text">
        {label(
          "The task ends as cancelled. Files and execution records are retained.",
          "任务将结束为已取消，文件和已有运行记录都会保留。",
        )}
      </p>
      <form
        data-dirty={!!reason}
        onSubmit={async (event) => {
          event.preventDefault();
          if (!reason.trim()) return;
          if (
            await action.submit("taskbelay_cancel_task", {
              reason: reason.trim(),
            })
          )
            close();
        }}
      >
        <fieldset disabled={busy}>
          <label>
            {label("Reason", "取消原因")}
            <textarea
              data-initial-focus
              required
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={label(
                "Why is this task ending?",
                "说明取消此任务的原因。",
              )}
            />
          </label>
        </fieldset>
        <ActionFeedback action={action} />
        {action.error && <DialogRefresh inspect={inspect} />}
        <div className="form-actions">
          <button type="button" disabled={action.pending} onClick={close}>
            {label("Keep task", "保留任务")}
          </button>
          <button className="danger-button" disabled={busy || !reason.trim()}>
            {action.pending
              ? label("Saving…", "正在保存…")
              : label("Cancel task", "取消任务")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
