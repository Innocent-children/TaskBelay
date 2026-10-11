<!-- Generated from skills/taskbelay/core/protocol.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Current Core protocol

Tools are discovered from the actual server; no historical tool catalog is accepted. Core publishes
closed input/output Schemas from the same DTOs used by CLI and HTTP. Use `codex` for new Tasks.

| Tool | Inputs and purpose |
| --- | --- |
| `taskbelay_server_info` | Empty object: actual tools, coverage and evidence boundaries |
| `taskbelay_open_task` | operation_id, host, repositories, authorization, checks |
| `taskbelay_get_task` | task_id, optional refresh (default true; false reads saved state) |
| `taskbelay_checkpoint` | identity; optional assessment, progress, checks, issue and Host cost deltas |
| `taskbelay_revise_task` | identity, actual revised authorization, checks, reason |
| `taskbelay_run_check` | identity, check_id; records reservation before starting a child |
| `taskbelay_record_evidence` | identity, check_id, source claim/human, status, summary, content_digest, actual user_ref for human |
| `taskbelay_get_run` | run_id; reads actual output and status without rerunning |
| `taskbelay_recover_run` | identity, run_id, resolution_basis, actual user_ref if needed |
| `taskbelay_finish_task` | identity, summary, per-goal acceptance and risks |
| `taskbelay_cancel_task` | identity, reason; releases claims and retains files |
| `taskbelay_list_tasks` | Empty object; latest 200 summaries |

Identity means task_id, current revision and a stable operation_id. Typical successful results are
the task view (`task`, digests, `decision`); run/recover return `view` and the actual `run`. Read saved
results and provenance. `refreshed:false` never asserts current applicability.

Complete read request:

```json
{}
```

Send it to `taskbelay_server_info`. The success envelope has `ok:true`, the actual `request_id` and
`tool`, and `result` containing product, current version, schema, tools, host_preferences, coverage,
semantic_evidence and cost. The machine-readable complete example and output Schema are checked
by `internal/mcp/server_test.go`; variable request identities and product versions come from Core.

Adjacent same-tool error request (concrete trigger: an unknown member):

```json
{"unexpected":true}
```

Complete encoded error, with the response identity represented as `example-request`:

<!-- example:mcp-error taskbelay_server_info unknown_member -->
```json
{"ok":false,"request_id":"example-request","tool":"taskbelay_server_info","error":{"code":"INVALID_ARGUMENT","message":"member is not allowed by the current request","details":[{"path":"arguments.unexpected","rule":"requirement","message":"member is not allowed by the current request"}]},"recovery":{"action":"correct_request","retry_safe":false,"message":"Correct only the identified fields from existing facts; preserve operation identity and real authorization.","allowed_paths":["arguments.unexpected"]}}
```

Implementation: `mcp.Decode`, `Server.Dispatch`, `OutputSchema`; tests reproduce the unknown-member
trigger and compare the entire normalized error. Do not add source, user decisions or passing
results while correcting fields. Budget failures retain counters. Uncertain execution requires
reading the saved run; storage incompatibility requires preserving data, not resetting it.
