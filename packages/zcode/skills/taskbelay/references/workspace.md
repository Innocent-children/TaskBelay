<!-- Generated from skills/taskbelay/core/workspace.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Choose the workspace for each new Task

Before every new Task, show the actual directories, current branches and initial changes, and ask
the user to choose: current branch, a new branch in the current directory, or a dedicated worktree.
Do not infer a default or reuse a workspace choice from a different Task. Retain the actual answer
as authorization_ref when preparing a workspace. Resume an existing Task in its saved workspace;
ordinary follow-ups and recovery do not repeat this question.

Before editing any selected repository, use the Host freshness operation for all roots with actual Git authorization. Inspect the corresponding upstream rather than assuming main. Fetch changes tracking metadata; it does not authorize integration. Dirty/diverged/failed/no-upstream states preserve work and remain explicit. Supply each report as remote_freshness when opening Core; checkpoint.repository_freshness retains later observations. Recheck before a later commit. Codex/Claude/ZCode use host-launch freshness; DeepSeek uses workspace_coordinator freshness with freshness_repositories. Supply a separate integration_authorization_ref for integrate:true; only separately authorized clean fast-forwards are allowed after assessing every root.

Prepare exactly the selected source, branch and carry choice. Reuse saved receipts after interruption;
never repeat an uncertain Git or Host operation. Helper scope output is the repositories array for
the current Core open request. No helper changes requirements, acceptance, cost or Task status.

Use the packaged zcode workspace helper for the selected operation. Deletion requires the exact
current terminal Core task, the per-repository provisioning_receipt_id returned by scope, the matching
workspace and independent user authorization. Dirty, unpushed, unknown and uncertain resources are
retained. Verify Host-specific relaunch or new-chat capabilities in the real Host.

Implementation: the workspace/launch helpers in `packages/zcode/lib/` and `internal/application/service.go`.

Run the first assessment on source checkouts before freezing/creating a workspace. Once bound, pass all original workspace_origin.source_type, remote_name and base_branch fields, including when a task branch has its own upstream. A remote origin uses its remote-tracking source and needs no local source branch. Never substitute main or a task branch upstream implicitly, and never call cached/no-upstream state latest.

For integrate:true, retain the stable operation_id and returned receipt_path. The Host stores a bounded intent and each repository result under its configured data directory/freshness before Git mutation. The full selection is preflighted, but sequential Git integration is not an atomic transaction: later drift can leave an earlier repository applied. Read its saved partial result; preserve all work. Identical requests read the old receipt. recover:true with the original request checks a stopped owner and actual Git facts without fetch/merge, rollback or continuation. Unknown command outcomes remain unknown, even when the frozen target is present. Inspect original effects before any new separately authorized assessment for remaining roots. Recovery locks retain owner PID, a random token and file identity. A stopped owner may be reclaimed only after an exclusive claim and unchanged identity are verified; PID reuse, a live owner, denied liveness query or incomplete identity is a blocker. Receipt writes and lock release are fenced against replacement. A leftover reclaim-claim with unknown outcome remains blocked for inspection; no age-based deletion is provided. Never delete locks or edit receipts to bypass uncertainty.
