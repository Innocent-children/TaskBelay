<!-- Generated from skills/taskbelay/core/core-lifecycle-examples.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Core relocation and terminal operations: examples

Read the [core relocation and terminal operations instructions](core-lifecycle.md) first. Use current values and the live schema; sample IDs and user decisions are never defaults.

Implementation: `internal/mcp/skill_examples_test.go`; `internal/mcp/skill_error_examples_test.go`;
`internal/mcp/skill_success_examples_test.go`.

Every request and response envelope is an object. Success `result` is an object. Error
`error`/`recovery` are objects, `error.details` is an array of objects when present, and
`ok`/`recovery.retry_safe` are booleans. See the
[shared field-type rules](tool-results.md#json-field-types).

## taskbelay_prepare_task_relocation-prepare

Request types: `host`/`task_id` are strings; `revision` is an integer.
Success `result.relocation_id` is a string and `result.task` is an object.

<!-- example:mcp taskbelay_prepare_task_relocation prepare -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "revision": 8
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_prepare_task_relocation-prepare.md).

Possible error for this request: the supplied `task_id` has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_prepare_task_relocation prepare-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_prepare_task_relocation",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```

## taskbelay_resolve_blocker-relocation

Request types: `host`, `task_id`, `action_id` and `relocation_id` are strings;
`relocation_destinations` is an array of objects with string `key` and `repository_path`.
Success `result` is the Task object.

<!-- example:mcp taskbelay_resolve_blocker relocation -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "blocked-action",
  "relocation_id": "relocation-example",
  "relocation_destinations": [
    {
      "key": "primary",
      "repository_path": "/work/tasks/relocated-endpoint"
    }
  ]
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_resolve_blocker-relocation.md).

Possible error for this request: `relocation_id` is present but `relocation_destinations` is empty.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"relocation_destinations","value":[]} -->
<!-- example:mcp-output taskbelay_resolve_blocker relocation-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_resolve_blocker",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "relocation_destinations: the current transition requires at least one item",
    "details": [
      {
        "path": "relocation_destinations",
        "rule": "required_collection_non_empty",
        "message": "the current transition requires at least one item"
      }
    ]
  },
  "recovery": {
    "retry_safe": true,
    "action": "correct_request",
    "message": "Correct only allowed_paths using established values and user decisions. Keep the same tool and existing request identity fields unless listed. Do not add the response request_id to tools that do not accept it. Submit once; ask only for missing facts or decisions, and stop if the corrected request fails.",
    "allowed_paths": [
      "relocation_destinations"
    ]
  }
}
```

## taskbelay_cancel_task-cancel

Request types: `request_id`, `host`, `task_id` and `reason` are strings; `revision` is an integer.
Success `result` is the Task object, `result.current_action` is `null`, and `result.outcome`
is an object.

<!-- example:mcp taskbelay_cancel_task cancel -->
```json
{
  "request_id": "cancel-request-example",
  "host": "deepseek",
  "task_id": "task-example",
  "revision": 8,
  "reason": "The user cancelled this endpoint change."
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_cancel_task-cancel.md).

Possible error for this request: the supplied `task_id` has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_cancel_task cancel-error -->
```json
{
  "ok": false,
  "request_id": "cancel-request-example",
  "tool": "taskbelay_cancel_task",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```

## taskbelay_abandon_task-abandon

Request types: `host`, `task_id` and `reason` are strings; `revision` is an integer.
Success `result` is the Task object, `result.current_action` is `null`, and `result.outcome`
is an object.

<!-- example:mcp taskbelay_abandon_task abandon -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "revision": 8,
  "reason": "The original worktree is unavailable and the user explicitly abandoned the Task."
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_abandon_task-abandon.md).

Possible error for this request: the supplied `task_id` has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_abandon_task abandon-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_abandon_task",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```

## taskbelay_prepare_task_branch_rename-prepare-branch-rename

IDs, names, reason and choice are strings; revision, after and limit are integers. Use current Core values.

<!-- example:mcp taskbelay_prepare_task_branch_rename prepare-branch-rename -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "revision": 8,
  "repository_key": "primary",
  "target_branch": "task/renamed-endpoint",
  "reason": "Correct the branch name for the existing Task."
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_prepare_task_branch_rename-prepare-branch-rename.md).

Possible error for this request: task_id has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`; `internal/mcp/results.go` — `EncodeError`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_prepare_task_branch_rename prepare-branch-rename-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_prepare_task_branch_rename",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```

## taskbelay_resolve_blocker-rename-complete

IDs, names, reason and choice are strings; revision, after and limit are integers. Use current Core values.

<!-- example:mcp taskbelay_resolve_blocker rename-complete -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "blocked-action",
  "rename_id": "rename-example",
  "rename_choice": "complete"
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_resolve_blocker-rename-complete.md).

Possible error for this request: task_id has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`; `internal/mcp/results.go` — `EncodeError`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_resolve_blocker rename-complete-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_resolve_blocker",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```

## taskbelay_resolve_blocker-rename-cancel

IDs, names, reason and choice are strings; revision, after and limit are integers. Use current Core values.

<!-- example:mcp taskbelay_resolve_blocker rename-cancel -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "action_id": "blocked-action",
  "rename_id": "rename-example",
  "rename_choice": "cancel"
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_resolve_blocker-rename-cancel.md).

Possible error for this request: task_id has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`; `internal/mcp/results.go` — `EncodeError`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_resolve_blocker rename-cancel-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_resolve_blocker",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```

## taskbelay_get_task-history-page

This reads the complete retained array by pages. Use returned total/revision/next_after; a page
may contain fewer entries than limit because of response bytes. Only next_after=null means complete.
The Task projection also exposes first-page history_total/history_next_after/history_revision.

IDs, names, reason and choice are strings; revision, after and limit are integers. Use current Core values.

<!-- example:mcp taskbelay_get_task history-page -->
```json
{
  "host": "deepseek",
  "task_id": "task-example",
  "baseline_history": {
    "revision": 0,
    "after": 0,
    "limit": 16
  }
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_get_task-history-page.md).

Possible error for this request: task_id has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`; `internal/mcp/results.go` — `EncodeError`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_get_task history-page-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_get_task",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "No saved Task matches the requested Task identity or workspace claim."
  },
  "recovery": {
    "retry_safe": false,
    "action": "read_task",
    "message": "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."
  }
}
```
