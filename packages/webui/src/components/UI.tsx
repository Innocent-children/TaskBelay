import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { RequestError, type Mutate } from "../lib/api";
import { useLanguage } from "../lib/i18n";
import { statusLabel } from "../lib/presentation";

export function StatusBadge({ status }: { status: string }) {
  const { label } = useLanguage();
  const tone = ["done", "passed"].includes(status)
    ? "success"
    : ["failed", "timeout", "output_limit"].includes(status)
      ? "danger"
      : [
            "blocked",
            "interrupted",
            "interrupted_unknown",
            "invalidated",
          ].includes(status)
        ? "warning"
        : "neutral";
  return (
    <span className={`status-badge ${tone}`}>
      <span aria-hidden="true">
        {tone === "success"
          ? "✓"
          : tone === "danger"
            ? "×"
            : tone === "warning"
              ? "!"
              : "·"}
      </span>
      {statusLabel(status, label)}
    </span>
  );
}

export function ErrorNotice({ error }: { error: Error | null }) {
  const { label } = useLanguage();
  if (!error) return null;
  const messages: Record<string, string> = {
    RESPONSE_LOST: label(
      "The response was lost. Update records before another action; the operation may have been saved.",
      "响应丢失，操作可能已保存。请先更新记录，再决定下一步。",
    ),
    SAVED_REFRESH_FAILED: label(
      "Saved successfully, but the display could not be updated. Update records to read the result.",
      "操作已保存，但页面更新失败。请更新记录查看结果。",
    ),
    REVISION_CONFLICT: label(
      "This task changed. Update records before submitting again.",
      "任务记录已发生变化，请更新记录后再提交。",
    ),
    INVALID_ARGUMENT: label(
      "Some submitted values were rejected. Check the details below.",
      "提交内容未被接受，请查看下方说明并检查填写内容。",
    ),
    EXECUTION_RUNNING: label(
      "The original process is still running. Inspect it before trying recovery again.",
      "原进程仍在运行，请核对原运行后再处理恢复。",
    ),
    EXECUTION_PENDING: label(
      "The original execution must be handled first.",
      "请先处理尚未确定结果的原运行。",
    ),
    EXECUTION_UNCERTAIN: label(
      "The original result remains unknown. Inspect its side effects before continuing.",
      "原运行结果仍未知，请先核对实际副作用。",
    ),
  };
  const message =
    error instanceof RequestError ? messages[error.code] : undefined;
  return (
    <div className="notice notice-error" role="alert">
      <p>
        {message ||
          error.message ||
          label(
            "The request failed. Try updating records.",
            "读取失败，请尝试更新记录。",
          )}
      </p>
      {message && (
        <details>
          <summary>{label("Details", "详细说明")}</summary>
          <p className="technical-text">{error.message}</p>
        </details>
      )}
    </div>
  );
}

export function useAction(mutate: Mutate) {
  const [error, setError] = useState<Error | null>(null);
  const [saved, setSaved] = useState(false);
  const [pending, setPending] = useState(false);
  const sending = useRef(false);
  async function submit(tool: string, fields: Record<string, unknown>) {
    if (sending.current) return false;
    sending.current = true;
    setError(null);
    setSaved(false);
    setPending(true);
    try {
      const result = await mutate(tool, fields);
      if (result.ok) setSaved(true);
      else setError(result.error);
      return result.ok;
    } finally {
      sending.current = false;
      setPending(false);
    }
  }
  return {
    error,
    saved,
    pending,
    submit,
    reset: () => {
      setError(null);
      setSaved(false);
    },
  };
}

export function ActionFeedback({
  action,
  success,
}: {
  action: { error: Error | null; saved: boolean };
  success?: string;
}) {
  const { label } = useLanguage();
  return (
    <>
      <ErrorNotice error={action.error} />
      {action.saved && (
        <p className="form-success" role="status">
          ✓ {success || label("Saved", "已保存")}
        </p>
      )}
    </>
  );
}

export function DialogRefresh({ inspect }: { inspect: () => Promise<void> }) {
  const { label } = useLanguage();
  const [reading, setReading] = useState(false);
  return (
    <button
      type="button"
      disabled={reading}
      onClick={async () => {
        setReading(true);
        try {
          await inspect();
        } finally {
          setReading(false);
        }
      }}
    >
      {reading
        ? label("Updating…", "正在更新…")
        : label("Update and inspect records", "更新并核对记录")}
    </button>
  );
}

export function Dialog({
  title,
  busy,
  onClose,
  children,
}: {
  title: string;
  busy: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const { label } = useLanguage();
  useEffect(() => {
    const element = dialog.current!;
    element.showModal();
    element.querySelector<HTMLElement>("[data-initial-focus]")?.focus();
    return () => element.close();
  }, []);
  return (
    <dialog
      className="action-dialog"
      ref={dialog}
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <div className="dialog-heading">
        <h2 id={titleId}>{title}</h2>
        <button
          type="button"
          className="icon-button"
          disabled={busy}
          onClick={onClose}
          aria-label={label("Close dialog", "关闭对话框")}
        >
          ×
        </button>
      </div>
      {children}
    </dialog>
  );
}
