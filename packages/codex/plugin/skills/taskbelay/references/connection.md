<!-- Generated from skills/taskbelay/core/connection.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Connect, open and read a Task

Read [response handling](tool-results.md) and the [Host transport](transport.md) before calling Core.

## Server handshake

Implementation: `internal/mcp/server.go` — `dispatch`;
`internal/mcp/results.go` — `ServerInfoResult`;
`internal/mcp/schemas.go` — `ToolNames`.

After admission/provisioning or on explicit resume, the first Core call is `taskbelay_server_info`.
Its complete argument is `{}`; unlike other Core tools, it accepts no `host` member.

[Complete request, successful response and error example](connection-examples.md#taskbelay_server_info-handshake).

Read `result.product`, `version`, `transport`, `health`, `supported_hosts`, `supported_processes`,
`method_profiles`, `tools`, and `host_preferences.codex.codebase_memory`. Require product `taskbelay`,
canonical version, stdio/ready, codex support, one supported `standard-development` process with a
canonical definition digest and `new_task_supported:true`, profiles plain/spec-kit/openspec, and a
boolean codebase-memory preference. Core/package versions are independent.

The complete tool-name set is:

- `taskbelay_server_info`
- `taskbelay_open_task`
- `taskbelay_get_task`
- `taskbelay_get_next_action`
- `taskbelay_submit_requirements`
- `taskbelay_submit_design`
- `taskbelay_submit_tasks`
- `taskbelay_submit_implementation`
- `taskbelay_submit_test`
- `taskbelay_submit_comprehension`
- `taskbelay_submit_refactor`
- `taskbelay_submit_delivery`
- `taskbelay_prepare_task_relocation`
- `taskbelay_resolve_blocker`
- `taskbelay_recover_action`
- `taskbelay_cancel_task`
- `taskbelay_abandon_task`

On success proceed without a handshake approval question. On absent/truncated original data, unhealthy
or incompatible identity/catalog, stop before opening a Task and report the exact failed condition.
Installation repair follows [Host diagnosis](host-lifecycle.md#installation-and-diagnosis-commands)
after appropriate user authority; do not start another
MCP server or inspect binaries to bypass the result. Setup owns installed-resource/version checks.

## Open or resume a Task

Implementation: `internal/application/open_task.go` — `OpenTask`.

For new creation, all receipts/workspaces must be verified first. The default mode is `new_branch` in the current
directory; `current_branch` keeps the existing branch, and `dedicated_worktree` uses an isolated directory.
Local modes use local source/current base branch/starting HEAD, and accept initial modifications only
with `carry_changes:true`. The Host checks directory availability before local branch preparation. Copy the complete verified repository descriptor from [Host admission](admission.md)
into the repository members. `new_task.request` is the admitted request string; scope/exclusions/
known acceptance are arrays derived from the actual request. Profile follows explicit plain, Spec Kit
or OpenSpec intent, otherwise plain. No verification budget is selected at creation.

| Creation field | JSON type |
| --- | --- |
| `host`, `repository_path`, optional `primary_repository_key` | `string` |
| `new_task`, `workspace_origin` | `object`; pass their members directly, never JSON-encoded strings |
| `new_task.request`, `new_task.method_profile` | `string` |
| `new_task.initial_scope`, `new_task.initial_out_of_scope`, `new_task.known_acceptance_criteria` | `array<string>` |
| `workspace_origin.carry_changes` | `boolean`; every other origin member is a `string` |
| Optional `additional_repositories` | `array<object>`; each entry has string `key`/`repository_path` and object `workspace_origin` |

[Complete request, successful response and error example](connection-examples.md#taskbelay_open_task-create).

For a complete confirmed multi-repository Scope, the call includes both origins. These fields come
from one verified Host launch descriptor after both selected workspaces are prepared and writable:

[Complete request, successful response and error example](connection-examples.md#taskbelay_open_task-multiple).

For an existing Task, return to its original worktree instance, preserve its immutable Scope/profile,
and omit all creation members. `new_task` may also be actual `null`; `"null"` and `"{}"` are strings
and are not valid resume values:

[Complete request, successful response and error example](connection-examples.md#taskbelay_open_task-resume).

Success projection: `{"ok":true,"result":{"created":false,"task":{"task_id":"task-example",
"revision":4,"current_cursor":"IMPLEMENT","current_action":{"action_id":"action-example"}},
"recovery_assessment":null}}`. Handle recovery before `result.task.current_action`. `created:true`
identifies creation; neither value permits another Task for the same launch.

Response types: `ok` and `result.created` are booleans; `result` and `result.task` are objects;
`result.recovery_assessment` and `result.task.current_action` are objects or `null`.
`task_id` and `current_cursor` are strings; `revision` is an integer.

Creation errors such as `ACTIVE_TASK_CONFLICT`, `WORKTREE_PROVISIONING_REQUIRED`, ownership failure or
unavailable workspace stop. After an uncertain creation, call the resume example on the same worktree;
compare returned intent/origin/scope with the confirmed launch. `TASK_NOT_FOUND`, mismatch or another
uncertain result stops for inspection. A recreated directory is not the original worktree instance.

## Read saved state and the next Action

Implementation: `internal/application/next_action.go` — `GetNextAction`.
Implementation: `internal/mcp/server.go` — `dispatch`.

`taskbelay_get_task` reads saved records. Use the Task ID from a complete earlier result:

[Complete request, successful response and error example](connection-examples.md#taskbelay_get_task-read).

Success projection: `{"ok":true,"result":{"task":{"task_id":"task-example","revision":4},
"recovery_assessment":null}}`. Read the full `result.task`, including baselines, verification,
`test.evidence_ids`, evidence, blocker, outcome and last_operation. Do not fabricate `operation_probe`;
ordinary reads automatically return the retained assessment.

Before new repository work following a saved-state read, obtain the guarded Action:

[Complete request, successful response and error example](connection-examples.md#taskbelay_get_next_action-guarded-read).

Success projection: `{"ok":true,"result":{"action":{"action_id":"action-example"},
"blocker":null,"outcome":null,"recovery_assessment":null}}`. This operation observes Git and may
persist a workspace blocker or invalidate old verification; it is not a pure saved-state read.
Handle recovery, blocker and outcome before action. If the returned assessment says `read_next_action`,
consume this complete guarded Action once; do not keep querying because the completed assessment remains.
