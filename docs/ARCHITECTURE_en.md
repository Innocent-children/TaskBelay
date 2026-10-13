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
| internal/mcp and cmd/taskbelay | MCP/CLI projections of shared DTOs and service |
| internal/webui and packages/webui | Restricted local HTTP, results and explicit user observations |
| Host packages | Native permissions, real authorization sources, structured path hooks and optional preparation; no second task cursor |
| scripts and release | Build, complete install artifacts and standalone publication; no acceptance authority |

## Current contract and decisions

Task retains authorization, plan, approval, exceptions, archived, checks, repositories, assessment, progress, issue, evidence, attempts, facts, usage, acceptance, revision and timestamps. No fixed development nodes remain. Complete lifecycle edges are active→done (complete acceptance), active→cancelled (explicit cancellation, no unresolved execution), and active→active (checkpoint, revision, execution, observation, recovery). Terminals have no outgoing edges. continue/self_correct/verify/stop/ask_user are derived advice, not another state machine.

Authorization includes goals, non-goal/interface/data/dependency/design constraints, predicted, allowed and forbidden paths, SHA-256 pins, permitted dependency names and lifetime limits. Hosts supply actual user_ref. Programs check deterministic restrictions. Predicted paths, exported Go declarations, data files and magnitude call for semantic review bound to current content_digest. Each conclusion retains basis, verdict and uncertainty. Programs do not prove semantic testimony true.

Checks retain argv, repository_key, cwd, inputs, input_basis, environment, conditions, constraint_ids, timeout_ms, retry and rationale. CheckDigest includes check semantics, related goals and constraints, excluding timeout_ms, retry, full_suite, rationale and input_basis. PlanDigest still binds all these execution settings and explanations. Duplicate execution is refused only when the current check has an applicable passing result. Inputs fingerprint selected content/mode, missing files and directory membership; environment includes platform, selected variable hashes, cwd, conditions and the actual executable hash. Default inputs include tracked and non-ignored new files in every bound repository. Explicit inputs may include ignored files/directories. Symlink/unknown inputs cannot silently become valid. Narrowing dependency scope is a Host judgment; default to all bound repositories.

Core execution produces core_execution; claim never satisfies checks; human represents completed explicit user observations. Freshness is calculated independently from these digests. Failed observation cannot permit delivery. The tasks table is the single current state; runs retain actual attempts and operations retain request identities. Frontend/Host caches cannot rewrite results.

## Persistence and recovery

Only one current schema is supported. Existing databases are inspected read-only before writable open; incompatibility never triggers migration/deletion. A transaction CASes revision and saves operation identity, charges, runs and claims. Before a child starts, save running identity and timeout reservation. Before command activation, conditionally update runs.snapshot against the reserved snapshot with process.pid/identity, without changing Task revision. A failed identity write cannot activate the command. On macOS the launch pipe closes on Core death, preventing an unactivated child from executing. Windows resumes the suspended process only after Job assignment and identity persistence. After return, write an integrity-signed local receipt, then atomically save the result. Signatures resist accidental damage/forged submissions but do not isolate a hostile writer using the same OS account.

Read original operation_id/run_id after response loss. Receipt recovery never starts a process. Without a receipt, recovery checks both Core and the recorded command. macOS checks the whole group, including non-zombie descendants after the leader exits. Running or unobservable processes remain pending and block new execution, completion and cancellation. Only after both stop can the run become interrupted_unknown, never passed; inspect side effects and required user decisions according to retry policy. Startup identity protects against PID reuse. Windows uses hidden processes and Job lifetime; macOS uses process groups. These are platform responsibilities, not a sandbox.

Hot Tasks retain current evidence and at most 64 recent attempts. Keep operations within 256 revisions plus open/unresolved identities. Once compacted, stale revision still prevents replay. Runs retain 128 recent records plus live references and unresolved executions. Retired owned receipts are cleaned without reducing cost. Current snapshots have an 8 MiB ceiling and bounded observation/input collections. Explicit purge applies only to the exact terminal revision.

Core measures calls, wall, observation and execution time; Hosts separately report analysis, waiting and tokens. Categories overlap. Unknown time is conservatively charged rather than claimed precise. Core cannot measure bypassed Host activity. Compaction, revised requirements and check refinement never reset usage.

## Interfaces and trust

Sixteen MCP tools, CLI call and HTTP /api/core/{tool} share DTOs, Schemas and service. Unknown and duplicate JSON members are rejected; success/failure envelopes are exclusive. HTTP uses random-port loopback, exact Host, Origin and session CSRF. Desktop summaries are saved observations; current applicability requires explicit refresh. Path hooks do not cover arbitrary shell, external services or bypassed Core calls. See [commands](COMMANDS_en.md) and [responses](CORE-RESPONSES_en.md).

Every local Core call, including queries without a Task, reserves its identity before work and settles wall time afterwards. control_cost retains global calls, wall_ms, unknown_ms and pending. A crash without an exact end time retains a separately labeled conservative unknown amount, not measured time. Compaction preserves totals. Task Core wall time includes execution and waiting and cannot be added to those categories.

A newly submitted assessment binds current global digests, while Core scope_digest values preserve fragments independently. Path reasoning binds related goals and actual content; constraints bind their definition and declared paths; facts bind related content. checkpoint can submit only affected judgments. Independent additions retain reasoning and do not resolve unanswered user decisions. Evidence retains check/source/input/environment contexts: the latest failure replaces passing in the same context, while a failure in another context does not delete it. The 192-record hot bound protects all currently applicable results before retaining recent history.

## Plan approval and acceptance protection

Plans contain understanding, root cause, approach, simpler alternative, necessary work and owners, verification basis and stopping conditions. plan_digest binds authorization, plan, checks, repository origins and proposed_content_digest. Approval saves the actual user reference, response and time. Unconfirmed plans or unresolved authorization block implementation and checks; content drift before confirmation is rejected. Only revise_task changes checks, invalidating approval. Work mappings govern local necessity; DesignReview governs the overall structure.

Runs retain rationale/new_information/full_suite_basis. Repetition keys use actual argv, requirement meaning and inputs/environment; labels and timeout do not reset them. Unchanged passing contexts reject duplicate execution; failed retries cite the prior run and new diagnostic facts. Default observation covers all repositories; explicit key::path narrows dependencies. KnownFailure only applies to regression and approved comparisons, binding current plan/content and actual evidence. Lists have bounded pagination without a global 200-item cutoff; archiving only changes terminal-task visibility.
