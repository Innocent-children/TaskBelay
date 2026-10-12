# TaskBelay product definition

[中文](PRODUCT.md) | [English](PRODUCT_en.md)

## Goal and responsibilities

Help developers complete real work with existing AI coding agents, at the same acceptance standard, with necessary changes, reasonable verification effort and fewer repeated decisions. The agent interprets requirements, makes technical decisions and edits code. Local Go Core relates actual authorization, changes and evidence and decides what still needs correction, verification, waiting or a user decision. It provides no coding model, general workflow or sandbox.

## User workflow

Describe outcomes, non-goals and necessary limits. The agent first presents its understanding, root cause, complete approach, ownership, scope, verification and limits. Explicit user confirmation precedes implementation. Approval binds that exact plan; material revisions require confirmation, ordinary progress does not. Before each new Task, show directories, branches and initial changes, then ask the user to choose the current branch, a new local branch or a dedicated worktree; there is no default. Resume existing Tasks in their bound workspace. Workspace preparation, new chats and Git operations retain separate authorization. Implementation and verification can alternate after approval.

Core retains goals, interface/data/dependency/design constraints, acceptance checks and a user-message reference. The reference is Host audit testimony, not conversation authentication. Programs observe changes within files, exported Go declarations, manifests, data-contract files, forbidden paths and pinned content. Predicted paths and change magnitude are review signals; semantic scores cannot override deterministic restrictions. The agent maps changes to goals and records a basis and uncertainty for every constraint. Correct within existing authority first; only changed authority or an important tradeoff goes to the user.

## Verification, revision and recovery

Core executes authorized local checks and retains actual process results. Agent claims and explicit human observations are separate and cannot manufacture automated execution. Results bind goals, check definitions, inputs, selected environmental conditions and executables. Git-visible inputs across all bound repositories are the default; narrowing requires dependency rationale. Ignored content and external services are not automatically known conditions. A passing command does not prove an acceptance conclusion; adequacy and semantics require technical review.

Revision retains unaffected work and evidence. Changes to covered goals, check semantics, relevant constraints, inputs or conditions expire related results. Timeout, retry policy, full-suite flag and explanatory text changes preserve passing results but still require renewed plan approval. Lifetime cost includes execution, Core calls/observation and separately labelled Host analysis, waiting and tokens. Replanning never resets it. Time categories overlap and must not simply be added as a total bill. Limits never waive acceptance; gaps remain visible and necessary increases require real authorization. Three failures with the same check and conditions require changed code, conditions or verification approach.

Recovery reads the latest authorization, progress, applicable/expired results, failed attempts and unresolved executions. Operation identity is saved before process start; response loss never reruns it. Signed receipts recover actual results. A live Core or recorded command/process group requires waiting/inspection. Both must stop before reconciling a missing receipt; their exit never implies passing. Inspect side effects and record resolution rationale; non-repeatable operations also need a user decision. Unknown duration is conservatively charged. Recent-history compaction preserves live results, unresolved operations and cumulative cost.


Actual changes cite approved work and necessity. Predicted paths grant no permission; explicit allowed/forbidden paths are checked. Reuse applicable passing evidence; unchanged failures need new diagnostic information and full suites need a gap targeted checks cannot cover. Ordinary progress cannot replace checks or weaken acceptance. Delivery also reviews root cause, responsibility, complexity and verification, explicitly reconsidering the resulting structure. Only approved regression checks can receive named existing-failure exceptions backed by current comparison evidence and a user decision. Failures stay failed; content or plan changes expire the exception.

## Completion and interfaces

Delivery requires completed progress for every goal, supported current scope review without unresolved uncertainty, applicable passing evidence or a current explicitly accepted regression exception for every required check, and no unresolved execution. Save a conclusion and evidence references for every goal. Completion/cancellation releases claims and retains files; neither performs commits, pushes, merges, publication or deletion.

Local WebUI presents results, evidence, gaps, necessary choices and cumulative cost. The desktop pet reads saved summaries, not live agent activity. Users need no state-machine knowledge or workflow forms. Structured hooks cover supported paths; shell/external edits are observed at checkpoints. Missing workspaces and conflicting history require restoration of the original binding; cross-workspace relocation is not provided.

## Support and acceptance boundaries

This upgrade is source capability; published packages and stable support records are unchanged. Old protocols/databases are incompatible and never migrated or automatically deleted. Small mechanical work is often simpler with a native agent. Windows Core journeys, protocol/storage tests, adapter tests and real Host sessions are recorded separately. Unexecuted checks cannot establish benefits or support. See [verification](PROJECT-STATUS_en.md), [architecture](ARCHITECTURE_en.md), [commands](COMMANDS_en.md).
