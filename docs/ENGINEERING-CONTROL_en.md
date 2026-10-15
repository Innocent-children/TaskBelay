# Authorization, acceptance and recovery during development

[中文](ENGINEERING-CONTROL.md) | [English](ENGINEERING-CONTROL_en.md)

## Problem addressed

Long tasks must continue from actual results without repeat approval for ordinary method changes or rerunning independent completed checks. Accepting existing regressions cannot replace acceptance of the requested outcome, and response loss cannot replay uncertain commands. Core retains one Task; the agent owns requirement understanding and technical judgment. There are no fixed development nodes or general workflow.

## Request and implementation arrangements

Authorization stores the actual user_ref, request, goals, explicit restrictions and user-set limits. Plan owns approach, verification_basis, checks, expected_paths and review_after_lines. Forecasts grant no permission; Core checks allowed/forbidden paths, content pins and dependency restrictions.

revise_task replaces supplied authorization or plan objects. Methods, checks, forecasts and review hints can change within the request; changed goals/restrictions and expansion/removal of existing positive limits need the actual new decision. Unaffected progress/results remain, usage never resets, and revisions do not answer pending questions. Progress.NextStep preserves the next action; changes retains up to 64 recent decision and adjustment reasons.

## Checks and acceptance

Check.GoalIDs is the only association from checks to outcomes. Every goal needs an acceptance check. Acceptance and regression checks must be resolved; diagnostics inform reasoning only. Checks retain exact commands, inputs, conditions and rationale. Narrow input scopes must explain actual dependencies; undeclared files, databases and services are not verified.

Reuse passes under unchanged conditions; related inputs, requirements or environment changes invalidate affected results. Unchanged failures need new diagnostic information to retry; after three, change code, conditions or method. Full suites need a current gap. Core verifies actual execution and applicability, not check sufficiency or semantic truth.

Known-failure exceptions apply only to regression checks and require an actual failure, a passed comparison, explicit user acceptance and current applicability. Delivery acceptance cites passed acceptance results only. Completion requires completed goals, resolved required checks, reviewed changes and no uncertain execution or unresolved decision. Claims never satisfy checks; human results represent actual user observations.

## Engineering questions and verification coverage

Hosts use existing goal_ids, rationale, conditions and verification_basis to explain checked behavior, principal failure paths and omitted coverage. Conditions are not executable assertions. Signature/interface/data changes require inspecting actual callers and whether affected tests build. Core verifies goal relationships, actual results and applicability; it neither grades semantic adequacy nor adds a fixed verification ceremony.

Actual unresolved questions live in assessment.concerns with stable IDs, original descriptions and relevant goals/checks/paths. Omission and empty arrays preserve them. verified cites applicable actual passing results; not_blocking explains why the current outcome remains satisfied and derives a delivery limitation; scope_changed references a saved real user-boundary change. Relevant dependency changes require a new resolution. None can waive required checks, deterministic violations or uncertain runs. The final review submits summary only, so clearing uncertainty prose cannot hide the original question.

## Native execution and recovery

The Host native process tool invokes Core check run using JSON stdin. Native tools own permissions, environment, asynchronous waiting and signals; Core owns durable reservation, processes and receipts. MCP/HTTP never start commands. Long checks retain their native session and run identity rather than extending MCP requests or adding a scheduler.

Read the original Task/run after response loss. Recovering a receipt does not execute a command; live or unobservable processes stay pending. If all stopped without a receipt, inspect effects and record a real decision where required, retaining interrupted_unknown. Recovery never invents a pass. Completion, cancellation and archive neither delete workspaces nor authorize Git publication.

## Metering and limits

Zero means no user-set cap; usage from mutations and actual execution remains cumulative. Positive caps apply at execution/reservation, and shortages cannot lower acceptance requirements. Actual, reserved and unknown quantities are separate. Core and Host times overlap and must not be added.

Mutations require successful metering preparation before work. Saved-state reads neither write metering nor query process identity, keeping recovery information accessible; cumulative Core time excludes these reads. Host-reported analysis, waiting and tokens may be incomplete and do not measure all Host activity.

## User interface

WebUI offers search, Host/status filters, language selection, actual user answers, regression exceptions and execution recovery. Answers bind issue_digest and retain their reason without changing restrictions. Mutations are never replayed automatically. Response loss requires refresh; saved changes followed by display failure are distinguished, and late responses cannot replace another task.

A checkpoint may omit or repeat the current issue unchanged, but cannot replace an unresolved question. Record the actual user answer with resolve_issue before saving a different question.

## Verification

Use temporary Git repositories and isolated SQLite to verify rejection of goals without acceptance, exceptions separate from acceptance, retained independent results, user answers preserving restrictions, saved reads under denied process queries without metering writes, no MCP/HTTP process execution, durable native interruption recovery, and filtering before paging. Browser checks cover decision/budget writes and unknown-run recovery; simulated response loss and stale selection requests check UI behavior.

The integrated entrypoint covers Go, frontend, real execution journeys, four independent Host packages and retained publisher/desktop build checks. Tests use isolated resources and perform no publication. Real model sessions, native execution on other platforms and product benefits are reported separately; scripted semantic input is not real user validation. Historical data is preserved and refused without migration or reset.

Host reports use cumulative host_usage per non-overlapping Task source_id, with observed_through and coverage. Duplicate samples are counted once; unknown is null and measured zero is 0. Missing report coverage blocks new commands only when an explicit Host cap needs it, while saved reads and recovery remain usable.
