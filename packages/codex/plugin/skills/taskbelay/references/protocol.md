<!-- Generated from skills/taskbelay/core/protocol.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Current Core interface

Use the actual server's tool names. Core publishes closed input/output Schemas with field descriptions,
defaults and supported values. The same methods serve MCP, CLI call and local HTTP. New Tasks use host codex.

## Discover a method without source

Read taskbelay_server_info for actual capabilities. Its optional contracts array selects exact method names
for complete schemas and adjacent success/failure examples; it does not run those methods. Native check.run
and host-check.pre-file-write/workspace-available also have discoverable contracts, but are not MCP execution tools.
For native calls, use the configured Core or Host launcher's call <tool> --help --json, check run --help --json,
or host-check <operation> --help --json. Help does not open the Task database. Keep package help/resources
available when installed away from the repository.

| Method | Input and result |
| --- | --- |
| taskbelay_server_info | Optional contracts; actual tools, capabilities and selected complete method contracts |
| taskbelay_open_task | operation_id, host, repositories, authorization, plan; identity, current context and gaps |
| taskbelay_get_task | task_id, refresh (default true), detail (default false); current context and optional full records |
| taskbelay_checkpoint | identity; optional assessment/progress/issue/repository_freshness/host_usage; compact current state |
| taskbelay_revise_task | identity, reason, complete authorization and/or plan to replace; omissions preserved |
| taskbelay_resolve_issue | identity, issue_digest, real user_ref, decision; resolves only that saved question |
| taskbelay_record_evidence | identity, check_id, source claim/human, status, summary, content_digest; human needs real user_ref |
| taskbelay_get_run | run_id; real saved status/output without re-execution |
| taskbelay_recover_run | identity, run_id, resolution_basis and user_ref when required; original result or unresolved outcome |
| taskbelay_finish_task | identity, summary, per-goal acceptance; limitations derived from saved concerns |
| taskbelay_cancel_task | identity, reason; cancels and releases claims only when no unresolved execution remains |
| taskbelay_list_tasks | search/host/status/page/page_size/include_archived; bounded saved summaries |
| taskbelay_accept_known_failure | identity, check_id, failed/comparison_evidence_id, failures, actual user_ref, reason |
| taskbelay_update_budget | identity, authorization_digest, complete budget {runs,execution_ms,work_ms,tokens}, real user_ref/response |
| taskbelay_archive_task | identity, archived; terminal visibility only |

identity inputs are task_id/current revision/stable operation_id. Responses return identity.task_id and
identity.revision; native run/recover place this state under result.view. Copy the returned revision; a run
can advance it more than once. Returned digests are authorization/content/plan under result.digests.

Successful mutations return compact state, progress, checks, concerns, limitations, issue and budget/usage.
open/get include context with authorization, plan and repository summaries. get_task detail=true adds
result.details (assessment, facts, evidence_applicability, repositories, evidence, attempts, changes and delivery records), not details.task.
get_run contains output.stdout/output.stderr, truncation, process/conditions and reconciliation. Claims never
become passing checks. recorded_evidence identifies a newly saved observation; changed_limits reports a budget change.

## Requests and actual authority

authorization contains user_ref/request/goals and only explicit restrictions/limits. plan contains approach,
verification_basis/checks and optional forecasts. Each check names goal_ids and purpose acceptance/regression/diagnostic;
every goal needs a non-waivable acceptance check. conditions describes assumptions, not executable assertions.
Default cwd and inputs are .; timeout is 60000 ms and retry is requires_user. Declare actual narrower dependencies
with input_basis. full_suite requires a current full_suite_basis at execution.

revise_task replaces supplied objects completely; omitted objects stay. Ordinary method changes reuse the
user request; changed boundaries and increased/removed positive caps require the real new user decision.
Predicted paths are not an allowlist. Native permissions remain in force.

assessment binds current authorization_digest/content_digest and summary. paths contain path/goal_ids/basis;
constraints/facts contain actual id/verdict/basis/uncertainty. Final review input is only {summary}. Core supplies
all scope/plan/content bindings; caller-supplied server fields are rejected. concerns retain stable original
questions and dependencies. resolution is verified with actual evidence_ids, not_blocking with its concrete basis,
or scope_changed with the saved actual new user_ref. Omission/empty arrays cannot delete a concern. issue=null
does not clear a pending question; use resolve_issue with the actual answer.

host_usage contains source_id/observed_through/totals/coverage. Totals tokens/active_ms/wait_ms are cumulative
per non-overlapping Task source. Unknown metrics are null, not zero. Repeated samples are counted once, older
or decreasing samples are rejected. Report only measurements the Host actually exposes. Zero limits mean unset;
remaining=null is not exhausted capacity. work_ms sets both overlapping Core/Host caps. Recovery never resets cost.

## Failures and recovery

Failure contains error.code/message/details plus concrete gaps/current/existing_result when available.
outcome is not_applied/applied/unknown for semantic state; metering is separate. Native execution failures also
report execution when known. recovery.operation means reuse/replace/read_before_retry/none. Never infer no
side effects merely from a nonzero exit or an error envelope.

INVALID_ARGUMENT identifies exact members. ASSESSMENT_STALE requires reviewing actual changed contents before
copying new digests. REVISION_CONFLICT needs a current read and new operation identity. ACCEPTANCE_INCOMPLETE
contains the same current gaps as get_task. CHECK_ALREADY_SATISFIED supplies the result to reuse.
EXECUTION_PENDING/RUNNING/UNCERTAIN refer to the original run; do not rerun. HOST_USAGE_UNAVAILABLE applies only
when an explicit Host cap depends on incomplete reporting. Preserve incompatible/corrupt data; never reset it.

Complete read request to taskbelay_server_info:

```json
{}
```

Success returns ok=true with actual capabilities. Complete success and failure results for every method are
published in its contracts entry and native --help output. Adjacent same-tool error: an unknown member.

```json
{"unexpected":true}
```

<!-- example:mcp-error taskbelay_server_info unknown_member -->
```json
{"ok":false,"request_id":"example-request","tool":"taskbelay_server_info","error":{"code":"INVALID_ARGUMENT","message":"member is not allowed by the current request","details":[{"path":"arguments.unexpected","rule":"requirement","message":"member is not allowed by the current request"}]},"recovery":{"action":"correct_request","operation":"reuse","message":"Correct only the identified fields from existing facts; preserve operation identity and real authorization.","allowed_paths":["arguments.unexpected"]},"outcome":"not_applied"}
```

Maintainer references: `internal/protocol/handler.go`, `internal/protocol/schema.go`, `internal/protocol/discovery.go` and `internal/protocol/examples_test.go`. Tests execute
the actual producers, compare complete encoded errors and validate actual success/failure against output Schema.
These references are for maintainers, not prerequisites for invoking a method.

## Native execution and delivery channels

Use the Host's native process tool to invoke check run with one RunRequest on stdin. Codex/Claude/ZCode use
taskbelay-<host>; DeepSeek activation provides the exact bundled executable and data directory. Keep stdout
JSON separate from stderr. ok=true can still contain run.status=failed; it never means acceptance passed by itself.
Long commands keep their native session; read saved state and the original run after response loss.

MCP always preserves canonical structuredContent. ZCode's adapter requests a short text summary because its
bridge also renders the structured object. Text-rendering Hosts retain a complete text envelope. The delivery
mode does not change data/authority semantics. Actual native Host session verification remains distinct from
SDK transport and fixture tests.
