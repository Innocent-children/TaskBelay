<!-- Generated from skills/taskbelay/core/nodes/comprehension-examples.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# COMPREHENSION submission examples

Read [comprehension instructions](comprehension.md) and use only the example for the transition returned by Core.
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
| `explained_components`, `unresolved_questions`, `unnecessary_abstractions`, `maintenance_risks`, `findings` | `array` |
| `user_confirmation` | `object` or `null` |

Use the object or `null` shown for the selected transition; they are not interchangeable.
Success response types: `ok` is a boolean, `request_id`/`tool` are strings, `result` is the Task
object, and `result.current_action` is an object or `null`. Error `error`/`recovery` are objects;
`error.details`, when present, is an array of objects, and `recovery.retry_safe` is a boolean.
See the [shared field-type rules](../tool-results.md#json-field-types).

## taskbelay_submit_comprehension-comprehension_passed

<!-- example:mcp taskbelay_submit_comprehension comprehension_passed -->
```json
{
  "host": "codex",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "comprehension_passed",
  "summary": "Completed the current COMPREHENSION_REVIEW work.",
  "reason": "",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "comprehension.explain": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "comprehension.identify_complexity": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "comprehension.obtain_user_verdict": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "problem_class": "none",
    "explained_components": [
      "Endpoint response mapper and its targeted check."
    ],
    "unresolved_questions": [],
    "unnecessary_abstractions": [],
    "maintenance_risks": [],
    "user_confirmation": {
      "source": "user",
      "status": "passed",
      "summary": "The developer explicitly confirmed they can explain and maintain the result."
    },
    "findings": []
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_comprehension-comprehension_passed.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.user_confirmation.summary` is omitted from the request.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/action_schema.go` — `ValidateSubmissionNodeResult`;
`internal/workflow/payloads.go` — `requiredMemberViolations`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"node_result.user_confirmation.summary"} -->
<!-- example:mcp-output taskbelay_submit_comprehension comprehension_passed-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_comprehension",
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

## taskbelay_submit_comprehension-implementation_defect

<!-- example:mcp taskbelay_submit_comprehension implementation_defect -->
```json
{
  "host": "codex",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "implementation_defect",
  "summary": "The implementation omits a required response variant.",
  "reason": "The implementation omits a required response variant.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "comprehension.explain": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.identify_complexity": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.obtain_user_verdict": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "implementation_defect",
    "explained_components": [
      "Endpoint response mapper and its targeted check."
    ],
    "unresolved_questions": [],
    "unnecessary_abstractions": [],
    "maintenance_risks": [],
    "user_confirmation": null,
    "findings": [
      "The implementation omits a required response variant."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_comprehension-implementation_defect.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_comprehension implementation_defect-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_comprehension",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "implementation_defect_identified",
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

## taskbelay_submit_comprehension-code_too_complex

<!-- example:mcp taskbelay_submit_comprehension code_too_complex -->
```json
{
  "host": "codex",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "code_too_complex",
  "summary": "A redundant wrapper obscures the existing response mapper.",
  "reason": "A redundant wrapper obscures the existing response mapper.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "comprehension.explain": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.identify_complexity": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.obtain_user_verdict": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "code_complexity",
    "explained_components": [
      "Endpoint response mapper and its targeted check."
    ],
    "unresolved_questions": [],
    "unnecessary_abstractions": [
      "A redundant wrapper obscures the existing response mapper."
    ],
    "maintenance_risks": [],
    "user_confirmation": null,
    "findings": [
      "A redundant wrapper obscures the existing response mapper."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_comprehension-code_too_complex.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_comprehension code_too_complex-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_comprehension",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "code_complexity_identified",
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

## taskbelay_submit_comprehension-design_too_complex

<!-- example:mcp taskbelay_submit_comprehension design_too_complex -->
```json
{
  "host": "codex",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "design_too_complex",
  "summary": "The design adds an unnecessary mapping layer.",
  "reason": "The design adds an unnecessary mapping layer.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "comprehension.explain": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.identify_complexity": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.obtain_user_verdict": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "design_complexity",
    "explained_components": [
      "Endpoint response mapper and its targeted check."
    ],
    "unresolved_questions": [],
    "unnecessary_abstractions": [
      "The design adds an unnecessary mapping layer."
    ],
    "maintenance_risks": [],
    "user_confirmation": null,
    "findings": [
      "The design adds an unnecessary mapping layer."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_comprehension-design_too_complex.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_comprehension design_too_complex-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_comprehension",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "design_complexity_identified",
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

## taskbelay_submit_comprehension-evidence_insufficient

<!-- example:mcp taskbelay_submit_comprehension evidence_insufficient -->
```json
{
  "host": "codex",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "evidence_insufficient",
  "summary": "The required response variant has no current passed check.",
  "reason": "The required response variant has no current passed check.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "comprehension.explain": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.identify_complexity": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.obtain_user_verdict": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "verification_gap",
    "explained_components": [
      "Endpoint response mapper and its targeted check."
    ],
    "unresolved_questions": [],
    "unnecessary_abstractions": [],
    "maintenance_risks": [],
    "user_confirmation": null,
    "findings": [
      "The required response variant has no current passed check."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_comprehension-evidence_insufficient.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_comprehension evidence_insufficient-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_comprehension",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "verification_gap_identified",
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

## taskbelay_submit_comprehension-requirement_unclear

<!-- example:mcp taskbelay_submit_comprehension requirement_unclear -->
```json
{
  "host": "codex",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "requirement_unclear",
  "summary": "The requested field semantics for an absent source value are unresolved.",
  "reason": "The requested field semantics for an absent source value are unresolved.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "comprehension.explain": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.identify_complexity": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "comprehension.obtain_user_verdict": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "requirement_gap",
    "explained_components": [
      "Endpoint response mapper and its targeted check."
    ],
    "unresolved_questions": [
      "The requested field semantics for an absent source value are unresolved."
    ],
    "unnecessary_abstractions": [],
    "maintenance_risks": [],
    "user_confirmation": null,
    "findings": [
      "The requested field semantics for an absent source value are unresolved."
    ]
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_comprehension-requirement_unclear.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_comprehension requirement_unclear-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_comprehension",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "comprehension_requirement_gap_identified",
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
