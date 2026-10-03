# Host and Task lifecycle

Use only for the requested lifecycle operation. Core preparation/cancellation/abandonment have their
own identities; Host launch receipts track Git/Host effects. They do not create another Core cursor.
Host commands use the [stdin/stdout transport](admission.md#command-transport).

## Core lifecycle operations

Use shared [cancellation](core-lifecycle.md#cancellation) and
[abandonment](core-lifecycle.md#abandon-an-unavailable-workspace) inputs with the current user authority.
Their readback uses the same retained Task and lifecycle identity, separately from Action recovery.

## Relocation

Relocation applies only when every participating repository uses `dedicated_worktree`. Local branch
Tasks retain their original directory; do not prepare relocation or invoke Host handoff for them.

Use [Core relocation preparation](core-lifecycle.md#prepare-relocation) after user authorization.
It retains the source binding and relocation blocker before the Host move. Readback of uncertain
preparation and cancellation/abandonment is defined in [Core lifecycle](core-lifecycle.md#cancellation).

A coordinator other than the moving task performs Handoff. The calling task cannot move itself.
Use the actual Host thread ID, which is distinct from Core task_id.

### handoff-start

Implementation: `packages/codex/lib/task-launch.mjs` — `beginTaskHandoff`.
[Complete handoff-start example](lifecycle-examples.md#host-handoff-start-start).

Copy launch/repository from the provisioned receipt and relocation ID from Core. Only
`should_dispatch:true` permits one Host call, using the complete returned `host_request`:

```js
const handoff_response = await tools.mcp__codex_app__handoff_thread(saved_handoff.host_request);
store("handoff_response", handoff_response);
text(handoff_response);
```

The producer supplies threadId and followUpPrompt. Persisted `handoff_dispatching` precedes the Host
call. `should_dispatch:false` means read the existing attempt; it does not permit another Handoff.

### handoff-result

Implementation: `packages/codex/lib/task-launch.mjs` — `recordTaskHandoff`.
[Complete handoff-result example](lifecycle-examples.md#host-handoff-result-record).

Forward the complete actual Host response, including its wrapper; the sample shows its inner shape.
Read `receipt.operation_status.host_operation_id` and `host_operation_revision`; valid values produce
`handoff_pending`, while a missing/malformed response produces `uncertain`. Read the known Host
operation using the Host status capability; no missing result authorizes redispatch. For example,
where the current Host exposes this schema:

```js
const status = await tools.mcp__codex_app__get_handoff_status({
  operationId: saved_receipt.operation_status.host_operation_id,
  afterRevision: saved_receipt.operation_status.host_operation_revision,
  waitMs: 30000
});
text(status);
```

Read its actual changed revision/status/destination before recording the next command.

### handoff-status

Implementation: `packages/codex/lib/task-launch.mjs` — `recordTaskHandoffStatus`.
[Complete handoff-status example](lifecycle-examples.md#host-handoff-status-pending).
[Complete handoff-status example](lifecycle-examples.md#host-handoff-status-succeeded).

Use the Host operation revision, not the Core revision. Record failed status in the same shape with
`status:"failed"` and the observed path or null. Output retains receipt/changed/relocation_id. Only
actual Host success proceeds to Core resolution; failure keeps the original binding and claims.

Resolve at the destination with the current blocked Action and every actual destination root:

See the complete [Core relocation resolution input](core-lifecycle.md#complete-relocation).

Core checks repository group, frozen base, equivalent content/surface and claim conflicts, then replaces
bindings together. Success is the complete Task in `result`; follow `result.current_action`.
`ACTIVE_TASK_CONFLICT` during creation does not authorize this process. Relocation is same-machine;
Git merge/rebase/commit/push and cross-machine transfers require their own operations/authority.



## Terminal presentation

Implementation: `internal/application/apply_action_results.go` — `applyDeliveryResult`.
Implementation: `packages/codex/lib/worktree-lifecycle.mjs` — `terminalCleanupDecision`.

For local branch modes, retain the original directory, branch and all modifications.
`cleanup-decision` with `surface:"current_session"` returns `not_applicable` for both deletion choices.
A later Task sees any still-uncommitted work as initial content; obtain its acceptance again.

On DONE/CANCELLED, present the actual source/base/base commit, task branch/current HEAD, worktree path,
clean/dirty state, current changed paths, performed checks and remaining risks. Example: “Task is DONE.
The change remains in codex/endpoint-field at /work/tasks/endpoint-field. The targeted check passed;
the worktree is dirty and remains available for review.” Replace every fact with current Core/Git data.
No terminal result implies a commit, push, pull request, Handoff or deletion.

### cleanup-decision

Read fresh Core/Git facts and query eligibility; this command grants no deletion authority:

[Complete cleanup-decision example](lifecycle-examples.md#host-cleanup-decision-keep).

Output members are `automatic_cleanup` (always false), `worktree_cleanup` and `branch_cleanup`.
Keep active, dirty, uncertain or unpushed resources under the returned decision. Managed-worktree
cleanup belongs to the Host; do not replace it with shell Git commands. An authorized terminal
Host-only Handoff does not prepare Core relocation or mutate a terminal Task.

### cleanup-worktree

Implementation: `packages/codex/lib/task-launch.mjs` — `cleanupCliTaskWorktree`.

Only for a verified terminal, clean CLI worktree after explicit deletion authorization. `terminal`
comes from a fresh Core result and `authorized` from that user decision; they are not default booleans.
Pass that complete terminal Task as `core_task`. Its saved effective branch, original provisioning
identity and HEAD must match this receipt and the live workspace. A missing stored workspace instance
proof (including older Codex receipts) requires manual review; never infer it from a branch name.

[Complete cleanup-worktree example](lifecycle-examples.md#host-cleanup-worktree-remove-worktree).

The helper saves the attempt before removal, verifies the exact receipt-owned repository/worktree,
and removes without force. Read changed/uncertain and receipt cleanup status. A requested-but-uncertain
attempt is inspected, not retried blindly. The branch remains until a separate decision.

### cleanup-branch

Implementation: `packages/codex/lib/task-launch.mjs` — `cleanupTaskBranch`.

After completed worktree cleanup and separate current branch-deletion authorization:

[Complete cleanup-branch example](lifecycle-examples.md#host-cleanup-branch-remove-branch).

Success records completed branch cleanup. The helper uses non-force `git branch -d`; an unmerged
branch remains on Git refusal. A missing identity, wrong repository, prior uncertain attempt or
missing authorization stops. Worktree deletion authorization alone does not supply this authorization.

## Installation and diagnosis commands

Implementation: `packages/codex/bin/taskbelay-codex.mjs` — `runCLI, writeStatusSuccess, writeLifecycleSuccess, writeSetupSuccess`.

Use lifecycle commands when the user requests installation/diagnosis/removal or a failure calls for
repair. They are not repeated preconditions for every Action. They use argv, not JSON on stdin.
The package supports `--json` only on status/setup/remove; unknown arguments exit 2. Operational errors
exit 1 with a `taskbelay-codex:` stderr message. Use the current executable's help before repair.

| Interaction and complete example | Result and next step |
| --- | --- |
| `taskbelay-codex --help` | Usage plus Host/artifact help; no setup, stdin or Core operation. |
| `taskbelay-codex status --json` | operation/status/changed/package_version/core_version/receipt_path/registration. Inspect readiness/registration; this is not Task state. |
| `taskbelay-codex --version` | Text `taskbelay-codex <package-version> (core <core-version>)`; versions are independent. |
| `taskbelay-codex setup --json` | Setup result from registration/configuration checks, including completed changes and next steps. If setup partially fails, report the actual completed changes and returned repair step. |
| `taskbelay-codex remove --json` | operation/status/changed/receipt_path/next_step. Stops matching WebUI and removes owned registration; read the npm uninstall handoff. Task/Git data remain. |
| `taskbelay-codex mcp` | Installed stdio server entry used by plugin registration. It streams MCP messages, not a one-shot JSON result; normal Skill work calls the registered tools instead of starting a second server. |

Implementation of registration: `packages/codex/plugin/.mcp.json`;
`packages/codex/lib/lifecycle.mjs` — `setupRegistration`, `removeRegistration`.
`TASKBELAY_DATA_DIR`, when used, is set before starting Codex and is shared with MCP, artifact commands
and Hook. It must be an existing canonical absolute directory. Help does not resolve it. A changed
launch environment takes effect in a new session. The internal Hook/host-check argv and message examples
are in [artifacts](artifacts.md#pretooluse-hook).

## Explicit launch supersession

Implementation: `packages/codex/lib/task-launch.mjs` — `supersedeTaskLaunch`, `resumeTaskLaunchSupersession`.

Read `host-launch status` and retain `receipt_digest`. After an explicit revised choice and a fresh
assessment, `host-launch supersede` takes the predecessor identity, `expected_receipt_digest`, a
concrete `reason` and a complete `replacement` prepare input with a distinct launch ID. It accepts
unexecuted launches, or failed preparations with recorded positive proof that no target call occurred.
A dispatch claim, partial effects or uncertainty requires reconciliation first; empty Host IDs alone
are not proof. The old receipt permanently retains one complete successor seed and becomes unusable.
Use the returned `prepare_input` for ordinary successor preparation. After interruption, read the
predecessor and call `supersede-resume` with its exact `supersession.seed_digest`; this only completes
the same successor and grants no Git or Host execution.

[Supersession request and result](launch-examples.md#host-supersede-replace).
[Resume the saved successor](launch-examples.md#host-supersede-resume-recover).

For an active Core Task, use the shared [prepared branch rename](core-lifecycle.md) contract.
Core confirms the effective branch while preserving the creation origin. Cleanup uses the effective
branch from `core_task`, including when another branch later reuses the creation name.
