# Codex lifecycle examples

Read [host-lifecycle instructions](host-lifecycle.md) before using the matching example.
Replace sample paths, IDs and decisions with current values. Preserve complete actual results.
The examples do not grant authorization or establish that a Host operation occurred.

Implementation: `packages/codex/bin/taskbelay-codex.mjs` — `runHostLaunchCommand`;
`packages/codex/lib/task-launch.mjs`.

## host-handoff-start-start

<!-- example:host handoff-start start -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "relocation_id": "relocation-example",
  "thread_id": "thread-example"
}
```

Complete successful request and response: [view every returned field](successes/host-handoff-start-start.md).

## host-handoff-result-record

<!-- example:host handoff-result record -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "host_result": {
    "operationId": "host-operation-example",
    "revision": 1
  }
}
```

Complete successful request and response: [view every returned field](successes/host-handoff-result-record.md).

## host-handoff-status-pending

<!-- example:host handoff-status pending -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "status": "pending",
  "revision": 2,
  "worktree_path": null
}
```

Complete successful request and response: [view every returned field](successes/host-handoff-status-pending.md).

## host-handoff-status-succeeded

<!-- example:host handoff-status succeeded -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "status": "succeeded",
  "revision": 3,
  "worktree_path": "/work/tasks/relocated-endpoint"
}
```

Complete successful request and response: [view every returned field](successes/host-handoff-status-succeeded.md).

## host-cleanup-decision-keep

<!-- example:host cleanup-decision keep -->
```json
{
  "lifecycle": "DONE",
  "surface": "cli_worktree",
  "clean": false,
  "pushed": false,
  "stateCertain": true
}
```

Complete successful request and response: [view every returned field](successes/host-cleanup-decision-keep.md).

The cleanup examples use supplied Core observations for adapter execution. In a live call, copy the entire
actual terminal Core Task into `core_task`; never construct terminal state or identity fields.

## host-cleanup-worktree-remove-worktree

<!-- example:host cleanup-worktree remove-worktree -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "source_repository_path": "/work/project",
  "terminal": true,
  "authorized": true,
  "core_task": {
    "task_id": "task-cleanup",
    "origin_host": "codex",
    "current_cursor": "DONE",
    "revision": 10,
    "primary_repository_key": "primary",
    "workspace_origin": {
      "task_branch": "codex/endpoint-field",
      "canonical_worktree_root": "/work/tasks/endpoint-field",
      "provisioning_receipt_id": "receipt-from-core"
    },
    "repository": {
      "current_branch": "codex/endpoint-field",
      "current_head": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "detached": false,
      "worktree_instance_digest": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "binding_digest": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
    }
  }
}
```

Complete successful request and response: [view every returned field](successes/host-cleanup-worktree-remove-worktree.md).

## host-cleanup-branch-remove-branch

<!-- example:host cleanup-branch remove-branch -->
```json
{
  "launch_id": "launch-example",
  "repository_key": "primary",
  "source_repository_path": "/work/project",
  "terminal": true,
  "authorized": true,
  "core_task": {
    "task_id": "task-cleanup",
    "origin_host": "codex",
    "current_cursor": "DONE",
    "revision": 10,
    "primary_repository_key": "primary",
    "workspace_origin": {
      "task_branch": "codex/endpoint-field",
      "canonical_worktree_root": "/work/tasks/endpoint-field",
      "provisioning_receipt_id": "receipt-from-core"
    },
    "repository": {
      "current_branch": "codex/endpoint-field",
      "current_head": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "detached": false,
      "worktree_instance_digest": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "binding_digest": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
    }
  }
}
```

Complete successful request and response: [view every returned field](successes/host-cleanup-branch-remove-branch.md).
