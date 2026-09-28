<!-- Generated from skills/taskbelay/core/tool-results.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Core response handling

Read this when interpreting a Core result. Read only the matching operation guide:

| Operation | Guide |
| --- | --- |
| Handshake, creation, resume, saved Task or guarded Action read | [Connection](connection.md) |
| Lost Action result, scope/history blocker or repeated verification | [Recovery](recovery.md) |
| Relocation, cancellation or unavailable-workspace abandonment | [Core lifecycle](core-lifecycle.md) and [Host lifecycle](host-lifecycle.md) |

Complete example requests retain adjacent errors and linked full success results. They are checked
against Core with fixed repository observations; example identities and user verdicts are never
permission or execution results. Source paths are maintenance pointers; normal work uses the installed
tools and live schemas. Report a real reference/schema disagreement before mutation.

## MCP transport and value sources

Use the actual tool name and result retention procedure in [Host transport](transport.md).
Names in this reference and example markers are Core raw names. The complete example arguments use
`host="deepseek"`, except the `{}` server handshake. The Host adapter determines the callable name,
wrapper and current-turn permission.
Inputs are closed JSON objects. Success envelopes have ok/request_id/tool/result; error envelopes
have ok/request_id/tool/error/recovery and no successful result. The output tool field remains the
Core raw name even when the Host calls a qualified name.

### JSON field types

Pass the tool arguments as an object. Keep every nested field in its declared JSON type:
`object` means `{...}`, `array` means `[...]`, `boolean` means `true` or `false`, and `null`
is unquoted. `array<string>` and `array<object>` below describe the element type; they are not
literal input values. Omitted, `null`, `{}` and `[]` are different values; use only the form
allowed by the current tool and operation.

| Request field | JSON type |
| --- | --- |
| `host`, `task_id`, `action_id`, `repository_path`, `request_id`, `summary`, `reason` | `string`, when declared by the tool |
| `new_task` | `object` for creation; omitted or `null` for resume |
| `workspace_origin`, `artifacts`, `method_results`, `node_result`, `history_resolution` | `object`, when declared by the tool |
| `additional_repositories`, `relocation_destinations` | `array<object>` |
| `operation_probe` | `object` or `null`; may be omitted |
| `revision` | `integer` |

Do not turn an object or array field into JSON text. In particular, never use
`JSON.stringify(new_task)` as the value of `new_task`. MCP handles encoding the complete message;
a CLI helper's stdin receives the complete request encoded as JSON text once, with nested types
preserved.

For responses, `structuredContent` is already an object. If the envelope is supplied in a
`content` text block, its `text` is a string: parse that complete JSON envelope once. Do not parse
ordinary string fields or stringify nested response objects.

| Response field | JSON type |
| --- | --- |
| Complete envelope, success `result`, failure `error` and `recovery` | `object` |
| `ok`, `recovery.retry_safe` | `boolean` |
| `request_id`, `tool`, `error.code`, `error.message`, `recovery.action`, `recovery.message` | `string` |
| `error.details`, `error.guard.failures`, when present | `array<object>`; each entry's `path`, `rule`, `message` is a `string` |
| `error.guard`, `error.budget`, when present | `object`; budget `used`, `requested`, `limit` are `integer` |
| `recovery.allowed_paths`, when present | `array<string>` |

Success `result` members depend on the tool. A Task or Action is an object, not JSON text;
nullable members such as `current_action` and `recovery_assessment` use actual `null`.

`request_id` in an envelope is Core's response identity; the cancellation input supplies its own
request_id. `result.recovery_assessment` concerns retained Action operations and is distinct from
error-envelope `recovery`. Output examples labeled projection show only the fields being discussed;
retain the complete real Task/Action before choosing another operation.

## Core response contract

Core success and failure envelopes are exclusive. Success requires `ok:true`, `request_id`, `tool`
and `result`; failure requires `ok:false`, `request_id`, `tool`, `error`, `recovery` and excludes `result`.
The output schema validates both branches. Core owns error classification and permitted recovery;
Host instructions own retaining the response and executing that recovery.

