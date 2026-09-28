<!-- Generated from skills/taskbelay/core/connection-examples.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Connect, open and read a Task: examples

Read the [connect, open and read a task instructions](connection.md) first. Use current values and the live schema; sample IDs and user decisions are never defaults.

Implementation: `internal/mcp/skill_examples_test.go`; `internal/mcp/skill_error_examples_test.go`;
`internal/mcp/skill_success_examples_test.go`.

All requests and response envelopes below are objects. In every error example, `ok` and
`recovery.retry_safe` are booleans; `request_id`/`tool` are strings; `error`/`recovery` are objects;
`error.details` is an array of objects when present. Follow the
[shared field-type rules](tool-results.md#json-field-types); do not stringify nested values.

## taskbelay_server_info-handshake

Request type: an empty object, not the string `"{}"`. In the linked success response, `result`
is an object; `product`, `version`, `transport` and `health` are strings; `supported_hosts`,
`method_profiles` and `tools` are arrays of strings; `supported_processes` is an array of objects;
`host_preferences` is an object whose Host entries contain boolean `codebase_memory`.

<!-- example:mcp taskbelay_server_info handshake -->
```json
{}
```

Complete successful request and response: [view every returned field](successes/taskbelay_server_info-handshake.md).

Possible error for this request: `host` is added to the handshake arguments; this tool accepts an empty object.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"host","value":"claude"} -->
<!-- example:mcp-output taskbelay_server_info handshake-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_server_info",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "host: the closed contract does not declare this member",
    "details": [
      {
        "path": "host",
        "rule": "unknown_member",
        "message": "the closed contract does not declare this member"
      }
    ]
  },
  "recovery": {
    "retry_safe": true,
    "action": "correct_request",
    "message": "Correct only allowed_paths using established values and user decisions. Keep the same tool and existing request identity fields unless listed. Do not add the response request_id to tools that do not accept it. Submit once; ask only for missing facts or decisions, and stop if the corrected request fails.",
    "allowed_paths": [
      "host"
    ]
  }
}
```

## taskbelay_open_task-create

Request types: `host`/`repository_path` are strings; `workspace_origin` and `new_task` are objects.
`workspace_origin.carry_changes` is a boolean; the other origin fields are strings.
`new_task.request`/`method_profile` are strings, and its three scope/acceptance fields are arrays
of strings. Pass `new_task` directly as shown, never as a JSON-encoded string.
Success types: `result`/`result.task` are objects, `result.created` is a boolean, and
`result.recovery_assessment` is an object or `null`.

<!-- example:mcp taskbelay_open_task create -->
```json
{
  "host": "claude",
  "repository_path": "/work/tasks/endpoint-field",
  "workspace_origin": {
    "mode": "dedicated_worktree",
    "source_type": "local",
    "carry_changes": false,
    "remote_name": "",
    "base_branch": "main",
    "base_commit": "1111111111111111111111111111111111111111",
    "task_branch": "task/endpoint-field",
    "provisioning_receipt_id": "receipt-example"
  },
  "new_task": {
    "request": "Return the requested field from the endpoint.",
    "initial_scope": [
      "Endpoint response"
    ],
    "initial_out_of_scope": [
      "Other endpoints"
    ],
    "known_acceptance_criteria": [
      "The response contains the requested field."
    ],
    "method_profile": "plain"
  }
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_open_task-create.md).

Possible error for this request: `new_task` is present but the primary `workspace_origin` preparation receipt is omitted.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"workspace_origin"} -->
<!-- example:mcp-output taskbelay_open_task create-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_open_task",
  "error": {
    "code": "WORKTREE_PROVISIONING_REQUIRED",
    "message": "workspace_origin: workspace_origin must identify the confirmed workspace preparation and its receipt",
    "details": [
      {
        "path": "workspace_origin",
        "rule": "workspace_origin_required",
        "message": "workspace_origin must identify the confirmed workspace preparation and its receipt"
      }
    ]
  },
  "recovery": {
    "retry_safe": false,
    "action": "provision_worktree",
    "message": "Read the confirmed workspace preparation receipt and submit its exact origin; prepare the workspace only if preparation has not completed."
  }
}
```

## taskbelay_open_task-multiple

Request types: `host`, `repository_path` and `primary_repository_key` are strings;
`workspace_origin`/`new_task` are objects, with the same field types as the create example above.
`additional_repositories` is an array of objects; each entry has string `key`/`repository_path`
and object `workspace_origin`. Success types: `result`/`result.task` are objects,
`result.created` is a boolean, and `result.recovery_assessment` is an object or `null`.

<!-- example:mcp taskbelay_open_task multiple -->
```json
{
  "host": "claude",
  "repository_path": "/work/tasks/api",
  "primary_repository_key": "api",
  "workspace_origin": {
    "mode": "dedicated_worktree",
    "source_type": "local",
    "carry_changes": false,
    "remote_name": "",
    "base_branch": "main",
    "base_commit": "1111111111111111111111111111111111111111",
    "task_branch": "task/endpoint-field",
    "provisioning_receipt_id": "receipt-api"
  },
  "additional_repositories": [
    {
      "key": "web",
      "repository_path": "/work/tasks/web",
      "workspace_origin": {
        "mode": "dedicated_worktree",
        "source_type": "local",
        "carry_changes": false,
        "remote_name": "",
        "base_branch": "main",
        "base_commit": "1111111111111111111111111111111111111111",
        "task_branch": "task/endpoint-field",
        "provisioning_receipt_id": "receipt-web"
      }
    }
  ],
  "new_task": {
    "request": "Return the field and show it in the client.",
    "initial_scope": [
      "API response",
      "Client rendering"
    ],
    "initial_out_of_scope": [
      "Other endpoints"
    ],
    "known_acceptance_criteria": [
      "The client shows the response field."
    ],
    "method_profile": "plain"
  }
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_open_task-multiple.md).

