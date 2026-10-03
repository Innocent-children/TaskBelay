<!-- Generated from skills/taskbelay/core/successes/taskbelay_submit_requirements-requirements_ready.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_submit_requirements: requirements_ready

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_requirements requirements_ready -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "host": "claude",
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
    "baseline": {
      "acceptance_criteria": [
        "The response contains the requested field."
      ],
      "assumptions": [],
      "constraints": [],
      "goal": "Return the requested field from the endpoint.",
      "out_of_scope": [
        "Other endpoints"
      ],
      "scope": [
        "Endpoint response"
      ]
    },
    "problem_class": "none",
    "unresolved_questions": []
  },
  "reason": "",
  "summary": "Completed the current REQUIREMENTS work.",
  "task_id": "task-example",
  "transition_id": "requirements_ready"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_requirements requirements_ready -->
```json
{
  "ok": true,
  "request_id": "request-success-example",
  "result": {
    "baselines": {
      "design": null,
      "history": null,
      "history_next_after": null,
      "history_revision": 2,
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
      "task_plan": null
    },
    "blocker": null,
    "completed_at": null,
    "comprehension": null,
    "created_at": "2026-09-10T00:00:00Z",
    "current_action": {
      "action_id": "action-generated-1",
      "action_kind": "COMPLETE_DESIGN",
      "allowed_effects": [
        "read_repository",
        "edit_process_artifacts",
        "request_user_decision"
      ],
      "available_transitions": [
        {
          "description": "Move from DESIGN to TASKS after completing the declared node obligations.",
          "destination_node": "TASKS",
          "guard_id": "design_baseline_complete",
          "reason_required": false,
          "selection_condition": "Choose this transition only when the design baseline complete condition is satisfied.",
          "transition_id": "design_ready"
        },
        {
          "description": "Move from DESIGN to REQUIREMENTS after completing the declared node obligations.",
          "destination_node": "REQUIREMENTS",
          "guard_id": "material_requirement_gap",
          "reason_required": true,
          "selection_condition": "Choose this transition only when the material requirement gap condition is satisfied.",
          "transition_id": "design_requires_requirements"
        }
      ],
      "completion_conditions": [
        "design_approach_defined",
        "design_components_bounded",
        "design_decisions_explicit",
        "design_alternatives_considered",
        "design_complexity_justified",
        "design_risks_recorded",
        "design_presented_for_discussion"
      ],
      "current_node": "DESIGN",
      "entry_conditions": [
        "requirements_current",
        "repository_context_available"
      ],
      "guidance": "Complete the current node contract and select one available transition.",
      "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "issued_at": "2026-09-10T00:00:00Z",
      "method_profile": "plain",
      "method_steps": [
        {
          "purpose": "Select the simplest viable approach for the current requirements.",
          "required": true,
          "step_id": "design.choose_approach"
        },
        {
          "purpose": "Identify unnecessary abstractions and justify retained complexity.",
          "required": true,
          "step_id": "design.review_complexity"
        },
        {
          "purpose": "Record components, decisions, rejected alternatives, and risks.",
          "required": true,
          "step_id": "design.record_decisions"
        }
      ],
      "node_purpose": "Select and explain the simplest viable design for the current requirements baseline.",
      "payload_contract": "design-result",
      "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
      "process_id": "standard-development",
      "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "required_evidence": [
        {
          "kind": "repository_observation",
          "required": true
        },
        {
          "kind": "design_baseline",
          "required": true
        }
      ],
      "revision": 2,
      "submission_tool": "taskbelay_submit_design",
      "task_id": "task-example"
    },
    "current_changed_paths": [],
    "current_cursor": "DESIGN",
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
      "from_revision": 1,
      "kind": "apply_action",
      "operation_id": "request-success-example",
      "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "to_revision": 2
    },
    "origin_host": "claude",
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
    "revision": 2,
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
  "tool": "taskbelay_submit_requirements"
}
```
