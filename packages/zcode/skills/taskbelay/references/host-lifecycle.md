# ZCode lifecycle and recovery

Implementation: `packages/zcode/lib/lifecycle.mjs`, `packages/zcode/lib/workspace.mjs` and `packages/zcode/bin/taskbelay-zcode.mjs`.

## Installation and diagnosis commands

`taskbelay-zcode setup --json` verifies the self-contained package, Core, plugin and marketplace identities and saves the local preparation receipt. It returns `action_required` with exact ZCode UI instructions. Add the reported marketplace file in Settings → Plugins → Create → Add marketplace, install/enable the plugin, and start a new session. Confirm that the Skill, MCP server and Write/Edit Hook are present. CLI status cannot observe ZCode registration or loaded state, and never calls a nonexistent ZCode plugin-management CLI.

`status --json` is read-only. `partial` means local setup is incomplete or its package/receipt no longer matches. `action_required` means local preparation or removal succeeded but the described UI operations remain outside CLI observation. The `registration.host` value is `unverified`. Do not treat either status as automatic proof of a running Host, working model authentication or completed Task.

For a changed package, refresh the marketplace in ZCode. ZCode compares the marketplace entry's version with the installed plugin manifest. If the version is unchanged, uninstall and reinstall the plugin to replace its cached contents. Start a new session because Hook configuration is captured at session startup.

The unified `taskbelay` manager supports install, upgrade, repair, reinstall, status, doctor and uninstall through `--host zcode`. Use the source/local package route associated with the installation; publication is separate. `TASKBELAY_DATA_DIR` must be the same canonical existing directory for MCP, Hook and helpers. Normal operations retain Task data and unrelated ZCode settings.

## Removal and explicit confirmation

`taskbelay-zcode remove --json` saves a `removal_required` receipt and reports `action_required`; it cannot remove or observe ZCode's private plugin cache. Uninstall the plugin and remove its marketplace in ZCode UI, then close affected sessions. The first manager uninstall retains the npm Adapter and this pending-removal record so the confirmation helper remains available.

Only after the user explicitly confirms those UI operations and closed sessions, `taskbelay-zcode remove --confirm-host-removed --json` clears the owned receipt and reports `registration.host=user_confirmed_removed`. This is a human confirmation, not an automated Host check. Then repeat `taskbelay uninstall --host zcode --yes`; an Adapter-only installation uses `npm uninstall --global taskbelay-zcode`. If the package was manually removed first, use the same local package to run this confirmation helper. Do not infer confirmation from missing files, a stopped process or elapsed time. Shared data reset remains a separate, explicitly confirmed manager operation.

## Startup and uncertain operations

Read `host-launch status` with the saved `launch_id` before acting on incomplete or uncertain preparation. Preserve receipt identity and complete original output. A launch receipt has no Core process cursor and no ZCode session cursor. `open`/`resume` produce UI guidance only; they do not prove Core creation or that a session opened. If an earlier Core write result is uncertain, recover that exact operation instead of creating another Task.

The first recovery read sends this complete body on [Host command stdin](transport.md) and returns the complete saved receipt shape from [preparation](admission.md#host-launch-requests), including the current repository phase:

<!-- example:host-launch status request -->
```json
{"launch_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
```

If `prepare` fails after writing its launch receipt, the CLI exits 1, leaves stdout empty and reports `Saved launch_id: <actual ID>` and `Receipt: <actual path>` on stderr. Read that exact `status` before another Host action. An existing operation lock is never reclaimed automatically, including after its owner stops. Inspect the receipt and workspaces, establish that the prior caller stopped, and explicitly clear only that lock when appropriate. Clearing it does not make an uncertain operation safe to replay.

## Relocation

Obtain `taskbelay_prepare_task_relocation` and retain its complete successful `result` from Core first. With user authorization, call `host-launch relocate` with `launch_id`, `relocation_id`, `core_preparation` (that exact `result`, containing `relocation_id` and `task`), `destinations` (all `repository_key`/`repository_path` pairs), and `authorized=true`. The helper checks the bound Task, ZCode identity, current relocation blocker and source workspace paths. Only all-dedicated provisioned workspaces can move. Each move is saved; uncertain or partial moves remain for inspection.

Construct the complete Host request by passing the exact retained Core result as `core_preparation` without summarizing or editing it:

```js
const input = {
  launch_id: retained_launch_id,
  relocation_id: core_result.relocation_id,
  core_preparation: core_result,
  destinations: [{repository_key: "primary", repository_path: confirmed_destination}],
  authorized: true
};
```

The CLI receives `JSON.stringify(input)` on stdin. A successful Host move returns:

<!-- example:host-launch-output relocate success -->
```json
{"relocation_id":"relocation-from-core","relocation_destinations":[{"key":"primary","repository_path":"/work/relocated-project"}],"core_resolution_required":true}
```

Pass returned `relocation_id` and `relocation_destinations` unchanged to Core's relocation blocker resolution; each returned destination uses `key`/`repository_path`. Until Core verifies destinations, its original binding remains authoritative. Open the actual destination workspaces in ZCode and resume the same Task. Do not relaunch a guessed CLI or create another Task.

After a successful move, repeating the same relocation ID with its original preparation and destinations only reads back the retained outcome after checking destination identity; it never moves twice. A later relocation requires a new actual Core preparation from the current destination workspaces and a newer Core revision. A still-incomplete prior move remains blocked for inspection.

## Terminal operations and cleanup

Cancel/abandon through Core using the shared references. Do not infer DONE from ZCode stopping. After an actual terminal Task, `host-launch cleanup-worktree` receives `launch_id`, `repository_key`, `terminal=true`, `authorized=true`, `core_task` (the complete actual terminal Core Task) based on real state and explicit deletion authorization. It removes only clean receipt-owned dedicated worktrees without force. `cleanup-branch` requires its own authorization after worktree removal and uses non-force deletion. Never automatically delete active, dirty, unpushed or uncertain resources. Local directories and branches remain.

Each cleanup call takes this complete body with a separate actual user authorization; choose the operation name `cleanup-worktree` or `cleanup-branch` and retain the returned updated receipt:

```js
const cleanup_input = {
  launch_id: saved_launch_id,
  repository_key: selected_repository_key,
  terminal: true,
  authorized: actual_cleanup_authorization,
  core_task: terminal_core_task
};
```

## Terminal presentation

After Core returns `DONE` or `CANCELLED`, report its actual outcome, completed work, verification results and remaining limitations. Preserve the distinction between automatic checks, Host observations and user-performed checks. Present only the Git or cleanup actions the user actually authorized; terminal state does not automatically commit, push or delete anything.

## Prepared branch rename

Follow the shared [branch rename contract](core-lifecycle.md) before executing one authorized
non-force `git branch -m`. Core owns the pending rename, verification and effective binding; the
Host launch receipt preserves its creation branch. For relocation or cleanup, forward complete
actual Core results. The helpers verify repository key, original receipt, workspace instance,
effective branch and HEAD; missing or mismatched facts stop the operation. Never use the old
receipt target to choose a branch for deletion. Recreating that old name must not make it a cleanup target.
