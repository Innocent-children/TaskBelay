# TaskBelay architecture

[中文](ARCHITECTURE.md) | [English](ARCHITECTURE_en.md)

## Component ownership

| Component | Responsibility |
| --- | --- |
| internal/domain | Authorization, goals, constraints, applicability relationships, lifecycle and pure decisions |
| internal/application | Compose observation, semantic submissions, real execution and atomic persistence; no OS choices |
| internal/repository | Retained read-only Git observer, workspace identity/history, index/worktree content and within-file facts |
| internal/execution | Input/environment/executable fingerprints, real processes, output, platform lifetime and receipts |
| internal/store | Current SQLite schema, CAS, claims, operation identity, cost and compaction |
| internal/protocol | Shared DTOs, Schemas, results and errors without task state |
| internal/mcp and cmd/taskbelay | MCP/CLI projections of shared DTOs and service |
| internal/webui and packages/webui | Restricted local HTTP, results and explicit user observations |
| Host packages | Native permissions, real authorization sources, structured path hooks and optional preparation; no second task cursor |
| scripts and release | Build, complete install artifacts and standalone publication; no acceptance authority |

Host workspace operations use private atomic receipts for frozen requests, bound worktrees and per-root integration results; they are not another Task. Intent precedes mutation, response loss reads the original identity, and read-only recovery observes current Git while retaining unknown command outcomes without rollback or continuation. Core still only observes Git and verifies immutable source selection. Control metering retains original errors by storage, process-query or other initialization responsibility; query denial is not proof of process death.

In packages/taskbelay, terminal.mjs owns keys and input mode; menu.mjs retains only unfinished user choices; upgrade-menu.mjs separates current, available and failed updates and scopes explicit versions to one component; cli.mjs normalizes commands and menu choices into one request. plan.mjs builds one confirmed multi-Agent maintenance plan, lifecycle.mjs coordinates effects, launcher.mjs updates the entrypoint without changing its channel, and cleanup.mjs enumerates user data and prepares empty directories. Registration records, desktop executables and cleanup targets remain distinct. Journals retain the current result and unresolved records. Platform implementations own process shutdown; uncertain executions are inspected through Core's read-only API. The manager neither reads SQLite directly nor keeps a second Task.

macOS WebUI receipt_darwin.go reads process start time with LC_ALL=C so start, status, reuse and stop use the same identity across terminal languages. Desktop platform implementations verify pet runtime status; the manager’s pet-files.mjs compares application content, so a current plugin does not cause an unchanged desktop copy.

## Current contract and decisions

Task retains authorization, plan, budget_decision, exceptions, archived, changes, repositories, assessment, progress, issue, evidence, attempts, facts, usage, acceptance, revision and timestamps. No fixed development nodes remain. Complete lifecycle edges are active→done (complete acceptance), active→cancelled (explicit cancellation, no unresolved execution), and active→active (checkpoint, revision, execution, observation, recovery). Terminals have no outgoing edges. continue/review/self_correct/verify/stop/ask_user are derived advice, not another state machine.

Authorization includes goals, non-goal/interface/data/dependency/design constraints, allowed and forbidden paths, SHA-256 pins, permitted dependency names and lifetime limits. Hosts supply actual user_ref. Programs check deterministic restrictions. Predicted paths, exported Go declarations, data files and magnitude call for semantic review bound to current content_digest. Each conclusion retains basis, verdict and uncertainty. Programs do not prove semantic testimony true.

Checks retain argv, repository_key, cwd, inputs, input_basis, environment, conditions, constraint_ids, timeout_ms, retry and rationale. CheckDigest includes check semantics, related goals and constraints, excluding timeout_ms, retry, full_suite, rationale and input_basis. PlanDigest still binds all these execution settings and explanations. Duplicate execution is refused only when the current check has an applicable passing result. Inputs fingerprint selected content/mode, missing files and directory membership; environment includes platform, selected variable hashes, cwd, conditions and the actual executable hash. Default inputs include tracked and non-ignored new files in every bound repository. Explicit inputs may include ignored files/directories. Symlink/unknown inputs cannot silently become valid. Narrowing dependency scope is a Host judgment; default to all bound repositories.

Core execution produces core_execution; claim never satisfies checks; human represents completed explicit user observations. Freshness is calculated independently from these digests. Failed observation cannot permit delivery. The tasks table is the single current state; runs retain actual attempts and operations retain request identities. Frontend/Host caches cannot rewrite results.

## Persistence and recovery

Only one current schema is supported. Existing databases are inspected read-only before writable open; incompatibility never triggers migration/deletion. A transaction CASes revision and saves operation identity, charges, runs and claims. Before a child starts, save running identity and timeout reservation. Before command activation, conditionally update runs.snapshot against the reserved snapshot with process.pid/identity, without changing Task revision. A failed identity write cannot activate the command. On macOS the launch pipe closes on Core death, preventing an unactivated child from executing. Windows resumes the suspended process only after Job assignment and identity persistence. After return, write an integrity-signed local receipt, then atomically save the result. Signatures resist accidental damage/forged submissions but do not isolate a hostile writer using the same OS account.

Read original operation_id/run_id after response loss. Receipt recovery never starts a process. Without a receipt, recovery checks both Core and the recorded command. macOS checks the whole group, including non-zombie descendants after the leader exits. Running or unobservable processes remain pending and block new execution, completion and cancellation. Only after both stop can the run become interrupted_unknown, never passed; inspect side effects and required user decisions according to retry policy. Startup identity protects against PID reuse. Windows uses hidden processes and Job lifetime; macOS uses process groups. These are platform responsibilities, not a sandbox.

