# Core response contract

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

CLI, MCP and HTTP use the producer in internal/mcp/server.go. Success contains ok=true, request_id, tool, result; failure contains ok=false, request_id, tool, error, recovery. Envelopes are exclusive. refreshed=false means saved state, not current evidence inspection. run_check/recover_run return view and run; other task operations return the task view.

error.code identifies the category; message describes the actual requirement; details name path/rule/message. Invalid requests, stale observations, scope violations, incomplete acceptance, cost limits, uncertain execution and incompatible storage remain distinct. Quantity limits include budget.used/requested/limit; permissions/scope are not disguised as shortages. Public errors omit sensitive parameters and stack traces. Unknown failures require reading retained state, not invented zero-write guarantees.

recovery.action selects correct_request, read_task, self_correct, recover_run, request_authorization or repair_storage. Correct only indicated fields from known facts while retaining actual authority and operation identity; never manufacture user decisions, semantic passing or execution results. Post-commit cleanup failure can still return an error and requires reading saved state. Cost accounting writes are distinct from semantic submission; an error does not prove no task/process effects occurred.

Read the original Task/run after uncertainty. Durable execution reservation forbids blind replay. A matching signed receipt records the original result; a stopped Core with a live command/process group returns EXECUTION_RUNNING and retains pending. An unobservable process returns EXECUTION_UNCERTAIN. Only after both stop can a missing receipt become interrupted_unknown. Old schemas preserve data and refuse open. Shared protocol.md has a complete same-tool error example; tests compare the full encoded error and validate output Schemas. In-memory SDK tests exercise transport. See [commands](COMMANDS_en.md) and [architecture](ARCHITECTURE_en.md).

JSON unknown members, duplicate members and type errors identify the specific arguments path, including array indexes such as arguments.checks[0].argv[0]. Adjacent examples use complete actual producer encoding.

PLAN_CONFIRMATION_REQUIRED, PLAN_CHANGED and AUTHORIZATION_PENDING require an actual user decision. UNAPPROVED_CHANGES requires restored proposal contents or a disclosed revision. CHECK_ALREADY_SATISFIED reuses existing results; RETRY_BASIS_REQUIRED requires the previous run and new diagnostic facts. FAILURE_ACCEPTANCE_NOT_ALLOWED prevents exceptions from waiving acceptance. These rejections start no process, but call cost is still recorded.
