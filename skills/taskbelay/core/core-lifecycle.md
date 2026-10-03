# Core relocation and terminal operations

Read [response handling](tool-results.md) and the [Host transport](transport.md) before calling Core.

Read the [Host lifecycle](host-lifecycle.md) for available workspace operations and their authorization.

Field types: request IDs, `host`, `reason` and repository paths are strings; `revision` is an
integer. `relocation_destinations` is an array of objects with string `key` and `repository_path`.
Success `ok` is a boolean and `result` is an object. Relocation preparation returns string
`result.relocation_id` and object `result.task`; the other operations return the Task object
directly in `result`. Follow the [shared field-type rules](tool-results.md#json-field-types).

## Rename one active Task branch

Implementation: `internal/application/branch_rename.go` — `PrepareTaskBranchRename`, `resolveTaskBranchRename`.

After authorization for the specific rename, call `taskbelay_prepare_task_branch_rename` with the
current host, Task ID, revision, one repository_key, target_branch and reason. The result contains
rename_id and the complete BLOCKED Task. Retain its source/target names, frozen repository/index
facts, new Action and resume node. A pending earlier Action must be recovered first.

[Preparation and paired error](core-lifecycle-examples.md#taskbelay_prepare_task_branch_rename-prepare-branch-rename).

The authorized Host executes exactly one `git -C <saved-root> branch -m <source> <target>` without
force. Core never executes Git mutations. Verify the retained identity again before execution.
If interrupted, read the same Task and inspect refs; never repeat Git blindly. After the actual
rename, [resolve complete](core-lifecycle-examples.md#taskbelay_resolve_blocker-rename-complete) with
its current Action ID and rename_id. Core requires the source ref to be absent, target/current branch
to match, and every repository instance, HEAD, index and content to remain unchanged. No partial or
uncertain state authorizes completion. [Cancel the rename](core-lifecycle-examples.md#taskbelay_resolve_blocker-rename-cancel)
only while all original source facts and absent target remain unchanged. Either decision returns to
the saved node with a fresh Action. A saved unrecorded resolution uses `taskbelay_recover_action`.

WorkspaceOrigin and receipt target_branch retain the creation selection. Subsequent history review,
relocation, display and authorized cleanup must use the Core repository.current_branch as the
effective branch. Do not substitute an observed unapproved branch or edit a provisioning receipt.

## Read complete baseline references

Implementation: `internal/domain/baseline_history.go` — `ReadBaselineHistory`;
`internal/store/baseline_history.go` — `validateBaselineHistoryMutation`.

The Task retains its complete saved history without an archive tier or configured count limit;
resources and revision-number ranges still apply. Every Task response provides a bounded first page
at baselines.history plus history_total, history_next_after and history_revision. Do not treat the
first page as the complete history. [Read a page](core-lifecycle-examples.md#taskbelay_get_task-history-page)
with baseline_history {revision:0, after:0, limit:16}, then retain result.baseline_history.revision and
next_after for each following page (limit 1..32). Pages shrink for actual JSON bytes, so never compute
the next cursor by adding limit. next_after=null alone ends the read. If the inline page is empty
with nonzero total and cursor 0, explicitly request from 0. A changed Task rejects the cursor; restart
at after=0 without merging revisions. References preserve saved summaries/digests/times; no historical
full documents are reconstructed. History growth does not relax other input/evidence/response limits.

## Prepare relocation

Implementation: `internal/application/control_center_lifecycle.go` — `PrepareTaskRelocation`.
Implementation: `internal/application/relocation.go` — `PrepareTaskRelocation`.

First verify that [Host lifecycle](host-lifecycle.md#relocation) supplies an available, authorized
relocation procedure. The Core tool alone does not supply a Host move. After explicit user authority, retain the current Core Task ID/revision and prepare once:

Relocation requires every repository to use `dedicated_worktree`. Local modes retain their original
directory; end or resume that Task there. Core rejects local relocation before changing Task state.

[Complete request, successful response and error example](core-lifecycle-examples.md#taskbelay_prepare_task_relocation-prepare).

Success projection: `{"ok":true,"result":{"relocation_id":"relocation-example",
"task":{"task_id":"task-example","current_cursor":"BLOCKED"}}}`. Retain the complete returned
Task, relocation ID, source bindings/content/surface and resume node. Claims remain bound to the
source during Handoff. If the response is lost, get the same Task and verify the saved relocation
and blocker at the expected successor revision; reuse only that matching identity.

## Complete relocation

Implementation: `internal/application/relocation.go` — `resolveTaskRelocation`, `validateTaskRelocationDestination`.

After actual Host success, use the current blocked Action, saved relocation ID and all verified destination roots:

[Complete request, successful response and error example](core-lifecycle-examples.md#taskbelay_resolve_blocker-relocation).

Core checks repository group, frozen base, equivalent content/surface and claim conflicts, then replaces bindings together. Success returns the complete Task directly; follow its current_action. Failed or uncertain Host movement keeps the original binding and claims and does not permit this resolution.

## Cancellation

Implementation: `internal/application/cancel_task.go` — `CancelTask`.
Implementation: `internal/mcp/schemas.go` — `buildCatalog`.

After the user explicitly cancels an active Task, obtain its current revision and create one fresh
cancellation request ID. Retain it before the call; use the actual user reason.

[Complete request, successful response and error example](core-lifecycle-examples.md#taskbelay_cancel_task-cancel).

Success returns the complete Task directly: `result.current_cursor` is CANCELLED,
`result.current_action` is null and `result.outcome` explains termination. It does not delete Git data.
Core must still observe the worktree. After response loss, call get_task and compare
`task.last_operation.operation_id` with the retained cancellation request, kind `cancel_task`, and
terminal outcome. Report a different terminal operation accurately; active/mismatched/uncertain reads
stop for inspection. Do not invoke Action recovery or blindly repeat cancellation.

## Abandon an unavailable workspace

Implementation: `internal/application/abandon_task.go` — `AbandonTask`.

Use only when the original workspace instance is unavailable and the user explicitly abandons the
Task instead of restoring that instance. Core attempts an observation to establish unavailability.

[Complete request, successful response and error example](core-lifecycle-examples.md#taskbelay_abandon_task-abandon).

Success returns the CANCELLED Task and releases its claims while retaining the last known binding.
After a lost result, get_task and verify the terminal outcome, `last_operation.kind=abandon_task`
and expected successor revision. A live workspace, conflicting revision or mismatched result stops;
a same-named new directory cannot stand in for the original instance.
