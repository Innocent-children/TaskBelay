<!-- Generated from skills/taskbelay/core/successes/taskbelay_submit_test-tests_accepted_with_known_failures.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_submit_test: tests_accepted_with_known_failures

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_test tests_accepted_with_known_failures -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "host": "codex",
  "method_results": {
    "test.classify_failure": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "test.record_evidence": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "test.run_budgeted_checks": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "budget_adjustment": null,
    "checks": [
      {
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": "",
        "name": "python-suite",
        "source": "automated",
        "status": "failed",
        "summary": "The suite retains the existing failures; its exit status is nonzero."
      },
      {
        "command_count": 1,
        "full_suite": false,
        "full_suite_reason": "",
        "name": "regression-comparison",
        "source": "automated",
        "status": "passed",
        "summary": "Compared failure identities and error contents with the starting commit; no new or changed failures."
      }
    ],
    "failed_items": [
      "python-suite"
    ],
    "findings": [],
    "known_failure_acceptance": {
      "comparison_check": "regression-comparison",
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "failed_checks": [
        "python-suite"
      ],
      "source": "user",
      "summary": "The developer accepts these existing failures after reviewing the comparison.",
      "task_plan_revision": 1
    },
    "manual_handoff_items": [],
    "problem_class": "none",
    "unverified_items": []
  },
  "reason": "The comparison found no new or changed failures and the developer explicitly accepted this set.",
  "summary": "Related checks pass; the user accepts the listed existing suite failure.",
  "task_id": "task-example",
  "transition_id": "tests_accepted_with_known_failures"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_test tests_accepted_with_known_failures -->
