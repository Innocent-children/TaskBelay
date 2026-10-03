# TaskBelay Product Definition

[中文](PRODUCT.md) | [English](PRODUCT_en.md)

## Product position

**Long-running AI coding, on belay.**

In climbing, a belayer manages the rope while the climber chooses the route. TaskBelay applies that idea to coding: the agent makes technical decisions, while TaskBelay checks approved scope and verification limits and uses saved records to recover from failures or uncertain results. It is neither a sandbox nor another coding agent.

Your coding agent decides what to do next. TaskBelay keeps the task under control.

The Host / coding agent—Codex, DeepSeek, Claude Code or ZCode—understands requirements, reads code, reasons about technical solutions, chooses the next action, edits code and runs commands. TaskBelay does not replace that reasoning. It maintains a durable control boundary around the engineering task:

- **Explicit Scope:** check actual changes against the approved plan. Unplanned files need a recorded decision; expanding the plan requires renewed approval.
- **Bounded Verification:** plan relevant checks and effort limits. Additional verification needs a concrete recorded reason; spare capacity alone does not justify wider testing.
- **Durable State:** Go Core retains the one authoritative task state. Progress and valid checks come from saved records, not reconstructed chat context.
- **Safe Recovery:** failures retain reasons and handling paths. Read saved operation records before deciding how to recover or retry an uncertain result.

A Task is a persisted development job; an Action is an operation Core issues for its current stage. Core checks the worktree and submitted results to determine legal workflow transitions; the agent still chooses the technical action. New requests are assessed for fit, and small mechanical edits usually need only direct development.

## Target users and use cases

The product serves developers using Codex, DeepSeek, Claude Code or ZCode on real repositories over multiple sessions or days. It fits public-interface, persistence, multi-component and recovery-sensitive changes, as well as work requiring explicit file scope and verification effort.

Other changes in a shared checkout can obscure ownership. Chat history alone may not establish whether tests remain valid, an operation succeeded, or work remains after an interruption. TaskBelay defaults to a new branch in the current directory, also supports the current branch or a dedicated worktree, and retains requirements, plans, check results, blocker reasons and recovery information.

Direct Host use is usually simpler for one-off questions, explanations, status queries and small mechanical edits. A few explicitly selected repositories may share one Task, but multi-repository work and worktree handoff are advanced capabilities, not the primary use case.

## Task creation and resume

A new request receives a read-only assessment of discovered impact, unknowns and a recommendation. The developer chooses direct work, TaskBelay or clarification; an explicit selector does not skip assessment. Before selection there is no Task creation, Git mutation or development-session dispatch.

After choosing TaskBelay, the default creates a task branch from the current HEAD in the current
directory. Explicit alternatives retain the current branch or create a dedicated worktree. Local modes
reuse dependencies, local configuration and build outputs, continuing the same session when it has all
required directory permissions. The Host shows directories, current branches and initial changes and
obtains missing target-branch or initial-content choices. Accepted initial changes all belong to the
Task scope, with staged state retained. Unaccepted content stops preparation without automatic cleanup
or exclusion of unrelated work.

Dedicated worktrees select a local or remote source, base, target branch and local-content choice per
repository. Local preparation works offline; remote preparation fetches the selected branch and freezes
its commit. Carrying copies staged, unstaged and non-ignored untracked contents while preserving the
source and index. Any preparation failure prevents a partial Task; retain local branches and changes
for inspection.

One directory has one active Task. The Host checks Core claims read-only before local branch operations,
and Core acquires all directory claims together at creation. That check does not lock later Git operations;
users and external tools must avoid starting another writer during preparation. Manual and external edits
in local directories remain subject to scope and verification rules. Keep the bound branch during a Task.

Discovery follows user instructions, applicable repository rules and Host permissions. Discovery may identify candidates; Task membership requires confirmation and becomes fixed at creation.

When Codex starts a new session, it retains relevant requirements discussion and confirmed requirements, distinguishing unaccepted suggestions and unresolved questions. Explicit choices and authorizations that remain valid are retained; only unresolved decisions or required inputs need another question.

