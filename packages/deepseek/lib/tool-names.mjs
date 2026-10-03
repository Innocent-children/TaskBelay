export const TASKBELAY_SERVER_NAME = "taskbelay";
export const TASKBELAY_TOOL_NAMESPACE_PREFIX = `mcp__${TASKBELAY_SERVER_NAME}__`;

export const TASKBELAY_RAW_TOOL_NAMES = Object.freeze([
  "taskbelay_server_info",
  "taskbelay_open_task",
  "taskbelay_get_task",
  "taskbelay_get_next_action",
  "taskbelay_submit_requirements",
  "taskbelay_submit_design",
  "taskbelay_submit_tasks",
  "taskbelay_submit_implementation",
  "taskbelay_submit_test",
  "taskbelay_submit_comprehension",
  "taskbelay_submit_refactor",
  "taskbelay_submit_delivery",
  "taskbelay_prepare_task_branch_rename",
  "taskbelay_prepare_task_relocation",
  "taskbelay_resolve_blocker",
  "taskbelay_recover_action",
  "taskbelay_cancel_task",
  "taskbelay_abandon_task",
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
