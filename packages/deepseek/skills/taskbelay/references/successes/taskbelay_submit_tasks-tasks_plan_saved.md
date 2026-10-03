<!-- Generated from skills/taskbelay/core/successes/taskbelay_submit_tasks-tasks_plan_saved.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_submit_tasks: tasks_plan_saved

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_tasks tasks_plan_saved -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "host": "deepseek",
  "method_results": {
    "tasks.analyze_consistency": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.decompose": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.map_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "tasks.plan_verification": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "baseline": {
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
    },
    "findings": [],
    "problem_class": "none",
    "user_confirmation": null
  },
  "reason": "",
  "summary": "Completed the current TASKS work.",
  "task_id": "task-example",
  "transition_id": "tasks_plan_saved"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_tasks tasks_plan_saved -->
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
      "history_revision": 4,
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
        "confirmation": null,
        "confirmed_at": null,
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
      "action_kind": "COMPLETE_TASKS",
      "allowed_effects": [
        "read_repository",
        "edit_process_artifacts",
        "request_user_decision"
      ],
      "available_transitions": [
        {
          "description": "Move from TASKS to TASKS after completing the declared node obligations.",
          "destination_node": "TASKS",
          "guard_id": "task_plan_baseline_complete",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the task plan baseline complete condition is satisfied.",
          "transition_id": "tasks_plan_saved"
        },
        {
          "description": "Move from TASKS to IMPLEMENT after completing the declared node obligations.",
          "destination_node": "IMPLEMENT",
          "guard_id": "current_plan_user_confirmed",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the current plan user confirmed condition is satisfied.",
          "transition_id": "tasks_ready"
        },
        {
          "description": "Move from TASKS to DESIGN after completing the declared node obligations.",
          "destination_node": "DESIGN",
          "guard_id": "design_not_decomposable",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the design not decomposable condition is satisfied.",
          "transition_id": "tasks_require_design"
        },
        {
          "description": "Move from TASKS to REQUIREMENTS after completing the declared node obligations.",
          "destination_node": "REQUIREMENTS",
          "guard_id": "material_requirement_gap",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the material requirement gap condition is satisfied.",
          "transition_id": "tasks_require_requirements"
        }
      ],
      "completion_conditions": [
        "task_items_nonempty",
        "task_dependencies_valid",
        "task_acceptance_covered",
        "task_paths_bounded",
        "task_verification_defined",
        "task_verification_plan_defined",
        "complete_plan_presented",
        "current_plan_user_confirmed_before_implementation"
      ],
      "current_node": "TASKS",
      "entry_conditions": [
        "requirements_current",
        "design_current"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Decompose the current design into bounded, ordered work items.",
          "required": true,
          "step_id": "tasks.decompose"
        },
        {
          "purpose": "Map every current acceptance criterion to work and verification.",
          "required": true,
          "step_id": "tasks.map_acceptance"
        },
        {
          "purpose": "Check requirements, design, and tasks for gaps or contradictions.",
          "required": true,
          "step_id": "tasks.analyze_consistency"
        },
        {
          "purpose": "Set the initial verification plan after analyzing scope, impact, and the existing test structure.",
          "required": true,
          "step_id": "tasks.plan_verification"
        }
      ],
      "node_purpose": "Decompose the current design into bounded work and set verification effort from the analyzed scope, impact, and test structure.",
      "payload_contract": "tasks-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "task_plan_baseline",
          "required": true
        }
      ],
      "revision": 4,
      "submission_tool": "taskbelay_submit_tasks",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "TASKS",
    "evidence": null,
    "file_scope_records": null,
    "implementation": null,
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
      "from_revision": 3,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 4
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
    "revision": 4,
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
  "tool": "taskbelay_submit_tasks"
}
```
