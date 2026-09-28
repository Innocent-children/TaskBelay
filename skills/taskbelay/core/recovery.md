# Recover an Action or resolve a blocker

Read [response handling](tool-results.md) and the [Host transport](transport.md) before calling Core.

Use the actual returned recovery advice or blocker; a missing display is not a missing retained response.

Field types: `host`, `task_id`, `action_id`, `choice` and `reason` are strings;
`history_resolution` is an object with string `choice` and `reason`. Success `result` is an object.
In read responses, `result.task` is an object and `result.recovery_assessment` is an object or
`null`; assessment `operation` is an object, IDs/`next_advice` are strings, and
`task_revision`/`expected_revision` are integers. Follow the
[shared field-type rules](tool-results.md#json-field-types).

## Uncertain Action recovery

Implementation: `internal/application/recover_action.go` — `RecoverAction`.

If an ordinary submit/resolve/recover response is genuinely missing or malformed, retain the original
Task/Action IDs and first call the [saved-state `taskbelay_get_task`](connection.md#read-saved-state-and-the-next-action). On fresh resume use
`recovery_assessment.operation.action_id`, not a possibly newer `current_action.action_id`.
A missing/mismatched/incomplete recovery assessment stops. Core retains the normalized payload;
the Host does not rebuild it.

Recovery response projection:

```json
{"ok":true,"result":{"task":{"task_id":"task-example","revision":4},"recovery_assessment":{"task_revision":4,"operation":{"action_id":"saved-action","operation_id":"saved-operation","expected_revision":4},"next_advice":"submit_recovery_apply"}}}
```

| Returned next_advice | Next interaction |
| --- | --- |
| `retry_current_action` | After a saved-state read, obtain `taskbelay_get_next_action` and handle its recovery/blocker first. Re-perform only the returned Action's allowed work, then recover the saved Action. |
| `submit_recovery_apply` | Recover the saved Action immediately. |
| `read_next_action` | Consume the complete guarded Action already returned by open/next-action; after get_task obtain one get_next_action. |
| `resolve_blocker` | Restore the required repository condition, then resolve the current blocked Action. |
| `stop_for_repository_drift` | Report the retained condition and stop. |

Recovery call example for either of the first two returned instructions:

[Complete request, successful response and error example](recovery-examples.md#taskbelay_recover_action-saved-operation).

Success is the complete Task in `result`, with next `result.current_action` or terminal outcome.
Follow it; do not resubmit the old node payload. An unknown instruction stops. This table applies
Core's returned advice; it does not recompute recovery classification from file state.
Creation uses the [connection readback rules](connection.md#open-or-resume-a-task); cancellation,
abandonment and relocation use [Core lifecycle](core-lifecycle.md), never Action recovery.

## Resolve blockers

Implementation: `internal/mcp/schemas.go` — `buildCatalog`.
Implementation: `internal/workflow/submission_schema.go` — `CurrentSubmissionSchema`.

Read the fresh blocker and Action first. File-scope choices require an explicit user choice and reason.
Show the retained paths and proposed purpose. Example: “The write includes src/extra.js outside the
plan. Allow this exact write once, revise the plan, or restore the path?” Reuse only a still-valid
answer for this exact write. The three complete input alternatives are:

| User choice | Complete request and responses |
| --- | --- |
| Allow this exact write once | [allow_once](recovery-examples.md#taskbelay_resolve_blocker-allow_once) |
| Revise the plan | [expand_scope](recovery-examples.md#taskbelay_resolve_blocker-expand_scope) |
| Restore the path | [reject](recovery-examples.md#taskbelay_resolve_blocker-reject) |

`allow_once` is restricted to the same prepared intent/path set; `expand_scope` returns to TASKS to
save revised expected paths with tasks_plan_saved and explicitly confirm that current plan before tasks_ready (use its requirements return only for a requirement change); `reject` resumes
only after actual restoration. None of these expands immutable Repository Scope.

For `repeated_verification_failure`, `unchanged_verification_result` or
`unchanged_test_implementation_loop`, stop and ask for a different approach, one further attempt or
cancellation. After the explicit retry/approach answer, or after a recovery blocker's required
repository condition is restored, use only identity fields:

[Complete request, successful response and error example](recovery-examples.md#taskbelay_resolve_blocker-verification-or-recovery).

For a workspace-history blocker, complete the separately authorized Git operation first, then:

[Complete request, successful response and error example](recovery-examples.md#taskbelay_resolve_blocker-history).

Success returns the full Task directly; continue only from its current Action. Rejection leaves the
condition unresolved. Relocation uses the distinct [relocation input](core-lifecycle.md#complete-relocation);
do not mix file-scope, history and relocation members. Workspace unavailability requires restoration
of the original instance or explicit abandonment, not recreation of a same-named directory.
