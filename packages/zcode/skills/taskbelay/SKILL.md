---
name: taskbelay
description: "Use TaskBelay for authorized development across components or sessions: track actual scope, verification provenance, cumulative cost and recovery. Preserve existing user authorization and ask only for necessary decisions."
---

# TaskBelay for ZCode

Follow [engineering work](references/engineering.md), the [current protocol](references/protocol.md) and [optional workspaces](references/workspace.md).
The agent performs coding and technical judgment; Core owns Task decisions and evidence.

Use actually visible local MCP tools. If unavailable, report it and repair the configured integration
within authorization; do not silently claim controlled execution. The packaged MCP launcher exposes the same current Core contract.

The structured write hook covers the registered native structured write events; other edits are observed at checkpoints.
Hooks check known paths and actual authorization restrictions. They cannot determine the semantics
of future edits or authenticate user conversation. Never describe unsupported shell interception as
implemented.

Start in the authorized current workspace and session when possible. Existing authorization remains
valid across ordinary follow-ups; no repeated selector, plan verdict or phase confirmation is needed.
A new chat or Git branch/worktree operation is optional and requires actual authorization. Use taskbelay-zcode host-launch only for explicitly authorized workspace preparation; its receipt tracks that operation, not a second Task cursor.
Use its returned repositories with open_task. Task workspaces remain bound; cross-workspace
relocation is unsupported. Restore the original workspace or cancel without deleting work.

Native tool permissions apply to Core-executed checks. Avoid passing credentials in argv. Retain
complete responses and operation identities. After response loss read the saved run; do not rerun
commands from memory. A real user observation, an agent claim and a Core process result are distinct.

When a task is done or cancelled, report the actual result and remaining limitations. Commit, push,
PR, merge, publish and deletion still need their own authorization.

Implementation: `internal/application/service.go`, `internal/mcp/server.go`, `packages/zcode/lib/` and the packaged structured write hook.
