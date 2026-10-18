import { useEffect, useRef, useState } from "react";
import { read, type Summary } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { dateTime, repositoryName, taskTitle } from "../lib/presentation";
import { ErrorNotice, StatusBadge } from "./UI";

export function TaskList({
  selectedId,
  revision,
  select,
}: {
  selectedId: string;
  revision: number;
  select: (id: string) => void;
}) {
  const { language, label } = useLanguage();
  const [tasks, setTasks] = useState<Summary[]>([]);
  const [error, setError] = useState<Error | null>(null);
  const [loading, setLoading] = useState(true);
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [host, setHost] = useState("");
  const [lifecycle, setLifecycle] = useState("");
  const [includeArchived, setIncludeArchived] = useState(false);
  const [page, setPage] = useState(1);
  const [hasNext, setHasNext] = useState(false);
  const [retry, setRetry] = useState(0);
  const request = useRef(0);
  const previousQuery = useRef("");
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(searchDraft);
      setPage(1);
    }, 220);
    return () => window.clearTimeout(timer);
  }, [searchDraft]);
  useEffect(() => {
    const ticket = ++request.current;
    const query = new URLSearchParams({
      page: String(page),
      include_archived: String(includeArchived),
      search,
      host,
      lifecycle,
    }).toString();
    const changed = query !== previousQuery.current;
    previousQuery.current = query;
    if (changed) {
      setTasks([]);
      setLoading(true);
      setHasNext(false);
    }
    setError(null);
    void read<{ items: Summary[]; has_next: boolean }>("/api/tasks?" + query)
      .then((result) => {
        if (ticket === request.current) {
          setTasks(result.items);
          setHasNext(result.has_next);
        }
      })
      .catch((value) => {
        if (ticket === request.current)
          setError(value instanceof Error ? value : new Error(String(value)));
      })
      .finally(() => {
        if (ticket === request.current) setLoading(false);
      });
    return () => {
      request.current++;
    };
  }, [page, includeArchived, search, host, lifecycle, revision, retry]);
  const filtered = !!(searchDraft || host || lifecycle || includeArchived);
  function clearFilters() {
    setSearchDraft("");
    setSearch("");
    setHost("");
    setLifecycle("");
    setIncludeArchived(false);
    setPage(1);
  }
  return (
    <aside className="task-sidebar" aria-label={label("Task list", "任务列表")}>
      <div className="sidebar-controls">
        <div className="section-heading">
          <h2>{label("Tasks", "任务")}</h2>
          <span className="eyebrow">{label("LOCAL", "本机记录")}</span>
        </div>
        <label className="search-field">
          <span className="sr-only">
            {label("Search request or task ID", "搜索需求或任务 ID")}
          </span>
          <span aria-hidden="true">⌕</span>
          <input
            type="search"
            placeholder={label("Search tasks…", "搜索任务…")}
            value={searchDraft}
            onChange={(e) => setSearchDraft(e.target.value)}
          />
        </label>
        <details className="filter-menu">
          <summary>
            {label("Filters", "筛选")}
            <span className="filter-count">
              {[host, lifecycle, includeArchived].filter(Boolean).length || ""}
            </span>
            <span className="muted">
              {label("Host · state", "宿主 · 状态")}
            </span>
          </summary>
          <div className="filter-fields">
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
              {label("State", "状态")}
              <select
                value={lifecycle}
                onChange={(e) => {
                  setLifecycle(e.target.value);
                  setPage(1);
                }}
              >
                <option value="">{label("All states", "全部状态")}</option>
                <option value="active">{label("Active", "进行中")}</option>
                <option value="blocked">
                  {label("Needs attention", "需处理")}
                </option>
                <option value="done">{label("Completed", "已完成")}</option>
                <option value="cancelled">
                  {label("Cancelled", "已取消")}
                </option>
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
            {filtered && (
              <button
                type="button"
                className="text-button"
                onClick={clearFilters}
              >
                {label("Clear filters", "清除筛选")}
              </button>
            )}
          </div>
        </details>
      </div>
      <div className="task-list-scroll" aria-busy={loading}>
        <ErrorNotice error={error} />
        {error && (
          <button onClick={() => setRetry((value) => value + 1)}>
            {label("Read list again", "重新读取列表")}
          </button>
        )}
        {loading && (
          <div className="list-loading" role="status">
            {label("Reading tasks…", "正在读取任务…")}
          </div>
        )}
        <nav aria-label={label("Saved tasks", "已保存任务")}>
          {tasks.map((item) => (
            <a
              href={"/tasks/" + encodeURIComponent(item.task_id)}
              key={item.task_id}
              className={
                "task-item" + (selectedId === item.task_id ? " selected" : "")
              }
              aria-current={selectedId === item.task_id ? "page" : undefined}
              title={item.request_summary}
              onClick={(event) => {
                if (
                  event.metaKey ||
                  event.ctrlKey ||
                  event.shiftKey ||
                  event.altKey
                )
                  return;
                event.preventDefault();
                select(item.task_id);
              }}
            >
              <span className="task-item-title">
                {taskTitle(item.request_summary)}
              </span>
              <span className="task-item-repository">
                {item.repositories
                  .map((r) => repositoryName(r.repository_path))
                  .join(" · ")}
              </span>
              <span className="task-item-meta">
                <span>{item.host}</span>
                <StatusBadge status={item.lifecycle} />
              </span>
              <span className="task-item-time">
                {item.archived ? label("Archived · ", "已归档 · ") : ""}
                {dateTime(item.updated_at, language)}
              </span>
            </a>
          ))}
        </nav>
        {!loading && !error && !tasks.length && (
          <div className="sidebar-empty">
            <p>
              {filtered
                ? label("No matching tasks", "没有匹配的任务")
                : label("No saved tasks yet", "还没有任务记录")}
            </p>
            <p className="muted">
              {filtered
                ? label(
                    "Try another search or clear your filters.",
                    "可以更换关键词或清除筛选条件。",
                  )
                : label(
                    "Start a task in your coding agent. Its progress will appear here.",
                    "在编码代理中开始任务，进展会显示在这里。",
                  )}
            </p>
            {filtered && (
              <button onClick={clearFilters}>
                {label("Clear filters", "清除筛选")}
              </button>
            )}
          </div>
        )}
      </div>
      <div className="pagination">
        <button
          className="icon-button"
          aria-label={label("Previous page", "上一页")}
          disabled={page <= 1 || loading}
          onClick={() => setPage(page - 1)}
        >
          ←
        </button>
        <span>
          {label("Page ", "第 ")}
          {page}
          {label("", " 页")}
        </span>
        <button
          className="icon-button"
          aria-label={label("Next page", "下一页")}
          disabled={!hasNext || loading}
          onClick={() => setPage(page + 1)}
        >
          →
        </button>
      </div>
    </aside>
  );
}
