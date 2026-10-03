# Claude Code guide

[中文](CLAUDE.md) | [English](CLAUDE_en.md)

This guide covers installing the Claude Code integration, starting or resuming tasks, maintenance and removal. See the [support matrix](SUPPORT-MATRIX_en.md) for public-package and source availability.

## Choose an installation method

The Claude Adapter is published as `taskbelay-claude@latest`. It requires Node.js `>=24`, Git and Claude Code `>=2.1.270`. Follow Claude Code's sign-in prompts before starting development work. Runtime targets are Windows x64 and macOS arm64; the support matrix identifies what has actually been verified.

### Install from npm

```sh
npm install --global taskbelay@latest
taskbelay install --host claude --yes
taskbelay status --host claude
```

The manager installs the Adapter. Reload Claude plugins or start a new session, then review the permission prompts. Check the support matrix for verified environments.

### Install from this repository

This route also requires Go `>=1.26` and pnpm `>=11 <12`. Run commands from the repository root. See the [contribution guide](../CONTRIBUTING.md) for development setup.

```sh
pnpm taskbelay:local -- install --host claude --yes
```

This builds local packages and installs the Claude Adapter using a temporary manager. It does not upgrade an existing global `taskbelay` command. Stay in the repository root to inspect the installation:

```sh
node packages/taskbelay/bin/taskbelay.mjs status --host claude
node packages/taskbelay/bin/taskbelay.mjs doctor --host claude
```

### Install a prebuilt Adapter package

If a maintainer supplies a local `.tgz`, you do not need to compile Go. Replace the placeholder with the actual package path:

```sh
npm install --global "<path-to-taskbelay-claude.tgz>"
taskbelay-claude setup --json
taskbelay-claude status --json
```

This package provides `taskbelay-claude`, not the global `taskbelay` manager or desktop pet. See the [desktop pet guide](DESKTOP-PETS_en.md) for complete Windows development packages. Maintainers can find source build commands in the [scripts guide](../scripts/README_en.md).

## Verify installation

The installer registers the plugin in Claude user scope. Its name is `taskbelay-claude`, from the `taskbelay-claude-local` marketplace. Reload plugins or start a new Claude session, then review Claude's plugin and permission prompts.

The `status` from `taskbelay-claude status --json` should be `ready`. For `partial`, inspect installer output and diagnostics before starting a new TaskBelay task. `ready` describes installation checks; it does not mean Claude is signed in or a development task is complete.

## Start a task

Open the code repository you intend to change and send this in Claude:

```text
/taskbelay-claude:taskbelay Add failed-login rate limiting, changing only authentication files.
```

Claude first assesses the request and offers direct work or TaskBelay. TaskBelay defaults to a new branch in the current directory; you can also select the current branch or a dedicated worktree. Specify the branch and whether existing uncommitted changes belong to the task. Dedicated worktrees also require a source, base branch and destination.

A task can include up to eight explicitly selected repositories. Task creation waits until all are prepared. Before implementation, review and approve the requirements, design, work items, expected files and verification plan. Changes to the plan or file scope require approval of the revised proposal.

Explicitly request OpenSpec or Spec Kit if needed; otherwise use plain development. Resolve missing method tools instead of treating their absence as completed work.

## Resume, cancel and clean up

Return to the original working directory and Claude conversation and send:

```text
/taskbelay-claude:taskbelay Continue the saved task; first explain its current state and remaining work.
```

If startup fails, the outcome is uncertain or the directory was replaced, retain the original error and session information so Claude can inspect existing records. Do not recreate the task, delete its worktree or clear data merely to retry.

To cancel, explicitly ask Claude to cancel the current TaskBelay task while retaining files. Completion and cancellation leave files and branches in place. Request dedicated-worktree cleanup and branch cleanup separately; exiting Claude does not cancel the task.

To move a workspace, first ask Claude to relocate the current task. This is available only when every repository uses a dedicated worktree. Current-directory modes resume in place. Follow the returned recovery instructions for both directories if a move is incomplete.

## File scope and permissions

The trusted write check covers Claude Write, Edit and NotebookEdit operations. For an unplanned file, choose whether to allow that operation, revise the plan or restore the file. Allowing a path does not override Claude permissions.

Shell or external-program changes can occur before task checks observe them. Do not switch tools to bypass a rejected write.

## Progress and maintenance

Source-install users should keep using the source entry from the repository root:

```sh
node packages/taskbelay/bin/taskbelay.mjs webui start
node packages/taskbelay/bin/taskbelay.mjs webui stop
pnpm taskbelay:local -- repair --host claude --yes
```

WebUI can filter Claude Code tasks and display their saved state. Use `taskbelay` to maintain an npm installation; source maintenance rebuilds local packages through the repository entry point.

The pet also requires an installed desktop application; Adapter `ready` status alone does not establish pet availability. See the [desktop pet guide](DESKTOP-PETS_en.md).

## Removal and data

End the relevant Claude sessions first. If WebUI is running, stop it with the same manager entry used to start it. Source-install users can run:

```sh
node packages/taskbelay/bin/taskbelay.mjs uninstall --host claude --yes
```

With only the Adapter package and no source manager:

```sh
taskbelay-claude remove --json
npm uninstall --global taskbelay-claude
```

Ordinary maintenance and removal retain task data and unrelated Claude settings. Use the manager's `factory-reset` only when you intend to clear data and have confirmed its exact listed directories. Ordinary removal does not require it.

`CLAUDE_CONFIG_DIR` selects Claude settings. `TASKBELAY_DATA_DIR` selects an existing canonical absolute data directory; keep it consistent when starting Claude and manager commands. Task data defaults to `~/.taskbelay/data` on macOS or `%LOCALAPPDATA%\taskbelay\data` on Windows.

See the [command reference](COMMANDS_en.md) for further operations and [project status](PROJECT-STATUS_en.md) for recorded checks and unverified areas.

## Continued revision and Task branch renaming

The same Task can continue through requirements, design and plan revisions beyond 32 older baseline
references. Retained references remain available through pagination; unsaved full documents cannot
be reconstructed. To rename an active Task branch, have Core prepare one repository's rename, let
the authorized Host run one non-force `git branch -m`, then have Core verify and confirm it. The
worktree, HEAD, index, contents and other repositories must stay unchanged. Cancel preparation only
while the original facts remain unchanged; after a rename with a missing response, read back before acting.

Later relocation and terminal cleanup use the effective branch confirmed by Core; the creation branch remains in the origin. Cleanup requires the complete terminal `core_task`; Claude relocation also requires the actual `core_preparation`.
See [workspace sources and recovery](WORKTREE-SOURCES_en.md) for exact conditions and recovery steps.
