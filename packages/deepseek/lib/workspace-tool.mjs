import contracts from "../contracts/interfaces.json" with { type: "json" };
import { assessRepositoryFreshness } from "./repository-freshness.mjs";
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
    description: "Operate on the workspace actually selected for this request: current branch, new local branch or dedicated worktree. Reuse existing authority; ask only for missing choices or deletion authority. freshness returns per-repository remote_freshness and any retained integration operation; copy observations unchanged. provision returns launch_id, receipt_path, workspace_root and status; preserve them before switching sessions. consume returns open_task.repositories for Core open_task; it does not itself create a Task. prepare_cleanup returns the separately authorized source-workspace continuation. cleanup_worktree/cleanup_branch require the exact terminal task_id/revision and retained launch identity; successful cleanup may conclude the turn. action_required, partial and uncertain are not ready or proof of a new session. After failure inspect the original receipt/Task before repeating effects. Complete per-operation input/output schemas, field notes and adjacent success/failure examples are shipped in contracts/interfaces.json; no source or database access is required.",
    parameters: {
      operation: { type: "string", required: true, description: "Exact workspace operation; preserve the original launch_id during continuation or cleanup.", enum: ["freshness", "provision", "consume", "prepare_cleanup", "cleanup_worktree", "cleanup_branch"] },
      integration_authorization_ref:{type:"string",description:"Separate actual integration authorization; required for integrate=true."},
      operation_id:{type:"string",description:"Stable integration receipt identity; required with integrate=true. Reuse it after response loss."},
      recover:{type:"boolean",description:"Read-only stopped-owner reconciliation of the original integration; never replays fetch or merge."},
      fetch: {type:"boolean",description:"Fetch corresponding upstream refs; default true."},
      integrate: {type:"boolean",description:"Separately authorized clean fast-forward integration; default false."},
      freshness_repositories: {type:"array",items:{type:"object",additionalProperties:false,properties:{key:{type:"string",required:true,description:"Stable bound repository key."},repository_path:{type:"string",required:true,description:"Absolute actual worktree root."},source_type:{type:"string",enum:["local","remote"],description:"Original workspace_origin.source_type."},remote_name:{type:"string",description:"Original workspace_origin.remote_name."},base_branch:{type:"string",description:"Original workspace_origin.base_branch; remote sources do not require a local branch."}}}},
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
            repository_key: { type: "string", description: "Stable bound repository key.", required: true },
            workspace_mode: { type: "string", description: "Actual selected workspace mode; no implicit new-workspace authority.", required: true, enum: ["new_branch", "current_branch", "dedicated_worktree"] },
            source_repository_path: { type: "string", description: "Absolute source checkout inside the current Workspace Root.", required: true },
            source_type: { type: "string", description: "Original selected source type.", required: true, enum: ["local", "remote"] },
            carry_changes: { type: "boolean", description: "Actual user choice to retain existing local edits.", required: true },
            remote_name: { type: "string", description: "Original source remote, empty for local.", required: true },
            base_branch: { type: "string", description: "Original selected base branch.", required: true },
            target_branch: { type: "string", description: "Selected branch to create or retain.", required: true },
          },
        },
      },
    },
    output: {
      schema: { type: "json", description: "Operation-specific canonical result. Preserve receipt and launch identity; the installed contract specifies every output and failure rule.", examples: contracts.filter(entry=>entry.name.startsWith("deepseek.workspace_coordinator.")).map(entry => ({operation:entry.name.split('.').at(-1), description:entry.description, input_schema:entry.input_schema, output_schema:entry.output_schema, examples:entry.examples})) },
      render(_arguments, value) {
        return [{ type: "text", text: JSON.stringify(value) }];
      },
    },
    timeoutMs: 10 * 60_000,
    async execute(arguments_, execution) {
      authorizeWorkspaceExecution({ ...execution, name: WORKSPACE_COORDINATOR_TOOL, arguments: arguments_ });
      if (arguments_.operation === "freshness") return await assessRepositoryFreshness({repositories:arguments_.freshness_repositories,fetch:arguments_.fetch,integrate:arguments_.integrate,authorization_ref:arguments_.authorization_ref,integration_authorization_ref:arguments_.integration_authorization_ref,operation_id:arguments_.operation_id,recover:arguments_.recover},{receiptRoot:options?.dataDirectory});
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
    arguments: { task_id: taskID, refresh:false, detail:true },
    agent: execution.agent,
    signal,
  });
  const text = result.content.filter((block) => block.type === "text").map((block) => block.text).join("\n");
  const start = text.indexOf("{");
  if (result.isError || start < 0) throw new Error("terminal Core Task read failed");
  const envelope = JSON.parse(text.slice(start));
  if (envelope?.ok !== true || envelope.result?.identity === undefined || envelope.result?.details === undefined) throw new Error("terminal Core Task read failed");
  return envelope.result;
}
