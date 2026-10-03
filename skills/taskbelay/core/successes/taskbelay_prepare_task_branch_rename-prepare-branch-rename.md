# taskbelay_prepare_task_branch_rename: prepare-branch-rename

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_prepare_task_branch_rename prepare-branch-rename -->
```json
{
  "host": "{{host}}",
  "reason": "Correct the branch name for the existing Task.",
  "repository_key": "primary",
  "revision": 1,
  "target_branch": "task/renamed-endpoint",
  "task_id": "task-example"
}
```

Complete response:

<!-- example:mcp-success taskbelay_prepare_task_branch_rename prepare-branch-rename -->
```json
{
  "ok": true,
  "request_id": "request-success-example",
  "result": {
    "rename_id": "rename-generated-1",
    "task": {
      "baselines": {
        "design": null,
        "history": null,
        "history_next_after": null,
        "history_revision": 2,
        "history_total": 0,
        "requirements": null,
        "task_plan": null
      },
      "blocker": {
        "blocker_id": "blocker-generated-1",
        "cause": "task_branch_rename_pending",
        "code": "TASK_BLOCKED",
        "condition": {
          "expected_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "expected_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "expected_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "expected_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "kind": "resolve_task_branch_rename",
          "rename_id": "rename-generated-1"
        },
        "created_at": "2026-09-10T00:00:00Z",
        "message": "One pure branch rename is prepared for the authorized Host.",
        "observed_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "required_resolution": "Execute one non-force git branch -m, then complete; cancel only while all original facts remain unchanged.",
        "resume_node": "REQUIREMENTS"
      },
      "branch_rename": {
        "prepared_at": "2026-09-10T00:00:00Z",
        "reason": "Correct the branch name for the existing Task.",
        "rename_id": "rename-generated-1",
        "repositories": [
          {
            "binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "index_digest": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
            "key": "primary"
          }
        ],
        "repository_key": "primary",
        "resume_node": "REQUIREMENTS",
        "source_branch": "task/endpoint-field",
        "target_branch": "task/renamed-endpoint"
      },
      "completed_at": null,
      "comprehension": null,
      "created_at": "2026-09-10T00:00:00Z",
      "current_action": {
        "action_id": "action-generated-1",
        "action_kind": "RESOLVE_BLOCKER",
        "allowed_effects": [
          "read_repository",
          "resolve_blocker"
        ],
        "available_transitions": [],
        "completion_conditions": [
          "blocker_condition_resolved"
        ],
        "current_node": "BLOCKED",
        "entry_conditions": [
          "blocker_recorded"
        ],
        "guidance": "Complete the current node contract and select one available transition.",
        "issuance_content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "issuance_history_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "issuance_identity_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "issued_at": "2026-09-10T00:00:00Z",
        "method_profile": "plain",
        "method_steps": [
          {
            "purpose": "blocker.resolve",
            "required": true,
            "step_id": "blocker.resolve"
          }
        ],
        "node_purpose": "Preserve a safety or recovery blocker.",
        "payload_contract": "blocker-resolution",
        "process_definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
        "process_id": "standard-development",
        "repository_binding_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "required_evidence": [
          {
            "kind": "repository_observation",
            "required": true
          },
          {
            "kind": "blocker_resolution",
            "required": true
          }
        ],
        "revision": 2,
        "submission_tool": "taskbelay_resolve_blocker",
        "task_id": "task-example"
      },
      "current_changed_paths": [],
      "current_cursor": "BLOCKED",
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
        "from_revision": 1,
        "kind": "prepare_task_branch_rename",
        "operation_id": "request-success-example",
        "payload_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "to_revision": 2
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
      "resume_cursor": "REQUIREMENTS",
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
    }
  },
  "tool": "taskbelay_prepare_task_branch_rename"
}
```
