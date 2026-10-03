# Codex launch examples

Read [admission instructions](admission.md) before using the matching example.
Replace sample paths, IDs and decisions with current values. Preserve complete actual results.
The examples do not grant authorization or establish that a Host operation occurred.

Implementation: `packages/codex/bin/taskbelay-codex.mjs` — `runHostLaunchCommand`;
`packages/codex/lib/task-launch.mjs`.

## host-inspect-single

<!-- example:host inspect single -->
```json
{
  "request": "Return the requested field from the endpoint.",
  "repositories": [
    {
      "key": "primary",
      "repository_path": "/work/project"
    }
  ]
}
```

Complete successful request and response: [view every returned field](successes/host-inspect-single.md).

## assessment-assessment-standard

<!-- example:assessment assessment standard -->
```json
{
  "change_level": "standard",
  "observed_repositories": [
    "/work/project"
  ],
  "candidate_components": [
    "endpoint"
  ],
  "candidate_paths": [
    "src/endpoint.js"
  ],
  "public_contract_flags": [
    "Response field changes"
  ],
  "persistence_or_state_flags": [],
  "host_or_platform_flags": [],
  "verification_shape": [
    "Endpoint response check"
  ],
  "unknowns": [],
  "recommendation": "taskbelay",
  "reasons": [
    "The response is a public contract."
  ],
  "anchor": {
    "request_digest": "6e1ecf5454bf017b0a842e6d3f7f537dd21f1e4b5741ce8967a54a4d33e662e6",
    "repositories": [
      {
        "repository_key": "primary",
        "canonical_root": "/work/project",
        "head": "1111111111111111111111111111111111111111",
        "status_digest": "2222222222222222222222222222222222222222222222222222222222222222",
        "dirty_paths": [],
        "dirty_paths_truncated": false
      }
    ]
  }
}
```

## host-prepare-local-managed

<!-- example:host prepare local-managed -->
```json
{
  "request": "Return the requested field from the endpoint.",
  "repository_key": "primary",
  "repository_path": "/work/project",
  "workspace_mode": "dedicated_worktree",
  "source_type": "local",
  "carry_changes": false,
  "remote_name": "",
  "base_branch": "main",
  "target_branch": "codex/endpoint-field",
  "surface": "managed_worktree",
  "worktree_path": null,
  "handoff_file": "/private/tmp/taskbelay-handoff.json",
  "assessment": {
    "change_level": "standard",
    "observed_repositories": [
      "/work/project"
    ],
    "candidate_components": [
      "Endpoint response"
    ],
    "candidate_paths": [
      "src/endpoint.js"
    ],
    "public_contract_flags": [
      "Response field changes"
    ],
    "persistence_or_state_flags": [],
    "host_or_platform_flags": [],
    "verification_shape": [
      "Endpoint response check"
    ],
    "unknowns": [],
    "recommendation": "taskbelay",
    "reasons": [
      "The response is a public contract."
    ],
    "anchor": {
      "request_digest": "6e1ecf5454bf017b0a842e6d3f7f537dd21f1e4b5741ce8967a54a4d33e662e6",
      "repositories": [
        {
          "repository_key": "primary",
          "canonical_root": "/work/project",
          "head": "1111111111111111111111111111111111111111",
          "status_digest": "2222222222222222222222222222222222222222222222222222222222222222",
          "dirty_paths": [],
          "dirty_paths_truncated": false
        }
      ]
    }
  },
  "user_choice": {
    "source": "user",
    "mode": "taskbelay",
    "summary": "The user selected TaskBelay after reading the assessment."
  }
}
```

Complete successful request and response: [view every returned field](successes/host-prepare-local-managed.md).

## host-prepare-remote-cli

