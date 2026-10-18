import type { CheckSummary, View } from "./api";

export type Label = (english: string, chinese: string) => string;

export function repositoryName(path: string) {
  return (
    path
      .replace(/[\\/]+$/, "")
      .split(/[\\/]/)
      .pop() || path
  );
}

export function taskTitle(request: string, length = 48) {
  const sentence = request
    .trim()
    .split(/[\n。！？]/u)[0]
    .replace(/\s+/g, " ");
  const lead = sentence.split(/[，,；;]/u)[0];
  const text = Array.from(lead).length >= 12 ? lead : sentence;
  return Array.from(text).length > length
    ? Array.from(text).slice(0, length).join("") + "…"
    : text;
}

export function duration(ms: number | null, label: Label) {
  if (ms === null) return label("Unknown", "未知");
  if (ms < 1000) return `${ms} ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)} ${label("s", "秒")}`;
  const seconds = Math.round(ms / 1000);
  return label(
    `${Math.floor(seconds / 60)}m ${seconds % 60}s`,
    `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`,
  );
}

export function dateTime(value: string | null | undefined, language: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "—"
    : date.toLocaleString(language === "zh" ? "zh-CN" : "en", {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
}

export function statusLabel(status: string, label: Label) {
  return (
    (
      {
        active: label("Active", "进行中"),
        blocked: label("Needs attention", "需处理"),
        done: label("Completed", "已完成"),
        cancelled: label("Cancelled", "已取消"),
        planned: label("Not started", "待执行"),
        doing: label("In progress", "进行中"),
        passed: label("Passed", "通过"),
        failed: label("Failed", "失败"),
        not_run: label("Not run", "未执行"),
        running: label("Running · saved", "运行中 · 保存记录"),
        reserved: label("Reserved", "已预留"),
        timeout: label("Timed out", "超时"),
        interrupted: label("Interrupted", "已中断"),
        interrupted_unknown: label("Unknown result", "结果未知"),
        invalidated: label("Invalidated", "已失效"),
        output_limit: label("Output limit", "输出超限"),
        unknown: label("Unknown", "未知"),
      } as Record<string, string>
    )[status] ?? status
  );
}

export function sourceLabel(source: string, label: Label) {
  return (
    (
      {
        core_execution: label("Local command", "本机命令执行"),
        human: label("User inspection", "用户实际检查"),
        claim: label("Agent statement", "代理说明"),
      } as Record<string, string>
    )[source] ?? "—"
  );
}

export function applicability(check: CheckSummary, view: View, label: Label) {
  if (!check.evidence_id) return label("No recorded result", "尚无实际结果");
  if (view.status !== "active")
    return label("Saved at task end", "结束时保存的记录");
  if (!view.refreshed)
    return label("Current code not inspected", "尚未核对当前代码");
  return check.applicable
    ? label("Applies to inspected conditions", "适用于已核对条件")
    : label("Does not apply to current conditions", "不适用于当前条件");
}
