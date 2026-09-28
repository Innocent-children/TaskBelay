<!-- Generated from skills/taskbelay/core/nodes/tasks-examples.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# TASKS submission examples

Read [tasks instructions](tasks.md) and use only the example for the transition returned by Core.
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
| `baseline`, `user_confirmation` | `object` or `null` |
| `findings` | `array` |

Use the object or `null` shown for the selected transition; they are not interchangeable.
Success response types: `ok` is a boolean, `request_id`/`tool` are strings, `result` is the Task
object, and `result.current_action` is an object or `null`. Error `error`/`recovery` are objects;
`error.details`, when present, is an array of objects, and `recovery.retry_safe` is a boolean.
See the [shared field-type rules](../tool-results.md#json-field-types).

## taskbelay_submit_tasks-tasks_plan_saved

<!-- example:mcp taskbelay_submit_tasks tasks_plan_saved -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tasks_plan_saved",
  "summary": "Completed the current TASKS work.",
  "reason": "",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "tasks.decompose": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.map_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.analyze_consistency": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.plan_verification": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "problem_class": "none",
    "user_confirmation": null,
    "baseline": {
      "work_items": [
        {
          "work_item_id": "work-endpoint",
          "summary": "Return the field and cover its response contract.",
          "expected_paths": [
            "src/endpoint.js",
            "tests/endpoint.test.js"
          ],
          "acceptance_indexes": [
            0
          ],
          "verification_steps": [
            "Run endpoint-check."
          ],
          "dependencies": []
        }
      ],
      "verification_plan": {
        "checks": [
          {
            "name": "endpoint-check",
            "rationale": "Check the changed response contract."
          }
        ],
        "initial_budget": {
          "level": "targeted",
          "max_automatic_commands": 2,
          "allow_full_suite": false,
          "allow_manual_handoff": false
        },
        "full_suite_expected": false,
        "test_code_changes_expected": true
      }
    },
    "findings": []
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_tasks-tasks_plan_saved.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.baseline.work_items[0].verification_steps` is omitted from the request.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/action_schema.go` — `ValidateSubmissionNodeResult`;
`internal/workflow/payloads.go` — `requiredMemberViolations`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"node_result.baseline.work_items.0.verification_steps"} -->
<!-- example:mcp-output taskbelay_submit_tasks tasks_plan_saved-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_tasks",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "node_result.baseline.work_items[0].verification_steps: the closed contract requires this member",
    "details": [
      {
        "path": "node_result.baseline.work_items[0].verification_steps",
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
      "node_result.baseline.work_items[0].verification_steps"
    ]
  }
}
```

## taskbelay_submit_tasks-tasks_ready

<!-- example:mcp taskbelay_submit_tasks tasks_ready -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tasks_ready",
  "summary": "The user approved the displayed complete current plan.",
  "reason": "",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "tasks.decompose": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.map_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.analyze_consistency": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.plan_verification": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "problem_class": "none",
    "baseline": null,
    "findings": [],
    "user_confirmation": {
      "source": "user",
      "status": "passed",
      "summary": "The developer approved the requirements, design, work items, files and verification.",
      "requirements_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "design_digest": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "task_plan_digest": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "task_plan_revision": 1
    }
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_tasks-tasks_ready.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.user_confirmation.summary` is omitted from the request.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/action_schema.go` — `ValidateSubmissionNodeResult`;
`internal/workflow/payloads.go` — `requiredMemberViolations`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"node_result.user_confirmation.summary"} -->
<!-- example:mcp-output taskbelay_submit_tasks tasks_ready-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_tasks",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "node_result.user_confirmation.summary: the closed contract requires this member",
    "details": [
      {
        "path": "node_result.user_confirmation.summary",
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
      "node_result.user_confirmation.summary"
    ]
  }
}
```

## taskbelay_submit_tasks-tasks_require_design

<!-- example:mcp taskbelay_submit_tasks tasks_require_design -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tasks_require_design",
  "summary": "The selected response mapper cannot access the required source value.",
  "reason": "The selected response mapper cannot access the required source value.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "tasks.decompose": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "tasks.map_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "tasks.analyze_consistency": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "tasks.plan_verification": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "design_gap",
    "user_confirmation": null,
    "baseline": null,
    "findings": [
      "The selected response mapper cannot access the required source value."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_tasks-tasks_require_design.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_tasks tasks_require_design-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_tasks",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "design_not_decomposable",
      "failures": [
        {
          "path": "node_result.findings",
          "rule": "problem_findings_present",
          "message": "findings must not be empty when problem_class is not none"
        }
      ]
    }
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_next_action",
    "message": "Read the complete current transition set."
  }
}
```

## taskbelay_submit_tasks-tasks_require_requirements

<!-- example:mcp taskbelay_submit_tasks tasks_require_requirements -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tasks_require_requirements",
  "summary": "The requested field semantics for an absent source value are unresolved.",
  "reason": "The requested field semantics for an absent source value are unresolved.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "tasks.decompose": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "tasks.map_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "tasks.analyze_consistency": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "tasks.plan_verification": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "requirement_gap",
    "user_confirmation": null,
    "baseline": null,
    "findings": [
      "The requested field semantics for an absent source value are unresolved."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_tasks-tasks_require_requirements.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_tasks tasks_require_requirements-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_tasks",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "material_requirement_gap",
      "failures": [
        {
          "path": "node_result.findings",
          "rule": "problem_findings_present",
          "message": "findings must not be empty when problem_class is not none"
        }
      ]
    }
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_next_action",
    "message": "Read the complete current transition set."
  }
}
```
