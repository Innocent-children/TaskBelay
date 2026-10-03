# taskbelay_server_info: handshake

Implementation: `internal/mcp/server.go` — `dispatch`; `internal/application/submit_action.go` — `SubmitAction`.

Complete successful call from the documented scenario. The repository observer is a fixed test fixture;
Host authorization and Git operations are not executed here. Runtime IDs and timestamps use stable
example values, and operation digests use a sample digest. All other fields are compared with the
actual Core response. The resolved request below shows the current Task values substituted for the
identity and confirmation placeholders in the calling reference.

Resolved request:

<!-- example:resolved-mcp taskbelay_server_info handshake -->
```json
{}
```

Complete response:

<!-- example:mcp-success taskbelay_server_info handshake -->
```json
{
  "ok": true,
  "request_id": "request-success-example",
  "result": {
    "health": "ready",
    "host_preferences": {
      "claude": {
        "codebase_memory": false
      },
      "codex": {
        "codebase_memory": false
      },
      "deepseek": {
        "codebase_memory": false
      },
      "zcode": {
        "codebase_memory": false
      }
    },
    "method_profiles": [
      "plain",
      "spec-kit",
      "openspec"
    ],
    "product": "taskbelay",
    "supported_hosts": [
      "codex",
      "deepseek",
      "claude",
      "zcode"
    ],
    "supported_processes": [
      {
        "definition_digest": "eb35dcd623a1673abf768209fd4e7cb07979afb31b102c527bd4066b74a24430",
        "new_task_supported": true,
        "process_id": "standard-development"
      }
    ],
    "tools": [
      "taskbelay_server_info",
      "taskbelay_open_task",
      "taskbelay_get_task",
      "taskbelay_get_next_action",
      "taskbelay_submit_requirements",
      "taskbelay_submit_design",
      "taskbelay_submit_tasks",
      "taskbelay_submit_implementation",
      "taskbelay_submit_test",
      "taskbelay_submit_comprehension",
      "taskbelay_submit_refactor",
      "taskbelay_submit_delivery",
      "taskbelay_prepare_task_branch_rename",
      "taskbelay_prepare_task_relocation",
      "taskbelay_resolve_blocker",
      "taskbelay_recover_action",
      "taskbelay_cancel_task",
      "taskbelay_abandon_task"
    ],
    "transport": "stdio",
    "version": "0.20.1"
  },
  "tool": "taskbelay_server_info"
}
```
