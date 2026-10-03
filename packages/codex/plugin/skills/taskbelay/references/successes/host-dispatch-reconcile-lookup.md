# dispatch-reconcile: lookup

Implementation: `packages/codex/bin/taskbelay-codex.mjs` — `runHostLaunchCommand`.

Complete result from the adapter implementation, executed in a temporary Git fixture.
Host session creation, handoff completion and Core terminal reads are supplied test observations,
not live Host calls. Paths, generated identities, digests and timestamps use stable example values;
all fields and their references are retained and compared.

Resolved request:

<!-- example:resolved-host dispatch-reconcile lookup -->
```json
{
  "candidates": [
    {
      "initial_prompt": "$taskbelay-codex:taskbelay\n\nResume the confirmed TaskBelay launch launch-example for repository primary.\n\nBefore any Core call, read the saved material and every confirmed repository receipt. Follow the Skill bootstrap route for each receipt surface/phase: first managed initialization verifies the frozen commit and clean destination and applies the selected snapshot through the helper; provisioned worktrees retain carried content and subsequent work without repeating initialization. Inspect current worktree identity and permissions separately from saved receipt data. Then follow the Skill creation/resume rules for the actual Core state, using workspace_origin unchanged only for creation.\n\nRequest overview: Return the requested field from the endpoint.\n\nComplete handoff and original discussion: \"/example/support/provisioning/codex/launch-example/handoffs/primary.md\"\n\nStructured material with original message text: \"/example/support/provisioning/codex/launch-example/handoffs/primary.json\"\n\nThe saved handoff contains the full development request. Its labeled sections separate confirmed requirements from suggestions, assumptions, and historical discussion.\n\n## Goal and expected result\n\nReturn the requested field from the endpoint.\n\n## Confirmed requirements\n\n1. Return the requested field from the endpoint.\n\n   Discussion: m1\n\n## Terminology and examples\n\nNone recorded.\n\n## Scope, exclusions, and constraints\n\nNone recorded.\n\n## Code investigation\n\nNone recorded.\n\n## Working instructions and authorizations\n\nNone recorded.\n\n## Suggestions not accepted by the user\n\nNone recorded.\n\n## Assumptions, not confirmed requirements\n\nNone recorded.\n\n## Unresolved questions\n\nNone recorded.",
      "thread_id": "thread-example"
    }
  ],
  "launch_id": "launch-example",
  "repository_key": "primary"
}
```

Complete response:

<!-- example:host-success dispatch-reconcile lookup -->
```json
{
  "matched": true,
  "receipt": {
    "admission": {
      "assessment": {
        "anchor": {
          "repositories": [
            {
              "canonical_root": "/work/project",
              "dirty_paths": [],
              "dirty_paths_truncated": false,
              "head": "0000000000000000000000000000000000000001",
              "repository_key": "primary",
              "status_digest": "0000000000000000000000000000000000000000000000000000000000000001"
            }
          ],
          "request_digest": "0000000000000000000000000000000000000000000000000000000000000002"
        },
        "candidate_components": [
          "Endpoint response"
        ],
        "candidate_paths": [
          "src/endpoint.js"
        ],
        "change_level": "standard",
        "host_or_platform_flags": [],
        "observed_repositories": [
          "/work/project"
        ],
        "persistence_or_state_flags": [],
        "public_contract_flags": [
          "Response field changes"
        ],
        "reasons": [
          "The response is a public contract."
        ],
        "recommendation": "taskbelay",
        "unknowns": [],
        "verification_shape": [
          "Endpoint response check"
        ]
      },
      "user_choice": {
        "mode": "taskbelay",
        "source": "user",
        "summary": "The user selected TaskBelay after reading the assessment."
      }
    },
    "base_branch": "main",
    "base_commit": "0000000000000000000000000000000000000001",
    "carry_changes": false,
    "created_at": "2026-09-10T00:00:00.000Z",
    "handoff_digest": "0000000000000000000000000000000000000000000000000000000000000003",
    "host": "codex",
    "launch_id": "launch-example",
    "operation_status": {
      "branch_cleanup": "not_requested",
      "dispatch_attempt_id": "0000000000000000000000000000000000000000000000000000000000000004",
      "dispatch_recovery_reason": null,
      "host_client_thread_id": null,
      "host_operation_id": null,
      "host_operation_revision": null,
      "host_request": {
        "prompt": "$taskbelay-codex:taskbelay\n\nResume the confirmed TaskBelay launch launch-example for repository primary.\n\nBefore any Core call, read the saved material and every confirmed repository receipt. Follow the Skill bootstrap route for each receipt surface/phase: first managed initialization verifies the frozen commit and clean destination and applies the selected snapshot through the helper; provisioned worktrees retain carried content and subsequent work without repeating initialization. Inspect current worktree identity and permissions separately from saved receipt data. Then follow the Skill creation/resume rules for the actual Core state, using workspace_origin unchanged only for creation.\n\nRequest overview: Return the requested field from the endpoint.\n\nComplete handoff and original discussion: \"/example/support/provisioning/codex/launch-example/handoffs/primary.md\"\n\nStructured material with original message text: \"/example/support/provisioning/codex/launch-example/handoffs/primary.json\"\n\nThe saved handoff contains the full development request. Its labeled sections separate confirmed requirements from suggestions, assumptions, and historical discussion.\n\n## Goal and expected result\n\nReturn the requested field from the endpoint.\n\n## Confirmed requirements\n\n1. Return the requested field from the endpoint.\n\n   Discussion: m1\n\n## Terminology and examples\n\nNone recorded.\n\n## Scope, exclusions, and constraints\n\nNone recorded.\n\n## Code investigation\n\nNone recorded.\n\n## Working instructions and authorizations\n\nNone recorded.\n\n## Suggestions not accepted by the user\n\nNone recorded.\n\n## Assumptions, not confirmed requirements\n\nNone recorded.\n\n## Unresolved questions\n\nNone recorded.",
        "target": {
          "environment": {
            "startingState": {
              "branchName": "0000000000000000000000000000000000000001",
              "type": "branch"
            },
            "type": "worktree"
          },
          "projectId": "project-example",
          "type": "project"
        },
        "title": "TaskBelay launch-example primary"
      },
      "host_thread_id": "thread-example",
      "phase": "dispatched",
      "relocation_id": null,
      "surface": "managed_worktree",
      "target_effects": "may_have_effects",
      "worktree_cleanup": "not_requested"
    },
    "remote_name": "",
    "repository_key": "primary",
    "request_digest": "0000000000000000000000000000000000000000000000000000000000000002",
    "snapshot_commit": null,
    "source_repository_identity": "0000000000000000000000000000000000000000000000000000000000000005",
    "source_type": "local",
    "target_branch": "codex/endpoint-field",
    "workspace_mode": "dedicated_worktree",
    "worktree_path": null
  },
  "receipt_path": "/example/support/provisioning/codex/launch-example/primary.json",
  "should_dispatch": false
}
```
