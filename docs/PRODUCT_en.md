# TaskBelay product definition

[中文](PRODUCT.md) | [English](PRODUCT_en.md)

## Goal and responsibilities

Help developers complete real work with existing AI coding agents, at the same acceptance standard, with necessary changes, reasonable verification effort and fewer repeated decisions. The agent interprets requirements, makes technical decisions and edits code. Local Go Core relates actual authorization, changes and evidence and decides what still needs correction, verification, waiting or a user decision. It provides no coding model, general workflow or sandbox.

## User workflow

Describe the outcome, explicit non-goals and necessary limits. When authorization already exists, create a Task in the authorized current workspace/session without phase approvals. Workspace preparation, new chats and Git operations are optional Host actions; retain actual authority and ask only for missing material choices. Implementation and checks can alternate; all implementation need not finish before checking.

Core retains goals, interface/data/dependency/design constraints, acceptance checks and a user-message reference. The reference is Host audit testimony, not conversation authentication. Programs observe changes within files, exported Go declarations, manifests, data-contract files, forbidden paths and pinned content. Predicted paths and change magnitude are review signals; semantic scores cannot override deterministic restrictions. The agent maps changes to goals and records a basis and uncertainty for every constraint. Correct within existing authority first; only changed authority or an important tradeoff goes to the user.

## Verification, revision and recovery

Core executes authorized local checks and retains actual process results. Agent claims and explicit human observations are separate and cannot manufacture automated execution. Results bind goals, check definitions, inputs, selected environmental conditions and executables. Whole-repository Git-visible inputs are the default; narrowing requires dependency rationale. Ignored content and external services are not automatically known conditions. A passing command does not prove an acceptance conclusion; adequacy and semantics require technical review.

Revision retains unaffected work and evidence. Changes to covered goals, definitions, relevant constraints, inputs or conditions expire related results. Lifetime cost includes execution, Core calls/observation and separately labelled Host analysis, waiting and tokens. Replanning never resets it. Time categories overlap and must not simply be added as a total bill. Limits never waive acceptance; gaps remain visible and necessary increases require real authorization. Three failures with the same check and conditions require changed code, conditions or verification approach.

Recovery reads the latest authorization, progress, applicable/expired results, failed attempts and unresolved executions. Operation identity is saved before process start; response loss never reruns it. Signed receipts recover actual results. A live owner requires waiting/inspection; a stopped owner without a receipt never implies passing. Inspect side effects and record resolution rationale; non-repeatable operations also need a user decision. Unknown duration is conservatively charged. Recent-history compaction preserves live results, unresolved operations and cumulative cost.

## Completion and interfaces

Delivery requires completed progress for every goal, supported current scope review without unresolved uncertainty, applicable passing evidence for every required check and no unresolved execution. Save a conclusion and evidence references for every goal. Completion/cancellation releases claims and retains files; neither performs commits, pushes, merges, publication or deletion.

Local WebUI presents results, evidence, gaps, necessary choices and cumulative cost. The desktop pet reads saved summaries, not live agent activity. Users need no state-machine knowledge or workflow forms. Structured hooks cover supported paths; shell/external edits are observed at checkpoints. Missing workspaces and conflicting history require restoration of the original binding; cross-workspace relocation is not provided.

## Support and acceptance boundaries

This upgrade is source capability; published packages and stable support records are unchanged. Old protocols/databases are incompatible and never migrated or automatically deleted. Small mechanical work is often simpler with a native agent. Windows Core journeys, protocol/storage tests, adapter tests and real Host sessions are recorded separately. Unexecuted checks cannot establish benefits or support. See [verification](PROJECT-STATUS_en.md), [architecture](ARCHITECTURE_en.md), [commands](COMMANDS_en.md).
