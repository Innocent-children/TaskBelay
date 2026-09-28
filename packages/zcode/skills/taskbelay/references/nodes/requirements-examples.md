<!-- Generated from skills/taskbelay/core/nodes/requirements-examples.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# REQUIREMENTS submission examples

Read [requirements instructions](requirements.md) and use only the example for the transition returned by Core.
Replace sample identities, artifacts, checks and user verdicts with current values.

Implementation: `internal/mcp/skill_examples_test.go`; `internal/mcp/skill_error_examples_test.go`;
`internal/mcp/skill_success_examples_test.go`.

Request types for every example: `host`, `task_id`, `action_id`, `transition_id`, `summary` and
`reason` are strings; `artifacts`, `method_results` and `node_result` are objects. Artifact slots
are arrays of objects, and each method result is an object with string `capability`/`summary`.
Keep nested values in their shown JSON types; do not stringify objects or arrays.

| Field inside `node_result` | JSON type in these examples |
| --- | --- |
| `problem_class` | `string` |
| `baseline` | `object` |
| `unresolved_questions` | `array` |

Success response types: `ok` is a boolean, `request_id`/`tool` are strings, `result` is the Task
object, and `result.current_action` is an object or `null`. Error `error`/`recovery` are objects;
`error.details`, when present, is an array of objects, and `recovery.retry_safe` is a boolean.
See the [shared field-type rules](../tool-results.md#json-field-types).

## taskbelay_submit_requirements-requirements_ready

<!-- example:mcp taskbelay_submit_requirements requirements_ready -->
```json
{
  "host": "zcode",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "requirements_ready",
  "summary": "Completed the current REQUIREMENTS work.",
  "reason": "",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "requirements.capture": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "requirements.clarify": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "requirements.validate": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "problem_class": "none",
    "baseline": {
      "goal": "Return the requested field from the endpoint.",
      "scope": [
        "Endpoint response"
      ],
      "out_of_scope": [
        "Other endpoints"
      ],
      "acceptance_criteria": [
        "The response contains the requested field."
      ],
      "constraints": [],
      "assumptions": []
    },
    "unresolved_questions": []
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_requirements-requirements_ready.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.baseline.goal` is omitted from the request.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/action_schema.go` — `ValidateSubmissionNodeResult`;
`internal/workflow/payloads.go` — `requiredMemberViolations`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"node_result.baseline.goal"} -->
<!-- example:mcp-output taskbelay_submit_requirements requirements_ready-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_requirements",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "node_result.baseline.goal: the closed contract requires this member",
    "details": [
      {
        "path": "node_result.baseline.goal",
        "rule": "required_member_missing",
        "message": "the closed contract requires this member"
      }
    ]
  },
  "recovery": {
    "retry_safe": true,
    "action": "correct_current_action",
    "message": "Correct only the members listed in allowed_paths, using facts already confirmed in the current Action work, and resubmit through the same submission tool once. Do not re-expand requirements, change more code, or guess a user decision; stop when the resubmission fails.",
    "allowed_paths": [
      "node_result.baseline.goal"
    ]
  }
}
```