Explicit resume returns to the original working-directory instance and saved state without creating another task or selecting another worktree. The destination of a confirmed launch verifies the retained launch record before continuing initialization. A missing or replaced original worktree pauses progress; the developer can restore that instance or explicitly abandon the Task.
An active Task can prepare a branch-name correction for one repository and continue the same work
after verifying unchanged directory, HEAD, index and content. The preparation pauses work; cancellation
requires unchanged original facts. Codex can explicitly replace an unexecuted launch, or a failed launch
with positive evidence that no target operation was called. Partial or uncertain execution is reconciled
first, and the replaced launch cannot be resumed. Repeated planning revisions retain real saved baseline
references in the Task, without an archive tier or configured count limit. Resources and revision-number
ranges still constrain growth. Responses provide a bounded first page and total count; pagination
reads the complete history without reconstructing old documents.


## Planning discussion before implementation

The Host shows the requirements and acceptance criteria, the design and its impact, and the complete work, expected-file and verification plan, then discusses the developer's changes. Separate approval at every stage is unnecessary, but implementation requires explicit approval of the complete plan. Choosing TaskBelay or workspace parameters does not approve an unseen plan.

Core saves the draft and stays in task planning while waiting. Approval references the exact current requirements, design, plan content and planning round. Revisions invalidate earlier approval; a resumed session can continue from the same saved draft. Prefer exact files and explain the purpose and coverage of directory ranges. Expanding scope requires saving and confirming a revised plan.

Codex launch preparation requires the complete impact assessment and the developer's mode choice. Missing data or unresolved unknowns prevent worktree preparation. Still-valid choices, explicit task resume and confirmed launch continuation remain effective. The Host owns actual presentation and user replies; Core cannot authenticate conversation content or intercept editing that never calls TaskBelay.

## Change and verification rules

| Situation | Current behavior |
| --- | --- |
| Worktree changes | Core derives actual changes from the bound directory, including local manual/external edits; dedicated worktrees remain separate from later source edits |
| Work leaves the plan | Supported structured tools ask before writing; later observation finds other writes, and unexplained paths cannot reach testing or delivery |
| An unplanned file needs handling | The developer chooses one-time allowance, replanning or restoration; Core retains the decision |
| Analysis completes | The task plan records checks, rationales, initial command budget, full-suite expectation and test-code expectation |
| Verification capacity is insufficient | Record a concrete new impact, risk, failure or gap and the required increase before more testing; remaining work or reruns of existing checks can also receive capacity |
| A full suite is proposed | Reassess whether focused checks suffice, what remains uncovered and current repository requirements every time; spare capacity is not a reason to widen testing |
| Testing repeats exactly | The third identical failure, result or change-and-failure loop pauses for an explicit decision |
| Code is reviewed after a change | Cover the current change, direct or indirect impact and acceptance needs; fixes receive related rechecks; explicit review stays read-only and awaits separate repair authorization |
| Content changes | Invalidate tests and comprehension based on older content; linear commits of identical content retain their results |
| Workspace history is abnormal | Branch switches, detached HEAD, rewinds, unprepared history rewrites or instance replacement trigger a blocker or unavailable state |

Carried local content belongs to the Task change scope. Codex plans preservation and new development separately; preservation checks do not certify existing behavior as tested.

Task creation does not freeze the final verification budget. Consumption belongs to the current task plan; formally rebuilding the plan starts its budget while retaining old records. Capacity increases produce no passed results, and Hosts must report the checks actually performed and their sources.

## Completion and uncertain operations

Every planned work item must complete before testing. Delivery explicitly links each acceptance criterion to corresponding completed work and passed current checks. Automated, static, Host-observed and explicit manual checks are supported; developer comprehension confirmation is retained separately and cannot replace acceptance checks.

An uncertain operation is read from Core's saved record before recovery or retry. Reopening WebUI still discovers pending submissions. Host handoff and worktree provisioning likewise use their own retained records to avoid duplicate execution.

Codex retains the complete tool response before checking `ok` and reading success data; explicit rejections follow the returned error and handling instruction, and local caching or presentation errors do not change the original response.

Workspace handoff requires every repository to use a dedicated worktree. Local modes resume in the original directory. Same-machine handoff starts with Core retaining recovery conditions, followed by one Host handoff. Core verifies the destination and replaces repository bindings atomically. Failure retains the original bindings and claims.

