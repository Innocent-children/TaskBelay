# TEST submission examples

Read [test instructions](test.md) and use only the example for the transition returned by Core.
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
| `checks`, `failed_items`, `unverified_items`, `manual_handoff_items`, `findings` | `array` |
| `budget_adjustment` | `object` or `null` |
| `known_failure_acceptance`, when present in the example | `object` |

Use the object or `null` shown for the selected transition; they are not interchangeable.
Success response types: `ok` is a boolean, `request_id`/`tool` are strings, `result` is the Task
object, and `result.current_action` is an object or `null`. Error `error`/`recovery` are objects;
`error.details`, when present, is an array of objects, and `recovery.retry_safe` is a boolean.
See the [shared field-type rules](../tool-results.md#json-field-types).

## taskbelay_submit_test-tests_passed

<!-- example:mcp taskbelay_submit_test tests_passed -->
```json
{
  "host": "{{host}}",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tests_passed",
  "summary": "Completed the current TEST work.",
  "reason": "",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "test.classify_failure": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "problem_class": "none",
    "checks": [
      {
        "name": "endpoint-check",
        "source": "automated",
        "status": "passed",
        "summary": "The targeted response check passed.",
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": ""
      }
    ],
    "failed_items": [],
    "unverified_items": [],
    "manual_handoff_items": [],
    "findings": [],
    "budget_adjustment": null
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_test-tests_passed.md).

Possible error for this request: Assume the retained Task and Action are current. the first check is supplied as `source:"user"` while its `command_count` remains 1.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/verification_budget.go` — `EvidenceViolations`;
`internal/workflow/payloads.go` — `ValidatePayload`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.checks.0.source","value":"user"} -->
<!-- example:mcp-output taskbelay_submit_test tests_passed-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_test",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "node_result.checks[0].command_count: command_count must equal 0 when source is user, static or host_observed",
    "details": [
      {
        "path": "node_result.checks[0].command_count",
        "rule": "non_automated_command_count_zero",
        "message": "command_count must equal 0 when source is user, static or host_observed"
      }
    ]
  },
  "recovery": {
    "retry_safe": true,
    "action": "correct_current_action",
    "message": "Correct only the members listed in allowed_paths, using facts already confirmed in the current Action work, and resubmit through the same submission tool once. Do not re-expand requirements, change more code, or guess a user decision; stop when the resubmission fails.",
    "allowed_paths": [
      "node_result.checks[0].command_count"
    ]
  }
}
```

## taskbelay_submit_test-tests_accepted_with_known_failures

<!-- example:mcp taskbelay_submit_test tests_accepted_with_known_failures -->
```json
{
  "host": "{{host}}",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tests_accepted_with_known_failures",
  "summary": "Related checks pass; the user accepts the listed existing suite failure.",
  "reason": "The comparison found no new or changed failures and the developer explicitly accepted this set.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "test.classify_failure": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "problem_class": "none",
    "checks": [
      {
        "name": "python-suite",
        "source": "automated",
        "status": "failed",
        "summary": "The suite retains the existing failures; its exit status is nonzero.",
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": ""
      },
      {
        "name": "regression-comparison",
        "source": "automated",
        "status": "passed",
        "summary": "Compared failure identities and error contents with the starting commit; no new or changed failures.",
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": ""
      }
    ],
    "failed_items": [
      "python-suite"
    ],
    "unverified_items": [],
    "manual_handoff_items": [],
    "findings": [],
    "budget_adjustment": null,
    "known_failure_acceptance": {
      "source": "user",
      "summary": "The developer accepts these existing failures after reviewing the comparison.",
      "failed_checks": [
        "python-suite"
      ],
      "comparison_check": "regression-comparison",
      "task_plan_revision": 1,
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    }
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_test-tests_accepted_with_known_failures.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.known_failure_acceptance` is omitted from the request.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/test_acceptance.go` — `validateKnownFailureAcceptance`;
`internal/recovery/correction.go` — `ActionCorrectionPaths`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"node_result.known_failure_acceptance"} -->
<!-- example:mcp-output taskbelay_submit_test tests_accepted_with_known_failures-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_test",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "node_result.known_failure_acceptance: the closed contract requires this member",
    "details": [
      {
        "path": "node_result.known_failure_acceptance",
        "rule": "required_member_missing",
        "message": "the closed contract requires this member"
      }
    ]
  },
  "recovery": {
    "retry_safe": false,
    "action": "none",
    "message": "Inspect the reported fields and current schema. This response does not authorize automatic resubmission."
  }
}
```

## taskbelay_submit_test-tests_failed_implementation

<!-- example:mcp taskbelay_submit_test tests_failed_implementation -->
```json
{
  "host": "{{host}}",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tests_failed_implementation",
  "summary": "The response mapper omits the requested field for a valid request.",
  "reason": "The response mapper omits the requested field for a valid request.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "test.classify_failure": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "implementation_failure",
    "checks": [
      {
        "name": "endpoint-check",
        "source": "automated",
        "status": "failed",
        "summary": "The response mapper omits the requested field for a valid request.",
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": ""
      }
    ],
    "failed_items": [
      "endpoint-check"
    ],
    "unverified_items": [
      "The corrected response behavior remains unverified."
    ],
    "manual_handoff_items": [],
    "findings": [
      "The response mapper omits the requested field for a valid request."
    ],
    "budget_adjustment": null
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_test-tests_failed_implementation.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_test tests_failed_implementation-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_test",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "implementation_failure_identified",
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

## taskbelay_submit_test-tests_expose_design_issue

<!-- example:mcp taskbelay_submit_test tests_expose_design_issue -->
```json
{
  "host": "{{host}}",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tests_expose_design_issue",
  "summary": "The selected mapping approach cannot represent the required response variant.",
  "reason": "The selected mapping approach cannot represent the required response variant.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "test.classify_failure": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "design_failure",
    "checks": [
      {
        "name": "endpoint-check",
        "source": "automated",
        "status": "failed",
        "summary": "The selected mapping approach cannot represent the required response variant.",
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": ""
      }
    ],
    "failed_items": [
      "endpoint-check"
    ],
    "unverified_items": [
      "The corrected response behavior remains unverified."
    ],
    "manual_handoff_items": [],
    "findings": [
      "The selected mapping approach cannot represent the required response variant."
    ],
    "budget_adjustment": null
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_test-tests_expose_design_issue.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_test tests_expose_design_issue-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_test",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "test_design_failure_identified",
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

## taskbelay_submit_test-tests_expose_requirement_issue

<!-- example:mcp taskbelay_submit_test tests_expose_requirement_issue -->
```json
{
  "host": "{{host}}",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "tests_expose_requirement_issue",
  "summary": "The requested field semantics for an absent source value are unresolved.",
  "reason": "The requested field semantics for an absent source value are unresolved.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "test.classify_failure": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "problem_class": "requirement_gap",
    "checks": [
      {
        "name": "endpoint-check",
        "source": "automated",
        "status": "failed",
        "summary": "The requested field semantics for an absent source value are unresolved.",
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": ""
      }
    ],
    "failed_items": [
      "endpoint-check"
    ],
    "unverified_items": [
      "The corrected response behavior remains unverified."
    ],
    "manual_handoff_items": [],
    "findings": [
      "The requested field semantics for an absent source value are unresolved."
    ],
    "budget_adjustment": null
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_test-tests_expose_requirement_issue.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.findings` is empty while this transition reports a problem requiring remediation.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload, validateProblemClass`;
`internal/workflow/standard_process.go` — `standardTransitions`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.findings","value":[]} -->
<!-- example:mcp-output taskbelay_submit_test tests_expose_requirement_issue-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_test",
  "error": {
    "code": "TRANSITION_NOT_ALLOWED",
    "message": "node_result.findings: findings must not be empty when problem_class is not none",
    "guard": {
      "guard_id": "test_requirement_gap_identified",
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

## taskbelay_submit_test-verification_budget_increased

<!-- example:mcp taskbelay_submit_test verification_budget_increased -->
```json
{
  "host": "{{host}}",
  "task_id": "task-example",
  "action_id": "action-example",
  "transition_id": "verification_budget_increased",
  "summary": "One newly identified response variant needs an additional targeted check before execution.",
  "reason": "One newly identified response variant needs an additional targeted check before execution.",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "method_results": {
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Recorded the pre-run budget need."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Recorded the pre-run budget need."
    },
    "test.classify_failure": {
      "capability": "",
      "summary": "Recorded the pre-run budget need."
    }
  },
  "node_result": {
    "problem_class": "none",
    "checks": [],
    "failed_items": [],
    "unverified_items": [],
    "manual_handoff_items": [],
    "findings": [],
    "budget_adjustment": {
      "basis": "verification_gap",
      "additional_checks": [
        {
          "name": "endpoint-check",
          "rationale": "The response variant discovered during verification requires one additional targeted command."
        }
      ],
      "additional_automatic_commands": 1,
      "allow_full_suite": false,
      "allow_manual_handoff": false
    }
  }
}
```

Complete successful request and response: [view every returned field](../successes/taskbelay_submit_test-verification_budget_increased.md).

Possible error for this request: Assume the retained Task and Action are current. `node_result.budget_adjustment.additional_checks` is empty, including when only a permission is being changed.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/workflow/payloads.go` — `ValidatePayload`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"node_result.budget_adjustment.additional_checks","value":[]} -->
<!-- example:mcp-output taskbelay_submit_test verification_budget_increased-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_submit_test",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "node_result.budget_adjustment.additional_checks: additional_checks must include at least one check name and a specific explanation, including for a permission-only adjustment",
    "details": [
      {
        "path": "node_result.budget_adjustment.additional_checks",
        "rule": "budget_checks_required",
        "message": "additional_checks must include at least one check name and a specific explanation, including for a permission-only adjustment"
      }
    ]
  },
  "recovery": {
    "retry_safe": true,
    "action": "correct_current_action",
    "message": "Correct only the members listed in allowed_paths, using facts already confirmed in the current Action work, and resubmit through the same submission tool once. Do not re-expand requirements, change more code, or guess a user decision; stop when the resubmission fails.",
    "allowed_paths": [
      "node_result.budget_adjustment.additional_checks"
    ]
  }
}
```
