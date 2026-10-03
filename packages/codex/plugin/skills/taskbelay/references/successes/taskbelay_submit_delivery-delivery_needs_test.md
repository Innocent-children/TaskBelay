<!-- Generated from skills/taskbelay/core/successes/taskbelay_submit_delivery-delivery_needs_test.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_submit_delivery: delivery_needs_test

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_delivery delivery_needs_test -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "host": "codex",
  "method_results": {
    "delivery.prepare_summary": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "delivery.reconcile_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "delivery.reconcile_method_artifacts": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "acceptance": [],
    "findings": [
      "The response criterion lacks a current passed check."
    ],
    "problem_class": "test_gap",
    "risks": [],
    "unverified_items": []
  },
  "reason": "The response criterion lacks a current passed check.",
  "summary": "The response criterion lacks a current passed check.",
  "task_id": "task-example",
  "transition_id": "delivery_needs_test"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_delivery delivery_needs_test -->
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
      "history_revision": 9,
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
      "action_kind": "COMPLETE_TEST",
      "allowed_effects": [
        "read_repository",
        "run_verification_commands",
        "edit_process_artifacts",
        "request_user_decision"
      ],
      "available_transitions": [
        {
          "description": "Move from TEST to COMPREHENSION_REVIEW after completing the declared node obligations.",
          "destination_node": "COMPREHENSION_REVIEW",
          "guard_id": "current_tests_pass",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the current tests pass condition is satisfied.",
          "transition_id": "tests_passed"
        },
        {
          "description": "Move from TEST to COMPREHENSION_REVIEW after completing the declared node obligations.",
          "destination_node": "COMPREHENSION_REVIEW",
          "guard_id": "current_known_failures_accepted",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the current known failures accepted condition is satisfied.",
          "transition_id": "tests_accepted_with_known_failures"
        },
        {
          "description": "Move from TEST to IMPLEMENT after completing the declared node obligations.",
          "destination_node": "IMPLEMENT",
          "guard_id": "implementation_failure_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the implementation failure identified condition is satisfied.",
          "transition_id": "tests_failed_implementation"
        },
        {
          "description": "Move from TEST to DESIGN after completing the declared node obligations.",
          "destination_node": "DESIGN",
          "guard_id": "test_design_failure_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the test design failure identified condition is satisfied.",
          "transition_id": "tests_expose_design_issue"
        },
        {
          "description": "Move from TEST to REQUIREMENTS after completing the declared node obligations.",
          "destination_node": "REQUIREMENTS",
          "guard_id": "test_requirement_gap_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the test requirement gap identified condition is satisfied.",
          "transition_id": "tests_expose_requirement_issue"
        },
        {
          "description": "Move from TEST to TEST after completing the declared node obligations.",
          "destination_node": "TEST",
          "guard_id": "verification_budget_adjustment_justified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the verification budget adjustment justified condition is satisfied.",
          "transition_id": "verification_budget_increased"
        }
      ],
      "completion_conditions": [
        "test_checks_classified",
        "test_failures_classified",
        "test_unverified_items_recorded",
        "test_budget_obeyed_or_justifiably_increased"
      ],
      "current_node": "TEST",
      "entry_conditions": [
        "implementation_current",
        "repository_binding_current",
        "verification_plan_current"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Choose the closest necessary checks and record a justified budget increase before any extra command runs.",
          "required": true,
          "step_id": "test.run_budgeted_checks"
        },
        {
          "purpose": "Record actual evidence sources and outcomes, or the exact pre-run budget adjustment.",
          "required": true,
          "step_id": "test.record_evidence"
        },
        {
          "purpose": "Classify new versus known failures, record their comparison and obtain explicit acceptance of the exact known-failure set when appropriate.",
          "required": true,
          "step_id": "test.classify_failure"
        }
      ],
      "node_purpose": "Verify the current repository behavior within the analyzed plan and record any justified budget increase before more commands run.",
      "payload_contract": "test-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "test_summary",
          "required": true
        }
      ],
      "revision": 9,
      "submission_tool": "taskbelay_submit_test",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "TEST",
    "evidence": [
      {
        "command_count": 1,
        "digest": "55059b32e86da4881ce8aa5f47ea4a294495243f648aff2bd6f4c2e50c071d31",
        "evidence_id": "evidence-generated-1",
        "full_suite": false,
        "full_suite_reason": "",
        "name": "endpoint-check",
        "recorded_at": "2026-09-10T00:00:00Z",
        "source": "automated",
        "status": "passed",
        "summary": "The targeted response check passed.",
        "task_plan_revision": 1
      },
      {
        "command_count": 0,
        "digest": "26fe169d4c1e91084c345dfc9be96e7aeb7853d3d4af2a5051db104ae8a6d34b",
        "evidence_id": "evidence-generated-2",
        "full_suite": false,
        "full_suite_reason": "",
        "name": "comprehension_confirmation",
        "recorded_at": "2026-09-10T00:00:00Z",
        "source": "user",
        "status": "passed",
        "summary": "The developer explicitly confirmed they can explain and maintain the result.",
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
      "from_revision": 8,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 9
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
    "revision": 9,
    "task_id": "task-example",
    "test": null,
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
        "automatic_commands": 1,
        "evidence_items": 2,
        "full_suite_runs": 0
      }
    },
    "verification_attempts": [
      {
        "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "destination_node": "COMPREHENSION_REVIEW",
        "evidence_ids": [
          "evidence-generated-1"
        ],
        "failed": false,
        "failure_digest": "",
        "implementation_paths": null,
        "implementation_revision": 1,
        "recorded_at": "2026-09-10T00:00:00Z",
        "result_digest": "7c25a5c757e4abee5cb55b1dcdcac639bd833f0a614c578eee638ea458f18a1d",
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
  "tool": "taskbelay_submit_delivery"
}
```
