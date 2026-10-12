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

Use taskbelay_open_task to save authorization, plan and checks. Show that complete proposal to the
user and obtain an explicit answer approving both understanding and plan. Then call
taskbelay_confirm_plan with the returned plan_digest, actual user_ref and response. Never infer
approval of an unseen plan or invent a user answer. Reuse an earlier answer only when it explicitly
approved this exact unchanged proposal. Core cannot authenticate Host testimony. The local WebUI
can display the complete plan and accept a direct user confirmation.

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
policies are deterministic restrictions. The structured write hook checks supported operations before
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
prose/timeouts cannot reset the failure count. Execution settings (timeout, retry, full-suite flag) and
rationale/input_basis text remain bound by plan approval but do not invalidate unchanged passing
results. Duplicate execution is refused only when this check has an applicable passing result; a
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
