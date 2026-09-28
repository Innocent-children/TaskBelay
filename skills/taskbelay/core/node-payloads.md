# Action submissions

Implementation: `internal/mcp/schemas.go` — `actionSubmissionSchema`;
`internal/workflow/submission_schema.go` — `ActionSubmissionSchema`;
`internal/workflow/action_schema.go` — `SubmissionNodeResultSchema`;
`internal/application/submit_action.go` — `SubmitAction`.

Read this procedure once for the current Task, then only the current node guide below. Each guide
keeps its prerequisites and every legal transition; open the linked example for the selected
transition when constructing its input. Other nodes and example catalogs need not be loaded.

## Planning and current authority

Show requirements and acceptance criteria, then design and impact, then the complete work/file/
verification plan. Discuss feedback and reuse explicit answers covering the same content. Save the
draft with `tasks_plan_saved` and wait in TASKS until the user approves its current content and
revision; only then submit `tasks_ready`. Selecting TaskBelay or a workspace does not approve an
unseen plan. Revised plans and expanded file scope require a new saved plan and confirmation;
follow the exact fields in [TASKS](nodes/tasks.md).

Before any write mechanism, compare intended files with the approved scope; directory ranges need
a visible purpose and reason. Keep the entire returned Action: identity/revision, process/digests,
purpose, conditions, allowed effects, required records, method steps, transitions, payload contract,
guidance and issuance time. Follow [response handling](tool-results.md) before acting; a saved
`get_task` is not a fresh workspace guard. Missing original Action data stops execution.

Use [method profiles](method-profiles.md) for current method steps, [Host artifacts](artifacts.md)
for file preparation and write gates, and [verification](verification.md) for checks. Only actual
work and user answers supply results, approvals or comprehension. Continue authorized work when no
required input remains; explanations and progress updates need no acknowledgment.

## Common submission procedure

1. Start from the complete fresh Action after handling recovery/blocker/outcome. Use its
   `submission_tool`, `task_id`, `action_id`, method steps and returned transitions.
2. Perform the current node work under its allowed effects. Record actual facts and user decisions.
   Required method steps must be completed by the selected capability or ordinary equivalent work.
3. [Collect and prepare artifacts](artifacts.md) for this Action. Keep every collected entry,
   classify purpose, and use the prepared result directly. Make no further repository writes before
   submission; recollect after a change.
4. Read the live input schema for that exact tool. Select a returned transition matching actual facts
   and its reason rule. Check all required/allowed members, types, arrays, enums and method-step keys.
5. Submit the complete JSON input once. Use [response handling](tool-results.md#submission-response-handling)
   to retain the full response and inspect `ok` before accessing success fields.

The common top-level members are exactly `host`, `task_id`, `action_id`, `transition_id`, `summary`,
`reason`, `artifacts`, `method_results`, `node_result`. `host` is {{host}}. Artifacts have required
`other_process` and a required `current` only when the tool exposes it. Entries have only path/digest/
summary. Method results have exactly the returned step IDs, each with capability/summary; empty
capability denotes completed ordinary work. Core fills roles, method order/status, revisions,
process/issuance identity and its normalized internal payload. These are absent from Host input.

| Submission field | JSON type |
| --- | --- |
| `host`, `task_id`, `action_id`, `transition_id`, `summary`, `reason` | `string` |
| `artifacts`, `method_results`, `node_result` | `object` |
| `artifacts.other_process`, `artifacts.current` when exposed | `array<object>`; entry `path`, `digest`, `summary` are strings |
| Each `method_results` step value | `object` with string `capability` and `summary` |

Nested `node_result` fields keep the types shown for the selected transition: objects, arrays,
strings, integers, booleans or actual `null`. Pass these values directly; do not encode an object
or array as a string. The [shared field-type rules](tool-results.md#json-field-types) also apply
to every request and response example.

The linked examples use one illustrative endpoint task and show every current ordinary transition.
They are alternatives selected from an actual Action, not an execution script or permission to take
an edge. Empty artifact arrays mean preparation found no process files for that sample. Replace them
with the complete actual prepared output. Findings are empty for problem_class none and concrete for
problem edges. Example identities, checks and verdicts cannot be reused as real results.

## Node references

| Current node | Tool and examples |
| --- | --- |
| `REQUIREMENTS` | [`taskbelay_submit_requirements`](nodes/requirements.md) |
| `DESIGN` | [`taskbelay_submit_design`](nodes/design.md) |
| `TASKS` | [`taskbelay_submit_tasks`](nodes/tasks.md) |
| `IMPLEMENT` | [`taskbelay_submit_implementation`](nodes/implementation.md) |
| `TEST` | [`taskbelay_submit_test`](nodes/test.md) |
| `COMPREHENSION_REVIEW` | [`taskbelay_submit_comprehension`](nodes/comprehension.md) |
| `REFACTOR` | [`taskbelay_submit_refactor`](nodes/refactor.md) |
| `DELIVERY` | [`taskbelay_submit_delivery`](nodes/delivery.md) |

## Success and failure result formats

Every ordinary success uses `{"ok":true,"request_id":"…","tool":"…","result":<complete Task>}`.
The next Action is `result.current_action`, not `result.task.current_action`. A terminal success has
`result.current_action:null`. `ok` is a boolean; `request_id` and `tool` are strings; `result` is
an object, and `result.current_action` is an object or `null`. After TEST, a repetition blocker
can replace the expected destination; read the actual result. The [result reference](tool-results.md) defines complete-error and uncertain-
operation handling. Examples in each node state their expected next node, and Core decides whether
its current guards allow it.

Stop repository work on BLOCKED or a terminal result and follow the corresponding recovery or
Host lifecycle instructions. DONE/CANCELLED grants no Git publication or resource-deletion authority.

The checkable examples enumerate existing contracts for maintenance; the runtime always selects from
the fresh Action. No example adds a transition, changes Scope, grants Git authority or supplies a human
verdict. Source contracts and examples are checked together by `internal/mcp/skill_examples_test.go`.
