# Choose the workspace for each new Task

Before every new Task, show the actual directories, current branches and initial changes, and ask
the user to choose: current branch, a new branch in the current directory, or a dedicated worktree.
Do not infer a default or reuse a workspace choice from a different Task. Retain the actual answer
as authorization_ref when preparing a workspace. Resume an existing Task in its saved workspace;
ordinary follow-ups and recovery do not repeat this question.

Prepare exactly the selected source, branch and carry choice. Reuse saved receipts after interruption;
never repeat an uncertain Git or Host operation. Helper scope output is the repositories array for
the current Core open request. No helper changes requirements, acceptance, cost or Task status.

Use the packaged {{host}} workspace helper for the selected operation. Deletion requires the exact
current terminal Core task, the per-repository provisioning_receipt_id returned by scope, the matching
workspace and independent user authorization. Dirty, unpushed, unknown and uncertain resources are
retained. Verify Host-specific relaunch or new-chat capabilities in the real Host.

Implementation: the workspace/launch helpers in `packages/{{host}}/lib/` and `internal/application/service.go`.
