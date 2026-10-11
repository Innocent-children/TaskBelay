# Engineering control design and acceptance

[中文](ENGINEERING-CONTROL.md) | [English](ENGINEERING-CONTROL_en.md)

## User problem and outcome

Users need necessary, proportionate and verifiable work within confirmed requirements, without speculative design, excessive verification, unrelated fixes, weakened acceptance or unauthorized effects. File or test counts alone do not define excess. Necessary root-cause repairs and responsibility changes must remain possible. Each work item explains its goal, consequence of omission, owner and simpler alternative.

## Data and responsibility

Core owns the single Task, authorization, complete plan, exact user confirmation, observations, checks, evidence, cumulative cost and completion. Hosts show the plan, obtain an actual user answer and supply supported semantic reasoning. Message references are Host testimony, not conversation authentication. Repository observation owns Git/file facts; execution owns argv processes and receipts; storage owns atomic updates, deduplication, claims and bounded records. CLI, MCP and HTTP share the service.

## Confirmation before implementation

open_task saves a pending plan: understanding, root cause, approach, simpler alternative, necessary work and ownership, verification basis and completion. plan_digest binds goals, constraints, allowed/forbidden paths, dependency policies, checks, limits, workspaces and proposed content. confirm_plan records the actual answer and digest. Before confirmation, implementation writes, checks, progress and human acceptance are denied. Changed content after proposal prevents confirmation; restore it or disclose it in a revised plan. Initial investigation uses already authorized read-only Host operations.

revise_task replaces authorization, plan and checks. Material changes clear confirmation and need a newly displayed plan and actual answer. Unchanged goal progress and applicable results remain; cost never resets. checkpoint only records progress, semantic review, issues and reported cost; it rejects checks. Reuse a still-valid confirmation across sessions without asking again for ordinary progress.

## Scope and design necessity

expected_paths predicts work; allowed_paths restricts permission, defaulting to bound repositories; forbidden_paths takes precedence. Qualify paths as key::path in multiple repositories; a dot means the whole scope. Structured writes check confirmation, unresolved authorization, workspace identity, allowed paths, exclusions and pins. Shell and external edits are checked on observation; this is not a global sandbox.

Each path judgment cites approved work_id, goal_ids, basis and necessity. Core checks references and current contents, not semantic truth. Delivery additionally needs a current plan/content-bound review of root cause, responsibilities, proportionality, verification and whether the same structure would be chosen again. Uncertainty or a negative conclusion prevents completion. Scores cannot override deterministic restrictions.

## Verification and repeated cost

The confirmed plan defines checks. purpose distinguishes acceptance, regression and diagnostic; diagnostics must also serve goals. Each run supplies rationale, and a full suite also supplies full_suite_basis explaining the current gap targeted checks cannot cover. Reuse a passing command under unchanged requirements and observed conditions. Retrying unchanged failures requires previous_run_id and distinct new_information. Three failures require changed actual inputs, requirements or method. Renaming a check, changing prose or timeout does not reset the same execution's failure count.

Default inputs cover Git-visible content in every bound repository. Narrowing needs input_basis; key::path can include other repositories and explicit ignored inputs. Applicability binds check, relevant goals/constraints, inputs, environment and executable. Unknown conditions are not verified. Failed results retain their status, and claims cannot replace execution. Limits are ceilings: spare budget does not justify another check and exhaustion does not waive acceptance.

## Known failures, recovery and delivery

accept_known_failure is restricted to checks approved as regression. It needs a current actual failure, an applicable pass from an approved comparison command, named existing failures and the user's decision. The comparison declares compares; its adequacy still needs technical review. Acceptance checks cannot be waived. Exceptions bind plan/content, expire on changes and leave evidence failed. Delivery cites current evidence and exceptions without claiming all checks passed.

Reserve identity and cost before process start. Read the original result after response loss; receipt recovery does not rerun. A stopped owner without a receipt remains unknown, never passed. Unresolved runs prevent new execution, completion and cancellation. Execution, Core and reported Host cost survive revisions and compaction. Lifecycle remains active/done/cancelled. Completion requires current approval, necessity review, all goals/checks and resolved executions. Cancellation retains files and releases claims. Terminal tasks can be archived/restored; pagination reaches beyond the first 200 tasks and detail links select the correct task.

## Risks and non-goals

No historical-data compatibility, migration or automatic deletion; no general workflow engine, coding model, sandbox, automatic Git publication or workspace relocation. Host semantic claims cannot be authenticated and actions outside integration cannot be intercepted. Actual success does not prove adequate tests or lower net cost. Control has overhead and must not demand a form or poll after every mechanical edit.

## Acceptance and proposal assessment

Use isolated Windows Git repositories and SQLite to verify: no execution before confirmation; old confirmation cannot approve a changed plan; checkpoint cannot replace failing acceptance; unauthorized writes are denied or observed as blockers; cross-repository changes invalidate dependent evidence while independent results survive; unjustified retry and renaming bypass fail; exceptions retain failures and expire; actual interruption does not replay; pagination, archive and detail links work.

Also check false positives: necessary approved repairs proceed, implementation and verification can alternate, unchanged verified content reuses real results and explicit existing regression exceptions remain truthful. Core, protocol, storage, adapter and journey checks have separate provenance. Fixtures do not establish native Host chats or other-platform operation.

The design restores actual approval and results across sessions using Git and execution facts, with one necessary pre-development confirmation and one Core Task. It adds no development-phase forms. Benefits require real-task comparisons, not field completeness or passing tests. See [project status](PROJECT-STATUS_en.md).
