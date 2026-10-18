export const TASKBELAY_SERVER_NAME = "taskbelay";
export const TASKBELAY_TOOL_NAMESPACE_PREFIX = `mcp__${TASKBELAY_SERVER_NAME}__`;

export const TASKBELAY_RAW_TOOL_NAMES = Object.freeze([
  "taskbelay_server_info",
  "taskbelay_open_task",
  "taskbelay_get_task",
  "taskbelay_checkpoint",
  "taskbelay_resolve_issue",
  "taskbelay_revise_task",
  "taskbelay_record_evidence",
  "taskbelay_get_run",
  "taskbelay_recover_run",
  "taskbelay_finish_task",
  "taskbelay_cancel_task",
  "taskbelay_list_tasks",
  "taskbelay_accept_known_failure",
  "taskbelay_update_budget",
  "taskbelay_archive_task"
]);

export const TASKBELAY_QUALIFIED_TOOL_NAMES = Object.freeze(
  TASKBELAY_RAW_TOOL_NAMES.map((rawName) => `${TASKBELAY_TOOL_NAMESPACE_PREFIX}${rawName}`),
);

const expectedQualifiedTools = new Set(TASKBELAY_QUALIFIED_TOOL_NAMES);

export function isTaskBelayNamespaceTool(name) {
  return typeof name === "string" && name.startsWith(TASKBELAY_TOOL_NAMESPACE_PREFIX);
}

export function isExpectedTaskBelayTool(name) {
  return expectedQualifiedTools.has(name);
}

export function assertQualifiedToolCatalog(toolNames, { allowUnavailable = false } = {}) {
  const namespaceTools = [...toolNames]
    .filter(isTaskBelayNamespaceTool)
    .sort();
  if (allowUnavailable && namespaceTools.length === 0) return Object.freeze([]);

  const expected = [...TASKBELAY_QUALIFIED_TOOL_NAMES].sort();
  if (
    namespaceTools.length !== expected.length
    || namespaceTools.some((name, index) => name !== expected[index])
  ) {
    throw new Error(
      `TaskBelay tool catalog mismatch: received ${JSON.stringify(namespaceTools)}; expected ${JSON.stringify(expected)}`,
    );
  }
  return TASKBELAY_QUALIFIED_TOOL_NAMES;
}
