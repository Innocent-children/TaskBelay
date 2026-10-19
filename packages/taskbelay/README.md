<p align="center">
  <img src="https://raw.githubusercontent.com/Innocent-children/taskbelay/main/packages/webui/src/assets/taskbelay-app-icon-light.svg" width="112" height="112" alt="TaskBelay" />
</p>

# taskbelay

This package installs and maintains TaskBelay integrations and provides the local WebUI and desktop pet commands. Development tasks are started from the chosen Host conversation.

## Installation channels

The public package installs the integrations available in its release:

```sh
npm install -g taskbelay@latest
taskbelay
```

Check the [support matrix](https://github.com/Innocent-children/taskbelay/blob/main/docs/SUPPORT-MATRIX_en.md) for verified Host and platform coverage. Follow the [Claude](https://github.com/Innocent-children/taskbelay/blob/main/docs/CLAUDE_en.md) or [ZCode](https://github.com/Innocent-children/taskbelay/blob/main/docs/ZCODE_en.md) guide for their setup and required UI steps.

The manager targets macOS arm64 and Windows 10/11 desktop x64. Other OS/CPU combinations are rejected by its runtime selection. Source setup and build requirements are documented separately from public installation.

## Inspect and maintain an installation

Run `taskbelay` for an operation-first keyboard menu: **Upgrade**, **Install into Agents**, **Check and repair**, **Open Control Center**, **Desktop pet**, **Installation information**, **Uninstall and delete data**, and **Exit**. Arrows move, Enter confirms, Space toggles multiple selections, A selects all, Esc goes back with choices retained, and Ctrl+C exits. `NO_COLOR` keeps keyboard navigation; non-TTY use never prompts. Results remain visible until the next choice.

Upgrade resolves each installed component separately and shows current and target versions. Lookup errors remain errors, not “up to date”. When everything is current, choose Home or Check again. R rechecks; V chooses one component and then its version. Only local installations offer L for a complete launcher distribution. Text fields support cursor movement, Ctrl+U and inline validation; cancelling confirmation runs nothing and prints no approval command. Install accepts any Agent subset; DeepSeek then offers multiple Profiles and N to enter another name. Diagnosis shows a concise summary with optional details and relevant next steps; reinstall remains a secondary repair action.

```sh
taskbelay status --host all
taskbelay doctor --host codex
taskbelay install --host codex --host claude --yes
taskbelay upgrade --host all --all-known-profiles --self --yes
```

Repeat `--host` for `codex`, `deepseek`, `claude`, or `zcode`; `all` cannot be combined with an individual Agent. Repeat `--profile` for DeepSeek, whose default is `web`, or use `--all-known-profiles` for every managed Profile.

| Operation | Result |
| --- | --- |
| `status` | Installed versions, Agent availability, issues, launcher channel and local paths. |
| `doctor` | Read-only installation and configuration checks with concrete next steps. |
| `install` | Keep installed versions and install missing plugins. |
| `upgrade` | Update installed plugins to latest stable; add `--self` to update the launcher last. |
| `repair` | Restore the current installation, retaining its version by default. |
| `reinstall` | Replace the current installation even when checks pass; retain data. |
| `uninstall` | Remove selected plugins and registrations; retain data and configuration. |
| `delete-data` | Clear tasks, configuration and pet preferences; keep plugins and registrations, and recreate empty data directories at the original paths. |
| `delete-all` | Remove all managed plugins and confirmed user data; keep the launcher and desktop application. |

Use `--version latest|<x.y.z>` for versioned operations. Downgrades need the separate plan-specific `--confirm-downgrade` token; `--yes` alone is insufficient. Complete local development distributions use their bundled digest-verified plugin artifacts.

`taskbelay upgrade --self` updates only the current global launcher. npm installations use npm; a local development installation requires `--self-package <absolute-complete-distribution.tgz>` and never silently changes channel. The local distribution must contain all four plugin archives and valid digests. The launcher is replaced last; the old menu exits and asks you to run `taskbelay` again. Temporary source entrypoints cannot update a different global installation.

```sh
taskbelay repair --help
taskbelay status --host all --json
taskbelay delete-data --json
taskbelay delete-all --json
```

`--json` returns one complete result and never asks for input. Confirmation-required results provide the exact command to approve the displayed plan. Progress identifies actual components and steps; partial failures retain completed actions and the operation ID. Read current state before retrying.

## Activation and failures

After installation, review/trust the TaskBelay hook in Codex, restart the selected DeepSeek Profile, or reload Claude plugins/start a new Claude session. For ZCode, follow the returned Settings → Plugins installation/enablement steps and start a new session. Local preparation reports `action_required`; it cannot verify that ZCode loaded the plugin. Follow the actual prompts for the installed Host.

An absent optional Host is informational in an all-Host diagnostic when another integration is healthy. Installation problems retain available package and registration information for repair. Codex and Claude removal can handle their owned registration after the Adapter package has disappeared; unknown ownership is not permission to remove another installation. Existing unmanaged DeepSeek contributions require explicit `--adopt`.

ZCode removal takes two steps: ordinary uninstall retains the Adapter while you remove its plugin and marketplace in ZCode and close affected sessions. Run `taskbelay-zcode remove --confirm-host-removed` to record that completed action, then repeat manager uninstall to remove the package. Shared-data reset is blocked while the ZCode package or record remains, because cached Host processes cannot be verified automatically.

Doctor checks existing user configuration through an installed Core. If no usable Core is available, it reports that configuration validation could not be completed. A missing configuration file uses Core defaults.

If maintenance fails, keep its output. The result identifies completed work and the next recovery command. Do not remove files manually to make the status appear clean.

## Task data and deletion

Default Task data lives at `~/.taskbelay/data` on macOS or `%LOCALAPPDATA%\taskbelay\data` on Windows. An explicit `TASKBELAY_DATA_DIR` must name an existing canonical directory and remain consistent across Agents and management commands.

The cleanup menu distinguishes **Uninstall plugins (keep data)**, **Delete data (keep plugins)** and **Delete everything (plugins + data)**. Plugin uninstall supports individual Agents and Profiles; shared Task data does not belong to one Agent, so both deletion operations cover every managed Agent and Profile and show the exact directories. Neither removes project code, Agent conversation history, other plugins, earlier recovery directories, the launcher or the desktop executable.

Both data operations clear Task records, TaskBelay configuration, pet preferences/imported appearances and ordinary completed maintenance records. Delete data retains plugin packages, Agent registrations, required ownership records and data-directory bindings. It recreates empty default and explicitly confirmed data directories and restores default preferences; restart Agent sessions before creating new tasks. The current deletion receipt and unresolved maintenance records remain readable.

Deletion requires its displayed `--confirm-delete <token>`, bound to the current plan. Default recoverable deletion uses macOS Trash or `%LOCALAPPDATA%\taskbelay\trash` on Windows; the latter is not Windows Recycle Bin. Permanent deletion additionally requires `--permanent --confirm-permanent <token>`. Each extra data directory requires `--confirm-explicit-data <absolute-path>`. `--yes` approves none of these decisions. Directories that contain retained installation resources, or overlap other cleanup targets, are rejected.

Cleanup stops the pet, local WebUI and identifiable STDIO Core processes, then checks saved active Tasks for running or uncertain executions through a compatible Core. Those executions must be recovered first. An existing database without a usable compatible Core blocks deletion; install a compatible plugin to inspect it. A process that reconnects, a changed resource or an unfinished plugin removal stops cleanup and retains the remaining data.

ZCode's private plugin cache cannot be completely enumerated. Close all ZCode windows and sessions and explicitly confirm `--confirm-zcode-closed`; the manager still checks identifiable processes. Delete data keeps its installed plugin. Delete everything also requires completing ZCode's actual UI removal and recorded removal confirmation; closing windows alone does not uninstall the plugin.

## WebUI and desktop pet

With a configured Adapter that supplies Core:

```sh
taskbelay webui start
taskbelay webui open
taskbelay webui status
taskbelay webui stop
```

WebUI shows existing tasks, results, blockers and supported recovery operations. Start a new development task in the Host conversation.

The desktop pet additionally requires the desktop application supplied by an appropriate complete package. Installing an Adapter alone does not provide it.

```sh
taskbelay pet start
taskbelay pet stop
```

These are the only pet command forms; `pet status` and `pet start --json` are not supported. Stopping the pet leaves tasks and WebUI intact. Maintenance, including launcher self update, refreshes a bundled desktop application while retaining settings and imported appearances. Data deletion removes preferences and appearances while preserving the application.

See the [desktop guide](https://github.com/Innocent-children/taskbelay/blob/main/docs/DESKTOP-PETS_en.md) for installation and controls, and the support matrix for verified platforms and signing status.

## Automation results

Lifecycle exit codes are `0` for success/no changes, `1` for check or execution failure, `2` for invalid arguments, `3` for required/declined confirmation, `4` for unmet authorization, `5` for partial execution or failed final verification, and `130` for an interrupted running operation. Interrupted or uncertain replacement results report `changed: null`; inspect status before retrying. Invalid WebUI arguments return `2`; pet commands use `0`, `1` and `2` for success, runtime failure and invalid arguments.

Exact options, structured results and recovery inputs are in the [command reference](https://github.com/Innocent-children/taskbelay/blob/main/docs/COMMANDS_en.md).
