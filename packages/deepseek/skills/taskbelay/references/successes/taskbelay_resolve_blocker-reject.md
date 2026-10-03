<!-- Generated from skills/taskbelay/core/successes/taskbelay_resolve_blocker-reject.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_resolve_blocker: reject

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_resolve_blocker reject -->
```json
{
  "action_id": "action-example",
  "choice": "reject",
  "host": "deepseek",
  "reason": "The user rejected the write and the retained content has been restored.",
  "task_id": "task-example"
}
```

Complete response:

<!-- example:mcp-success taskbelay_resolve_blocker reject -->
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
      "action_kind": "COMPLETE_IMPLEMENTATION",
      "allowed_effects": [
        "read_repository",
        "edit_product_files",
        "edit_process_artifacts"
      ],
      "available_transitions": [
        {
          "description": "Move from IMPLEMENT to TEST after completing the declared node obligations.",
          "destination_node": "TEST",
          "guard_id": "implementation_report_complete",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the implementation report complete condition is satisfied.",
          "transition_id": "implementation_ready_for_test"
        },
        {
          "description": "Move from IMPLEMENT to DESIGN after completing the declared node obligations.",
          "destination_node": "DESIGN",
          "guard_id": "implementation_exposes_design_gap",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the implementation exposes design gap condition is satisfied.",
          "transition_id": "implementation_requires_design"
        },
        {
          "description": "Move from IMPLEMENT to REQUIREMENTS after completing the declared node obligations.",
          "destination_node": "REQUIREMENTS",
          "guard_id": "material_requirement_gap",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the material requirement gap condition is satisfied.",
          "transition_id": "implementation_requires_requirements"
        },
        {
          "description": "Move from IMPLEMENT to REFACTOR after completing the declared node obligations.",
          "destination_node": "REFACTOR",
          "guard_id": "implementation_complexity_identified",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the implementation complexity identified condition is satisfied.",
          "transition_id": "implementation_needs_refactor"
        }
      ],
      "completion_conditions": [
        "implementation_scope_completed",
        "all_planned_work_items_completed",
        "implementation_deviations_classified",
        "implementation_repository_observed"
      ],
      "current_node": "IMPLEMENT",
      "entry_conditions": [
        "requirements_current",
        "design_current",
        "task_plan_current",
        "current_plan_user_confirmed"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Complete every work item authorized by the current task plan before entering TEST.",
          "required": true,
          "step_id": "implementation.execute_plan"
        },
        {
          "purpose": "Record exact changed paths or the no-change state and deviations.",
          "required": true,
          "step_id": "implementation.record_surface"
        },
        {
          "purpose": "Classify implementation deviations as requirement, design, or complexity concerns.",
          "required": true,
          "step_id": "implementation.classify_deviations"
        }
      ],
      "node_purpose": "Execute the current task plan while Core records the actual changed surface.",
      "payload_contract": "implementation-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "implementation_summary",
          "required": true
        }
      ],
      "revision": 7,
      "submission_tool": "taskbelay_submit_implementation",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "IMPLEMENT",
    "evidence": null,
    "file_scope_records": [
      {
        "accepted_path_states": null,
        "applicability": "task_plan_revision",
        "consumed": false,
        "created_at": "2026-09-10T00:00:00Z",
        "decided_at": "2026-09-10T00:00:00Z",
        "decision": "reject",
        "intent_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "observed": false,
        "paths": [
          "src/extra.js"
        ],
        "reason": "The user rejected the write and the retained content has been restored.",
        "request_id": "scope-generated-1",
        "source_action_id": "action-generated-2",
        "source_node": "IMPLEMENT",
        "task_plan_revision": 1
      }
    ],
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
      "from_revision": 6,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 7
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
    "revision": 7,
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
  "tool": "taskbelay_resolve_blocker"
}
```
