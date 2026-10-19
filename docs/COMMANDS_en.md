# TaskBelay command reference

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

## Installation and local entrypoints

Public entry: npm install -g taskbelay@latest; taskbelay. Installation/status/doctor/upgrade/uninstall options remain defined by packages/taskbelay/lib/cli.mjs. Build the source contract with node scripts/taskbelay-local.mjs install --host codex --yes; select deepseek, claude or zcode as needed. This does not publish.

### Interactive menu and maintenance

A no-argument TTY uses arrows, Enter, Space for multiple selection, A for all, and Esc to go back. V changes the upgrade version, L selects a local launcher distribution, and N adds a DeepSeek Profile. Left/right do not navigate menus during text entry; Ctrl+C exits. NO_COLOR disables color, not keyboard selection. Non-TTY execution never prompts; no arguments print help.

| Command | Behavior |
| --- | --- |
| `status` | Actual plugin, Core and launcher versions, state and local paths |
| `doctor` | Read-only installation/configuration checks and relevant next steps |
| `install` | Install into selected Agents, retaining installed versions |
| `upgrade` | Update installed plugins only, defaulting to latest stable; `--self` includes or exclusively updates the launcher |
| `repair` / `reinstall` | Repair registration or replace current package files, retaining user data |
| `uninstall` | Remove selected plugins and registrations, retaining data |
| `delete-data` | Clear tasks, configuration and pet preferences; keep plugins, registrations and installation records; recreate empty directories in place |
| `delete-all` | Remove all managed plugins and confirmed data, retaining the launcher and desktop application |

Repeat `--host codex|deepseek|claude|zcode` for an Agent subset; `--host all` cannot be mixed with individual Agents. Repeat `--profile` or use `--all-known-profiles` for managed DeepSeek Profiles. Shared data deletion implies all Agents and Profiles and cannot target one Agent or Profile's shared data.

```sh
taskbelay install --host codex --host claude --yes
taskbelay upgrade --host all --all-known-profiles --self --yes
taskbelay upgrade --self --yes
taskbelay delete-data --json
taskbelay delete-all --json
```

`--version latest|<x.y.z>` applies to install, upgrade, repair and reinstall. Downgrades additionally require `--confirm-downgrade <token>`. Self update requires the current global npm installation and retains its channel. Local development installations use `--self --self-package <absolute-complete-local-distribution.tgz>`; all four contained plugins and their digests are verified before installation. They never silently switch to npm. A temporary source manager cannot update another global installation. `--yes` covers ordinary maintenance, not data deletion, permanent deletion or downgrade.

Data deletion first returns exact directories and a plan-bound `--confirm-delete <token>`. Permanent removal additionally requires `--permanent --confirm-permanent <token>`. Approve extra directories separately with `--confirm-explicit-data <absolute-path>`. When ZCode is installed, close all its windows and sessions before supplying `--confirm-zcode-closed`; this records an actual user action, while identifiable Core processes are still checked. delete-all retains data and returns pending UI steps while any plugin remains installed.

Cleanup stops the pet and associated Core processes, reads a compatible Core's saved-state API to rule out running or uncertain Task executions, then cleans exact directories. A database without a usable compatible Core blocks cleanup rather than guessing its historical contents. The current deletion result and unresolved maintenance records are retained; ordinary completed maintenance records are removed. Recoverable deletion uses macOS Trash or the Windows TaskBelay recovery directory, not Windows Recycle Bin. Earlier recovery directories, project code and Agent conversation history are retained. The old factory-reset, --confirm-reset and automatic reinstall-after-deletion options have been removed.

Core: taskbelay version; config validate (stdin); mcp --stdio; call <tool_name> (complete JSON stdin); host-check pre-file-write/workspace-available; webui start/open/status/stop (--json or --plain, start optionally --no-open). Set absolute TASKBELAY_DATA_DIR. Preserve old data and choose a new directory; no migration occurs. The unified npm launcher forwards call/webui to configured Core. Codex/Claude/ZCode taskbelay-<host> call <tool_name> forwards to bundled Core. DeepSeek uses the discovered MCP namespace.

pre-file-write takes host, repository_path, tool_name, paths, intent_digest, path_parse_complete and returns allow/deny with actual coverage, not future edit semantics. workspace-available takes repository_path or repositories [{key,repository_path}], returning available and repositories [{key,repository_path,active_task_id}] read-only. Observation failure never means no Task. Phase artifact commands have been removed.

