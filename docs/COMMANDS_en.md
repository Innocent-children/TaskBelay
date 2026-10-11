# TaskBelay command reference

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

## Installation and local entrypoints

Public entry: npm install -g taskbelay@latest; taskbelay. Installation/status/doctor/upgrade/uninstall options remain defined by packages/taskbelay/lib/cli.mjs. Build the source contract with node scripts/taskbelay-local.mjs install --host codex --yes; select deepseek, claude or zcode as needed. This does not publish.

Core: taskbelay version; config validate (stdin); mcp --stdio; call <tool_name> (complete JSON stdin); host-check pre-file-write/workspace-available; webui start/open/status/stop (--json or --plain, start optionally --no-open). Set absolute TASKBELAY_DATA_DIR. Preserve old data and choose a new directory; no migration occurs. The unified npm launcher forwards call/webui to configured Core. Codex/Claude/ZCode taskbelay-<host> call <tool_name> forwards to bundled Core. DeepSeek uses the discovered MCP namespace.

pre-file-write takes host, repository_path, tool_name, paths, intent_digest, path_parse_complete and returns allow/deny with actual coverage, not future edit semantics. workspace-available takes repository_path or repositories [{key,repository_path}], returning available and an existing task_id read-only. Observation failure never means no Task. Phase artifact commands have been removed.

## MCP tools

| Tool | Current input |
| --- | --- |
| taskbelay_server_info | Empty object; actual capabilities |
| taskbelay_list_tasks | Optional page, page_size (1..100), include_archived, status (active/done/cancelled); returns page and has_next |
| taskbelay_open_task | operation_id, host, repositories, authorization, plan, checks |
| taskbelay_get_task | task_id, refresh (default true; false reads saved state) |
| taskbelay_checkpoint | identity; optional assessment/progress/issue and host_ms/wait_ms/tokens |
| taskbelay_revise_task | identity, authorization, plan, checks, reason |
| taskbelay_run_check | identity, check_id, rationale; retry previous_run_id/new_information; full_suite_basis for full suites |
| taskbelay_record_evidence | identity, check_id, source claim/human, status, summary, content_digest; human user_ref |
| taskbelay_get_run | run_id; read actual saved result |
| taskbelay_recover_run | identity, run_id, resolution_basis, user_ref when needed |
| taskbelay_finish_task | identity, summary, acceptance [{goal_id,evidence_ids,basis}], risks |
| taskbelay_cancel_task | identity, reason |
| taskbelay_confirm_plan | identity, plan_digest, user_ref, response; actual user confirmation |
| taskbelay_accept_known_failure | identity, check_id, failed_evidence_id, comparison_evidence_id, failures, user_ref, reason |
| taskbelay_archive_task | identity, archived; terminal tasks only |

Identity is task_id, current revision and stable operation_id. Unknown/duplicate members are rejected. Exact constraints come from published Schemas, application DTOs and domain validation. Authorization requires user_ref/request/goals; goals have id/statement/checks. Checks require id/kind/rationale/conditions; commands also need argv. Default cwd/inputs are ., timeout_ms is 60000 and retry is requires_user. Default lifetime limits: runs=20, execution_ms/core_ms=600000, host_ms=3600000, tokens=1000000.

## Host workspace and HTTP

Optional host-launch prepare uses actual existing authorization_ref, request and exact source/branch/carry choices. No assessment form or repeated plan approval is required. scope returns current repositories; Core fills observed identity. Read receipt/status to resume without redispatching uncertainty. Cleanup requires real deletion authority and the current terminal Task. Cross-workspace relocation is unsupported. Workspace preparation does not replace Core confirmation of understanding and the complete plan.

GET /api/tasks?page=1&include_archived=false&status=active (status may be omitted), /api/tasks/{id}, /api/dashboard, /api/system/status; detail ?refresh=1 observes current applicability. POST /api/core/{tool} takes csrf and arguments using the shared contract. POST /api/tasks/{id}/purge takes csrf, revision, confirm_task_id, applying only to exact terminal local records. Mutations require current Origin and CSRF session.

Retired submit_*, get_next_action, resolve_blocker, relocation and phase-artifact protocols have no compatibility entrypoints. See [responses](CORE-RESPONSES_en.md).

control_cost is cumulative local-data-directory overhead across Tasks. server_info, list_tasks and Task views return calls, wall_ms, unknown_ms and pending. unknown_ms separately labels conservative crash uncertainty; overlapping categories must not be added.

checkpoint.assessment may submit only new or affected paths, constraints and facts. Global digests must be current; Core stamps scope_digest and merges unaffected judgments. revise_task does not clear issue automatically; after an actual user decision use checkpoint.clear_issue and resolution_ref.

plan contains understanding/root_cause/approach/simpler_alternative/verification_basis/completion and work (id, goal_ids, change, owner, necessity). Views return plan_digest; confirmation binds it and the proposed content. authorization.allowed_paths is a hard allowlist. checks.purpose is acceptance/regression/diagnostic (default acceptance); diagnostics need goal_ids. full_suite identifies full suites; compares names a regression check. Default inputs:["."] covers all bound repositories; narrowing needs input_basis, with key::path for cross-repository inputs. checkpoint rejects checks. assessment.paths requires work_id and necessity; final assessment.review contains root_cause/responsibilities/proportionality/verification/would_choose_again/uncertainty, with digests generated by Core.

HTTP lists also accept lifecycle=active/blocked/done/cancelled, filtering saved Core summaries before paging. status=active includes all active tasks, including blocked ones.