DONE or CANCELLED ends the Task and releases repository claims without automatically committing, pushing, opening a PR or deleting a worktree. Local directories and branches are retained, and uncommitted work becomes initial content for the next Task. Dedicated worktree and branch cleanup require separate authorization. Explicit abandonment of an unavailable worktree retains the last known state and ends the Task.

## Entry points and component responsibilities

| Component | User purpose |
| --- | --- |
| Codex / DeepSeek / Claude Code / ZCode | Assess requests, perform confirmed development and Host operations, and resume the same Core Task |
| Unified lifecycle CLI | Install, diagnose, maintain and remove Adapters, preserving Task data and configuration during ordinary maintenance |
| Local WebUI | Inspect tasks, results, blockers and recovery, and submit supported operations through Core |
| Desktop pet | Show multiple Tasks in stacked bubbles and open each WebUI; prioritize blocked tasks, follow unfinished work after completion, and provide pinning, custom appearances, animation controls, resizing and independent start/stop |
| OpenSpec / Spec Kit | Optionally organize requirements, design and tasks; method-tool results do not decide Core state |

When there are no tasks to display, only the character remains; task bubbles and the task count control are hidden. Bubbles return when tasks appear. Clicking the character still opens the task list.

The formal `taskbelay` npm package includes the macOS arm64 and Windows 10/11 x64 desktop apps and default artwork. A configured Adapter supplies Core. Maintenance commands refresh the app copy while preserving settings and appearances. Desktop presentation indicates neither live Host activity nor completion percentages. See the [desktop pet guide](DESKTOP-PETS_en.md) for installation and artwork.

## Product boundaries

- Core observes Git read-only; authorized Hosts perform fetch, worktree creation, branch operations, handoff and cleanup.
- Worktrees isolate source-change ownership, not processes, networks, credentials, ports, databases, containers or services.
- Core does not intercept every Host file operation or shell command; external writes may happen before observation checks them.
- The product does not automatically copy ignored files, install dependencies, expand repository scope or clean active, dirty, unpushed, unknown-owner or uncertain resources.
- General agents, arbitrary workflow DSLs, custom state machines, cross-machine transfer, remote MCP, cloud multi-user management and automatic Git publication are outside scope.

## Feature decisions and verified scope

Improvements should address actual task problems, help resume from the right state and reduce the effort needed to understand state and next steps. Decisions use Task, Action, repository observations and retained records; Core remains the sole authority for Task state. A new platform, Host or interface needs a concrete user result and repeatable acceptance method.

Source capability does not establish stable-package support. Results apply only to the actual artifacts, platforms and steps tested; simulations and static checks cannot replace real Host workflows. The project does not yet have enough external data to demonstrate lower defect rates, verification cost or recovery time.

See the [Support Matrix](SUPPORT-MATRIX_en.md) for stable support and unverified scope, and [Project Status](PROJECT-STATUS_en.md) for delivered capabilities and gaps. Protocols and implementation belong in [Architecture](ARCHITECTURE_en.md), and operation parameters in the [Command Reference](COMMANDS_en.md).

## Host interaction

The Host translates developer requests into the shared task workflow. Permissions and session operations differ between Hosts; task state, completion conditions and recovery decisions remain Core-owned. See the [Codex guide](CODEX_en.md), [DeepSeek guide](DEEPSEEK_en.md), [Claude guide](CLAUDE_en.md) and [ZCode guide](ZCODE_en.md) for their actual operations.

ZCode local package preparation and UI plugin loading are separate steps. Installation, updates and removal report the required UI actions; local preparation does not establish Host-session verification. Windows and macOS acceptance are recorded separately, retaining unexecuted native checks as pending.

## Acceptance with known failures

When related checks pass and an automated comparison finds no new or changed failures, developers may explicitly accept the listed existing failures. Core retains actual test results separately from acceptance; failed checks stay failed and delivery shows the accepted scope. Decisions apply only to the content and plan the user saw and require reassessment after either changes. Completion criteria still reference actual passed checks; unexecuted checks and new problems cannot pass as known failures.

Completed user checks are separate from pending manual execution. Errors identify affected fields, permission restrictions or actual usage. Hosts correct parameter mistakes from established facts within Core's returned scope and ask only for missing user decisions.