`webui start/open/status/stop --json` returns top-level `readiness`, `core_identity`, `data_root_digest`, and `url`. `core_identity` is the Core version without a product-name prefix. `pid` is omitted when no service process is recorded; when present, it is a non-negative integer. The response has no `operation` member because the caller already knows the command it issued. After reading these fields, the desktop launcher must still query `/api/system/status` and match the version identity, data directory digest, and loopback address against the live service.

## MCP tools

| Tool | Current input |
| --- | --- |
| taskbelay_server_info | Optional contracts array; actual capabilities and selected method schemas/examples |
| taskbelay_list_tasks | Optional page, page_size (1..100), include_archived, status (active/done/cancelled); returns page and has_next |
| taskbelay_open_task | operation_id, host, repositories, authorization, plan |
| taskbelay_get_task | task_id, refresh (default true; false reads saved state), detail (default false) |
| taskbelay_checkpoint | identity; optional assessment/progress/issue and host_usage {source_id,observed_through,totals,coverage} |
| taskbelay_revise_task | identity, reason; authorization/plan optional |
| taskbelay_resolve_issue | identity, issue_digest, user_ref, decision; records an answer without changing authorization |
| taskbelay_record_evidence | identity, check_id, source claim/human, status, summary, content_digest; human user_ref |
| taskbelay_get_run | run_id; read actual saved result |
| taskbelay_recover_run | identity, run_id, resolution_basis, user_ref when needed |
| taskbelay_finish_task | identity, summary, acceptance [{goal_id,evidence_ids,basis}] |
| taskbelay_cancel_task | identity, reason |
| taskbelay_accept_known_failure | identity, check_id, failed_evidence_id, comparison_evidence_id, failures, user_ref, reason |
| taskbelay_update_budget | identity, authorization_digest, budget {runs,execution_ms,work_ms,tokens}, user_ref, response; actual complete approved budget decision |
| taskbelay_archive_task | identity, archived; terminal tasks only |

Identity is task_id, current revision and stable operation_id. Unknown/duplicate fields are rejected. authorization includes actual user_ref/request and goals {id,statement}. Plan owns checks with required goal_ids and id/kind/rationale/conditions and argv for commands. Defaults: cwd/inputs ., timeout_ms 60000, retry requires_user, purpose acceptance. Omitted limit fields are zero (unset); positive fields cap cumulative usage.

view.budget shows limited, actual, reserved, uncertain, remaining and check timeouts. When limited=false, remaining is not an available allowance. Explicit insufficient caps reject execution; aggregate timeout estimates only warn. update_budget uses the current authorization_digest and actual budget decision; work_ms updates both Core and reported-work caps without resetting usage.

## Host workspace and HTTP

Codex/Claude/ZCode expose taskbelay-<host> host-launch freshness; DeepSeek uses workspace_coordinator operation freshness and freshness_repositories. Bound repository items are {key,repository_path,source_type,remote_name,base_branch}, copying the last three fields from the original workspace_origin. fetch defaults true and integrate defaults false. Fetch requires authorization_ref; integration additionally requires integration_authorization_ref and a stable operation_id.

Copy remote_freshness into open_task.repositories; retain later observations via checkpoint.repository_freshness [{key,observation}]. Integration saves intent before Git mutation and returns operation.receipt_path, status and per-root integration results. The same identity reads its receipt without execution. recover:true with the original request only observes actual Git after the original owner stops; it never repeats fetch/merge or rolls back applied work. partial/uncertain/reconciled do not mean whole-batch success; unknown remains unknown. Recovery locks retain PID, a random token and file identity. Reclamation requires a stopped owner, an exclusive reclaim claim and unchanged identity. Live/reused PIDs, denied liveness queries and incomplete identity are conservatively refused; locks are never deleted by age. An uncertain reclaim claim is retained for inspection. Inspect original effects before a fresh separately authorized assessment of remaining work. No upstream, skipped or failed fetch never means latest.

Optional host-launch prepare uses actual existing authorization_ref, request and exact source/branch/carry choices. No assessment form or repeated plan approval is required. scope returns current repositories; Core fills observed identity. Read receipt/status to resume without redispatching uncertainty. Cleanup requires real deletion authority and the current terminal Task. Cross-workspace relocation is unsupported. Workspace preparation grants no additional Task authority; a sufficient existing request needs no separate plan approval.

GET /api/tasks?page=1&include_archived=false&status=active (status may be omitted), /api/tasks/{id}, /api/dashboard, /api/system/status; detail ?refresh=1 observes current applicability. POST /api/core/{tool} takes csrf and arguments using the shared contract. POST /api/tasks/{id}/purge takes csrf, revision, confirm_task_id, applying only to exact terminal local records. Mutations require current Origin and CSRF session.

