import { useEffect, useRef, useState } from "react";
import { TaskList } from "../components/TaskList";
import { TaskDetail } from "../components/TaskDetail";
import { ErrorNotice } from "../components/UI";
import type { Mutate } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { useTask } from "../lib/useTask";

export function App() {
  const { language, setLanguage, label } = useLanguage();
  const task = useTask();
  const latest = useRef(task);
  latest.current = task;
  const [listRevision, refreshList] = useState(0);
  const [savedFor, setSavedFor] = useState("");
  const detailScroll = useRef<HTMLElement>(null);
  useEffect(() => {
    if (detailScroll.current) detailScroll.current.scrollTop = 0;
  }, [task.selectedId]);
  useEffect(() => {
    function navigate() {
      setSavedFor("");
      const match = /^\/tasks\/([^/]+)\/?$/.exec(window.location.pathname);
      if (match) {
        try {
          void latest.current.select(decodeURIComponent(match[1]));
        } catch {
          latest.current.clear();
        }
      } else latest.current.clear();
    }
    navigate();
    window.addEventListener("popstate", navigate);
    return () => window.removeEventListener("popstate", navigate);
  }, []);
  useEffect(() => {
    const timer = window.setInterval(() => {
      const current = latest.current;
      if (
        document.visibilityState !== "visible" ||
        current.busy ||
        current.mustRead ||
        document.querySelector('[data-dirty="true"]') ||
        document.activeElement?.matches("input,textarea,select")
      )
        return;
      refreshList((value) => value + 1);
      if (current.view?.status === "active")
        void current.select(current.view.identity.task_id, false, true);
    }, 15000);
    return () => window.clearInterval(timer);
  }, []);
  function select(id: string) {
    setSavedFor("");
    if (id !== task.selectedId)
      window.history.pushState(null, "", "/tasks/" + encodeURIComponent(id));
    void task.select(id);
  }
  function back() {
    window.history.pushState(null, "", "/");
    task.clear();
    setSavedFor("");
  }
  async function refresh(actual = false) {
    refreshList((value) => value + 1);
    if (task.selectedId) await task.select(task.selectedId, actual);
  }
  const change: Mutate = async (tool, fields) => {
    const id = latest.current.selectedId;
    setSavedFor("");
    const result = await task.mutate(tool, fields);
    if (result.ok && latest.current.selectedId === id) setSavedFor(id);
    refreshList((value) => value + 1);
    return result;
  };
  return (
    <div className={"app-shell" + (task.selectedId ? " has-selection" : "")}>
      <a className="skip-link" href="#task-content">
        {label("Skip to task details", "跳到任务详情")}
      </a>
      <header className="app-header">
        <a
          className="brand"
          href="/"
          onClick={(event) => {
            event.preventDefault();
            back();
          }}
        >
          TaskBelay
          <span className="brand-dot" aria-hidden="true" />
        </a>
        <span className="header-description">
          {label(
            "Development progress and check records",
            "开发进展与检查记录",
          )}
        </span>
        <div className="header-actions">
          <label className="language-picker">
            <span className="sr-only">{label("Language", "语言")}</span>
            <select
              aria-label={label("Language", "语言")}
              value={language}
              onChange={(e) => setLanguage(e.target.value as "en" | "zh")}
            >
              <option value="en">English</option>
              <option value="zh">中文</option>
            </select>
          </label>
          <button
            disabled={task.busy}
            onClick={() => refresh()}
            aria-label={label("Update records", "更新记录")}
          >
            <span aria-hidden="true">↻</span>
            <span className="record-button-label">
              {" "}
              {label("Update records", "更新记录")}
            </span>
          </button>
        </div>
      </header>
      <main className="workspace">
        <TaskList
          selectedId={task.selectedId}
          revision={listRevision}
          select={select}
        />
        <article
          className="detail-scroll"
          id="task-content"
          ref={detailScroll}
          aria-busy={task.loading}
        >
          <div className="detail-content">
            <button className="mobile-back text-button" onClick={back}>
              ← {label("Task list", "任务列表")}
            </button>
            <ErrorNotice error={task.error} />
            {savedFor === task.selectedId && savedFor && (
              <p className="form-success" role="status">
                ✓ {label("Your change has been saved.", "本次操作已保存。")}
              </p>
            )}
            {task.mustRead && (
              <div className="notice notice-warning">
                <p>
                  {label(
                    "Actions are paused until you explicitly update saved records.",
                    "操作暂时停用，请主动更新记录，确认已保存的实际结果。",
                  )}
                </p>
                <button disabled={task.busy} onClick={() => refresh()}>
                  {label("Update saved records", "更新保存记录")}
                </button>
              </div>
            )}
            {task.view ? (
              <TaskDetail
                key={task.view.identity.task_id}
                view={task.view}
                summary={task.summary}
                busy={task.mutationBlocked}
                saving={task.saving}
                readAt={task.readAt}
                checkedAt={task.checkedAt}
                inspect={() => refresh(true)}
                mutate={change}
              />
            ) : (
              <div className="welcome-state">
                <span className="welcome-mark" aria-hidden="true">
                  ✓
                </span>
                <span className="eyebrow">
                  TASKBELAY · {label("LOCAL", "本机")}
                </span>
                <h1>
                  {task.loading
                    ? label("Reading task…", "正在读取任务…")
                    : label("Your work, at a glance", "看清进展，再继续")}
                </h1>
                <p>
                  {task.loading
                    ? label(
                        "Loading its saved progress and check records.",
                        "正在加载保存的进展和检查记录。",
                      )
                    : label(
                        "Choose a task to see its results, remaining work and the decisions that need you.",
                        "选择一个任务，查看已完成的结果、剩余工作和需要你处理的事项。",
                      )}
                </p>
                <span className="welcome-note">
                  {label(
                    "Saved records stay local. Start new work in your coding agent.",
                    "任务记录保存在本机。新任务从编码代理中开始。",
                  )}
                </span>
              </div>
            )}
          </div>
        </article>
      </main>
    </div>
  );
}
