import { createHash } from "node:crypto";
import { isAbsolute, resolve } from "node:path";
import { coreJSON } from "../../lib/runtime.mjs";
export function preparedWriteFromHook(event) {
  if (event?.hook_event_name !== "PreToolUse" || !["Write", "Edit", "NotebookEdit"].includes(event.tool_name)) return null;
  if (typeof event.cwd !== "string" || !isAbsolute(event.cwd)) throw new Error("Hook cwd must be absolute");
  const raw = event.tool_input?.[event.tool_name === "NotebookEdit" ? "notebook_path" : "file_path"];
  const complete = typeof raw === "string" && raw.length > 0 && !raw.includes("\0");
  return { host: "claude", repository_path: event.cwd, tool_name: event.tool_name,
    paths: complete ? [resolve(event.cwd, raw)] : [], path_parse_complete: complete,
    intent_digest: createHash("sha256").update(JSON.stringify({ tool_name: event.tool_name, tool_input: event.tool_input })).digest("hex") };
}
export async function runHook(event, check = async input => (await coreJSON(["host-check", "pre-file-write"], input)).result) {
  const input = preparedWriteFromHook(event);
  if (!input) return null;
  const result = await check(input);
  if (!["allow", "deny"].includes(result?.decision)) throw new Error("Invalid Core write decision");
  if (result.decision === "allow") return null;
  return { hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "deny",
    permissionDecisionReason: result.reason || "TaskBelay stopped this write." } };
}