Retired submit_*, get_next_action, resolve_blocker, relocation and phase-artifact protocols have no compatibility entrypoints. See [responses](CORE-RESPONSES_en.md).

control_cost records global calls, wall_ms, unknown_ms and pending in server_info and list_tasks. Mutations return identity, digests, decision.gaps, progress, checks, concerns, limitations and Task usage/budget. open/get additionally return context.authorization/plan/repositories. get_task detail:true adds explicit details: facts, evidence_applicability, repository bindings, assessment, evidence, attempts, changes and terminal records. check run/recover_run return view and a run summary; get_run reads output.stdout/output.stderr and execution conditions.

checkpoint.assessment may submit only new or affected paths, constraints and facts. Global digests must be current; Core stamps scope_digest and merges unaffected judgments. revise_task does not clear issue automatically; after an actual user decision use resolve_issue with issue_digest, user_ref and decision.

plan contains approach, verification_basis, checks, expected_paths and review_after_lines. revise_task replaces only supplied fields. Actual changed user outcomes/restrictions or expanded existing caps require a new real user_ref; routine methods retain the request. plan.checks.purpose is acceptance/regression/diagnostic (default acceptance); all checks need goal_ids, full_suite needs execution justification, and compares names a regression check. Default inputs ["."] cover all repositories; narrow scopes need input_basis and use key::path for cross-repository inputs. checkpoint cannot change checks. assessment.paths uses path/goal_ids/basis; final assessment.review uses summary. Core generates applicability stamps.

HTTP lists also accept lifecycle=active/blocked/done/cancelled, filtering saved Core summaries before paging. status=active includes all active tasks, including blocked ones.

The run record’s process contains the command pid and startup identity. It is null before launch registration; activation requires the identity to be persisted. Missing-receipt recovery requires both Core and its managed command to stop; macOS checks the whole process group.

Only assess upstream when needed; for a requested new workspace, assess before freezing its source. Once bound, always use the original source_type/remote_name/base_branch: a local source resolves its original branch configuration; a remote source resolves the selected remote-tracking branch without a local branch. A task branch gaining its own upstream does not change the source. Never silently substitute main.

## Native execution and saved-state reads

Invoke taskbelay check run through the Host native process tool, with a complete RunRequest JSON on stdin: task_id, revision, operation_id, check_id, rationale; retries also supply previous_run_id/new_information, and full suites require full_suite_basis. The envelope tool is check.run. Codex/Claude/ZCode launchers also support check run. DeepSeek activation adds its exact bundled Core path and TASKBELAY_DATA_DIR to Skill context. The unified launcher forwards check run to its selected Core. MCP/HTTP cannot start commands; server_info.native_check points to check.run; select that contract for its complete request/result Schema. Keep native process sessions for long commands and inspect pending_runs/get_run rather than restarting after response loss.

get_task refresh:false, list_tasks, get_run and server_info read saved state without writing metering data or querying process identity. Mutations remain metered and fail before execution if metering is unavailable. Lists accept search (request or task ID) and host, filtering before paging. checkpoint.progress.next_step preserves the next action; details.changes retains at most 64 recent decisions and adjustment reasons. Check kind, purpose and retry enums are published from the same DTO. Each goal requires an acceptance check. Only regression checks can receive exceptions; finish acceptance references only passed acceptance results.

## Self-contained method help

Use `taskbelay call <tool> --help --json`, `taskbelay check run --help --json` or the configured native Core command. Codex/Claude/ZCode provide `host-launch <operation> --help --json`, `hook pre-tool-use --help --json` and `status|setup|remove --help --json`. The unified launcher provides `<lifecycle-operation> --help --json` and `pet start|stop --help --json`. Native Core also provides `config validate`, `webui <operation>`, `help`, `version` and `mcp` method help. These queries read packaged descriptions before opening data or executing the operation.

`server_info.auxiliary_contracts` lists native/HTTP contract names; request them through `contracts` without creating new MCP tools. Each package includes `contracts/interfaces.json`; DeepSeek's six workspace_coordinator branches use that installed reference. Full input/output fields, nullable/default semantics, actual success/error examples, required authority and recovery behavior are available without source or database access. See [interface reference](INTERFACES_en.md).

New task branches use `--no-track`; inspect the returned actual upstream and push_configuration. Original source selection remains separate. IDE push target is explicitly unknown. workspace-available returns a shared Core envelope with every selected root's active_task_id (null when unclaimed), never just the first conflict.
