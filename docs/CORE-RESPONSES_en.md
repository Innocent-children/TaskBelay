# Core response contract

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

CLI, MCP and HTTP use the producer in internal/protocol/handler.go. Success contains ok=true, request_id, tool, result; failure contains ok=false, request_id, tool, error, recovery. Envelopes are exclusive. refreshed=false means saved state, not current evidence inspection. check.run/recover_run return view and run; other task operations return the task view.

error.code identifies the category; message describes the actual requirement; details name path/rule/message. Invalid requests, stale observations, scope violations, incomplete acceptance, cost limits, uncertain execution and incompatible storage remain distinct. Quantity limits include budget.used/requested/limit/actual/reserved/uncertain/remaining; permissions/scope are not disguised as shortages. Public errors omit sensitive parameters and stack traces. Unknown failures require reading retained state, not invented zero-write guarantees.

recovery.action selects correct_request, read_task, self_correct, recover_run, request_authorization or repair_storage. Correct only indicated fields from known facts while retaining actual authority and operation identity; never manufacture user decisions, semantic passing or execution results. Post-commit cleanup failure can still return an error and requires reading saved state. Cost accounting writes are distinct from semantic submission; an error does not prove no task/process effects occurred.

Read the original Task/run after uncertainty. Durable execution reservation forbids blind replay. A matching signed receipt records the original result; a stopped Core with a live command/process group returns EXECUTION_RUNNING and retains pending. An unobservable process returns EXECUTION_UNCERTAIN. Only after both stop can a missing receipt become interrupted_unknown. Old schemas preserve data and refuse open. Shared protocol.md has a complete same-tool error example; tests compare the full encoded error and validate output Schemas. In-memory SDK tests exercise transport. See [commands](COMMANDS_en.md) and [architecture](ARCHITECTURE_en.md).

JSON unknown members, duplicate members and type errors identify the specific arguments path, including array indexes such as arguments.plan.checks[0].argv[0]. Adjacent examples use complete actual producer encoding.

AUTHORIZATION_REQUIRED means changed user outcomes/restrictions or expanded existing caps need an actual new user reference. AUTHORIZATION_PENDING retains a saved unresolved decision. AUTHORIZATION_CHANGED requires reading the current task before a budget decision. CHECK_ALREADY_SATISFIED means reuse the result; RETRY_BASIS_REQUIRED needs the previous run and new diagnostic information. FAILURE_ACCEPTANCE_NOT_ALLOWED refuses waived acceptance.

REMOTE_OBSERVATION_STALE means HEAD, configured upstream or tracking commit no longer matches the Host observation. server_info.startup_diagnostics reports that this Core was reached; CLI success cannot establish Host MCP availability. Meter initialization failure does not execute the request. error.details retains the original cause under control_meter.<phase>. PROCESS_IDENTITY_DENIED/PROCESS_IDENTITY_UNAVAILABLE directs inspection of process queries and Host access; query denial is not proof of death. STORAGE_UNAVAILABLE means actual storage read/write failure and retained STORAGE_CORRUPT or other domain causes keep their code; preserve data during repair. CONTROL_INITIALIZATION_FAILED identifies other initialization steps, while CONTROL_INITIALIZATION_CANCELLED identifies cancellation or timeout. Recovery follows the actual responsibility and never infers a missing MCP connection.

If the WebUI child exits before readiness, startup reports the process exit and its webui-runtime.log location; a readiness timeout also retains that location. Inspect the original startup before retrying, rather than classifying exit/storage errors as connection failures.

Routine task responses omit full file fingerprints and attempt history. taskbelay_get_task detail:true places the complete view in result.details. check.run/recover_run return run status summaries; get_run reads actual output. Unset caps have budget.limits[].limited=false; remaining=0 does not mean exhausted budget.

ISSUE_CHANGED means the displayed question changed; read it again. resolve_issue never changes authorization. Saved-state reads bypass meter initialization; metering errors block mutations only. check.run is a native CLI response, not an MCP tool.
