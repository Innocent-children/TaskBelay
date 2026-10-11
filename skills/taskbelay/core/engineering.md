# Engineering work with TaskBelay

The coding agent makes technical decisions and performs authorized edits. Core maintains the single
Task, approved goals and constraints, actual repository observations, execution provenance,
evidence applicability and cumulative cost. There are no planning, implementation or test phases.

## Start and continue

Use the actual user's request and still-valid authorization. Capture goals, explicit non-goals,
interface/data/dependency/design constraints and concrete acceptance checks. Cite the actual user
message in `authorization.user_ref`; a reference is auditable Host testimony, not conversation
authentication. Do not request approval merely to enter TaskBelay or to repeat an already authorized
ordinary technical choice. Ask only for missing authority or a material unresolved tradeoff.

Inspect `taskbelay_server_info`, then use `taskbelay_open_task` in the authorized existing workspace.
Starting a branch, a worktree, another chat, a dependency installation or a destructive operation
still follows the Host's native permissions and actual user authorization. Core does not mutate Git.
For an existing Task, read `taskbelay_get_task` and return to its exact bound workspace. Do not create
a replacement task to hide cost or avoid an unresolved operation. Old databases are unsupported;
preserve them and select a separate explicit data directory rather than migrating or resetting.

Every mutable request has `task_id`, current `revision` and a fresh stable `operation_id`. Retain the
complete request and response. Successful responses contain `result`; errors contain `error` and
`recovery`. Read `ok` before using any result. Response loss requires reading the original Task/run,
not repeating side effects. Reusing identical operation content returns saved state; changed content
with the same identity is rejected. Compacted old identities with stale revisions cannot be replayed.

## Work and verification

Alternate editing, targeted checks and review whenever useful. Use returned state directly; avoid
polling or checkpointing every mechanical edit. Save meaningful progress, failed approaches in the
progress summary, unresolved user decisions and deltas of Host analysis time, waiting and tokens
with `taskbelay_checkpoint`. Core execution/call/wall/observation cost is measured separately;
Host reports can be incomplete. Time categories overlap and must not be added as independent costs.

Paths and line thresholds are review signals. Map the actual changes to authorized goals and assess
each constraint, citing code/evidence and uncertainty in `assessment`. Bind the current
`authorization_digest` and `content_digest`. Inspect `task.facts`, including exported Go declarations,
dependency changes and data-contract files. Forbidden paths, content pins, unapproved dependencies
and workspace conflicts are deterministic restrictions. A semantic verdict cannot override them.
Correct within current authority first. Only changed authority or an important choice needs the user.
Do not use a model score as proof. Semantic assessments are reasoning, not verified correctness.

Checks describe exact `argv`, relative `cwd`, rationale, conditions and input scopes. The default is
the whole Git-visible repository (`inputs:["."]`). Narrow inputs only with concrete dependency basis;
include manifests, test helpers and relevant configuration. Ignored dependencies, services and
environment variables are included only when explicitly declared. Environment values are hashed,
not exposed. Select `retry:repeatable` only for operations whose side effects are actually safe to
repeat; otherwise use `requires_user`. Core is not a sandbox or a new permission grant.

Use `taskbelay_run_check` for real local verification within native permissions. Prefer executable
argv such as `node <script>` or `go test ...`. Windows command shims may require their actual Node
entrypoint or an explicitly authorized interpreter. No shell parsing or command-count claims create
evidence. `taskbelay_record_evidence` accepts `claim` or a completed `human` observation; a claim can
never satisfy a required check. Human evidence requires a real observation reference and a human
check. Never turn a pending manual check into a completed result.

Results are bound to the command, covered goals/constraints, inputs, selected environment and
executable. Input/condition changes expire related results; unrelated changes can preserve a check
with a justified dependency scope. An unchanged check may remain valid after a linear same-content
commit or an independent requirement addition. Unknown observation is never treated as valid.

## Revise, recover and deliver

Use `taskbelay_revise_task` for an actual revised user authorization. Update only the affected
requirements/checks; unchanged progress and results remain. Routine check refinements can use
`checkpoint.checks` under existing authority. Neither operation resets lifetime cost or creates
passing evidence. When limits are reached, preserve quality, improve the approach or request a real
authorized limit change. Three failures with identical check and observed inputs require an actual
change in code, conditions or verification approach.

After an uncertain check, use `taskbelay_get_run`. `taskbelay_recover_run` imports a matching signed
receipt without rerunning. If the original owner is alive, wait or inspect its existing execution.
If it stopped without a receipt, inspect side effects and record `resolution_basis`; non-repeatable
operations also require a real `user_ref`. An unknown interruption is never passed evidence and its
reserved time remains conservatively charged. A new check requires a new operation identity.

Deliver through `taskbelay_finish_task` only when Core reports `decision.ready`, current semantic
review is complete, every goal has a completed progress record, each required check has applicable
passing evidence and no execution is unresolved. Provide a concrete conclusion and evidence IDs
for every goal. Report real risks and unexecuted checks honestly. `done` and `cancelled` retain files;
they grant no commit, push, merge, publication, cleanup or new chat authority.

Implementation: `internal/domain/control.go`, `internal/application/`, `internal/execution/`,
`internal/store/`, `internal/mcp/server.go`. [Tools and response example](protocol.md).

Submit only affected assessment paths/constraints/facts with current batch digests. Core computes scope_digest and retains unaffected reasoning; never fabricate these stamps. Independent authorization revisions retain unresolved user decisions. Clear an issue only with the actual decision reference. Evidence for different input contexts remains available, so restoring checked inputs can reuse a real pass; a newer failure in the same context cannot be hidden by an older passing record.
