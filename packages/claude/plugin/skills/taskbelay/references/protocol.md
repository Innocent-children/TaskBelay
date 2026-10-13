<!-- Generated from skills/taskbelay/core/protocol.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Current Core protocol

Tools are discovered from the actual server; no historical tool catalog is accepted. Core publishes
closed input/output Schemas from the same DTOs used by CLI and HTTP. Use `claude` for new Tasks.

| Tool | Inputs and purpose |
| --- | --- |
| `taskbelay_server_info` | Empty object: actual tools, coverage and evidence boundaries |
| `taskbelay_open_task` | operation_id, host, repositories, authorization, plan, checks |
| `taskbelay_get_task` | task_id, optional refresh (default true; false reads saved state), detail:true for full saved data |
| `taskbelay_checkpoint` | identity; optional assessment, progress, issue and Host cost deltas |
| `taskbelay_revise_task` | identity, reason and only the authorization/plan/checks objects to replace |
| `taskbelay_run_check` | identity, check_id, rationale; previous_run_id/new_information for retries and full_suite_basis for full suites; records reservation before starting a child |
| `taskbelay_record_evidence` | identity, check_id, source claim/human, status, summary, content_digest, actual user_ref for human |
| `taskbelay_get_run` | run_id; reads actual output and status without rerunning |
| `taskbelay_recover_run` | identity, run_id, resolution_basis, actual user_ref if needed |
| `taskbelay_finish_task` | identity, summary, per-goal acceptance and risks |
| `taskbelay_cancel_task` | identity, reason; releases claims and retains files |
| `taskbelay_list_tasks` | Optional page, page_size, include_archived, status; bounded pagination |
| `taskbelay_update_budget` | identity, current authorization_digest, budget {runs,execution_ms,work_ms,tokens}, actual user_ref and response; all effective caps updated together |
| `taskbelay_accept_known_failure` | identity, check_id, failed_evidence_id, comparison_evidence_id, failures, actual user_ref, reason |
| `taskbelay_archive_task` | identity, archived; terminal tasks only |

Identity means task_id, current revision and a stable operation_id. Reuse the returned identity rather
than guessing it. Success is ok:true with result; failure has error and recovery. Task results contain
task (current request, plan, checks, progress, usage and evidence), current digests, decision and budget.
Repositories contain repository_path, branch and changed_paths. Routine results omit file fingerprints,
old attempts and full output. get_task detail:true adds details (the complete application view, including
details.task). run/recover return view plus run {run_id,check_id,status,exit_code,duration_ms,output_bytes,
truncated}; read get_run for actual output or failure diagnostics. refreshed:false never claims current
applicability. Inspect decision.advice and missing_checks; continue/review/verify are agent work,
self_correct identifies corrections, ask_user is an actual saved decision, and stop preserves an unmet
limit, pending operation or terminal outcome.

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

## Request fields

- authorization: user_ref and request from the actual user, goals [{id,statement,checks}], plus explicit
  constraints and restrictions. expected_paths is only a forecast. Omitted limits or zero fields mean
  no user-set cap; positive values cap cumulative runs/execution_ms/core_ms/host_ms/tokens.
- plan: approach and verification_basis. Explain responsibility and proportionality in those fields
  where relevant; no repeated understanding, completion or work-item form is required.
- repositories: key and absolute repository_path; omit workspace_origin for the current branch.
  An optional prepared workspace uses the helper's origin. remote_freshness is optional; omitted
  observations remain not_checked. See [workspaces](workspace.md) only for actual Git operations.
- checks: id, kind (command/human), rationale, conditions; commands require argv. Defaults are cwd .,
  inputs ["."], timeout_ms 60000, retry requires_user, purpose acceptance. Use repeatable only when
  justified. Narrower inputs require input_basis. Regression checks can have accepted known failures;
  diagnostic checks need goal_ids. full_suite requires a reason each run; compares names the regression
  check whose existing failures are being compared.

revise_task replaces only supplied fields; preserve other definitions. Changed user outcomes or
restrictions, or expanded/removed explicit limits, require a real new user_ref. Method/forecast/check
changes within that boundary do not. Do not weaken acceptance to avoid a failure. update_budget takes
one real budget decision; work_ms updates both overlapping Core and reported-work caps. Zero removes
an existing cap only with that actual decision. Counters never reset.

checkpoint records meaningful progress and review, not every edit. assessment binds current
authorization_digest and content_digest, plus summary, paths [{path,goal_ids,basis}], constraints/facts
[{id,verdict,basis,uncertainty}]. Submit only new/affected judgments; Core retains unaffected ones.
Final review is {summary,uncertainty}; explain the actual diff, resolved cause, responsibilities,
complexity and verification. Core stamps applicability; it does not prove the reasoning true.
Clear a saved user issue only with clear_issue and the actual resolution_ref.

A failed check remains failed after a known-failure decision. Only applicable regression failures with
current comparison results and actual user acceptance qualify. Required acceptance checks cannot be
waived. Related requirement/input changes invalidate results and exceptions; unrelated plan prose does
not require another user acceptance. Claims never satisfy a check.

Implementation: internal/application service and DTOs, internal/domain authorization/plan/control,
internal/mcp view and Schema. Saved details are for inspection, never an alternative writable Task.

Maintainer implementation reference: `internal/mcp/server.go`.