<!-- example:host prepare remote-cli -->
```json
{
  "request": "Return the requested field from the endpoint.",
  "repository_key": "primary",
  "repository_path": "/work/project",
  "workspace_mode": "dedicated_worktree",
  "source_type": "remote",
  "carry_changes": false,
  "remote_name": "origin",
  "base_branch": "main",
  "target_branch": "codex/endpoint-field",
  "surface": "cli_worktree",
  "worktree_path": "/work/tasks/endpoint-field",
  "handoff_file": "/private/tmp/taskbelay-handoff.json",
  "assessment": {
    "change_level": "standard",
    "observed_repositories": [
      "/work/project"
    ],
    "candidate_components": [
      "Endpoint response"
    ],
    "candidate_paths": [
      "src/endpoint.js"
    ],
    "public_contract_flags": [
      "Response field changes"
    ],
    "persistence_or_state_flags": [],
    "host_or_platform_flags": [],
    "verification_shape": [
      "Endpoint response check"
    ],
    "unknowns": [],
    "recommendation": "taskbelay",
    "reasons": [
      "The response is a public contract."
    ],
    "anchor": {
      "request_digest": "6e1ecf5454bf017b0a842e6d3f7f537dd21f1e4b5741ce8967a54a4d33e662e6",
      "repositories": [
        {
          "repository_key": "primary",
          "canonical_root": "/work/project",
          "head": "1111111111111111111111111111111111111111",
          "status_digest": "2222222222222222222222222222222222222222222222222222222222222222",
          "dirty_paths": [],
          "dirty_paths_truncated": false
        }
      ]
    }
  },
  "user_choice": {
    "source": "user",
    "mode": "taskbelay",
    "summary": "The user selected TaskBelay after reading the assessment."
  }
}
```

Complete successful request and response: [view every returned field](successes/host-prepare-remote-cli.md).

## host-status-launch

<!-- example:host status launch -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary"
}
```

Complete successful request and response: [view every returned field](successes/host-status-launch.md).

## host-prepare-local-branch

<!-- example:host prepare local-branch -->
```json
{
  "request": "Return the requested field from the endpoint.",
  "repository_key": "primary",
  "repository_path": "/work/project",
  "workspace_mode": "new_branch",
  "source_type": "local",
  "carry_changes": false,
  "remote_name": "",
  "base_branch": "main",
  "target_branch": "codex/endpoint-field",
  "surface": "current_session",
  "worktree_path": "/work/project",
  "handoff_file": null,
  "assessment": {
    "change_level": "standard",
    "observed_repositories": [
      "/work/project"
    ],
    "candidate_components": [
      "Endpoint response"
    ],
    "candidate_paths": [
      "src/endpoint.js"
    ],
    "public_contract_flags": [
      "Response field changes"
    ],
    "persistence_or_state_flags": [],
    "host_or_platform_flags": [],
    "verification_shape": [
      "Endpoint response check"
    ],
    "unknowns": [],
    "recommendation": "taskbelay",
    "reasons": [
      "The response is a public contract."
    ],
    "anchor": {
      "request_digest": "6e1ecf5454bf017b0a842e6d3f7f537dd21f1e4b5741ce8967a54a4d33e662e6",
      "repositories": [
        {
          "repository_key": "primary",
          "canonical_root": "/work/project",
          "head": "1111111111111111111111111111111111111111",
          "status_digest": "2222222222222222222222222222222222222222222222222222222222222222",
          "dirty_paths": [],
          "dirty_paths_truncated": false
        }
      ]
    }
  },
  "user_choice": {
    "source": "user",
    "mode": "taskbelay",
    "summary": "The user selected TaskBelay after reading the assessment."
  }
}
```

Complete successful request and response: [view every returned field](successes/host-prepare-local-branch.md).

## host-local-provision-current-session

<!-- example:host local-provision current-session -->
```json
{"launch_id":"launch-example","repository_key":"primary"}
```

Complete successful request and response: [view every returned field](successes/host-local-provision-current-session.md).

## host-dispatch-start-managed

<!-- example:host dispatch-start managed -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "project_id": "project-example"
}
```

Complete successful request and response: [view every returned field](successes/host-dispatch-start-managed.md).

## host-dispatch-call-managed

<!-- example:host dispatch-call managed -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "dispatch_attempt_id": "3333333333333333333333333333333333333333333333333333333333333333"
}
```

Complete successful request and response: [view every returned field](successes/host-dispatch-call-managed.md).

## host-dispatch-result-queued

<!-- example:host dispatch-result queued -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "host_result": {
    "clientThreadId": "client-example"
  }
}
```

Complete successful request and response: [view every returned field](successes/host-dispatch-result-queued.md).

