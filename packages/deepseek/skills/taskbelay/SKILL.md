---
name: taskbelay
description: "Use TaskBelay for authorized development across components or sessions: track actual scope, verification provenance, cumulative cost and recovery. Preserve existing user authorization and ask only for necessary decisions."
---

# TaskBelay for DeepSeek Harness

Follow [engineering work](references/engineering.md), the [current protocol](references/protocol.md) and [optional workspaces](references/workspace.md).
The agent performs coding and technical judgment; Core owns Task decisions and evidence.

Use actually visible local MCP tools. If unavailable, report it and repair the configured integration
within authorization; do not silently claim controlled execution. The installed adapter checks the exact 12-tool namespace and retains ordinary Host permissions.

The structured write hook covers write/edit/str_replace_editor; other edits are observed at checkpoints.
Hooks check known paths and actual authorization restrictions. They cannot determine the semantics
of future edits or authenticate user conversation. Never describe unsupported shell interception as
implemented.

Before every new Task, ask the user to choose the current branch, a new branch in the current directory,
or a dedicated worktree. Show the actual directory, branch and initial changes; do not select a default
or reuse another Task's workspace choice. Resume an existing Task in its saved workspace without
asking again. Existing authorization for that Task remains
valid across ordinary follow-ups. Show the understanding and complete plan, obtain the actual user answer, then record taskbelay_confirm_plan for that exact plan_digest before implementation. Reuse a prior answer only if it explicitly approved this unchanged displayed plan; never treat a general development request as approval of an unseen plan.
A new chat or Git branch/worktree operation is optional and requires actual authorization. Use workspace_coordinator for explicitly authorized preparation; it does not control Core progress.
Use its returned repositories with open_task. Task workspaces remain bound; cross-workspace
relocation is unsupported. Restore the original workspace or cancel without deleting work.

Native tool permissions apply to Core-executed checks. Avoid passing credentials in argv. Retain
complete responses and operation identities. After response loss read the saved run; do not rerun
commands from memory. A real user observation, an agent claim and a Core process result are distinct.

When a task is done or cancelled, report the actual result and remaining limitations. Commit, push,
PR, merge, publish and deletion still need their own authorization.

Implementation: `internal/application/service.go`, `internal/mcp/server.go`, `packages/deepseek/lib/` and the packaged structured write hook.