Hot Tasks retain current evidence and at most 64 recent attempts. Keep operations within 256 revisions plus open/unresolved identities. Once compacted, stale revision still prevents replay. Runs retain 128 recent records plus live references and unresolved executions. Retired owned receipts are cleaned without reducing cost. Current snapshots have an 8 MiB ceiling and bounded observation/input collections. Explicit purge applies only to the exact terminal revision.

Core measures calls, wall, observation and execution time; Hosts separately report analysis, waiting and tokens. Categories overlap. Unknown time is conservatively charged rather than claimed precise. Core cannot measure bypassed Host activity. Compaction, revised requirements and check refinement never reset usage.

## Interfaces and trust

Fifteen MCP tools, CLI call and HTTP /api/core/{tool} share DTOs, Schemas and service. Unknown and duplicate JSON members are rejected; success/failure envelopes are exclusive. HTTP uses random-port loopback, exact Host, Origin and session CSRF. Desktop summaries are saved observations; current applicability requires explicit refresh. Path hooks do not cover arbitrary shell, external services or bypassed Core calls. See [commands](COMMANDS_en.md) and [responses](CORE-RESPONSES_en.md).

Mutating calls reserve identity before work and settle wall time afterwards. Saved-state queries do not write metering data or query process identity; Service.SavedControlCost reads stored totals and pending records only. control_cost retains global calls, wall_ms, unknown_ms and pending. A crash without an exact end time retains a separately labeled conservative unknown amount, not measured time. Compaction preserves totals. Task Core wall time includes execution and waiting and cannot be added to those categories.

A newly submitted assessment binds current global digests, while Core scope_digest values preserve fragments independently. Path reasoning binds related goals and actual content; constraints bind their definition and declared paths; facts bind related content. checkpoint can submit only affected judgments. Independent additions retain reasoning and do not resolve unanswered user decisions. Evidence retains check/source/input/environment contexts: the latest failure replaces passing in the same context, while a failure in another context does not delete it. The 192-record hot bound protects all currently applicable results before retaining recent history.

## User boundaries and acceptance

The actual request, outcomes and explicit restrictions are separate from the plan's approach and verification_basis. Authorization.BoundaryDigest excludes only references and separately compared cost fields. Check mappings, forecasts and review hints now belong to Plan. Revise requires an actual new user_ref when that boundary changes or a positive user cap expands/disappears. Routine method changes retain authority; Host permissions still apply to every operation. No PlanApproval or confirmation tool remains. PlanDigest binds the current arrangement for technical review, not user approval.

Goal.RequirementDigest omits the check mapping, so adding a check does not reset completed implementation or independent results. CheckDigest still binds the particular method and its related outcomes/constraints. Path judgments bind goals and changed content. The final DesignReview contains summary, stamped with current plan/content. Engineering questions live in assessment.concerns with immutable IDs and original definitions. verified cites applicable actual results; not_blocking derives delivery limitations; scope_changed references a saved real user-boundary change. Relevant changes invalidate resolutions; omission never deletes the original question. Known failures rely on their actual failed/comparison results and current applicability, not unrelated plan prose.

Unset limits are zero; positive limits are enforced during reservation and execution. BudgetDecision separately records actual budget changes. Programs track all usage even without caps. No default budget creates user approval work.

internal/protocol/view.go projects the application view into current task summaries; the application and storage retain the complete Task. get_task detail:true adds details with full repository fingerprints, assessment and history. run/recover return result summaries; get_run returns saved output. This projection is not a second state store. Lists retain bounded pagination and archiving changes only terminal visibility.

## Implementation plan, native execution and acceptance

Plan owns approach, verification_basis, expected_paths, review_after_lines and checks. Check.GoalIDs is the only association to outcomes; every goal needs an acceptance check. Acceptance and regression checks must be resolved; diagnostics are not delivery requirements. Decide can recognize regression exceptions, but Finish accepts only actual passed acceptance checks as outcome proof. changes retains at most 64 recent authorization, plan, budget, user-answer and failure-exception reasons, not another workflow or full snapshot history. Progress.NextStep preserves the next useful action.

internal/protocol.Handler owns shared decoding, summaries and errors. MCP/HTTP dispatch only Catalog; NativeCheck reaches Service.Execute solely through CLI check run. Native Host tools own permission, environment and waiting; Core execution retains actual inputs, process identity and result receipts. Long checks retain native sessions rather than extending MCP requests or introducing a scheduler. Service.ResolveIssue binds issue_digest to refuse stale answers; recording a decision does not expand authorization.

Completion retains only applicable regression exceptions used for that delivery. Terminal reads return these recorded exceptions without losing accepted failures or claiming a fresh workspace verification.

## Response and usage ownership

internal/protocol exposes TaskState for mutations, context on open/get and explicit details only on demand. Task/run identities and Core-generated digests are separate from writable assessment inputs. Decision.gaps is the sole unmet-condition list, reused in finish errors. MCP retains canonical structuredContent; ZCode opts into concise text, while text-rendering Hosts receive the full envelope.

Store saves host_usage_sources by (task_id, source_id). A cumulative sample, its high-water delta, Task revision, operation identity and counters commit in one CAS transaction; retries under different operation IDs still cannot add a sample twice. Unknown metrics remain null. reported covers registered source samples, not unobserved Host activity. Explicit Host caps with missing coverage stop new command reservations while saved reads and original-run recovery remain available. No Host integration fabricates token measurements.

Shared workspace helpers create branches without implicit upstream tracking and return Git observations independently from IDE behavior. Packaged interface references are read-only descriptions; they contain no Task cursor, permission engine or workflow.
