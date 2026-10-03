<!-- Generated from skills/taskbelay/core/successes/taskbelay_submit_implementation-implementation_needs_refactor.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_submit_implementation: implementation_needs_refactor

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_implementation implementation_needs_refactor -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "other_process": []
  },
  "host": "deepseek",
  "method_results": {
    "implementation.classify_deviations": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "implementation.execute_plan": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "implementation.record_surface": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "completed_work_item_ids": [],
    "deviations": [
      "A redundant wrapper obscures the existing response mapper."
    ],
    "findings": [
      "A redundant wrapper obscures the existing response mapper."
    ],
    "problem_class": "code_complexity"
  },
  "reason": "A redundant wrapper obscures the existing response mapper.",
  "summary": "A redundant wrapper obscures the existing response mapper.",
  "task_id": "task-example",
  "transition_id": "implementation_needs_refactor"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_implementation implementation_needs_refactor -->
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
      "history_revision": 6,
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
      "action_kind": "COMPLETE_REFACTOR",
      "allowed_effects": [
        "read_repository",
        "edit_product_files",
        "edit_process_artifacts"
      ],
      "available_transitions": [
        {
          "description": "Move from REFACTOR to TEST after completing the declared node obligations.",
          "destination_node": "TEST",
          "guard_id": "refactor_report_complete",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the refactor report complete condition is satisfied.",
          "transition_id": "refactor_ready_for_test"
        },
        {
          "description": "Move from REFACTOR to DESIGN after completing the declared node obligations.",
          "destination_node": "DESIGN",
          "guard_id": "refactor_design_change_required",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the refactor design change required condition is satisfied.",
          "transition_id": "refactor_requires_design"
        },
        {
          "description": "Move from REFACTOR to REQUIREMENTS after completing the declared node obligations.",
          "destination_node": "REQUIREMENTS",
          "guard_id": "refactor_requirement_change_required",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the refactor requirement change required condition is satisfied.",
          "transition_id": "refactor_requires_requirements"
        }
      ],
      "completion_conditions": [
        "refactor_simplifications_reported",
        "all_planned_work_items_completed",
        "refactor_behavior_intent_explicit",
        "refactor_repository_observed"
      ],
      "current_node": "REFACTOR",
      "entry_conditions": [
        "simplification_reason_available",
        "current_authorities_available"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Remove unnecessary complexity within the approved behavior boundary.",
          "required": true,
          "step_id": "refactor.simplify"
        },
        {
          "purpose": "Reconcile affected process artifacts with the simplification.",
          "required": true,
          "step_id": "refactor.reconcile_artifacts"
        },
        {
          "purpose": "Record exact simplifications and the changed surface.",
          "required": true,
          "step_id": "refactor.record_surface"
        }
      ],
      "node_purpose": "Simplify the current design and code without silently changing approved behavior.",
      "payload_contract": "refactor-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "refactor_summary",
          "required": true
        }
      ],
      "revision": 6,
      "submission_tool": "taskbelay_submit_refactor",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "REFACTOR",
    "evidence": null,
    "file_scope_records": null,
    "implementation": {
      "action_changed_paths": null,
      "completed_work_item_ids": [],
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "created_at": "2026-09-10T00:00:00Z",
      "deviations": [
        "A redundant wrapper obscures the existing response mapper."
      ],
      "revision": 1,
      "summary": "A redundant wrapper obscures the existing response mapper.",
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
      "from_revision": 5,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 6
    },
    "origin_host": "deepseek",
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
    "revision": 6,
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
        "automatic_commands": 0,
        "evidence_items": 0,
        "full_suite_runs": 0
      }
    },
    "verification_attempts": null,
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
  "tool": "taskbelay_submit_implementation"
}
```
