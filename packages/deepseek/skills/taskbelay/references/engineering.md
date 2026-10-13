<!-- Generated from skills/taskbelay/core/engineering.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Engineering work with TaskBelay

The coding agent makes technical decisions and authorized edits. Core owns one Task, the displayed
plan and its approval, actual scope, evidence, cumulative cost, recovery and acceptance.

## Understand, propose and confirm before implementation

Read the actual request and repository rules. Investigate the cause and responsibility before
choosing a solution. Explain the outcome, non-goals, root cause, approach, simpler alternative,
necessary work and owners, allowed scope, verification and cost limits, and completion conditions.
A large necessary repair is not automatically overdesign; a small unrelated change is not justified
by its size. Do not add frameworks, speculative compatibility or unrelated fixes. Report independent
problems without editing them. Initial investigation uses already authorized read-only Host tools.

Before edits, assess every repository with the Host freshness operation. Bound callers copy workspace_origin.source_type, remote_name and base_branch into every repository item. Local sources use the original source branch configuration even when the task branch gains an upstream; remote sources use refs/remotes/<remote_name>/<base_branch> without requiring a local branch. Before binding, an omitted selection means the observed current local branch. Fetch within actual Git authority and record no upstream, skipped/failed fetch, dirty work and divergence explicitly. Fetch and integration are distinct. Integrate only separately authorized clean fast-forwards; supply a stable operation_id and retain operation.receipt_path and every repository result. Durable intent precedes fetch/merge. An identical identity reads the retained receipt; never repeat it after response loss. recover:true uses the original complete request and only reconciles current Git facts after the recorded owner stops. Reconciled does not mean the original command passed: unknown remains unknown, remaining roots are not executed, and applied roots are never rolled back. Inspect unresolved command/hook effects before any fresh separately authorized assessment of remaining work. Copy remote_freshness into open_task and later observations into checkpoint.repository_freshness; retain the initial baseline. Recheck before committing during a long task. Core never fetches or integrates Git.

Use taskbelay_open_task to save authorization, plan and checks. Show that complete proposal to the
user and obtain an explicit answer approving both understanding and plan. Then call
taskbelay_confirm_plan with the returned plan_digest, actual user_ref and response. Never infer
approval of an unseen plan or invent a user answer. Reuse an earlier answer only when it explicitly
approved this exact unchanged proposal. Core cannot authenticate Host testimony. The local WebUI
can display the complete plan and accept a direct user confirmation.

Review view.budget together with the plan: actual, reserved, uncertain, remaining and next-check timeouts. Required unreservable checks and insufficient run counts need correction; aggregate timeout plus retry estimates are risks, not hard failures. Choose proportionate timeouts and bounded retry headroom, not uniformly huge defaults. Record a concrete complete budget decision with taskbelay_update_budget; work_ms updates both overlapping Core/Host caps. Check all remaining limits and preserve acceptance.

Before confirmation, do not implement, run checks or submit implementation progress. If contents
changed after proposal, restore only your authorized changes or disclose the difference in a revised
plan. Do not turn retroactive approval into a claim that work was authorized before it happened.

Before every new Task, ask the user to choose its workspace: current branch, new local branch, or
dedicated worktree. Show current directories, branches and initial changes. Do not infer a default
or carry a choice from another Task. Resume an existing Task in its saved workspace. Branch/worktree preparation, new chats, installation,
Git operations, publication and deletion follow actual user authority and native permissions. Resume
an existing Task in its original workspace; do not create a replacement to hide cost or uncertainty.
No historical-data readers, migration or reset are provided; preserve unsupported data.

## Work within the approved plan

Confirmation binds authorization, plan, checks, limits, repository origins and proposed contents.
Implementation and targeted verification can alternate after confirmation. A material change uses
revise_task and a freshly displayed/confirmed plan. Unchanged ordinary progress needs no new approval.
checkpoint cannot replace checks. Changes to acceptance or verification coverage must not be disguised
as harmless metadata or progress. Existing checks may not be replaced with a command that always passes.

Predicted paths do not grant permission. Explicit allowed_paths, forbidden_paths, pins and dependency
policies are deterministic restrictions. Show actual per-repository manifest policies before confirmation; use package names (lucide-react, not dependencies:lucide-react) for npm grants. Conditional plan permission must be represented in the same actual authorization, not left only in prose. Dependency permission grants no separate file-write or install authority. The structured write hook checks supported operations before
writes. Other editors, shell commands and external effects are observed at checkpoints, not intercepted.
Respect the current Host's actual coverage; this is not a sandbox or an additional permission grant.