## host-dispatch-result-ready

<!-- example:host dispatch-result ready -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "host_result": {
    "threadId": "thread-example",
    "hostId": "local"
  }
}
```

Complete successful request and response: [view every returned field](successes/host-dispatch-result-ready.md).

## host-dispatch-recover-not-called

<!-- example:host dispatch-recover not-called -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "dispatch_attempt_id": "3333333333333333333333333333333333333333333333333333333333333333",
  "host_call_not_made": true,
  "previous_caller_stopped": true,
  "reason": "The prior caller stopped after parsing dispatch-call; its call log contains no create_thread invocation."
}
```

Complete successful request and response: [view every returned field](successes/host-dispatch-recover-not-called.md).

## host-dispatch-reconcile-lookup

<!-- example:host dispatch-reconcile lookup -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "candidates": [
    {
      "thread_id": "thread-example",
      "initial_prompt": "Complete original prompt read from the actual Host task."
    }
  ]
}
```

Complete successful request and response: [view every returned field](successes/host-dispatch-reconcile-lookup.md).

## host-bootstrap-managed

<!-- example:host bootstrap managed -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "worktree_path": "/work/tasks/endpoint-field"
}
```

Complete successful request and response: [view every returned field](successes/host-bootstrap-managed.md).

## host-cli-provision-single

<!-- example:host cli-provision single -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "source_repository_path": "/work/project",
  "additional_worktree_paths": []
}
```

Complete successful request and response: [view every returned field](successes/host-cli-provision-single.md).

## host-scope-single

<!-- example:host scope single -->
```json
{
  "launch_id": "launch-example",
  "repository_keys": [
    "primary"
  ],
  "primary_repository_key": "primary"
}
```

Complete successful request and response: [view every returned field](successes/host-scope-single.md).

## host-scope-multiple

<!-- example:host scope multiple -->
```json
{
  "launch_id": "launch-example",
  "repository_keys": [
    "api",
    "web"
  ],
  "primary_repository_key": "api"
}
```

Complete successful request and response: [view every returned field](successes/host-scope-multiple.md).

## host-supersede-replace

<!-- example:host supersede replace -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "expected_receipt_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
  "reason": "Use the corrected branch name before execution.",
  "replacement": {
    "request": "Return the requested field from the endpoint.",
    "repository_key": "primary",
    "repository_path": "/work/project",
    "workspace_mode": "dedicated_worktree",
    "source_type": "local",
    "carry_changes": false,
    "remote_name": "",
    "base_branch": "main",
    "target_branch": "codex/revised-endpoint",
    "surface": "managed_worktree",
    "worktree_path": null,
    "handoff_file": "/private/tmp/taskbelay-handoff.json",
    "assessment": {
      "change_level": "standard",
      "observed_repositories": [
        "/work/project"
      ],
      "candidate_components": [
        "Endpoint response"
      ],
      "candidate_paths": [
        "src/endpoint.js"
      ],
      "public_contract_flags": [
        "Response field changes"
      ],
      "persistence_or_state_flags": [],
      "host_or_platform_flags": [],
      "verification_shape": [
        "Endpoint response check"
      ],
      "unknowns": [],
      "recommendation": "taskbelay",
      "reasons": [
        "The response is a public contract."
      ],
      "anchor": {
        "request_digest": "6e1ecf5454bf017b0a842e6d3f7f537dd21f1e4b5741ce8967a54a4d33e662e6",
        "repositories": [
          {
            "repository_key": "primary",
            "canonical_root": "/work/project",
            "head": "1111111111111111111111111111111111111111",
            "status_digest": "2222222222222222222222222222222222222222222222222222222222222222",
            "dirty_paths": [],
            "dirty_paths_truncated": false
          }
        ]
      }
    },
    "user_choice": {
      "source": "user",
      "mode": "taskbelay",
      "summary": "The user selected TaskBelay after reading the assessment."
    },
    "launch_id": "launch-successor"
  }
}
```

Complete successful request and response: [view every returned field](successes/host-supersede-replace.md).

## host-supersede-resume-recover

<!-- example:host supersede-resume recover -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "seed_digest": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
}
```

Complete successful request and response: [view every returned field](successes/host-supersede-resume-recover.md).
