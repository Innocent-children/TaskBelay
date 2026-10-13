<!-- Generated from skills/taskbelay/core/engineering.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Development with TaskBelay

Use TaskBelay when saved progress, current verification or interruption recovery helps the actual
request. Small mechanical edits and ordinary questions usually need only the native agent. Honor an
explicit invocation or opt-out. The agent understands and edits; Core records one Task and checks
actual scope, result applicability, cost and completion.

## Start from existing authority

Read the request and repository rules, locate the cause and the responsible code, and explain a
proportionate approach. Reuse the user's actual authorization. Open the Task in the selected/current
workspace with the real user_ref, requested outcomes, explicit restrictions, checks and a short plan
(approach and verification_basis). No separate plan confirmation is required. Do not invent limits
or turn predicted files into a user allowlist. Unset cost limits remain unset; actual usage accumulates.

Show the directory, branch and initial changes. Ask only when the workspace is unclear or a real
conflict needs a choice. Read [workspace](workspace.md) only for requested branch/worktree preparation,
upstream integration or cleanup. These are optional operations with their own actual authority.

## Continue and adjust

Implement and run relevant checks in any useful order within the request and native Host permissions.
Record progress at meaningful points: a completed piece of work, a relevant scope change, an actual
verification result, a decision that needs the user, or a handoff. Do not checkpoint every edit or poll
unchanged state. Review real changes against goals and constraints, correcting within scope first.

Use revise_task with only the fields that need replacing. Routine approach, forecast and check changes
reuse the original request. Changed user outcomes or explicit restrictions, and increased/removed
user-set limits, need the actual new user decision. Never invent its user_ref. Changing the verification
method must still establish the same requested outcome; replacing a failing assertion with a passing
placeholder is not verification. Core checks applicability, not the truth of the agent's reasoning.

Preserve unaffected progress and passing results. Explicit allowed_paths, forbidden_paths, pins and
manifest dependency restrictions still apply. Dependencies without an explicit policy need a reasoned
necessity/compatibility review, not automatic permission escalation. Native installation permissions
remain in force. Structured hooks cover supported paths; shell and external edits are observed at
checkpoints. TaskBelay is not a sandbox.

## Verify the remaining gap

Each check names its purpose, exact argv, actual conditions and rationale. Default inputs ["."] cover
Git-visible inputs across all bound repositories. Narrow inputs only with a concrete input_basis that
includes relevant helpers, configuration and dependencies; key::path selects another repository.
Undeclared external services are not verified. Use repeatable only for checks whose uncertain effects
can safely be inspected and repeated; otherwise retain requires_user.

Run only necessary checks and reuse applicable passing results. Read get_run for output and failure
diagnostics. An unchanged failure retry needs previous_run_id and new_information; after three
unchanged failures, change the implementation, conditions or method. A full suite needs a current
full_suite_basis. Extra budget is not a reason for extra testing, and exhausted budget never waives
acceptance. Set or change explicit cost limits only from the actual user decision.

## Resume and deliver

Keep operation_id and returned run_id. After a lost response, read the saved Task/run before taking
another action. Recover signed results without re-execution. Wait while an original process is alive;
when it has stopped without a result, inspect effects and record what remains unknown. Obtain a real
user decision before repeating a non-repeatable operation. Do not replace a Task to erase costs or
unresolved work. Preserve unsupported historical data.

Before delivery, review the actual diff. assessment.paths ties each changed path to goal_ids and a
concrete basis. assessment.review.summary explains whether the cause is resolved, responsibility is
correct, complexity and validation are proportionate, and the resulting structure is still the one
you would choose; uncertainty names unresolved concerns. Do not fill separate ceremonial work tables.

Finish only when decision.ready. Report actual changes, passed/failed/unrun checks, accepted existing
regression failures and remaining limits. Claims never prove execution; human records require actual
completed user observations. Existing regression failures require real comparison results and explicit
user acceptance, and remain failures in delivery. Stop at the requested outcome. Completion grants no
Git, publication or deletion authority.

Tool fields and recovery: [protocol](protocol.md). Normal use should not require reading plugin source.

Maintainer implementation reference: `internal/application/service.go`.
