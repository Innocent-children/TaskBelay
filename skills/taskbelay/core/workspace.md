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

Use the packaged {{host}} workspace helper for the selected operation. Deletion requires the exact
current terminal Core task, the per-repository provisioning_receipt_id returned by scope, the matching
workspace and independent user authorization. Dirty, unpushed, unknown and uncertain resources are
retained. Verify Host-specific relaunch or new-chat capabilities in the real Host.

Implementation: the workspace/launch helpers in `packages/{{host}}/lib/` and `internal/application/service.go`.

Run the first assessment on source checkouts before freezing/creating a workspace. For a task branch without its own upstream, pass each retained workspace_origin.base_branch as repository.base_branch to assess the corresponding development upstream explicitly; never substitute main or call cached/no-upstream state latest.