Map each real changed path to its approved work_id and goal_ids, with concrete basis and necessity.
Review observed interfaces, dependencies, data contracts and unexpected paths. Correct within current
authority first. Save an authorization issue only for a real unresolved decision; clear it only with
its actual response. Semantic conclusions cannot override deterministic restrictions.

Before delivery submit assessment.review: root_cause, responsibilities, proportionality, verification,
would_choose_again and uncertainty. Read the actual diff and current needs, not just the agent's summary.
Explain whether the original cause is resolved and whether this structure would be chosen again.
Core binds review digests; it cannot prove the reasoning true. Do not submit a score as proof.

## Verify only an actual gap

Each check describes exact argv, cwd, purpose, rationale, conditions and inputs. acceptance checks
establish the requested result; regression checks detect unintended changes; diagnostic checks need
a declared goal_ids relationship. A full_suite check requires full_suite_basis on every execution:
explain why targeted verification is insufficient now. Extra budget never justifies extra tests.
Test-code changes also need a necessity explanation; do not mirror implementation or add unrelated cases.

Default inputs:["."] covers Git-visible inputs in every bound repository. Narrowing requires concrete
input_basis and must include helpers, manifests, configuration and cross-repository dependencies.
Use key::path to select another repository. Explicit scopes can include ignored files. Undeclared
services or environment are not verified. Selected environment values are hashed, not exposed.

Use run_check with a concrete rationale. The Core executes within native Host permissions and records
real process results. Reuse passing evidence under unchanged actual conditions. Retrying an unchanged
failure requires previous_run_id and distinct new_information explaining what was learned. Three
failures under the same method/requirements/conditions stop repetition. Renaming checks or changing
prose/timeouts cannot reset the failure count. Execution settings and rationale/input_basis text do not invalidate unchanged passing results. Timeout reductions/restoration and prose synchronization within recorded approval ceilings reuse the existing actual decision. Retry/full-suite policy, argv/input/permission changes and larger ceilings require an actual new decision; configuration synchronization never expands authority. Duplicate execution is refused only when this check has an applicable passing result; a
renamed or substantively changed check can establish its own result. A meaningful input or method change can justify another run.

Claims, completed human observations and Core executions are separate. record_evidence accepts only
claim or human; claims never satisfy checks. Human evidence needs a human check and actual user_ref.
A pending human check is not a completed result. Cumulative cost survives revision and compaction.
Core measures execution and control; reported Host time/tokens may be incomplete. Overlapping times
must not be added. Limits cannot lower acceptance quality.

## Existing failures, recovery and delivery

An explicitly approved regression check may have a known-failure exception. Run the approved comparison
whose compares names that check. It must actually establish no new/changed failures; a zero exit alone
is not a semantic proof. Supply current failed/comparison evidence, concrete failure names, reason
and the actual user's acceptance via accept_known_failure. Acceptance checks cannot be waived.
Exceptions expire with changed plan/content, and failed evidence remains failed in the handoff.

Retain complete tool requests/responses. Check ok before result. Every mutation uses current revision
and stable operation_id; identical replay reads saved results. After response loss read the Task/run.
recover_run imports the signed result without execution. A live original owner means wait for it;
a stopped Core with a live recorded command/process group remains pending. Process identity is saved
before command activation; a missing receipt cannot be reconciled until both Core and the command have stopped.
Then inspect effects and record resolution_basis, plus a
real user_ref for non-repeatable checks. Unknown interruption never becomes a pass. Never blindly replay.

Finish only when decision.ready: current confirmation and structure review, completed goals, applicable
passing evidence or valid regression exceptions, and no unresolved execution or user decision. Report
actual results, exceptions, remaining limits and unexecuted work. Stop when the approved outcome is
satisfied; do not add polish or broader tests without a new reason. Done/cancelled keeps files and
grants no Git or publication authority. Terminal tasks may be archived/restored without changing outcome.

Implementation: `internal/domain/plan.go`, `internal/domain/control.go`,
`internal/application/plan.go`, `internal/application/service.go`,
`internal/execution/fingerprint.go` and `internal/mcp/server.go`. See [protocol](protocol.md).
