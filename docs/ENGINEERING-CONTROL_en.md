# Authorization, verification and recovery during development

[中文](ENGINEERING-CONTROL.md) | [English](ENGINEERING-CONTROL_en.md)

## Scenarios and decisions

TaskBelay supports development needing saved progress, current verification or continuation across sessions. The agent works from the user's outcome and restrictions. Small mechanical edits, ordinary questions and work without useful saved state usually need only the native Host.

The previous flow coupled user authority to a complete plan, every check and an explicit workspace choice. Routine method changes prompted again, and default budgets demanded user decisions. This design separates user decisions from agent arrangements, retaining real requests, restrictions and cumulative usage while removing repeated approvals and plan forms.

## Authority and workspace

open_task uses the real user_ref, request, outcomes and explicit restrictions. Sufficient existing authority permits work immediately; there is no plan-confirmation tool. The agent explains the approach and verification and chooses methods within the user boundary and native Host permissions. Core cannot authenticate conversation or grant extra Host permissions.

Use the specified or clearly selected current workspace, showing directory, branch and initial changes. Ask only for genuine ambiguity, conflicts or needed new workspace operations. Upstream checks and integration follow actual need and Git authority, not a mandatory startup step. Unchecked remote state remains unknown.

revise_task replaces only supplied authorization, plan or checks, with a reason. Approach, method, forecast and review-hint changes reuse the request. Changed outcomes, request or explicit restrictions, and expanded user-set caps, need the actual new user decision. An old reference cannot authorize expansion. Unaffected progress and results remain; relevant requirements, inputs or conditions invalidate affected results. Usage never resets.

## Scope and necessity

The plan contains approach and verification_basis. Goals express requested outcomes; path judgments use goal_ids and basis to connect actual changes, without another work-item table or duplicate necessity field. The delivery review uses one summary to explain cause resolution, ownership, proportionate complexity and validation, and whether the resulting structure is still appropriate; uncertainty records unresolved concerns.

expected_paths is a forecast. allowed_paths, forbidden_paths, content pins and dependency restrictions express real user boundaries, not guesses about likely edits. Structured hooks check explicit scope and pending authority questions; other writes are observed at checkpoints. TaskBelay is not a sandbox.

## Verification and cost

Checks retain actual argv, inputs, environment, purpose and rationale. The agent can adjust methods under the same outcome without user approval of every command, while obeying native permissions and retaining acceptance quality. A new method needs its own actual result. Claims never satisfy acceptance; human records require actual user observations.

Reuse passes under unchanged conditions. Relevant changes invalidate them; unrelated outcomes or new independent checks do not. Unchanged failures require new diagnostic information to retry; three unchanged failures require different code, conditions or methods. Full suites need current justification, not mechanical repetition.

Unset limits use zero and continue recording usage. Explicit positive caps are enforced; increasing or removing them needs a real decision. Insufficient budget does not lower acceptance. Actual, reserved and uncertain usage are separate; Core and Host times overlap and must not be summed.

## Interruption and delivery

Save operation identity before running, and save results before returning. After response loss read the original Task/run; receipt recovery does not execute again. Wait for live processes. If execution stopped without a result, inspect side effects and retain uncertainty instead of claiming success.

Completion requires goal conclusions, applicable required results, current diff review and no unresolved execution or decision. Existing regression failures need real comparisons and explicit user acceptance; failures remain failed, and related changed conditions invalidate the exception. Cancellation, completion and archiving retain files and grant no commit, push, merge or publication authority.

## Validation and scope

Use temporary Git repositories and isolated storage to verify immediate authorized start, method revisions without repeat approval, new references for actual authority changes, unset and explicit limits, selective result invalidation after changed requirements or external edits, response-loss/interruption recovery, and refusal to finish without required checks. Verify tool Schemas, installed Host packages and the interface together.

Cover normal paths, common failures and relevant known regressions; do not add mechanisms for speculative combinations of rare conditions. Actual Core execution, scripted user/semantic input, adapter tests and real model sessions remain distinct. Task quality, user interventions, elapsed time and usage need controlled comparisons; passing tests do not establish product benefit. Preserve unsupported databases without migration or deletion.
