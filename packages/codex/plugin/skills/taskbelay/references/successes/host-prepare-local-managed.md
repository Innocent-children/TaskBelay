# prepare: local-managed

Implementation: `packages/codex/bin/taskbelay-codex.mjs` — `runHostLaunchCommand`.

Complete result from the adapter implementation, executed in a temporary Git fixture.
Host session creation, handoff completion and Core terminal reads are supplied test observations,
not live Host calls. Paths, generated identities, digests and timestamps use stable example values;
all fields and their references are retained and compared.

Resolved request:

<!-- example:resolved-host prepare local-managed -->
```json
{
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
  "base_branch": "main",
  "carry_changes": false,
  "handoff_file": "/private/tmp/taskbelay-handoff.json",
  "remote_name": "",
  "repository_key": "primary",
  "repository_path": "/work/project",
  "request": "Return the requested field from the endpoint.",
  "source_type": "local",
  "surface": "managed_worktree",
  "target_branch": "codex/endpoint-field",
  "user_choice": {
    "mode": "taskbelay",
    "source": "user",
    "summary": "The user selected TaskBelay after reading the assessment."
  },
  "workspace_mode": "dedicated_worktree",
  "worktree_path": null
}
```

Complete response:

<!-- example:host-success prepare local-managed -->
```json
{
  "fetch_performed": false,
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
      "dispatch_attempt_id": null,
      "dispatch_recovery_reason": null,
      "host_client_thread_id": null,
      "host_operation_id": null,
      "host_operation_revision": null,
      "host_request": null,
      "host_thread_id": null,
      "phase": "prepared",
      "relocation_id": null,
      "surface": "managed_worktree",
      "target_effects": "not_invoked",
      "worktree_cleanup": "not_requested"
    },
    "remote_name": "",
    "repository_key": "primary",
    "request_digest": "0000000000000000000000000000000000000000000000000000000000000002",
    "snapshot_commit": null,
    "source_repository_identity": "0000000000000000000000000000000000000000000000000000000000000004",
    "source_type": "local",
    "target_branch": "codex/endpoint-field",
    "workspace_mode": "dedicated_worktree",
    "worktree_path": null
  },
  "receipt_path": "/example/support/provisioning/codex/launch-example/primary.json",
  "resumed": false,
  "source_dirty": false,
  "source_status_digest": "0000000000000000000000000000000000000000000000000000000000000001"
}
```
