# Core Response Contract

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

This contract governs Core MCP responses, implemented by `internal/mcp/results.go` and
`output_schemas.go`. Core reports the actual outcome, specific failure conditions and permitted next
operation. Hosts retain the complete response and follow its structured fields. Requests are described
in [Commands](COMMANDS_en.md); Host execution is documented in the shared Skill's `tool-results.md`.

## Success and failure

Every response includes `ok`, `request_id` and `tool`. The request ID correlates the invocation and the
tool identifies the actual operation. `ok=true` requires `result` and excludes `error`/`recovery`;
`ok=false` requires `error`/`recovery` and excludes `result`. Output schemas enforce these exclusive shapes.

Success means the tool performed the operation it reports. A node submission still requires reading
the saved Task, which may be BLOCKED, DONE or CANCELLED. Ordinary submissions return the Task at
`result`; open/get use `result.task`; next-action uses `result.action`; relocation preparation returns
`result.relocation_id` and `result.task`. Reads may include `recovery_assessment`; follow its
`next_advice` before executing the current Action.

## Error fields

All 18 tools use the same rules. `error.code` is a stable category; `message` states the specific failed
condition. For parameter and transition-condition errors it summarizes field paths and requirements.
Long summaries are bounded; `details[]` or `guard.failures[]` retains the complete conditions. Known
causes are not replaced with generic invalid-argument or operation-failed text.
Identified parameter failures use `details[]`: each entry has a request `path`, fixed `rule` and
specific required behavior in `message`. Type failures name the required type. Text, identifier,
duplicate-member, field-dependency and work-item-dependency errors state their actual requirements.
Missing fields, malformed JSON, invalid UTF-8 and duplicate object members are distinct failures.
Unsafe member names are reported at their containing object without echoing the name or value.
Array positions use `[index]`. Budget paths under `verification.*` identify the current Task budget or usage; `arguments` names the entire MCP argument object, and other parameter-error paths identify submitted members. Transition conditions use
`guard.guard_id` and `guard.failures[]`. Core returns every relevant field it can establish rather than
requiring the Host to guess from generic prose.

`VERIFICATION_BUDGET_EXCEEDED` identifies a quantity limit; `error.budget` provides `used`, `requested`
and `limit`, while `details[].rule` distinguishes automatic commands from retained records.
`VERIFICATION_NOT_ALLOWED` identifies full-suite or pending manual-check permission restrictions,
with the affected field in `details[]`. Completed user checks and existing acceptance decisions consume
no automatic commands and are independent of the pending manual-handoff permission.

Public failures exclude submitted content, configuration values, secrets, internal directories and
stacks. `repository_paths` is reserved for existing missing-manifest feedback. Counters expose only
the non-sensitive quantities described above.

Storage and repository-observation errors retain the failed check and known cause, such as an
undecodable saved snapshot, a different worktree instance, a Git timeout, or a classified SQLite lock,
read-only, capacity or I/O failure. Outer callers preserve explanations already provided by the failing
check. When a lower-level cause is unknown, state the known failed operation and diagnostic limit;
do not invent a disk, permission or input problem.

Missing success results, JSON encoding failures, oversized responses and request-ID generation failures
have distinct explanations. Encoding failures retain the actual tool and valid request ID. Failed ID
generation uses `request-unavailable` and states that the tool was not executed. Original checks still
own error categories and recovery decisions: additional explanation never grants retry permission or
proves zero writes.

## Recovery guidance

`recovery` requires `action`, `retry_safe` and `message`. Machine behavior follows `action`; the message
must describe the same operation. `retry_safe=false` neither proves a write nor requires renewed
permission for the indicated read.

| action | Host behavior |
| --- | --- |
| `correct_request` | Core proved zero writes at the handshake/read/creation/lifecycle input boundary. Keep the same tool, preserve existing identity fields outside allowed_paths and authorization, correct only `allowed_paths` from existing facts, and submit once |
| `correct_current_action` | Core proved zero writes. Check the same Action and current schema, change only `allowed_paths` using established facts, and submit one correction |
| `read_next_action`, `read_task`, `retry_read` | Perform the indicated read and handle current state; do not replay the rejected submission |
| `resolve_blocker`, `restore_or_abandon`, `use_origin_host`, `provision_worktree` | Follow the operation indicated by current state; ask only for missing user decisions |
| `repair_storage`, `report_internal_error`, `none` | Stop automatic submissions and report the specific problem and required operation; the message must not also request automatic retry |

Only `correct_current_action` and `correct_request` return `retry_safe=true` and nonempty `allowed_paths`; every path must
come from the current failure details. Missing explanations for established checks can be corrected;
user confirmation, acceptance and unknown test results cannot be fabricated through correction.
Check required members before submission. Do not add the response request_id to tools that do not accept it. If the correction fails, report the new specific problem.

`internal/recovery/correction.go` owns correction eligibility and allowed members for ordinary Actions.
Every rule in a failure must permit correction. Unknown rules, unsafe field paths, user decisions or
unknown results prevent automatic correction. Filtering details from a public response must not make
an otherwise ineligible failure retryable.

For missing, malformed or timed-out responses and uncertain writes, read the saved operation of the
original Task/Action before using a Core-authorized recovery path. Never reinterpret a failure as
success or substitute an earlier successful operation's result.

## Verification

