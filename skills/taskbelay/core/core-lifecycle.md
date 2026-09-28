# Core relocation and terminal operations

Read [response handling](tool-results.md) and the [Host transport](transport.md) before calling Core.

Read the [Host lifecycle](host-lifecycle.md) for available workspace operations and their authorization.

Field types: request IDs, `host`, `reason` and repository paths are strings; `revision` is an
integer. `relocation_destinations` is an array of objects with string `key` and `repository_path`.
Success `ok` is a boolean and `result` is an object. Relocation preparation returns string
`result.relocation_id` and object `result.task`; the other operations return the Task object
directly in `result`. Follow the [shared field-type rules](tool-results.md#json-field-types).

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