```json
{
  "ok": true,
  "request_id": "request-success-example",
  "result": {
    "baselines": {
      "design": {
        "approach": "Add the field in the existing response mapper.",
        "artifact_refs": [],
        "complexity_justification": [
          "The existing mapper owns this response."
        ],
        "components": [
          "Endpoint response mapper"
        ],
        "created_at": "2026-09-10T00:00:00Z",
        "decisions": [
          "Reuse the existing response type."
        ],
        "digest": "f58cb160bc5f944c054a15febb34ddb2c12f06e583a81ea6358680f8f2063b82",
        "rejected_alternatives": [
          "A new response mapping layer is unnecessary."
        ],
        "requirements_revision": 1,
        "revision": 1,
        "risks": []
      },
      "history": null,
      "history_next_after": null,
      "history_revision": 7,
      "history_total": 0,
      "requirements": {
        "acceptance_criteria": [
          "The response contains the requested field."
        ],
        "artifact_refs": [],
        "assumptions": [],
        "constraints": [],
        "created_at": "2026-09-10T00:00:00Z",
        "digest": "e3bf09c133bbed3b78f20e94ec4c2cab91a9759ece00d88dcada67d5c35f14f8",
        "goal": "Return the requested field from the endpoint.",
        "out_of_scope": [
          "Other endpoints"
        ],
        "revision": 1,
        "scope": [
          "Endpoint response"
        ]
      },
      "task_plan": {
        "artifact_refs": [],
        "confirmation": {
          "design_digest": "f58cb160bc5f944c054a15febb34ddb2c12f06e583a81ea6358680f8f2063b82",
          "requirements_digest": "e3bf09c133bbed3b78f20e94ec4c2cab91a9759ece00d88dcada67d5c35f14f8",
          "source": "user",
          "status": "passed",
          "summary": "The developer approved the requirements, design, work items, files and verification.",
          "task_plan_digest": "800e4715f9902c4abf89c749cb3165aa4be1005a5ac5c52d0675aeac29f590d8",
          "task_plan_revision": 1
        },
        "confirmed_at": "2026-09-10T00:00:00Z",
        "created_at": "2026-09-10T00:00:00Z",
        "design_revision": 1,
        "digest": "800e4715f9902c4abf89c749cb3165aa4be1005a5ac5c52d0675aeac29f590d8",
        "revision": 1,
        "verification_plan": {
          "checks": [
            {
              "name": "endpoint-check",
              "rationale": "Check the changed response contract."
            }
          ],
          "full_suite_expected": false,
          "initial_budget": {
            "allow_full_suite": false,
            "allow_manual_handoff": false,
            "level": "targeted",
            "max_automatic_commands": 2
          },
          "test_code_changes_expected": true
        },
        "work_items": [
          {
            "acceptance_indexes": [
              0
            ],
            "dependencies": [],
            "expected_paths": [
              "src/endpoint.js",
              "tests/endpoint.test.js"
            ],
            "summary": "Return the field and cover its response contract.",
            "verification_steps": [
              "Run endpoint-check."
            ],
            "work_item_id": "work-endpoint"
          }
        ]
      }
    },
    "blocker": null,
    "completed_at": null,
    "comprehension": null,
    "created_at": "2026-09-10T00:00:00Z",
    "current_action": {
      "action_id": "action-generated-1",
      "action_kind": "COMPLETE_COMPREHENSION_REVIEW",
      "allowed_effects": [
        "read_repository",
        "edit_process_artifacts",
        "request_user_decision"
      ],
      "available_transitions": [
        {
          "description": "Move from COMPREHENSION_REVIEW to DELIVERY after completing the declared node obligations.",
          "destination_node": "DELIVERY",
          "guard_id": "current_user_comprehension_confirmed",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the current user comprehension confirmed condition is satisfied.",
          "transition_id": "comprehension_passed"
        },
        {
          "description": "Move from COMPREHENSION_REVIEW to IMPLEMENT after completing the declared node obligations.",
          "destination_node": "IMPLEMENT",
          "guard_id": "implementation_defect_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the implementation defect identified condition is satisfied.",
          "transition_id": "implementation_defect"
        },
        {
          "description": "Move from COMPREHENSION_REVIEW to REFACTOR after completing the declared node obligations.",
          "destination_node": "REFACTOR",
          "guard_id": "code_complexity_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the code complexity identified condition is satisfied.",
          "transition_id": "code_too_complex"
        },
        {
          "description": "Move from COMPREHENSION_REVIEW to DESIGN after completing the declared node obligations.",
          "destination_node": "DESIGN",
          "guard_id": "design_complexity_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the design complexity identified condition is satisfied.",
          "transition_id": "design_too_complex"
        },
        {
          "description": "Move from COMPREHENSION_REVIEW to TEST after completing the declared node obligations.",
          "destination_node": "TEST",
          "guard_id": "verification_gap_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the verification gap identified condition is satisfied.",
          "transition_id": "evidence_insufficient"
        },
        {
          "description": "Move from COMPREHENSION_REVIEW to REQUIREMENTS after completing the declared node obligations.",
          "destination_node": "REQUIREMENTS",
          "guard_id": "comprehension_requirement_gap_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the comprehension requirement gap identified condition is satisfied.",
          "transition_id": "requirement_unclear"
        }
      ],
      "completion_conditions": [
        "comprehension_explanation_complete",
        "comprehension_complexity_classified",
        "comprehension_questions_resolved_or_routed",
        "comprehension_user_verdict_recorded"
      ],
      "current_node": "COMPREHENSION_REVIEW",
      "entry_conditions": [
        "test_current_and_completed",
        "repository_binding_current",
        "requirements_design_plan_current"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Explain the current behavior, design, and code path in developer-readable terms.",
          "required": true,
          "step_id": "comprehension.explain"
        },
        {
          "purpose": "Identify unnecessary abstractions and maintenance risks.",
          "required": true,
          "step_id": "comprehension.identify_complexity"
        },
        {
          "purpose": "Obtain the developer's explicit understanding or remediation verdict.",
          "required": true,
          "step_id": "comprehension.obtain_user_verdict"
        }
      ],
      "node_purpose": "Verify that the developer can explain and maintain the current design and implementation.",
      "payload_contract": "comprehension-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "comprehension_assessment",
          "required": true
        }
      ],
      "revision": 7,
      "submission_tool": "taskbelay_submit_comprehension",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "COMPREHENSION_REVIEW",
    "evidence": [
      {
        "command_count": 1,
        "digest": "5843924d451b5d7615b4684ec91d92114ba7ef65911679aec4b19e3c0bceb3c0",
        "evidence_id": "evidence-generated-1",
        "full_suite": false,
        "full_suite_reason": "",
        "name": "python-suite",
        "recorded_at": "2026-09-10T00:00:00Z",
        "source": "automated",
        "status": "failed",
        "summary": "The suite retains the existing failures; its exit status is nonzero.",
        "task_plan_revision": 1
      },
      {
        "command_count": 1,
        "digest": "5db4c8bc07003a25292c408b9d08d2f6766f66244a70911c10fae0cf9c8e9782",
        "evidence_id": "evidence-generated-2",
        "full_suite": false,
        "full_suite_reason": "",
        "name": "regression-comparison",
        "recorded_at": "2026-09-10T00:00:00Z",
        "source": "automated",
        "status": "passed",
        "summary": "Compared failure identities and error contents with the starting commit; no new or changed failures.",
        "task_plan_revision": 1
      }
    ],
    "file_scope_records": null,
    "implementation": {
      "action_changed_paths": null,
      "completed_work_item_ids": [
        "work-endpoint"
      ],
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "created_at": "2026-09-10T00:00:00Z",
      "deviations": [],
      "revision": 1,
      "summary": "Completed the current IMPLEMENT work.",
      "task_plan_revision": 1
    },
    "intent": {
      "initial_out_of_scope": [
        "Other endpoints"
      ],
      "initial_scope": [
        "Endpoint response"
      ],
      "known_acceptance_criteria": [
        "The response contains the requested field."
      ],
      "method_profile": "plain",
      "request": "Return the requested field from the endpoint."
    },
    "last_operation": {
      "action_id": "action-example",
      "committed_at": "2026-09-10T00:00:00Z",
      "from_revision": 6,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 7
    },
    "origin_host": "codex",
    "outcome": null,
    "primary_repository_key": "primary",
    "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
    "process_id": "standard-development",
    "relocation": null,
    "repository": {
      "base_commit_ancestor": true,
      "binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "changed_entries": null,
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "current_branch": "task/endpoint-field",
      "current_head": "1111111111111111111111111111111111111111",
      "detached": false,
      "head_tree": "1111111111111111111111111111111111111111",
      "history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "history_relation": "exact",
      "identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "observed_at": "2026-09-10T00:00:00Z",
      "task_surface": null,
      "worktree_instance_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    },
    "resume_cursor": null,
    "revision": 7,
    "task_id": "task-example",
    "test": {
      "completed_at": "2026-09-10T00:00:00Z",
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "design_revision": 1,
      "evidence_ids": [
        "evidence-generated-1",
        "evidence-generated-2"
      ],
      "known_failure_acceptance": {
        "comparison_check": "regression-comparison",
        "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "failed_checks": [
          "python-suite"
        ],
        "source": "user",
        "summary": "The developer accepts these existing failures after reviewing the comparison.",
        "task_plan_revision": 1
      },
      "manual_handoff_items": [],
      "record_id": "test-generated-1",
      "requirements_revision": 1,
      "task_plan_revision": 1,
      "unverified_items": []
    },
    "updated_at": "2026-09-10T00:00:00Z",
    "verification": {
      "adjustments": null,
      "current_budget": {
        "allow_full_suite": false,
        "allow_manual_handoff": false,
        "level": "targeted",
        "max_automatic_commands": 2
      },
      "plan": {
        "checks": [
          {
            "name": "endpoint-check",
            "rationale": "Check the changed response contract."
          }
        ],
        "full_suite_expected": false,
        "initial_budget": {
          "allow_full_suite": false,
          "allow_manual_handoff": false,
          "level": "targeted",
          "max_automatic_commands": 2
        },
        "test_code_changes_expected": true
      },
      "usage": {
        "automatic_commands": 2,
        "evidence_items": 2,
        "full_suite_runs": 0
      }
    },
    "verification_attempts": [
      {
        "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "destination_node": "COMPREHENSION_REVIEW",
        "evidence_ids": [
          "evidence-generated-1",
          "evidence-generated-2"
        ],
        "failed": false,
        "failure_digest": "",
        "implementation_paths": null,
        "implementation_revision": 1,
        "recorded_at": "2026-09-10T00:00:00Z",
        "result_digest": "33ef4fc072477ba289326a6dca5f3a2f405c8326925fc02be627c3d56a8d96d2",
        "task_plan_revision": 1,
        "task_revision": 7
      }
    ],
    "workspace_origin": {
      "base_branch": "main",
      "base_commit": "1111111111111111111111111111111111111111",
      "canonical_worktree_root": "/work/tasks/endpoint-field",
      "carry_changes": false,
      "mode": "dedicated_worktree",
      "provisioning_receipt_id": "receipt-example",
      "remote_name": "",
      "source_repository_group_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "source_type": "local",
      "task_branch": "task/endpoint-field",
      "worktree_git_dir_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    }
  },
  "tool": "taskbelay_submit_test"
}
```
