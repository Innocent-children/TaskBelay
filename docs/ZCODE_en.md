# ZCode guide

[中文](ZCODE.md) | [English](ZCODE_en.md)

This guide covers the native TaskBelay plugin for Zhipu ZCode. The Adapter is published as `taskbelay-zcode@latest`. Targets are Windows x64 and macOS arm64; native macOS validation remains pending. See the [support matrix](SUPPORT-MATRIX_en.md) for actual records.

## Install and enable

Use Node.js `>=24`, Git and a ZCode installation providing native plugins, Skills, MCP and Hooks. Complete ZCode's own sign-in and permission setup.

Install from npm:

```sh
npm install --global taskbelay@latest
taskbelay install --host zcode --yes
taskbelay status --host zcode
```

The manager returns the local marketplace path and required ZCode UI steps. Check the support matrix for verified environments.

Source installation also requires Go `>=1.26` and pnpm `>=11 <12`. Run from the repository root:

```sh
pnpm taskbelay:local -- install --host zcode --yes
node packages/taskbelay/bin/taskbelay.mjs status --host zcode
node packages/taskbelay/bin/taskbelay.mjs doctor --host zcode
```

The source entry builds local packages through the repository manager. If a maintainer supplies a local tarball, install it without compiling Go:

```sh
npm install --global "<path-to-taskbelay-zcode.tgz>"
taskbelay-zcode setup --json
taskbelay-zcode status --json
```

`setup` validates and prepares the local plugin source, returning `action_required` and `next_steps`. Complete these steps in ZCode:

1. Open **Settings → Plugins → Create → Add marketplace**.
2. Use the returned local marketplace path, then install and enable `taskbelay-zcode`.
3. Start a new session to activate plugin hooks; review plugin, MCP and permission prompts.
4. Enter `/` in the input, open **Skills** and select `taskbelay`. Use the actual name shown by ZCode.

The Adapter has no reliable interface for observing ZCode's plugin loading or cache state. After successful preparation, `status` therefore remains `action_required`, with `registration.host=unverified`. This does not establish sign-in or a verified model session. Unified `doctor` exits 1 for this state because Host readiness remains unverified; `partial` means local preparation or validation is incomplete and requires resolving diagnostics first. See the official [ZCode plugin guide](https://zcode.z.ai/en/docs/plugin) and [Hooks guide](https://zcode.z.ai/en/docs/hooks).

## Start and resume tasks

Open the repository you intend to change, select the TaskBelay Skill and send:

```text
Use TaskBelay to add failed-login rate limiting, changing only authentication files.
```

ZCode first assesses the request and offers direct work or TaskBelay. TaskBelay defaults to a new branch in the current directory, with current-branch and dedicated-worktree alternatives. Specify the target branch and whether existing uncommitted contents belong to the task. Dedicated worktrees also require a source, starting branch and destination. A task can include up to eight explicitly selected repositories; all must be prepared before Core Task creation.

Review and approve the complete requirements, design, work items, expected files and verification plan before implementation. When another workspace is needed, the Adapter returns prepared directories and a continuation prompt. Open those directories through ZCode's actual interface and grant the required access. A returned descriptor does not establish that a workspace or session was opened.

To resume, return to the original directory, select the same Skill and explicitly request continuation of the saved task. Read existing state and launch records first; an uncertain result does not permit duplicate task or worktree creation. Restore a missing or replaced directory instance, or explicitly abandon the task.

You may request cancellation while retaining files. Completion and cancellation release task claims and retain files and branches; dedicated-worktree and branch cleanup need separate authorization. Workspace relocation requires every repository to use a dedicated worktree; local modes resume in place. Obtain and retain Core's complete preparation result before moving, verifying the same Task and all source directories. Read existing records when the outcome is uncertain instead of repeating the move. Another relocation can start after Core verifies the completed one.

## File scope and permissions

The native PreToolUse Hook checks targets before ZCode **Write** and **Edit** operations. Unplanned writes and unavailable checks deny the operation. Follow Core's instructions for allowing one operation, revising the plan or restoring files. Allowing a path does not override ZCode permissions. Hook updates require a new session.

Shell and external-program writes may happen before Core observes them. Do not switch tools to bypass a rejection. Use OpenSpec, Spec Kit and code indexes only where actually available; a missing tool is not completed work.

## Maintenance and progress

Source-install users should continue from the repository root:

```sh
pnpm taskbelay:local -- repair --host zcode --yes
node packages/taskbelay/bin/taskbelay.mjs webui start
node packages/taskbelay/bin/taskbelay.mjs webui stop
```

Maintenance updates the local source. Follow `next_steps` to refresh or reinstall the plugin in ZCode and start a new session; source updates at the same version also require cache refresh. WebUI can filter ZCode Tasks and display saved progress. The desktop pet additionally requires the desktop app; installing an Adapter alone does not provide it. See the [desktop pet guide](DESKTOP-PETS_en.md).

## Removal and retained data

End relevant ZCode sessions, stop WebUI instances started through this Adapter, then request removal through the source manager:

```sh
node packages/taskbelay/bin/taskbelay.mjs uninstall --host zcode --yes
```

With only the Adapter package, first run `taskbelay-zcode remove --json`. Ordinary removal returns `action_required`; the unified manager also retains the Adapter package and removal record so the confirmation command remains available. It cannot confirm deletion of ZCode's cache. Uninstall the plugin and remove its marketplace in the ZCode UI, then close the relevant sessions.

After those UI actions have actually finished, run these commands while the global package is still present to confirm Host removal and uninstall the Adapter package:

```sh
taskbelay-zcode remove --confirm-host-removed --json
npm uninstall --global taskbelay-zcode
```

This returns `absent` and `registration.host=user_confirmed_removed`, recording user confirmation rather than automatic observation. After confirmation, you may alternatively repeat `uninstall --host zcode --yes` with the same manager entry above to remove the package. Ordinary maintenance and removal retain Task data and unrelated settings. The manager rejects `factory-reset` while a ZCode package or installation record remains because it cannot reliably enumerate Core processes in ZCode's cache. If you intend to clear data, finish UI removal, close sessions, confirm removal and uninstall the global package before using the manager's separate reset flow.

`TASKBELAY_DATA_DIR` selects an existing canonical absolute data directory and must match across ZCode, MCP and the manager. Defaults are `~/.taskbelay/data` on macOS and `%LOCALAPPDATA%\taskbelay\data` on Windows.

See the [command reference](COMMANDS_en.md) for all entries and [project status](PROJECT-STATUS_en.md) for actual checks and remaining acceptance work.

## Continued revision and Task branch renaming

The same Task can continue through requirements, design and plan revisions beyond 32 older baseline
references. Retained references remain available through pagination; unsaved full documents cannot
be reconstructed. To rename an active Task branch, have Core prepare one repository's rename, let
the authorized Host run one non-force `git branch -m`, then have Core verify and confirm it. The
worktree, HEAD, index, contents and other repositories must stay unchanged. Cancel preparation only
while the original facts remain unchanged; after a rename with a missing response, read back before acting.

Later relocation and terminal cleanup use the effective branch confirmed by Core; the creation branch remains in the origin. Cleanup requires the complete terminal `core_task`; missing or mismatched facts reject cleanup.
See [workspace sources and recovery](WORKTREE-SOURCES_en.md) for exact conditions and recovery steps.
