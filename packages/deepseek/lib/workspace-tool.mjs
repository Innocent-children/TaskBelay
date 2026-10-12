import { defineTool } from "@deepseek-ai/dsh-tools";

import {
  WORKSPACE_COORDINATOR_TOOL,
  authorizeWorkspaceExecution,
  createWorkspaceCoordinator,
} from "./workspace-coordinator.mjs";

export function registerWorkspaceCoordinator(ctx, options) {
  const coordinator = createWorkspaceCoordinator({
    ...options,
    readTask: options?.readTask ?? (async (request) => await readCoreTask(ctx, request)),
  });
  const disposeGuard = ctx.tools.guard((execution) => {
    if (execution?.name !== WORKSPACE_COORDINATOR_TOOL) return undefined;
    try {
      authorizeWorkspaceExecution(execution);
      return undefined;
    } catch (error) {
      return error.message;
    }
  });
  const disposeTool = ctx.tools.register(defineTool({
    name: WORKSPACE_COORDINATOR_TOOL,
    description: "Prepare the workspace selected for this new Task after asking the user: current branch, new local branch or dedicated worktree; there is no default. Local launches return ready for the current session. Reuse actual authorization; ask only for missing workspace or deletion authority.",
    parameters: {
      operation: { type: "string", required: true, enum: ["provision", "consume", "prepare_cleanup", "cleanup_worktree", "cleanup_branch"] },
      authorization_ref: { type: "string", description: "Actual workspace or deletion authorization reference; retained authorization remains valid." },
      request: { type: "string", description: "Exact admitted development request for a new provision operation." },
      profile: { type: "string", description: "Current DSH Profile name." },
      launch_id: { type: "string", description: "Provisioning launch identity returned by provision." },
      task_id: { type: "string", description: "Fresh terminal Core Task identity for cleanup." },
      revision: { type: "integer", description: "Fresh terminal Core Task revision for cleanup." },
      repository_key: { type: "string", description: "Receipt-owned repository selected for cleanup." },
      source_repository_path: { type: "string", description: "Current source checkout used only for separately authorized branch cleanup; never persisted." },
      repositories: {
        type: "array",
        description: "Confirmed repositories in primary-first order.",
        items: {
          type: "object",
          additionalProperties: false,
          properties: {
            repository_key: { type: "string", required: true },
            workspace_mode: { type: "string", required: true, enum: ["new_branch", "current_branch", "dedicated_worktree"] },
            source_repository_path: { type: "string", required: true },
            source_type: { type: "string", required: true, enum: ["local", "remote"] },
            carry_changes: { type: "boolean", required: true },
            remote_name: { type: "string", required: true },
            base_branch: { type: "string", required: true },
            target_branch: { type: "string", required: true },
          },
        },
      },
    },
    output: {
      schema: { type: "json" },
      render(_arguments, value) {
        return [{ type: "text", text: JSON.stringify(value) }];
      },
    },
    timeoutMs: 10 * 60_000,
    async execute(arguments_, execution) {
      authorizeWorkspaceExecution({ ...execution, name: WORKSPACE_COORDINATOR_TOOL, arguments: arguments_ });
      if (arguments_.operation === "provision") {
        return await coordinator.provision({
          request: arguments_.request,
          profile: arguments_.profile,
          repositories: arguments_.repositories,
          signal: execution.signal,
        });
      }
      if (arguments_.operation === "consume") return await coordinator.consume({ launchID: arguments_.launch_id, signal: execution.signal });
      if (arguments_.operation === "prepare_cleanup") {
        return await coordinator.prepareCleanup({
          launchID: arguments_.launch_id, repositoryKey: arguments_.repository_key,
          taskID: arguments_.task_id, revision: arguments_.revision,
          sourceRepositoryPath: arguments_.source_repository_path,
          signal: execution.signal, execution,
        });
      }
      if (arguments_.operation === "cleanup_worktree") {
        const result = await coordinator.cleanupWorktree({
          launchID: arguments_.launch_id, repositoryKey: arguments_.repository_key,
          taskID: arguments_.task_id, revision: arguments_.revision,
          signal: execution.signal, execution,
        });
        execution.concludeTurn();
        return result;
      }
      const result = await coordinator.cleanupBranch({
        launchID: arguments_.launch_id, repositoryKey: arguments_.repository_key,
        taskID: arguments_.task_id, revision: arguments_.revision,
        sourceRepositoryPath: arguments_.source_repository_path,
        signal: execution.signal, execution,
      });
      execution.concludeTurn();
      return result;
    },
  }));
  return () => {
    disposeTool();
    disposeGuard();
  };
}

async function readCoreTask(ctx, { taskID, signal, execution }) {
  const result = await ctx.tools.execute({
    callId: `${execution.callId}:terminal-task`,
    rootCallId: execution.rootCallId ?? execution.callId,
    parent: execution.token,
    name: "mcp__taskbelay__taskbelay_get_task",
    arguments: { task_id: taskID, refresh:false },
    agent: execution.agent,
    signal,
  });
  const text = result.content.filter((block) => block.type === "text").map((block) => block.text).join("\n");
  const start = text.indexOf("{");
  if (result.isError || start < 0) throw new Error("terminal Core Task read failed");
  const envelope = JSON.parse(text.slice(start));
  if (envelope?.ok !== true || envelope.result?.task === undefined) throw new Error("terminal Core Task read failed");
  return envelope.result.task;
}
