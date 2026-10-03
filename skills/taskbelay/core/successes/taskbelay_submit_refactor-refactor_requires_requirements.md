# taskbelay_submit_refactor: refactor_requires_requirements

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_refactor refactor_requires_requirements -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "other_process": []
  },
  "host": "{{host}}",
  "method_results": {
    "refactor.reconcile_artifacts": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "refactor.record_surface": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    },
    "refactor.simplify": {
      "capability": "",
      "summary": "Completed the current semantic work and recorded its findings."
    }
  },
  "node_result": {
    "behavior_change_intended": true,
    "findings": [
      "The proposed simplification changes the requested response semantics."
    ],
    "problem_class": "requirement_change",
    "simplifications": []
  },
  "reason": "The proposed simplification changes the requested response semantics.",
  "summary": "The proposed simplification changes the requested response semantics.",
  "task_id": "task-example",
  "transition_id": "refactor_requires_requirements"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_refactor refactor_requires_requirements -->
```json
{
  "ok": true,
  "request_id": "request-success-example",
  "result": {
    "baselines": {
      "design": null,
      "history": [
        {
          "created_at": "2026-09-10T00:00:00Z",
          "digest": "f58cb160bc5f944c054a15febb34ddb2c12f06e583a81ea6358680f8f2063b82",
          "kind": "design",
          "revision": 1,
          "summary": "Add the field in the existing response mapper."
        },
        {
          "created_at": "2026-09-10T00:00:00Z",
          "digest": "800e4715f9902c4abf89c749cb3165aa4be1005a5ac5c52d0675aeac29f590d8",
          "kind": "task_plan",
          "revision": 1,
          "summary": "Return the field and cover its response contract."
        }
      ],
      "history_next_after": null,
      "history_revision": 9,
      "history_total": 2,
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
      "task_plan": null
    },
    "blocker": null,
    "completed_at": null,
    "comprehension": null,
    "created_at": "2026-09-10T00:00:00Z",
    "current_action": {
      "action_id": "action-generated-1",
      "action_kind": "COMPLETE_REQUIREMENTS",
      "allowed_effects": [
        "read_repository",
        "edit_process_artifacts",
        "request_user_decision"
      ],
      "available_transitions": [
        {
          "description": "Move from REQUIREMENTS to DESIGN after completing the declared node obligations.",
          "destination_node": "DESIGN",
          "guard_id": "requirements_baseline_complete",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the requirements baseline complete condition is satisfied.",
          "transition_id": "requirements_ready"
        }
      ],
      "completion_conditions": [
        "requirements_goal_defined",
        "requirements_scope_bounded",
        "requirements_exclusions_explicit",
        "requirements_acceptance_nonempty",
        "requirements_material_questions_resolved",
        "requirements_user_decisions_recorded",
        "requirements_presented_for_discussion"
      ],
      "current_node": "REQUIREMENTS",
      "entry_conditions": [
        "intent_available",
        "repository_claimed",
        "requirements_context_available"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Capture a bounded goal, scope, exclusions, acceptance criteria, constraints, and assumptions.",
          "required": true,
          "step_id": "requirements.capture"
        },
        {
          "purpose": "Resolve material requirement questions with the developer.",
          "required": true,
          "step_id": "requirements.clarify"
        },
        {
          "purpose": "Verify that requirements are observable, bounded, and free of material ambiguity.",
          "required": true,
          "step_id": "requirements.validate"
        }
      ],
      "node_purpose": "Transform the immutable initial intent into the current requirements authority.",
      "payload_contract": "requirements-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "requirements_baseline",
          "required": true
        }
      ],
      "revision": 9,
      "submission_tool": "taskbelay_submit_requirements",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "REQUIREMENTS",
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
      }
    ],
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
      "from_revision": 8,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 9
    },
    "origin_host": "{{host}}",
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
      "current_budget": null,
      "plan": null,
      "usage": {
        "automatic_commands": 0,
        "evidence_items": 0,
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
  "tool": "taskbelay_submit_refactor"
}
```
