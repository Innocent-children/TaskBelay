# taskbelay_submit_delivery: delivery_complete

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_submit_delivery delivery_complete -->
```json
{
  "action_id": "action-example",
  "artifacts": {
    "current": [],
    "other_process": []
  },
  "host": "{{host}}",
  "method_results": {
    "delivery.prepare_summary": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "delivery.reconcile_acceptance": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    },
    "delivery.reconcile_method_artifacts": {
      "capability": "",
      "summary": "Completed the current semantic work for the endpoint change."
    }
  },
  "node_result": {
    "acceptance": [
      {
        "criterion": "The response contains the requested field.",
        "evidence_ids": [
          "evidence-generated-1"
        ],
        "status": "satisfied",
        "work_item_ids": [
          "work-endpoint"
        ]
      }
    ],
    "findings": [],
    "problem_class": "none",
    "risks": [],
    "unverified_items": []
  },
  "reason": "",
  "summary": "Completed the current DELIVERY work.",
  "task_id": "task-example",
  "transition_id": "delivery_complete"
}
```

Complete response:

<!-- example:mcp-success taskbelay_submit_delivery delivery_complete -->
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
    "completed_at": "2026-09-10T00:00:00Z",
    "comprehension": {
      "confirmed_at": "2026-09-10T00:00:00Z",
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "design_revision": 1,
      "explained_components": [
        "Endpoint response mapper and its targeted check."
      ],
      "maintenance_risks": [],
      "record_id": "comprehension-generated-1",
      "requirements_revision": 1,
      "task_plan_revision": 1,
      "test_record_id": "test-generated-1",
      "user_evidence_id": "evidence-generated-2"
    },
    "created_at": "2026-09-10T00:00:00Z",
    "current_action": null,
    "current_changed_paths": [],
    "current_cursor": "DONE",
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
    "origin_host": "{{host}}",
    "outcome": {
      "acceptance": [
        {
          "criterion": "The response contains the requested field.",
          "evidence_ids": [
            "evidence-generated-1"
          ],
          "status": "satisfied",
          "work_item_ids": [
            "work-endpoint"
          ]
        }
      ],
      "automated_evidence_ids": [
        "evidence-generated-1"
      ],
      "completed_at": "2026-09-10T00:00:00Z",
      "comprehension_record_id": "comprehension-generated-1",
      "final_repository_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "manual_evidence_ids": [
        "evidence-generated-2"
      ],
      "requirements_revision": 1,
      "risks": [],
      "status": "completed",
      "summary": "Completed the current DELIVERY work.",
      "test_record_id": "test-generated-1"
    },
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
    "test": {
      "completed_at": "2026-09-10T00:00:00Z",
      "content_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "design_revision": 1,
      "evidence_ids": [
        "evidence-generated-1"
      ],
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
