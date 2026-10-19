# TaskBelay Product Definition

[中文](PRODUCT.md) | [English](PRODUCT_en.md)

## Purpose and responsibilities

Help developers complete real work with their existing AI coding agent: less repeated explanation and confirmation, inspectable current results, useful recovery after interruption, and an honest account of completion and remaining gaps. The agent understands requirements, chooses methods and edits code. The local Go Core retains one Task relating actual requests, changes, results and cumulative usage.

Use it for development that benefits from saved progress, verification reuse or continuation across sessions. Small mechanical edits and ordinary questions usually need only the native Host. Honor explicit invocation and opt-out. TaskBelay is not a coding model, general workflow engine or sandbox.

## Starting and continuing

The user states the outcome and necessary restrictions. The agent reads project rules and relevant code, explains the cause, approach and verification, then works within existing authority and Host permissions. A Task records the actual user reference and request; there is no separate approval of every plan. References are Host testimony, not conversation authentication.

Use the workspace selected by the user or clear from context, showing its directory, branch and initial changes. Resolve a workspace choice only when it is unclear, conflicting or requires a new operation. Branches, worktrees, chats, upstream checks and integration follow actual need and authority, not a mandatory startup ritual. Unchecked upstream state remains unknown. Resume a saved Task in its bound directory.

Implementation and verification can alternate. Record meaningful progress after a piece of work, relevant change, check result, necessary decision or handoff; do not impose a checkpoint on every edit. Routine tool results contain current requirements, progress, changed paths, checks and usage. Full history, file fingerprints and output are available on demand.

## Requirement and check changes

User outcomes and explicit restrictions are separate from the agent's implementation arrangements. Approach, method, forecast and review-hint changes reuse the original request. Changed outcomes, request or explicit restrictions, and expansion of user-set limits, require an actual new user reference. Revisions replace only supplied fields, retain unaffected work and accumulated usage, and do not answer an unresolved question automatically.

Predicted paths grant no permission. Programs check explicit allowed/forbidden paths, content pins and dependency restrictions. Without an explicit dependency policy, changes require a necessity and compatibility review; violations of an explicit policy block. Native permissions always apply. Structured hooks cover supported paths, while other writes are observed at checkpoints.

Authorization retains only user outcomes and explicit restrictions; Plan owns check definitions, forecasts and review hints. Checks use goal_ids as the only outcome mapping, and each goal requires an acceptance check that cannot be waived. Checks serve current outcomes and retain actual commands, inputs, conditions and rationale. Default inputs cover Git-visible content across all bound repositories. Narrower scopes need a reason and actual dependencies. Ignored files, databases and external services are not automatically verified. A passing command does not establish test sufficiency; the agent must review whether it proves the requested result.

Relevant code, goal, constraint or execution-condition changes invalidate affected results. Unrelated work and additional independent checks preserve existing applicable results. Reuse a pass under the same conditions. Retrying an unchanged failure needs new diagnostic information; after three failures, change code, conditions or method. Full suites require a current gap. Weakening a check cannot establish acceptance.

## Usage and recovery

When the user has not set a cap, measure usage without inventing budget approvals. Positive limits cap cumulative executions, command time, Core time, reported Host time and tokens; zero means unset. Expanding or removing an existing cap requires an actual user decision. Actual, reserved and uncertain usage are separate; overlapping time categories must not be summed. Exhausted budget does not waive acceptance. Saved-state reads neither write metering data nor query process identity; cumulative Core time covers mutating calls.

Native Host process tools invoke Core commands under their permission, environment and long-running sessions; MCP and HTTP never start commands. Persist operation identity and reservation before execution, and save results before returning. After response loss, read the original operation; recovering a receipt never reruns a command. Wait for a live original owner or command group. If they stopped without a result, inspect effects and retain the unknown conclusion. Non-repeatable operations need a real user decision. History compaction retains current results, unresolved operations and cumulative cost.

## Delivery and interface

The unified terminal entry groups upgrade, install into Agents, check and repair, Control Center, desktop pet, installation information, and uninstall/data deletion by operation. Arrows move, Enter confirms, Space selects multiple items, and going back retains unfinished choices. Upgrade displays actual current and target versions of installed plugins and the launcher, retaining the installation channel and one ordinary maintenance confirmation. Lookup failure never means up to date. An all-current result allows returning or rechecking; explicit versions apply to one component because products release independently. Interactive cancellation is distinct from missing noninteractive confirmation and emits no approval command. A healthy installation with unchanged desktop files does not stop the pet or copy it again. Diagnostics read the actual state before offering relevant actions, and results remain visible until the user chooses the next step.

Uninstall retains data. Delete data keeps plugins, registrations and required installation records, then prepares empty data directories at the same paths. Delete everything (plugins + data) removes all managed plugins and confirmed data. Both data operations clear task records, configuration and pet preferences while retaining the launcher and desktop application. Shared data is confirmed by exact directory, not partitioned by Agent; project code, Agent conversation history and other plugins are retained. Recoverable deletion is the default. Permanent deletion, extra directories and Agent shutdown that cannot be automatically verified require explicit confirmation. Running or uncertain operations must be handled before cleanup, which cannot bypass recovery.


Review the actual diff before delivery: whether it resolves the cause, responsibilities are appropriate, complexity and validation are proportionate, and the structure remains the one you would choose. Engineering questions retain stable IDs in assessment.concerns; omission or an empty array never removes saved questions. A resolution cites applicable actual results, explains a limitation that does not affect the current outcome, or references a saved actual scope change. Related changes invalidate the resolution. Completion requires conclusions for all outcomes, applicable required results and concern resolutions, and no unresolved execution or decision. Delivery limitations come from the saved concerns, rather than a separate text field that can hide unresolved work. Agent claims do not prove execution; human results require actual user observations.

Existing regression failures may be accepted using current failed execution, comparison results and the user's explicit decision. Acceptance checks cannot be waived. Exceptions and outcome acceptance are checked separately; delivery conclusions cite only actual passed acceptance checks. Exceptions apply only while related results remain applicable, and failures remain failed. Delivery distinguishes passed, failed, unrun and unknown work; a blocked task is not completed work.

The local WebUI supports request/task-ID search, Host and status filters, language selection, actual user answers and execution recovery. A compact task list sits beside details; narrow screens show them separately. Overview prioritizes results, pending decisions, uncertain executions and remaining work; check output expands below its check, while requirements and detailed usage have dedicated tabs. Update records reads saved content; Check current code re-observes actual conditions. Failed forms retain input, and lost responses require reading the original record first, without a plan-confirmation form. Completion, cancellation and archiving retain files and do not commit, push, merge, publish or delete resources. The desktop pet reads saved summaries, not live agent activity.

## Support and validation boundaries

These changes are source capability. Public packages and stable support records follow independent release contracts. Historical protocols and databases are unsupported, without migration or automatic deletion. Cover ordinary development, common failures and known regressions; do not build mechanisms for speculative rare combinations.

Actual Core processes, scripted user/semantic input, adapter tests and real model sessions are recorded separately. Source tests do not establish reduced human intervention, elapsed time or cost. Compare the native agent, a concise prompt and the full plugin under the same task conditions to measure those benefits, and disclose unexecuted comparisons. See [project status](PROJECT-STATUS_en.md), [architecture](ARCHITECTURE_en.md) and [engineering control](ENGINEERING-CONTROL_en.md).