`error_reasons_test.go` checks JSON, member-type and response-encoding failures across all 18 tools,
and executes nested-field, work-item dependency and corrupt-snapshot failures. Request-ID failures
are checked over an in-memory MCP transport to verify the actual tool identity.

MCP tests validate real success/failure responses against output schemas, covering result locations,
invalid mixed envelopes, detailed failures, quantity versus permission restrictions, zero-write
correction and uncertain outcomes. Shared Skill examples validate against the same interface for
Codex, DeepSeek, Claude Code and ZCode. Error changes update the implementation, schema, this contract and affected examples.

## WebUI HTTP mapping

WebUI uses separate HTTP DTOs with the same Core failure code, details and budget counters, plus
field_paths for form highlighting. workflow_write_state distinguishes not_committed from unknown:
the former is established by the current boundary, while the latter requires reading the saved
operation. HTTP recovery keeps the same correction prerequisites and never treats an uncertain
outcome as a parameter correction.

HTTP `correct_current_action` also returns nonempty `recovery.allowed_paths` and permits one correction
from established facts. HTTP paths retain the request's `payload.` prefix; ordinary MCP submission
tools accept those semantic members directly and remove that prefix. Both adapters use the same
eligibility decision while retaining their own response shape, status codes and form highlighting.

Recovery validates a retained blocker decision with the same repository rules as ordinary submission.
A still-applicable decision replays its saved payload. Further repository changes retain the original
blocker and pending operation and return `stop_for_repository_drift`. Restore the repository state
required by that saved decision, then recover the same Action; a new submission cannot replace the
pending decision.

A decision for an unexecuted pre-write request checks the complete layer delta from the saved
binding to the current observation. Unchanged carried content does not conflict with this decision
and gains no permission to change. Additions, deletions or changes to any file layer outside the
pending and planned scope return `REPOSITORY_DRIFT` without staging a new Action operation.
Recovery applies the same check to a retained decision; a new decision cannot replace a pending
operation.


## Requests and complete response examples

All four Host Skills link a complete successful response and place an error response directly after every complete MCP request. Success files include the resolved request using current Task values. Tests execute it through the application and store with fixed repository observations and compare the complete response; generated identities, timestamps and operation digests use stable example values. Codex Host helpers and DeepSeek workspace requests also have complete responses checked by actual adapter operations in temporary Git repositories, with explicitly simulated Host sessions and terminal Core reads. Each error pair states
its failure condition and the functions responsible for validation and encoding. Tests construct the
described failed input from the preceding valid request and compare the complete response, including
message, details, guard and recovery. The examples illustrate supported failures rather than exhaust
all runtime conditions. Existing shared error examples also match the current encoder exactly.

History resolution reports enum failures at `history_resolution.choice` and text failures at `history_resolution.reason` independently, returning both when both members are invalid.

## History pages and branch rename

`taskbelay_get_task` may return `result.baseline_history` with `task_id`, Task `revision`, actual
cumulative `total`, `entries` (`sequence`, `reference`) and `next_after` (null only at the end). Every
Task response exposes a bounded first page as `baselines.history`, alongside mandatory `history_total`,
`history_next_after` and `history_revision`; the first page is not the complete history. Pages contain
at most 32 entries and 64 KiB of actual JSON. The entire response remains capped at 1 MiB, shrinking
pages further without skipping entries when necessary. An inline first page may be empty with a
nonzero total and next cursor 0; the Host then requests explicit pagination from 0. An explicitly
requested nonempty page retains at least one reference. Oversized non-history content still receives
the existing encoding error. Pagination retains actual references without archiving or reconstructing
full text. A changed Task revision returns `REVISION_CONFLICT`; restart at the new revision. Invalid
bounds return `INVALID_ARGUMENT`. Missing revision numbers, duplicate or malformed snapshot references
return `STORAGE_UNAVAILABLE`, never an empty success or deletion. Reads do not upgrade the database.

`taskbelay_prepare_task_branch_rename` returns `result.rename_id` and `result.task`. Preparation
replaces the Action with BLOCKED and retains `task.branch_rename`. `taskbelay_resolve_blocker` takes
the current `action_id`, the exact `rename_id` and `rename_choice:"complete"` or `"cancel"`, and returns
the complete Task. Do not combine rename with another resolution kind. Complete verifies source ref
absence, the target ref and unchanged repository, HEAD, index and content facts for every bound
repository; cancel verifies the unchanged original facts. Ref/history disagreement is
`WORKSPACE_HISTORY_CONFLICT`, changed index/content is `REPOSITORY_DRIFT`, stale revision/Action is
`REVISION_CONFLICT`/`ACTION_STALE`, and a pending ordinary operation is `RECOVERY_UNAVAILABLE`.
An uncertain decision commit retains its exact operation for `taskbelay_recover_action`; never
repeat Git from a missing response. Preparation readback uses the Task's saved rename ID. Ordinary
Task cancellation also verifies either unchanged original or fully renamed facts before releasing claims.

The response examples and concrete same-tool failures are maintained in
`skills/taskbelay/core/core-lifecycle-examples.md`; executable checks use MCP dispatch and the public
output schemas. Host launch supersession errors are separate adapter results and never imply a Core
transition. See [workspace sources](WORKTREE-SOURCES_en.md) for receipt and successor recovery.