`error.message` directly states the failed condition or summarizes the field failures. Read the full
`details`/`guard.failures` when the message is a bounded summary. JSON syntax, member types, duplicate
members, state restrictions and storage/encoding failures retain their specific reasons. Unknown
lower-level diagnostics are identified as unknown; do not infer a cause from the error code alone.
Response-encoding failures retain the actual tool and valid request identity; request-ID generation
failure uses `request-unavailable` and states that the tool was not executed.

`error.details[]` gives `path`, `rule`, `message` for identified field failures. Guard failures instead
use `error.guard.guard_id` and `failures[]`. Read these fields before deciding what to change.
`VERIFICATION_BUDGET_EXCEEDED` describes actual quantities: `error.budget.used`, `requested`, `limit`.
`VERIFICATION_NOT_ALLOWED` describes a permission restriction and identifies the field. Completed user
checks and explicit known-failure acceptance are independent of pending manual-handoff permission.
Never increase command counts to solve a permission restriction.

`recovery.action` determines the next operation. `retry_safe:false` does not prove a write happened;
`read_task`, `read_next_action` and `retry_read` authorize the specified read without another user
confirmation. `none` stops automatic resubmission. Only `correct_current_action` and `correct_request` carry
`retry_safe:true` and nonempty `allowed_paths`, grounded in a proven zero-write failure. Correct only
those fields using established facts and submit once. A user's missing or outdated acceptance is a
real decision, not a field value the Host may manufacture or refresh.

For budget counters and a correctable check explanation, see these complete failures:

[Complete error example](response-examples.md#taskbelay_submit_test-budget-exceeded).

[Complete error example](response-examples.md#taskbelay_submit_test-budget-checks-correction).

## Submission response handling

Implementation: `internal/mcp/results.go` — `EncodeSuccess`, `EncodeError`;
`internal/mcp/output_schemas.go` — `outputDescription`.

All eight submit tools, resolve_blocker and recover_action return the Task directly in result.
Retain the complete original response using [Host transport](transport.md), then check ok before
reading success fields. An error has no successful Task. A committed terminal Task has current_action
null. Read the complete current_action when constructing the next call, including all its transitions,
method steps and digests. Validate against the live output contract before choosing another operation.

A local caching/formatting exception or a shortened display does not make an already retained complete
result uncertain. Retrieve that original object or read it in bounded parts; use operation recovery
only when the original result itself cannot be established. Complete domain errors keep their returned
error/recovery instruction even when the Host marks the call as an error.

## Complete rejections and bounded corrections

Implementation: `internal/mcp/results.go` — `EncodeError, boundedCorrectionPaths`.

Example rejection (an error envelope, not a successful Task):

[Complete error example](response-examples.md#taskbelay_submit_test-guard-rejection).

Follow the returned `recovery.action`, not the sample's prose. `read_next_action` permits one guarded
read, not replay of the rejected input. The previous IMPLEMENT operation is not the outcome of a
rejected TEST submission. `retry_safe:false`/`action:none` stops. Internal errors and uncertain writes
never authorize payload guessing.

A correctable zero-write rejection has `retry_safe:true` and `allowed_paths`.
Ordinary node submissions use `action:correct_current_action`; handshake, reads, creation and
lifecycle request-field corrections use `action:correct_request`. The latter requires no new Action
lookup: keep the same tool and existing request identity fields unless listed, retain existing
authorization, and correct only the listed fields. The response request_id is not an argument for
tools that do not accept it.
See the [handshake examples](connection.md#server-handshake) and [TEST](nodes/test.md#tests_passed).

Confirm the same current Action/tool, reread its schema, and change only listed fields from facts
already established. For a failed implementation check, `findings` describes the actual defect; failed
check names alone do not fill it. A missing user verdict requires the user's answer. If the error lists
`repository_paths`/`artifact_manifest_incomplete`, collect/classify/prepare again and change only the
allowed artifact members. Preserve node meaning, transition and method conclusions.

Host policy permits one corrected submission, followed by a stop if it fails again. Core supplies the
zero-write decision and allowed fields; it does not count this Host retry limit. Report exact field,
rule and failure without displaying private submitted values. See the complete
[Test failure example](nodes/test.md#tests_failed_implementation) for the corrected call shape.
