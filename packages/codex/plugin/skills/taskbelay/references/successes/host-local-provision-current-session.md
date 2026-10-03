# local-provision: current-session

Implementation: `packages/codex/bin/taskbelay-codex.mjs` — `runHostLaunchCommand`.

Complete result from the adapter implementation, executed in a temporary Git fixture.
Host session creation, handoff completion and Core terminal reads are supplied test observations,
not live Host calls. Paths, generated identities, digests and timestamps use stable example values;
all fields and their references are retained and compared.

Resolved request:

<!-- example:resolved-host local-provision current-session -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary"
}
```

Complete response:

<!-- example:host-success local-provision current-session -->
```json
{
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
    "handoff_digest": null,
    "host": "codex",
    "launch_id": "launch-example",
    "operation_status": {
      "branch_cleanup": "not_requested",
      "dispatch_attempt_id": null,
      "dispatch_recovery_reason": null,
      "host_client_thread_id": null,
      "host_operation_id": null,
      "host_operation_revision": null,
      "host_request": null,
      "host_thread_id": null,
      "phase": "provisioned",
      "relocation_id": null,
      "surface": "current_session",
      "target_effects": "may_have_effects",
      "worktree_cleanup": "not_requested",
      "worktree_identity": "0000000000000000000000000000000000000000000000000000000000000003"
    },
    "remote_name": "",
    "repository_key": "primary",
    "request_digest": "0000000000000000000000000000000000000000000000000000000000000002",
    "snapshot_commit": null,
    "source_repository_identity": "0000000000000000000000000000000000000000000000000000000000000004",
    "source_type": "local",
    "target_branch": "codex/endpoint-field",
    "workspace_mode": "new_branch",
    "worktree_path": "/work/project"
  },
  "receipt_path": "/example/support/provisioning/codex/launch-example/primary.json",
  "workspace_origin": {
    "base_branch": "main",
    "base_commit": "0000000000000000000000000000000000000001",
    "carry_changes": false,
    "mode": "new_branch",
    "provisioning_receipt_id": "codex-0000000000000000000000000000000000000000000000000000000000000005",
    "remote_name": "",
    "source_type": "local",
    "task_branch": "codex/endpoint-field"
  }
}
```