Possible error for this request: `new_task` is present but the primary `workspace_origin` preparation receipt is omitted.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"remove","path":"workspace_origin"} -->
<!-- example:mcp-output taskbelay_open_task multiple-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_open_task",
  "error": {
    "code": "WORKTREE_PROVISIONING_REQUIRED",
    "message": "workspace_origin: workspace_origin must identify the confirmed workspace preparation and its receipt",
    "details": [
      {
        "path": "workspace_origin",
        "rule": "workspace_origin_required",
        "message": "workspace_origin must identify the confirmed workspace preparation and its receipt"
      }
    ]
  },
  "recovery": {
    "retry_safe": false,
    "action": "provision_worktree",
    "message": "Read the confirmed workspace preparation receipt and submit its exact origin; prepare the workspace only if preparation has not completed."
  }
}
```

## taskbelay_open_task-resume

Request types: `host` and `repository_path` are strings. Omit `new_task` as shown, or use actual
`null`, not `"null"` or an empty object. Success types: `result`/`result.task` are objects,
`result.created` is a boolean, and `result.recovery_assessment` is an object or `null`.

<!-- example:mcp taskbelay_open_task resume -->
```json
{
  "host": "claude",
  "repository_path": "/work/tasks/endpoint-field"
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_open_task-resume.md).

Possible error for this request: a creation-only `workspace_origin` member is retained in a resume request without `new_task`.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"set","path":"workspace_origin","value":{}} -->
<!-- example:mcp-output taskbelay_open_task resume-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_open_task",
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "workspace_origin: resume omits workspace_origin, primary_repository_key and additional_repositories; creation includes new_task",
    "details": [
      {
        "path": "workspace_origin",
        "rule": "creation_member_on_resume",
        "message": "resume omits workspace_origin, primary_repository_key and additional_repositories; creation includes new_task"
      }
    ]
  },
  "recovery": {
    "retry_safe": true,
    "action": "correct_request",
    "message": "Correct only allowed_paths using established values and user decisions. Keep the same tool and existing request identity fields unless listed. Do not add the response request_id to tools that do not accept it. Submit once; ask only for missing facts or decisions, and stop if the corrected request fails.",
    "allowed_paths": [
      "workspace_origin"
    ]
  }
}
```

## taskbelay_get_task-read

Request types: `host`/`task_id` are strings. Success types: `result`/`result.task` are objects;
`result.recovery_assessment` is an object or `null`; Task `revision` is an integer and
`current_action` is an object or `null`.

<!-- example:mcp taskbelay_get_task read -->
```json
{
  "host": "claude",
  "task_id": "task-example"
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_get_task-read.md).

Possible error for this request: the supplied `task_id` has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_get_task read-error -->
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

## taskbelay_get_next_action-guarded-read

Request types: `host`/`task_id` are strings. Success types: `result` is an object;
`result.action`, `result.blocker`, `result.outcome` and `result.recovery_assessment` are objects
or `null`.

<!-- example:mcp taskbelay_get_next_action guarded-read -->
```json
{
  "host": "claude",
  "task_id": "task-example"
}
```

Complete successful request and response: [view every returned field](successes/taskbelay_get_next_action-guarded-read.md).

Possible error for this request: the supplied `task_id` has no record in the connected Core store.

Implementation: `internal/mcp/tools.go` — `ValidateToolInput`;
`internal/application/service.go` — `loadOwned, mapStoreError`;
`internal/mcp/server.go` — `dispatch`;
`internal/mcp/results.go` — `EncodeError, publicFailure, boundedCorrectionPaths, requestCorrectionPaths`.

<!-- error-case: {"operation":"missing_task"} -->
<!-- example:mcp-output taskbelay_get_next_action guarded-read-error -->
```json
{
  "ok": false,
  "request_id": "request-error-example",
  "tool": "taskbelay_get_next_action",
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
