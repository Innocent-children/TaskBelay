<!-- Generated from skills/taskbelay/core/workspace.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Optional workspace operations

Use the user's selected workspace, or the current workspace when the request makes it clear. Show the
actual directory, branch and existing changes. Do not ask the same workspace question for every Task.
Resume an existing Task in its saved workspace; cross-workspace relocation is unsupported.

Read this reference only when branch/worktree preparation, upstream work or cleanup is actually needed.
Use the packaged deepseek helper and retain the actual authorization_ref. Prepare the requested source,
branch and carry choice. Its scope result provides repositories for open_task; Core observes identity
and owns Task progress. A workspace receipt is an operation record, not another Task.

Check upstream when the task requires it. Fetch and integration require actual Git authority; neither
is a universal prerequisite for local edits. Codex/Claude/ZCode use host-launch freshness; DeepSeek uses
workspace_coordinator freshness with freshness_repositories. Missing checks are not_checked, never
latest. Copy supplied observations into open_task or checkpoint.repository_freshness.

Use original workspace_origin.source_type, remote_name and base_branch when bound. Local sources use
that branch's upstream; remote sources use refs/remotes/<remote_name>/<base_branch>. Do not substitute
a task branch's new upstream. Integrate only separately authorized clean fast-forwards, with stable
operation_id and integration_authorization_ref. Inspect each repository result and retain receipt_path.

After interruption, read the original receipt. recover:true uses the original request and only reconciles
Git facts; it does not repeat commands, continue remaining roots or undo completed work. Unknown command
outcomes remain unknown. Preserve dirty, diverged, failed or partial results. Never delete locks or edit
receipts to bypass an unresolved operation.

Cleanup needs actual deletion authority, the exact terminal Core task, matching workspace and its
provisioning_receipt_id. Request get_task detail:true to read details.task when a helper needs complete
workspace bindings. Dirty or unpushed work is retained. Verify Host-specific new-chat capabilities only
when the user requests that operation.

Maintainer implementation reference: `internal/application/origin_input.go`.
