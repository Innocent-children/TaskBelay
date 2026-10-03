<!-- Generated from skills/taskbelay/core/successes/taskbelay_open_task-resume.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# taskbelay_open_task: resume

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_open_task resume -->
```json
{
  "host": "codex",
  "repository_path": "/work/tasks/endpoint-field"
}
```

Complete response:

<!-- example:mcp-success taskbelay_open_task resume -->
```json
{
  "ok": true,
  "request_id": "request-success-example",
  "result": {
    "created": false,
    "recovery_assessment": null,
    "task": {
      "baselines": {
        "design": null,
        "history": null,
        "history_next_after": null,
        "history_revision": 1,
        "history_total": 0,
        "requirements": null,
        "task_plan": null
      },
      "blocker": null,
      "completed_at": null,
      "comprehension": null,
      "created_at": "2026-09-10T00:00:00Z",
      "current_action": {
        "action_id": "action-example",
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
        "revision": 1,
        "submission_tool": "taskbelay_submit_requirements",
        "task_id": "task-example"
      },
      "current_changed_paths": [],
      "current_cursor": "REQUIREMENTS",
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
        "action_id": null,
        "committed_at": "2026-09-10T00:00:00Z",
        "from_revision": 0,
        "kind": "open_task",
        "operation_id": "setup-0-create",
        "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "to_revision": 1
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
      "revision": 1,
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
    }
  },
  "tool": "taskbelay_open_task"
}
```
