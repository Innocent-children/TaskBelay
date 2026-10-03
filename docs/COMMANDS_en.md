# TaskBelay Command Reference

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

> Most users only need to install the unified entry, run `taskbelay`, and use the corresponding
> selector in their Host. Other commands are mainly for diagnosis, recovery, and integration work.

This document lists every currently supported public or managed TaskBelay command entrypoint. The
command surface is derived from implementation: unified lifecycle commands from
`packages/taskbelay/package.json` and its CLI, Codex commands from `packages/codex/package.json`
and `packages/codex/bin/taskbelay-codex.mjs`, Claude commands from `packages/claude/bin/taskbelay-claude.mjs`, ZCode commands from `packages/zcode/bin/taskbelay-zcode.mjs`, DeepSeek lifecycle commands from the DSH CLI used by the
DSH lifecycle tests, Core commands from `cmd/taskbelay/main.go`, and MCP tools from the closed
catalog under `internal/mcp/`.

Public installation examples select npm's `latest` dist-tag so they install the current stable
package. Exact product versions remain in machine-readable release records.

The current source launcher and bundled Core accept exactly two runtime pairs: `darwin-arm64` and
`win32-x64`. The `@latest` commands below still describe the current npm stable channel. Validate the
Windows 10/11 desktop x64 source capability with packages built from this repository until an
explicitly confirmed release places those artifacts on the stable channel.

## Recommended entry for most users

```bash
npm install -g taskbelay@latest
taskbelay
```

After installation, Codex uses `$taskbelay-codex:taskbelay <task description>` and DeepSeek Harness
uses `/taskbelay <task description>`; Claude Code uses `/taskbelay-claude:taskbelay <task description>`. These are conversational Host selectors, not shell commands. In ZCode, select `taskbelay` from the input’s `/` → Skills menu before describing the task; see the [ZCode guide](ZCODE_en.md).

## Unified Adapter lifecycle

`taskbelay` provides one Host-neutral lifecycle and Control Center entry:

```bash
npm install -g taskbelay@latest
taskbelay
```

The supported operations are `status`, `doctor`, `install`, `upgrade`, `repair`, `reinstall`, `uninstall`, and
`factory-reset`. Host is `codex|deepseek|claude|zcode|all`; the default DeepSeek Profile is `web`. Ordinary uninstall, upgrade,
repair, and reinstall preserve configuration and Task data. Factory reset requires the token bound to the current
plan; `--yes` alone has no data-cleanup authority. Default cleanup moves data to the user's Trash on macOS and to the
recoverable `%LOCALAPPDATA%\taskbelay\trash` quarantine on Windows; the Windows target is not the system
Recycle Bin. Permanent removal requires another confirmation.
The Codex global package is observed independently from its receipt and Plugin registration. Even when registration
is already absent, `uninstall` and `factory-reset` still remove an installed global package.
Package, Plugin, and receipt version differences left by an upgrade do not prevent removal of the same owned Codex registration; paths and sources must still match.
When the entire Codex Adapter package is missing, the unified entry reads the remaining receipt, removes the matching registration through native Codex commands, and verifies readback without reinstalling the Adapter.
Cleanup with a missing Core proceeds only without a WebUI runtime receipt. A remaining or unreadable runtime record, or a failed stop, preserves the remaining resources.
The interactive interface reads the current locale: `zh*` uses Simplified Chinese and every other locale uses
English. JSON output remains language-neutral.
During install, upgrade, repair, and reinstall, text modes show each Host action and completed package, registration,
artifact, and readiness step; `--json` omits these progress lines.

| Entry | Purpose |
| --- | --- |
| `npm install -g taskbelay@latest` | Install the public `taskbelay` command globally. |
| `taskbelay` | Open the interactive lifecycle menu. |
| `taskbelay status\|doctor --host codex\|deepseek\|claude\|zcode\|all` | Inspect or diagnose without mutation. |
| `taskbelay install\|upgrade\|repair\|reinstall --host ... [--profile web] [--version latest] --yes` | Perform ordinary maintenance while preserving configuration and Task data. |
| `taskbelay install\|repair --host deepseek\|all --adopt ...` | Adopt an existing identity-verified DeepSeek Profile contribution; other operations and Codex-only targets reject `--adopt`. |
| `taskbelay install\|upgrade\|repair\|reinstall ... --confirm-downgrade <token>` | Explicitly confirm a downgrade with the token from the current plan when the target is older than the installed version. |
| `taskbelay uninstall --host ... [--all-known-profiles] --yes` | Remove selected Adapters while preserving configuration and Task data; Codex first stops the matching WebUI safely and retains registration and package state if that stop fails. |
| `taskbelay factory-reset --host all --all-known-profiles` | Produce a current-state-bound reset plan/token; `--yes` has no cleanup authority. |
| `taskbelay factory-reset ... --confirm-reset <token> [--reinstall]` | Move confirmed data to Trash and optionally perform a clean reinstall. |
| `taskbelay factory-reset ... --confirm-explicit-data <absolute-path>` | Confirm one explicit `TASKBELAY_DATA_DIR` listed by the plan; repeat the option for multiple directories. |
| `taskbelay factory-reset ... --permanent --confirm-reset <token> --confirm-permanent <token>` | Permanently remove the plan's exact targets; both the reset token and a separate permanent-removal token are required. |
| `taskbelay webui start\|open\|status\|stop` | Select and verify Core from an installed Adapter, then manage the shared local Control Center; `start` may create a missing default data directory with mode `0700` on macOS or inherited user-profile/LocalAppData ACLs on Windows. The other commands create nothing. |
| `--json` / `--plain` | Select one JSON object or ANSI-free plain output. |

### Lifecycle command behavior

The menu reads installation state before offering Adapter installation, maintenance, Control Center and pet entries. It supports input retry, back and exit, and returns to the menu after an operation. Without a terminal, bare `taskbelay` prints help. `taskbelay <lifecycle-command> --help` explains options and examples.

| Command | Target version and repeated execution |
| --- | --- |
| `install` | Keeps installed Adapter versions by default; missing installations use `latest`. A ready Adapter at the matching version is not replaced. |
| `upgrade` | Selects `latest` by default; a ready Adapter at the target version is not replaced. |
| `repair` | Repairs the current Adapter version by default, restoring damaged files and same-version owned registration. A healthy Adapter needs no repair. |
| `reinstall` | Reinstalls the current version by default on every invocation, preserving configuration and Task data. |
| `uninstall` | Already-removed Adapters need no further action; configuration and Task data are preserved. |
| `factory-reset` | Repeating completed cleanup is a no-op; actual cleanup targets still require confirmation of the current plan. |

Explicit `--version` selects a target. Every version-replacement command requires `--confirm-downgrade` for a downgrade; ordinary `--yes` is insufficient. Local development distributions always use their verified bundled versions and artifacts, replacing their contents during maintenance.

Before execution, the plan shows actions, current/target versions, resource paths and data handling. JSON never prompts: required confirmation returns `confirmation` and a copyable `next_step`. Explicit data-directory approval is checked before removing any Adapter. Cleanup directories bind canonical paths, filesystem identity and permissions, allowing managed shutdown to remove runtime records; individual file targets also bind size and modification time.

Installation, upgrade, repair and reinstall maintain Adapters. When the manager package contains the desktop pet app for the current platform, these operations also refresh its copy in the user directory while preserving settings and appearance assets. This application update still runs when the Adapter is healthy and needs no replacement. Update the public launcher and its bundled apps with `npm install -g taskbelay@latest`.

`status` retains absent targets and reports Host availability, Adapter/Core versions and issues. `doctor` adds installation and configuration checks and exits nonzero on failure. With all Hosts selected, an absent optional Adapter is informational when another Adapter is healthy. Codex self-check failures retain npm installation metadata for repair; DeepSeek checks Profile contribution, the managed receipt and the actual Core. An existing unmanaged DeepSeek contribution requires explicit `--adopt`.

`doctor` validates existing user configuration through the selected Core's `config validate` command. Without a usable Core, it reports that semantic validation is unavailable rather than passing the check. An absent configuration file uses Core defaults. Factory reset initializes configuration with `{}`; ordinary maintenance preserves valid existing configuration.

After reset confirmation, the manager stops WebUI, identifiable STDIO Core instances and the desktop
pet for managed Adapters. It checks again after removal and cleans data only after confirming exit.
If exit cannot be confirmed or a Host reconnects, cleanup does not proceed; close the corresponding
Host session and follow the returned retry instructions. An explicit `TASKBELAY_DATA_DIR` equal to the
default directory is cleaned once, and still requires explicit-path confirmation.
Retries after removal continue checking the previously confirmed Core locations until the associated
processes have exited.

Failures include `error.code/message/detail`, `operation_id`, `failed_action`, `completed_actions` and a recovery command. Text output preserves the same causes and completed steps. Repeating a command observes current installation state instead of replaying an old operation. Recovery commands pin the attempted version where applicable; reset generates a plan for the current state again. Successful installation retains hook review/trust and Profile restart instructions.

Lifecycle exit codes: `0` success or no changes, `1` check/execution failure, `2` invalid arguments, `3` confirmation required or declined, `4` unmet plan/cleanup authorization, `5` partial execution or failed final verification. Exiting the menu returns `0`. Invalid WebUI arguments return `2`; launcher failures under `--json` also return JSON.

When `TASKBELAY_DATA_DIR` is set, the public launcher accepts only an existing canonical, non-symbolic-link absolute
directory. No command creates an explicit directory.

Default local paths are platform-specific:

| Path | macOS arm64 | Windows 10/11 x64 |
| --- | --- | --- |
| Task data | `$HOME/.taskbelay/data` | `%LOCALAPPDATA%\taskbelay\data` |
| User configuration | `$HOME/.taskbelay/config.json` | `%USERPROFILE%\.taskbelay\config.json` |
| Lifecycle manager state | `$HOME/.taskbelay` | `%LOCALAPPDATA%\taskbelay` |
| Desktop pet and registrations | `$HOME/.taskbelay/pet`, `$HOME/.taskbelay/registrations` | `%LOCALAPPDATA%\taskbelay\pet`, `%LOCALAPPDATA%\taskbelay\registrations` |

Set an explicit data directory in PowerShell with:

```powershell
$env:TASKBELAY_DATA_DIR = 'C:\absolute\existing\taskbelay-data'
taskbelay status --host all
```

Native Host commands remain available for diagnostic recovery.

## Desktop pet (macOS arm64 and Windows x64)

Install `taskbelay@latest` for the bundled macOS arm64 and Windows 10/11 x64 desktop apps. Configure at least one Codex, DeepSeek, Claude or ZCode Adapter to provide Core; see the [Host guide](CLAUDE_en.md) for Claude installation channels. `install`, `upgrade`, `repair` and `reinstall` refresh the application copy while preserving settings and appearances, even when the Adapter is already current. See the [desktop pet guide](DESKTOP-PETS_en.md).

| Command | Behavior |
| --- | --- |
| `taskbelay pet start` | Start or restore the pet, verify Core and the data directory, and start WebUI if needed. An existing user-directory app takes priority over the bundled app. |
| `taskbelay pet stop` | Quit the pet normally, preserving WebUI, Tasks, settings, and appearances. |

Only these two argument forms are accepted. Output is plain text; exit codes are `0` for success, `1` for runtime failure, and `2` for invalid arguments.
`pet status` and `pet start --json` are not public entries.

```bash
taskbelay pet start
taskbelay pet stop
```

The menu provides task and appearance selection, import, Animations, Idle activities, hide, and quit. See the [desktop pet guide](DESKTOP-PETS_en.md)
for task selection, the scope of nine-clip support, triggers, and troubleshooting. Stop the pet before updating or removing its current Core Adapter or
unified-entry package; maintenance aborts if shutdown fails. Confirmed factory-reset clears `productRoot/pet`; ordinary quit and uninstall preserve user artwork and settings.

## Codex

### Install

```bash
npm install -g taskbelay-codex@latest
taskbelay-codex setup
taskbelay-codex --version
```

The global npm installation only places the `taskbelay-codex` launcher on `PATH`. `setup` is a
separate operation: it verifies the platform, package, bundled Core, and Codex version; registers the
local marketplace, Plugin, and MCP configuration; and reads back the resulting ownership. Setup retains configuration path, file-type and permission checks and validates existing configuration through its packaged Core without rewriting valid content. When
configuration is absent, setup creates `$HOME/.taskbelay/config.json` on macOS or
`%USERPROFILE%\.taskbelay\config.json` on Windows with `{}`, leaving preference defaults to Core; success then reports
actual configuration/receipt file changes and one next step. `--version`
reports both the host package and bundled Core identities.

Set `TASKBELAY_DATA_DIR` to an existing canonical absolute directory before starting Codex. The MCP server, hook and artifact commands use that same directory. `taskbelay-codex artifacts <collect|prepare> --help` returns JSON examples, field descriptions, outputs and the next step without starting Core. The Plugin forwards exactly this variable through `env_vars`; a new Codex session picks up a changed launch environment.

### Supported Codex commands

| Command | Purpose |
| --- | --- |
| `npm install -g taskbelay-codex@latest` | Install the package selected by the npm `latest` dist-tag and place `taskbelay-codex` globally on `PATH`. It does not register the Codex Plugin automatically. |
| `taskbelay-codex setup` | Create or validate fixed user configuration, validate the installation and Codex compatibility, register the marketplace, Plugin, MCP, and packaged hook, then direct the developer to review and trust the current hook through Codex `/hooks`. Repeated execution verifies the existing registration. |
| `taskbelay-codex setup --json` | Perform the same operation as `setup`, but emit one machine-readable JSON line retaining operation, status, changed, and receipt_path while adding configuration_path, file_changes, and next_step. |
| `taskbelay-codex status` | Read and display the current package/Core and registration state. |
| `taskbelay-codex status --json` | Read package, Core, receipt, marketplace, and Plugin state without creating configuration, registration, or data. |
| `taskbelay-codex --version` | Print `taskbelay-codex <package-version> (core <core-version>)` to identify the actual installed package and bundled Core. |
| `taskbelay-codex remove` | Validate the runtime receipt and stop the matching WebUI before removing the package-owned Codex Plugin, marketplace registration, and receipt. A stop failure leaves registration intact; Task data and the target Git repository are retained. |
| `taskbelay-codex remove --json` | Perform the same operation as `remove` and emit machine-readable JSON. Its `next_step` points to the separate global npm uninstall. |
| `npm uninstall -g taskbelay-codex` | Uninstall the global npm package after `remove` completes. Running it alone does not deregister the Codex integration first. |
| `taskbelay-codex artifacts <collect\|prepare>` | Read a closed JSON object from stdin and ask packaged Core to collect or prepare the current Action files. See [artifact collection and submission](ARTIFACTS_en.md). |
| `taskbelay-codex artifacts --help` | List the artifact operations and their help entry points without reading stdin or starting Core. |
| `taskbelay-codex artifacts <collect\|prepare> --help` | Return JSON examples, field sources, output fields and the next step. Help resolves no installation or data paths; Core owns input validation. |
| `taskbelay-codex mcp` | **Managed host command.** The Plugin MCP configuration invokes it to establish the data directory and Codex admission instructions, then launch the packaged Core with `mcp --stdio`. Normal users should not start it manually. |
| `taskbelay-codex hook pre-tool-use` | **Managed host command.** The packaged Codex hook invokes it through the package-owned launcher on `PATH`; it reads one hook event, extracts `apply_patch` targets, and performs the prewrite check. Normal users should not start it manually. |
| `taskbelay-codex host-check pre-file-write` | **Managed host command.** The `hook pre-tool-use` implementation invokes it so the launcher resolves the package-local Core and forwards stdin/stdout with the exact `host-check pre-file-write` arguments. Normal users should not start it manually. |
| `taskbelay-codex host-check workspace-available` | **Internal Host command.** Forwards the read-only Core directory-claim check used before local branch preparation. |
| `taskbelay-codex host-launch <operation>` | **Managed Host command.** Reads one closed JSON object from stdin and writes one JSON object. `operation` is exactly `inspect|prepare|supersede|supersede-resume|local-provision|status|dispatch-start|dispatch-call|dispatch-recover|dispatch-reconcile|dispatch-result|bootstrap|cli-provision|scope|handoff-start|handoff-result|handoff-status|cleanup-decision|cleanup-worktree|cleanup-branch`; it performs or records current-user-confirmed assessment, provisioning, relaunch, handoff, and cleanup steps and is not a generic Git CLI. |

Workspace selection defaults to `workspace_mode=new_branch`; alternatives are `current_branch` and `dedicated_worktree`. Local modes require local source, empty remote, the current base branch and starting HEAD, and `worktree_path=repository_path`. `carry_changes` accepts initial contents in place. Both local modes keep the session and use no requirements handoff file. New-branch targets must be unused; current-branch targets equal the base.

`taskbelay-codex host-launch <operation>` reads a UTF-8 JSON object of at most 1 MiB from the stdin stream, including chunked input and multibyte characters split across chunks. Read failures, invalid UTF-8, duplicate members, invalid JSON, arrays, and null are rejected before the operation runs; errors go to stderr and successful JSON results go to stdout.

New Codex sessions use the complete requirements handoff saved by the source session. These internal
operations accept closed JSON objects:

| Operation | Input fields and material handling |
| --- | --- |
| `prepare` | Requires `request`, `assessment`, `user_choice`, `repository_key`, `repository_path`, `workspace_mode`, `source_type`, `carry_changes`, `remote_name`, `base_branch`, `target_branch`, `surface`, `worktree_path`, and `handoff_file`; `launch_id` is optional. Default mode is `new_branch`; alternatives are `current_branch` and `dedicated_worktree`. Local modes use `current_session`, the original directory and `handoff_file=null`; dedicated worktrees require a complete requirements JSON file outside repositories, retained through `handoff_digest`. |
| `local-provision` | Accepts only `launch_id` and `repository_key`. Checks Core claims for a `current_session` record, creates the local branch or retains the current branch, and returns the prepared receipt and `workspace_origin`. After every root is ready, call `scope` and Core in the current session. |
| `dispatch-start` | Accepts only `launch_id`, `repository_key`, and `project_id`. Saves complete `host_request`; the caller forwards it unchanged after claiming permission with `dispatch-call`. |
| `cli-provision` | Accepts only `launch_id`, `repository_key`, `additional_worktree_paths`, and `source_repository_path`. Renders relaunch arguments from the same saved material; the caller uses them unchanged. |

Material includes the goal, confirmed requirements referencing original messages, terminology, scope
constraints, code findings, working instructions, unaccepted suggestions, assumptions, open questions,
and original discussion in order. See the [Codex sender format](../packages/codex/plugin/skills/taskbelay/references/task-handoff.md).

Working instructions in the handoff contain only session-specific instructions and authorizations.
The destination Codex session loads global and repository `AGENTS.md` files normally; the handoff
duplicates neither their contents nor summaries and excludes automatically injected rule blocks from
original requirements discussion. Actual user requests to change a rule remain requirements.
Applicable rules unavailable through destination discovery identify their source path, scope, and
concrete discovery gap, preferably referencing a readable source file; if that file is unreadable,
include only the necessary task-specific rule text and exclude credentials. Record unverified
availability in `open_questions` instead of copying entire files as a precaution.

File input is outside the 1 MiB stdin envelope. Complete structured prompts above 24 KiB UTF-8 use
complete file paths and reading instructions without truncating material. Neither launch path accepts
a newly written `request` summary. Missing or altered material fails sending before a desktop dispatch
is recorded or a CLI worktree is created. Task titles retain deterministic launch/repository markers.

`taskbelay-codex` accepts no other subcommands and has no implicit `help`, `update`, or `uninstall`
subcommand. Native Host recovery can update to `latest` by reinstalling globally and rerunning `setup`:

```bash
npm install -g taskbelay-codex@latest
taskbelay-codex setup
taskbelay-codex --version
```

To uninstall while retaining Task data, run `taskbelay-codex remove` and then
`npm uninstall -g taskbelay-codex`. Delete the shared default data directory at
`$HOME/.taskbelay` on macOS or `%LOCALAPPDATA%\taskbelay` on Windows only
after the Codex, DeepSeek, Claude and ZCode Adapters are removed and no Task is needed.

### Codex smart activation and explicit selector

```text
$taskbelay-codex:taskbelay <task description>
```

Codex retains explicit choices and authorizations that remain valid for the current request and assessment. When no decision or required input is outstanding, it continues without an acknowledgment pause for progress updates or Skill-rule explanations. When input is needed, it asks the concrete question in the same response; development mode, worktree parameters, comprehension confirmation, separate operation authorization, and blockers keep their respective rules.

This is not a shell command. It is the exact Skill selector in a Codex user message. The Host may also
select the Skill implicitly for bounded development; bare `$taskbelay` and a wrong namespace are not
explicit selectors. Under either activation, a new request first receives read-only assessment with
change level, candidate impact, unknowns, and recommendation, then stops for a developer choice. Before
confirmation there is no Core call, Task/receipt/child, or Git write; request, root, HEAD, or status
changes invalidate the assessment.

After selecting TaskBelay, default to a new branch in the current directory; current-branch and
dedicated-worktree modes are explicit alternatives. Local launches use prepare/local-provision/scope
in the existing session. Dedicated worktrees retain source selection, copying and managed dispatch
or CLI relaunch. One directory holds one active Task. Independent parallel items need distinct
directories or sequential execution. ACTIVE_TASK_CONFLICT stops creation for resolution of the existing
Task. Explicit resume returns to the original directory instance without repeating assessment.

## DeepSeek Harness

`taskbelay-deepseek` has no `bin` field in `package.json`, so it does not expose a standalone
`taskbelay-deepseek` executable. Installation, inspection, and removal use the DSH profile lifecycle.

### Install

Install DSH first, then add TaskBelay to a real profile from a writable directory. This example uses
`web`; change `PROFILE` for another profile and do not enter `<profile>` literally:

```bash
npm install -g @deepseek-ai/dsh@latest
dsh --version
PROFILE=web
TARBALL="$(npm pack taskbelay-deepseek@latest --silent)"
dsh plugin --profile "$PROFILE" add "$PWD/$TARBALL"
rm -f "$PWD/$TARBALL"
dsh --profile "$PROFILE" --dump-config
```

Windows PowerShell uses the same DSH profile lifecycle, with the `npm pack` result resolved to an
absolute path:

```powershell
npm install -g @deepseek-ai/dsh@latest
dsh --version
$ProfileName = 'web'
$Tarball = (npm pack taskbelay-deepseek@latest --silent | Select-Object -Last 1).Trim()
$TarballPath = (Resolve-Path -LiteralPath $Tarball).Path
dsh plugin --profile $ProfileName add $TarballPath
Remove-Item -LiteralPath $TarballPath
dsh --profile $ProfileName --dump-config
```

`npm pack` downloads the official package selected by `latest` and writes its tarball into the
current directory; command substitution retains the actual filename. DSH `plugin add` receives the
absolute tarball path and contributes the package, bundle layer, Skill, guard, and MCP child to the
selected profile. After installation, stop and restart that profile according to the DSH profile
lifecycle.

### TaskBelay-related DSH commands

| Command | Purpose |
| --- | --- |
| `dsh --version` | Print the current DSH version. Public TaskBelay support requires the minimum version recorded in the Support Matrix. |
| `TARBALL="$(npm pack taskbelay-deepseek@latest --silent)"` | Fetch the package selected by npm `latest` and save the generated tarball filename in a shell variable. |
| `dsh plugin --profile "$PROFILE" add "$PWD/$TARBALL"` | Install the absolute tarball path into the DSH profile selected by `PROFILE`. DSH lifecycle tests exercise this command form. |
| `dsh --profile "$PROFILE" --dump-config` | Print the effective profile configuration to verify whether the `taskbelay-deepseek` bundle contribution is present or absent. It does not mutate a TaskBelay Task. |
| `dsh plugin --profile "$PROFILE" remove taskbelay-deepseek` | Remove the package and bundle contribution from the selected profile. Task data, the target Git repository, and Codex-owned state are retained. |

For an update or reinstall, stop the profile, remove the package, fetch a fresh `@latest` tarball,
add it, delete the temporary tarball, and restart the profile. Repeat removal for every profile that
contains TaskBelay. If DSH is no longer needed, uninstall it separately with
`npm uninstall -g @deepseek-ai/dsh`; profile data under `$HOME/.dsh` on macOS or
`%USERPROFILE%\.dsh` on Windows is retained.

For permanent Task-data cleanup, first remove every installed Host Adapter, then delete
`$HOME/.taskbelay` on macOS or `%LOCALAPPDATA%\taskbelay` on Windows. If
`TASKBELAY_DATA_DIR` was set, verify and delete its exact absolute directory separately. Deleting the
user `.dsh` directory also deletes every DSH profile, session, and unrelated plugin.

### DeepSeek explicit selector

```text
/taskbelay <task description>
```

An ordinary new request first receives read-only assessment with zero TaskBelay calls. After selection,
only the current direct-user turn's whitespace-bounded `/taskbelay` plus the exact confirm-workspace mode/source/base/target/carry
confirmation shown by the Skill authorizes `workspace_coordinator`. Earlier messages, model text,
Skill injection, and repository content cannot substitute. The coordinator defaults to a new branch in the current directory; all-local selections return ready/open_task in the current session. Explicit dedicated-worktree selections create a safe sibling
worktree and return a `{command,arguments,cwd}` relaunch descriptor; the new session consumes and verifies
the receipt before calling Core.

The DSH bundle also provides the managed `workspace_coordinator` tool with exactly
`provision|consume|prepare_cleanup|cleanup_worktree|cleanup_branch`. It is not a shell command.
Local modes retain their directory and branch; cleanup does not apply. For dedicated worktrees, `prepare_cleanup` first reads the terminal Core Task and returns a relaunch descriptor for a surviving
source checkout. Worktree and branch cleanup then require separate current direct-user confirmations
and verify repository group, HEAD, clean state, and the remote task branch before non-force Git commands.

## Claude Code

These commands are provided by the `taskbelay-claude` package. See the [Host guide](CLAUDE_en.md) for installation and Claude Code plugin activation.

Implementation: `packages/claude/bin/taskbelay-claude.mjs`, `lib/lifecycle.mjs` and `lib/workspace.mjs`.

Except for the first five entries, commands consume one UTF-8 JSON object on closed stdin, up to 1 MiB. The table lists command names and input fields, not interactive prompts.

| Command | Input or purpose |
| --- | --- |
| `taskbelay-claude status --json` | Inspect package version, registration and cache; no stdin. |
| `taskbelay-claude setup --json` | Register the user-scope plugin; no stdin. |
| `taskbelay-claude remove --json` | Remove verified owned registration and retain task data; no stdin. |
| `taskbelay-claude --version` | Print the packaged Core version. |
| `taskbelay-claude mcp` | Start the stdio MCP service. |
| `taskbelay-claude artifacts collect` | Core artifact collection request. |
| `taskbelay-claude artifacts prepare` | Classified Core artifact collection. |
| `taskbelay-claude host-check workspace-available` | repository_path. |
| `taskbelay-claude host-check pre-file-write` | host, repository_path, tool_name, paths, intent_digest, path_parse_complete. |
| `taskbelay-claude hook pre-tool-use` | Original Claude PreToolUse event. |
| `taskbelay-claude host-launch inspect` | request, repositories[{key,repository_path}]; returns assessment anchor. |
| `taskbelay-claude host-launch prepare` | request, assessment, user_choice, repositories, handoff; returns preparation record. |
| `taskbelay-claude host-launch provision` | launch_id; provision every selected repository. |
| `taskbelay-claude host-launch status` | launch_id; read retained launch record. |
| `taskbelay-claude host-launch scope` | launch_id; verify workspaces and return Core creation scope, including primary_repository_key for a single repository. |
| `taskbelay-claude host-launch bind-task` | launch_id, task_id from a successful Core response. |
| `taskbelay-claude host-launch launch` | launch_id; return a session launch descriptor, without starting the interactive process. |
| `taskbelay-claude host-launch resume` | launch_id; return the retained session resume descriptor. |
| `taskbelay-claude host-launch record-session` | launch_id, actual session_id. |
| `taskbelay-claude host-launch retry-launch` | launch_id, previous_caller_stopped, session_not_started, reason; both factual flags must be true. |
| `taskbelay-claude host-launch relocate` | Input: launch_id, relocation_id, destinations[{repository_key,repository_path}], authorized. Returns relocation_id and relocation_destinations[{key,repository_path}] ready to submit directly to Core. |
| `taskbelay-claude host-launch cleanup-worktree` | launch_id, repository_key, terminal, authorized. |
| `taskbelay-claude host-launch cleanup-branch` | launch_id, repository_key, terminal, authorized, core_task; separate from worktree-removal authorization. |

`prepare.repositories` requires key, repository_path, workspace_mode, source_type, remote_name, base_branch, target_branch, carry_changes and worktree_path in every entry. Repository keys follow Core's `^[a-z0-9][a-z0-9._-]{0,127}$` rule, are checked before Git changes, and retain their identity through creation and relocation. Modes are `new_branch`, `current_branch` and `dedicated_worktree`. Assessment contains the original `inspect` anchor, impact, verification and resolved unknowns; user_choice records the actual user decision. Exact Host-operation prerequisites and value sources are in the [admission reference](../packages/claude/plugin/skills/taskbelay/references/admission.md) and [lifecycle reference](../packages/claude/plugin/skills/taskbelay/references/host-lifecycle.md).

`status`, `setup` and `remove` emit JSON with or without `--json`. Host-launch success emits the direct operation result. Artifact commands retain the Core `ok/result` or `ok/error` envelope; host-check returns its own check result. `mcp` uses the MCP protocol rather than a one-shot result. Ordinary CLI/input failures exit 1; a failed Hook exits 2; forwarded Core commands retain their failure output and exit code. Do not assume every command uses the MCP envelope.

The selector is `/taskbelay-claude:taskbelay <task>`. `CLAUDE_CONFIG_DIR` selects Claude settings. `TASKBELAY_DATA_DIR` selects an existing canonical absolute task data directory and must match across the Host, MCP and helpers.

## ZCode

These entries belong to the published `taskbelay-zcode` package. Install it with `taskbelay install --host zcode --yes`; the CLI prepares the local plugin source, and the [ZCode guide](ZCODE_en.md) covers the required UI installation and enablement. Implementations are `packages/zcode/bin/taskbelay-zcode.mjs`, `lib/lifecycle.mjs`, `lib/workspace.mjs` and `hooks/pre-tool-use.mjs`.

| Command | Input and result |
| --- | --- |
| `taskbelay-zcode status [--json]` | No stdin; inspect local package, Core and preparation record, without observing ZCode plugin loading. |
| `taskbelay-zcode setup [--json]` | No stdin; verify and save the local source, returning UI installation/enablement steps. |
| `taskbelay-zcode remove [--json]` | No stdin; retain a pending UI-removal record and preserve Task data. |
| `taskbelay-zcode remove --confirm-host-removed [--json]` | Clear verified owned records after the user has removed the plugin and marketplace in the UI and closed affected sessions; this is not automatic observation. |
| `taskbelay-zcode --version` | Print the packaged Core version. |
| `taskbelay-zcode mcp` | Start stdio MCP. |
| `taskbelay-zcode artifacts collect\|prepare` | Corresponding Core artifact collection or preparation request on stdin. |
| `taskbelay-zcode host-check workspace-available` | repository_path on stdin. |
| `taskbelay-zcode host-check pre-file-write` | host, repository_path, tool_name, paths, intent_digest, path_parse_complete on stdin. |
| `taskbelay-zcode hook pre-tool-use` | Original ZCode PreToolUse event on stdin; Write/Edit read tool_input.file_path. |
| `taskbelay-zcode host-launch inspect` | request, repositories[{key,repository_path}]; return an assessment anchor. |
| `taskbelay-zcode host-launch prepare` | request, assessment, user_choice, repositories, handoff; retain preparation records. |
| `taskbelay-zcode host-launch provision` | launch_id; prepare every repository. |
| `taskbelay-zcode host-launch status` | launch_id; read retained launch records. |
| `taskbelay-zcode host-launch scope` | launch_id; verify and return Core creation scope. |
| `taskbelay-zcode host-launch bind-task` | launch_id, task_id; bind the actual successful Core result. |
| `taskbelay-zcode host-launch open\|resume` | launch_id; return UI guidance, workspace paths and a complete continuation prompt, without launching a session. |
| `taskbelay-zcode host-launch relocate` | launch_id, relocation_id, destinations[{repository_key,repository_path}], authorized=true, core_preparation; core_preparation must be the actual complete result from a successful `taskbelay_prepare_task_relocation` response, including relocation_id/task; return destinations for Core verification. |
| `taskbelay-zcode host-launch cleanup-worktree\|cleanup-branch` | launch_id, repository_key, terminal, authorized, core_task; worktree and branch cleanup require separate authorization. |

Host-launch accepts one closed JSON object on stdin, up to 1 MiB. Each `prepare.repositories` entry requires key, repository_path, workspace_mode, source_type, remote_name, base_branch, target_branch, carry_changes and worktree_path. Modes are `new_branch`, `current_branch` and `dedicated_worktree`; keys follow Core's `^[a-z0-9][a-z0-9._-]{0,127}$` rule. Relocation checks the bound ZCode Task, current relocation blocker and all source workspaces in the preparation result; a relocation ID alone cannot authorize directory moves. An already moved ID only verifies destination identity and reads back the result; uncertain outcomes reject repeated moves. After Core completes a relocation, a new preparation result and ID can start another. See the packaged [admission](../packages/zcode/skills/taskbelay/references/admission.md) and [lifecycle](../packages/zcode/skills/taskbelay/references/host-lifecycle.md) references for assessment, authorization and relocation prerequisites.

Lifecycle commands always output JSON. Complete validation and saved preparation return `status=action_required`, `registration.host=unverified` and `next_steps` for UI installation, enablement or cache refresh; incomplete preparation returns `partial`. Ordinary `remove` saves `phase=removal_required`; confirmed removal returns `absent` and `registration.host=user_confirmed_removed`. These fields describe local preparation and the user's statement, not automatic Host readiness. The unified manager likewise retains `action_required` instead of reporting `ready`. The first unified `uninstall` retains the package for confirmation; after UI removal, session closure and the confirmation command, repeat unified `uninstall` to remove the package. `factory-reset` rejects shared-data cleanup while a ZCode package or record remains.

Successful local installation, maintenance or removal steps may exit 0 with `action_required`; follow `next_steps`. Unified `doctor` exits 1 until the result is `ready`; ZCode's `action_required` means Host UI state remains unverified, rather than failed local preparation.

Successful Host-launch commands directly output operation results; artifacts preserve Core success/error structures; host-check outputs check results; MCP uses protocol transport. Ordinary input or CLI errors exit 1, and forwarded Core commands preserve their output and exit code. Explicit Core denial makes the Hook output `permissionDecision=deny` and exit 0; event parsing or check execution errors exit 2. Exit 0 alone does not establish write permission, and a denial cannot be treated as having no Task. Shell or external writes remain subject to later observation.

The plugin uses `ZCODE_PLUGIN_ROOT` for packaged MCP and Hook entrypoints. `TASKBELAY_DATA_DIR` selects an existing canonical absolute data directory and must match across the Host, MCP and helpers. ZCode `open` and `resume` only describe UI continuation; they do not claim an unverified session CLI or session ID.

## Packaged Core

The Go Core bundled in host packages is not installed as a normal global user CLI. Its complete
accepted command surface is primarily for host integration, development, and diagnostics:

| Command | Purpose |
| --- | --- |
| `taskbelay` | Print help when invoked with no arguments. |
| `taskbelay help` | Print help. |
| `taskbelay -h` | Short-option form of `help`. |
| `taskbelay --help` | Long-option form of `help`. |
| `taskbelay version` | Print `taskbelay <core-version>`. |
| `taskbelay config validate` | Read raw UTF-8 configuration JSON from stdin, up to 16 KiB, and return the validation result. |
| `taskbelay config validate --help` | Show configuration-validation help without reading stdin, configuration files, Task data or Git. |
| `TASKBELAY_DATA_DIR=/absolute/path taskbelay mcp --stdio` | Start local STDIO MCP with an existing usable data directory. Startup fails when the path is missing or not a directory. |
| `$env:TASKBELAY_DATA_DIR = 'C:\absolute\existing\data'; taskbelay.exe mcp --stdio` | Start local STDIO MCP with an existing usable data directory from Windows PowerShell. |
| `taskbelay host-check pre-file-write` | **Managed Host command.** Read normalized structured-write targets from stdin, compare them with the active Task's cross-repository ExpectedPaths, and return `allow` or persist a file-scope blocker before returning `deny`. Codex, DeepSeek, Claude and ZCode Adapters call it; ordinary users do not. |
| `taskbelay host-check workspace-available` | **Internal Host command.** Reads `{"repository_path":"<absolute root>"}` from stdin and checks active directory claims read-only. Writes `available`, canonical `repository_path` and optional `task_id`; errors exit nonzero. It neither creates a database nor reserves the directory. |
| `taskbelay webui start [--no-open] [--plain\|--json]` | Start or reuse the shared loopback WebUI; open the browser by default. |
| `taskbelay webui open [--plain\|--json]` | Validate the receipt, process identity, and live Core status, then open the same URL. |
| `taskbelay webui status [--plain\|--json]` | Return `ready`, `read_only`, `incompatible`, or `unavailable`. |
| `taskbelay webui stop [--plain\|--json]` | Verify PID and process-start identity before stopping the shared instance. |

`taskbelay host-check pre-file-write` and `taskbelay webui serve` are internal Adapter/lifecycle entrypoints, not Host user commands. Core
has no remote transport, generic HTTP/SSE transport, generic shell, or Git-mutation commands. Codex users start it
through the managed `taskbelay-codex mcp` entrypoint; DeepSeek users start it through the DSH
integration process; the Claude plugin starts it through `taskbelay-claude mcp`, and ZCode uses `taskbelay-zcode mcp`.

For pre-write checks, `repository_path` locates the existing repository and `paths` retains the full
write targets, whose parent directories may not exist yet. The DeepSeek Adapter starts from the target's
nearest existing parent directory. Core observation failures such as timeouts or output limits exit
nonzero and cannot count as an absent Task. Writes in repositories without a Task and ordinary non-Git
directories remain allowed.

### Configuration validation

Here, `taskbelay` means the Go Core executable inside a Host package, not the global lifecycle manager. `config validate` accepts one UTF-8 JSON object on closed stdin. It takes no file-path argument, reads no user configuration, Task store or Git state, and writes no data. `internal/userconfig.Decode` defines configuration semantics: duplicate or unknown fields, invalid UTF-8, input exceeding 16 KiB and incorrect field types are rejected. An empty object `{}` uses defaults for every Host.

Success exits `0` and writes the resolved preferences for all four Hosts to stdout. For example, `{}` produces:

```json
{"ok":true,"result":{"codex":{"codebase_memory":false},"deepseek":{"codebase_memory":false},"claude":{"codebase_memory":false},"zcode":{"codebase_memory":false}}}
```

Invalid configuration exits `1` and writes the specific reason to stdout. For example, `{"other":true}` produces:

```json
{"ok":false,"error":{"code":"INVALID_CONFIGURATION","message":"unknown top-level field \"other\""}}
```

Invalid command arguments exit `2`. `taskbelay config validate --help` exits `0` after showing help without consuming stdin. This command does not require `TASKBELAY_DATA_DIR`.

## MCP tools

These eighteen tools are the complete public MCP tool list. Host adapters call them; they are not
terminal shell commands.

| Tool | Type | Purpose |
| --- | --- | --- |
| `taskbelay_server_info` | Read-only | Read Core product version, transport, health, supported process, hosts, method profiles, tool catalog, and effective host code-index preferences. It must be the first call after valid Host assessment and confirmation. |
| `taskbelay_open_task` | Read or create | Create only after every `workspace_origin` passes verification for the selected workspace mode; with null `new_task`, resume the same Task from its original instance after a workspace check. |
| `taskbelay_get_task` | Read-only | Read a persisted Task, including its verification plan, current budget/usage, adjustment reasons, and at most three recent test attempts; automatically returns a Recovery assessment when Core retains an Action submission. |
| `taskbelay_get_next_action` | Observe/maybe mutate | Observe the workspace first; idempotently create a workspace blocker when needed, otherwise return the Action, `submission_tool`, and legal transitions. |
| `taskbelay_submit_requirements` | Mutation | Submit the REQUIREMENTS node result. |
| `taskbelay_submit_design` | Mutation | Submit the DESIGN node result. |
| `taskbelay_submit_tasks` | Mutation | Use `tasks_plan_saved` to save the complete baseline including `verification_plan` in TASKS; `tasks_ready` confirms the saved plan before implementation. |
| `taskbelay_submit_implementation` | Mutation | Submit the IMPLEMENT node result. |
| `taskbelay_submit_test` | Mutation | Submit the TEST node result. `verification_budget_increased` records a concrete increase and stays in TEST; normal results send `budget_adjustment=null`; a third exact repetition pauses. |
| `taskbelay_submit_comprehension` | Mutation | Submit the COMPREHENSION_REVIEW node result. |
| `taskbelay_submit_refactor` | Mutation | Submit the REFACTOR node result. |
| `taskbelay_submit_delivery` | Mutation | Submit DELIVERY judgment, explicit acceptance links, risks and findings. Each criterion supplies work_item_ids and current Test evidence_ids. Core fills aggregate evidence IDs and Test/Comprehension record IDs; caller-supplied aggregate members are rejected. |
| `taskbelay_resolve_blocker` | Mutation | Resolve after Core verifies the condition. File scope uses `choice` and `reason`; history uses `history_resolution:{choice:"accept_current_history",reason}`; relocation uses `relocation_id` plus every `relocation_destinations[{key,repository_path}]`; verification/Recovery blockers use current identities. An unexecuted pre-write request checks the full layer delta from the saved binding; unchanged carried paths gain no permission to change. After `expand_scope`, save and confirm the revised exact plan. |
| `taskbelay_recover_action` | Mutation | Recover an uncertain Action from the normalized submission retained in an independent Action operation record; accepts no original payload. |
| `taskbelay_cancel_task` | Destructive mutation | Move a nonterminal Task to `CANCELLED` using the current revision and a non-empty reason. |
| `taskbelay_prepare_task_branch_rename` | Mutation | host, task_id, revision, repository_key, target_branch, reason; returns rename_id/task in BLOCKED for one pure branch rename. |
| `taskbelay_prepare_task_relocation` | Mutation | Retain relocation ID, source workspace/content/surface and resume node while source claims remain active during Host handoff. |
| `taskbelay_abandon_task` | Destructive mutation | When the original worktree is unavailable, use exact host/task/revision and a non-empty reason to enter `CANCELLED` and release claims after attempting repository observation to establish worktree unavailability. |

Each ordinary node submission tool accepts only `host`, `task_id`, `action_id`, `transition_id`,
`summary`, `reason`, `artifacts`, `method_results`, and that node's semantic `node_result`, which has no
`changed_paths` or `no_file_changes`. Core derives Action delta/current surface from Git and fills the
revision, Action kind, process identity, source cursor, repository binding, artifact roles, method
step identity/order/status, and internal payload envelope. `get_next_action.submission_tool` names the
only submission tool for the current Action.

`method_results` is keyed by the current `method_steps[].step_id`; each value contains only
`capability` and `summary`. Supply the actual capability ID after external completion, or an empty
string after completed plain-equivalent work. Core creates the internal `MethodEvidence` step
identity, order, and status. Artifacts use `artifacts.current` or `artifacts.other_process` as exposed
by the current schema, with only `path`, `digest`, and `summary` per item. Core assigns `role` from
the slot and node.

`node_result.baseline.requirements_revision` on `taskbelay_submit_design`,
`node_result.baseline.design_revision` on `taskbelay_submit_tasks`, and
`node_result.task_plan_revision` on `taskbelay_submit_implementation` are absent from the Host
submission contract. After validating the current Action identity, Core fills them from the same
Task snapshot; supplying one returns `unknown_member` at the exact path. Other
missing required members return exact `required_member_missing` paths. The Host may correct through the
same submission tool once only when Core proves zero writes and the value comes from facts already
established by the current node work, and may change only the exact members listed in
`recovery.allowed_paths`.

`taskbelay_submit_tasks.node_result` always contains `problem_class`, `baseline`, `findings` and `user_confirmation`. Save or revise with `tasks_plan_saved`: complete baseline, problem_class=none, empty findings and user_confirmation=null; the returned Task stays in TASKS. Confirm with `tasks_ready`: baseline=null and `{source:"user",status:"passed",summary,requirements_digest,design_digest,task_plan_digest,task_plan_revision}`. Copy the four references from current `baselines.requirements.digest`, `baselines.design.digest`, `baselines.task_plan.digest` and `baselines.task_plan.revision`, only after the user explicitly approves that content. Core returns `task_plan.confirmation` and its own `confirmed_at`. Missing or mismatched approval cannot enter implementation; waiting stays in TASKS. Upstream-return edges retain their findings/reason rules with null baseline and confirmation.

`host-launch prepare.assessment` contains `change_level` (small/standard/large/uncertain), observed_repositories, candidate_components, candidate_paths, public_contract_flags, persistence_or_state_flags, host_or_platform_flags, verification_shape, unknowns, recommendation, reasons and anchor. `user_choice` is `{source:"user",mode:"taskbelay",summary}`, recording the actual choice after showing the assessment. Missing inputs, unresolved unknowns, inconsistent roots, stale anchors and other mode choices are rejected before preparation. `receipt.admission` saves both complete objects; confirmed continuation reads the receipt without a repeated choice.

A new Task's `new_task` has no `verification_budget`. TASKS
`baseline.verification_plan` contains `checks[{name,rationale}]`, `initial_budget`,
`full_suite_expected`, and `test_code_changes_expected`. If TEST capacity becomes insufficient, the
Host may choose the returned `verification_budget_increased` transition with a `budget_adjustment`
containing `basis`, `additional_checks`, `additional_automatic_commands`, `allow_full_suite`, and
`allow_manual_handoff`; the transition `reason` states the concrete new impact, risk, failure, or
verification gap. A no-op, missing-check, or reasonless increase is rejected.

Every TEST check also sends `full_suite_reason`. It is empty when `full_suite=false`; a full suite
records the concrete risk this run covers. Core retains the result, while the Host still decides
necessity before executing the command.

Unknown CLI arguments, tools outside this catalog, and calls that do not satisfy the assessment and confirmation rules for implicit or explicit activation
are not supported entrypoints.

### Repository Scope and host-preference fields

Before Task creation, the Host discovers repositories read-only under current user instructions and
applicable `AGENTS.md`. When a project index is required, it combines the index, candidate project
documentation, and code/configuration to establish the complete proposed scope, then confirms and
provisions each repository. All reads respect existing Host permissions; Core retains the confirmed
immutable Scope.

When creating a multi-repository Task, `repository_path` identifies the primary repository. The call
may add one primary key and up to seven explicit additional repositories:

```json
{
  "host": "codex",
  "repository_path": "/workspace/core",
  "workspace_origin": {
    "mode": "dedicated_worktree",
    "source_type": "remote",
    "carry_changes": false,
    "remote_name": "origin",
    "base_branch": "main",
    "base_commit": "<frozen-commit>",
    "task_branch": "feature/core-docs",
    "provisioning_receipt_id": "launch-core-docs"
  },
  "primary_repository_key": "core",
  "additional_repositories": [
    {
      "key": "docs",
      "repository_path": "/workspace/docs",
      "workspace_origin": {
        "mode": "dedicated_worktree",
        "source_type": "remote",
        "carry_changes": false,
        "remote_name": "origin",
        "base_branch": "main",
        "base_commit": "<frozen-commit>",
        "task_branch": "feature/docs",
        "provisioning_receipt_id": "launch-core-docs"
      }
    }
  ],
  "new_task": {
    "request": "Synchronize interface documentation across the Core and docs repositories",
    "initial_scope": [],
    "initial_out_of_scope": [],
    "known_acceptance_criteria": [],
    "method_profile": "plain"
  }
}
```

This example shows the closed MCP input shape; it is not a shell command. Replace `<frozen-commit>`
with the actual object ID. Creation requires a receipt-backed `workspace_origin` for every repository
and non-null `new_task`; Core verifies Git and fills source group, canonical root, and worktree Git-dir.
Resume omits or sets `new_task=null`, points `repository_path` at the original participating worktree,
and omits all Scope/origin creation fields. A Scope contains one to eight repositories, additions are
sorted, and membership is immutable. Multi-repository payload paths use
`<repository-key>::<repository-relative-path>`.

The Task result retains the primary `repository` and adds `primary_repository_key` plus sorted
`additional_repositories`. The current Action's single `repository_binding_digest` remains the
primary binding digest for a single-repository Task and becomes the complete Scope aggregate for a
multi-repository Task. Every active Task's `repository_claims` are acquired, retained, or released
in the same SQLite transaction as the snapshot and event.

The identity in `repository_claims` is a directly observable worktree-instance identity, not the Git
common directory. Linked worktrees share a logical repository group but have different canonical
roots/worktree Git directories, so each may hold a Task; one instance holds one active Task. Control Center Task
summaries expose read-only `repository_group_id` and `worktree_path` fields, and every repository in
Task detail exposes its own `repository_group_id`.

The Task result's `verification` projection contains `plan`, `current_budget`, usage for the current
Task Plan revision, and `adjustments`. Before the first TASKS plan is saved, `plan` and `current_budget` are `null`.

The `taskbelay_server_info({})` result includes:

```json
{
  "host_preferences": {
    "codex": { "codebase_memory": false },
    "deepseek": { "codebase_memory": false },
    "claude": { "codebase_memory": false },
    "zcode": { "codebase_memory": false }
  }
}
```

These values come from the process-start snapshot of the read-only user configuration:
`$HOME/.taskbelay/config.json` on macOS or `%USERPROFILE%\.taskbelay\config.json` on Windows. They
express preference, not installed or available index capability. All four Hosts default to false when the file is
absent. Core only interprets configuration and does not create or modify the file. Codex setup and manager initialization write `{}` when configuration is missing and preserve valid existing content.

Current user instructions and applicable `AGENTS.md` take precedence over these defaults when the
Host chooses discovery tools. Without such instructions, false selects ordinary file/text search
and true may prefer an available code index. An unavailable or incomplete index prompts at most one
notice in the current session and a fallback to ordinary search; index results do not change an
existing Task's Scope.

## Artifact collection and preparation commands

`taskbelay-codex artifacts collect` and `taskbelay-codex artifacts prepare` forward to the packaged Core commands `taskbelay artifacts collect` and `taskbelay artifacts prepare`. Each reads one UTF-8 JSON object up to 1 MiB from stdin and writes `{ok:true,result:...}` or `{ok:false,error:...}` to stdout, exiting with 0 on success or 1 on failure. Collect accepts `{host,task_id,action_id}`; prepare accepts `{host,collection}`. Collect returns complete file facts; prepare checks classification and observation freshness and generates artifact arrays. Both read the existing Task and Git without creating storage or advancing the process. See [artifact collection and submission](ARTIFACTS_en.md) for all fields and steps.

## Codex Host operation help

```bash
taskbelay-codex --help
taskbelay-codex host-launch --help
taskbelay-codex host-launch prepare --help
taskbelay-codex host-launch scope --help
```

All help queries return before reading stdin, resolving installation paths or executing Core/Git operations. Operation help is JSON containing `input_schema`, `output_fields` and `next_step`; field descriptions identify values supplied by user confirmation, a previous result or a Host query. Help creates no configuration, workspace or receipt.

`inspect` returns the anchor for the complete `assessment.anchor`. `prepare` accepts the unchanged request, complete assessment, `user_choice`, confirmed workspace parameters and `handoff_file`; managed worktrees explicitly pass `worktree_path: null`. Reuse the first result's `receipt.launch_id` for every additional repository in the same Task. After managed dispatch and `bootstrap`, or CLI provisioning, run the read-only scope assembler:

```text
taskbelay-codex host-launch scope
stdin: {"launch_id":"<saved launch ID>","repository_keys":["api","web"],"primary_repository_key":"api"}
```

`repository_keys` must include all confirmed repositories. The command rejects missing, unprovisioned, duplicate or request-mismatched records. It returns `repository_path` and `workspace_origin`, plus `primary_repository_key` and `additional_repositories` for multiple repositories. Forward the complete result as the repository fields of `taskbelay_open_task`, adding `host` and `new_task` derived from the confirmed request.

## Reading MCP results

Each tool exposes input and result Schemas. Successful envelopes have `ok=true` and data in `result`; failures use `error` and `recovery` to describe the cause and permitted handling. `structuredContent` and text content contain the same JSON; read one complete result.

Codex retains the complete response and checks `ok` before extracting `result`. An `ok=false`
response has no success data; passing its `result` to session `store` sends `undefined`, causing a
local serialization error that masks the original rejection. Complete rejections still follow
`error` and `recovery`; uncertain-operation recovery applies only when the original response cannot
be obtained completely.

TEST with `tests_failed_implementation` requires `problem_class="implementation_failure"` and
nonempty `findings`. `failed_items` lists failed checks or items; `findings` describes the defects
requiring a return to implementation. Failure descriptions in other fields do not replace it.
Node results exposing `findings` use an empty array when `problem_class="none"`.

| Tools | Task / Action location |
| --- | --- |
| `taskbelay_open_task`, `taskbelay_get_task` | `result.task`; handle sibling `result.recovery_assessment` first |
| `taskbelay_get_next_action` | `result.action`; handle `result.recovery_assessment`, `result.blocker` and `result.outcome` first |
| Eight `taskbelay_submit_*` tools, `taskbelay_resolve_blocker`, `taskbelay_recover_action` | `result` is the Task itself; its next Action is `result.current_action` |
| `taskbelay_cancel_task`, `taskbelay_abandon_task` | `result` is the terminal Task itself |
| `taskbelay_prepare_task_relocation` | `result.task` and `result.relocation_id` |

On fresh-session resume, a retained `recovery_assessment` takes precedence over the source Action; recover the saved submission using `operation.action_id`. After uncertain creation, read the original worktree with `open_task` omitting `new_task`, checking origin, scope and intent. Cancellation compares the retained `request_id` to `task.last_operation.operation_id`, kind and outcome. Abandonment and relocation preparation check the original Task, expected revision, operation kind and saved result. Stop and retain resources when readback is inconclusive; lifecycle operations do not use ordinary Action recovery.

`read_next_action` consumes a guarded Action already returned by open/next-action lookup; an advice from saved-state `get_task` requires one guarded lookup. A retained completed assessment does not cause repeated queries.

Task Plan `expected_paths` supports exact paths and a directory suffix `/**`, not general globs; `src` does not cover every file below that directory. In a Task that declares additional repositories, every path is written as `key::relative-path`, including the primary repository's paths, which use `primary_repository_key`; a missing key or an undeclared key fails with `repository_path_invalid` on the exact member. A single-repository Task uses plain relative paths. `acceptance_indexes` starts at 0 in the current Requirements `acceptance_criteria` array; `dependencies` refers to work-item IDs in the same plan.

`host-launch prepare` generates `launch_id` when it is omitted and uses that ID for receipt checks. Retry with the returned `receipt.launch_id` to resume the same launch; a receipt already in `prepared` skips fetch. An explicit ID must match the saved receipt.

Use the complete original Codex response as `dispatch-result` input `host_result`: a direct result object, `result`, `structuredContent`, `structuredContent.result`, or JSON in a single `content` text block when no structured result is present. Structured results take precedence; text JSON must be valid and contain no duplicate members. `isError: true`, missing identifiers, parsing failures, or multiple text blocks record `uncertain`.

A valid `clientThreadId` is saved as `operation_status.host_client_thread_id` with phase `queued`; a valid `threadId` is saved as `host_thread_id` with phase `dispatched`. Resubmitting the retained original result for the same `launch_id` and `repository_key` allows `uncertain → queued`; a later ready result follows `queued → dispatched`, retaining the queued ID and original dispatch marker. `dispatch-start` returns `should_dispatch: false` in all these phases. The Host continues inspecting the same creation; `clientThreadId` is not a task ID usable with tools requiring `threadId`.

Codex `dispatch-start` saves the complete `host_request` in `receipt.operation_status.host_request` and enters `dispatch_prepared`; repeated calls and `status` can read it back. `dispatch-call` uses the current `dispatch_attempt_id` to enter `dispatching`; only its first `should_dispatch=true` result permits one creation call. The caller writes complete command stdout to a private file, checks the exit code and parses JSON from that file before forwarding the request unchanged, avoiding display truncation.

When the previous caller has stopped and the creation tool was demonstrably never called, `dispatch-recover` accepts the current attempt ID, `host_call_not_made=true`, `previous_caller_stopped=true` and a specific `reason`, retains the request and issues a new claim ID for `dispatch-call`. Empty task IDs alone do not prove non-invocation. When creation was called but its result is unknown, the Host searches tasks and archived tasks using the saved title, launch ID and repository marker, reads complete initial messages and submits `candidates` (`thread_id`, `initial_prompt`) to `dispatch-reconcile`. Exactly one complete prompt match saves the task ID; zero matches, multiple matches or unavailable inspection never authorize another creation. Core continues to own Task state.

Worktree creation first confirms a local or remote source, base and target branches, and whether to carry local content.
`source_type` and `carry_changes` are required; local sources use `remote_name=""`, remote sources use
`carry_changes=false`. See [worktree sources and local changes](WORKTREE-SOURCES_en.md).

## Verification capacity for existing checks

When increasing verification capacity, `additional_checks` may refer to check names in the current plan or earlier adjustments; `rationale` explains the remaining work or rerun. Names remain unique within one submission, and concrete reasons, an actual increase and the existing limits are still required. Increasing capacity does not create passed results.

## Package preparation and publication

### Host Adapters

All four Hosts share standalone release options while maintaining separate package, Tag and public-version identities:

| Host | Release command | Exact confirmation format |
| --- | --- | --- |
| Codex | `pnpm run release:codex -- ...` | `codex-v<VERSION>` |
| DeepSeek | `pnpm run release:deepseek -- ...` | `deepseek-v<VERSION>` |
| Claude Code | `pnpm run release:claude -- ...` | `claude-v<VERSION>` |
| ZCode | `pnpm run release:zcode -- ...` | `zcode-v<VERSION>` |

```bash
pnpm run release:claude -- --channel stable --version "<VERSION>" --output "<ABSOLUTE_DIRECTORY>" --confirm "claude-v<VERSION>"
node scripts/build-host-release.mjs --product <codex|deepseek|claude|zcode> --output "<ABSOLUTE_DIRECTORY>"
```

`--channel` defaults to `stable`, accepts `MAJOR.MINOR.PATCH` and requires clean `main` synchronized with `origin/main`; `beta` accepts `MAJOR.MINOR.PATCH-beta.N` and uses a clean named branch. `--version` and `--confirm` are required, and confirmation must match the selected Host and version. `--output` is optional, defaulting to `taskbelay-releases/<host>-v<VERSION>` under the user's home directory; an explicit path must be absolute and outside the repository, and its parent must already exist. Fixed checks precede selected package and plugin/marketplace version updates. When version files change, the command commits and pushes them, then checks the resulting remote commit; only afterward does it prepare artifacts for publisher verification and publication. Only stable updates that Host's public-version entry.

`build-host-release.mjs` only prepares and verifies the five-file artifact set containing both Core runtimes; it does not publish. `--product` and `--output` are required; the output must be an existing empty absolute directory outside the repository. Implementations are `scripts/release-<host>.mjs`, `release/host-command.mjs` and `scripts/build-host-release.mjs`. Before each release, maintainers verify npm ownership, the selected package's Trusted Publisher and the exact version. See [Release Ownership](../release/README.md) for the complete requirements and artifact format.

### Formal desktop package preparation

```bash
node release/taskbelay/prepare.mjs --output "/absolute/pet-release"
```

Run on macOS arm64 with the repository toolchain and Swift >=6.0. This builds both application payloads and verifies the final tarball without publishing; output must be outside the repository.

## Host call examples

Core interaction instructions and complete examples for Codex, DeepSeek, Claude Code and ZCode are maintained in `skills/taskbelay/core/` and rendered into each package by the build scripts. Each Host documents its actual authorization, workspace preparation and tool calls. Execution uses the current Action, installed interface and real user decisions. Node submissions, result handling, blocker recovery and verification use the same content, and all four rendered example sets pass through the same Core validation.

[Codex Skill](../packages/codex/plugin/skills/taskbelay/SKILL.md) · [DeepSeek Skill](../packages/deepseek/skills/taskbelay/SKILL.md) · [Claude Skill](../packages/claude/plugin/skills/taskbelay/SKILL.md) · [ZCode Skill](../packages/zcode/skills/taskbelay/SKILL.md)

The DeepSeek Skill packages `scripts/artifacts.mjs`. Invoke the same read-only Core preparation commands with `node <actual Skill directory>/scripts/artifacts.mjs collect` or `prepare`. Inputs and results use the shapes in this document with `host="deepseek"`. The script reuses the Adapter runtime/data-directory resolution and creates no store. Resolve its path from the actual DSH Skill resourceBase. `--help` reads no stdin and resolves no runtime. It is not a standalone taskbelay-deepseek CLI or an additional workspace_coordinator operation.

## Core responses and known-failure acceptance

Every tool follows the [Core response contract](CORE-RESPONSES_en.md). Success requires result, failure requires error/recovery, and both have request_id/tool. Quantity limits return VERIFICATION_BUDGET_EXCEEDED with error.budget used/requested/limit; permission restrictions return VERIFICATION_NOT_ALLOWED and the affected field. Empty budget_adjustment.additional_checks yields field detail and, after Core proves zero writes, permits one correction of allowed_paths on the same Action.

`taskbelay_submit_test` adds `tests_accepted_with_known_failures` → COMPREHENSION_REVIEW with a concrete reason, original failed checks, a separate automated/passed comparison, and `node_result.known_failure_acceptance`: source=user, summary, failed_checks, comparison_check, task_plan_revision, content_digest. The decision binds what the user saw; it covers every failure, while all other checks pass and none remain pending or unexecuted. Other transitions omit this member or use null. Ordinary tests_passed still requires passed checks. See [Architecture](ARCHITECTURE_en.md#accepting-known-failures) for storage and delivery rules.

`allow_manual_handoff` only restricts pending manual work; completed user checks and separate acceptance can be recorded. Permission-only adjustments permit additional_automatic_commands=0 but still require explained additional_checks. Recovery probes in `taskbelay_get_task` and `taskbelay_get_next_action` copy complete saved operations. The published input schema is a Host transport hint: it retains top-level and probe member names, types and closed structure, but omits repeated probe required declarations and the deep check-source enum. Three deep objects—`verification_plan`, `budget_adjustment` and `known_failure_acceptance`—project as typed, open objects so the Host can pass complete saved operations. Core performs the runtime check: private input validation still requires `operation_id`, `action_id` and `payload`, and checks every identity, check source and the complete payload's required and closed rules; omitted or extra fields cannot change a saved operation. Ordinary node submission tool schemas and their check-source enum remain unchanged.

MCP correction uses `correct_current_action` for ordinary node submissions and `correct_request` for handshake, read, creation and lifecycle requests. Both require Core's zero-write proof and limit corrections through allowed_paths. `correct_request` preserves request identity and existing authorization without asking for an Action that may not exist. Both Host Skills link each request to a complete successful response and include a code-verified error response with implementation locations. History resolution reports enum failures at `history_resolution.choice` and text failures at `history_resolution.reason` independently.

For all 18 tools, `error.message` states the specific failed condition. Field errors include the path, rule and requirement; multiple causes remain in `details` or `guard.failures`. JSON syntax, types, duplicate members, state restrictions, repository observations, storage and response encoding have distinct explanations. Encoding failures retain the actual tool and existing request ID. Unknown lower-level causes are stated explicitly; see the [Core response contract](CORE-RESPONSES_en.md).

## Task continuity inputs

`taskbelay_get_task` accepts optional baseline_history={revision,after,limit}. First page uses
revision=0/after=0; continue with returned revision/next_after and limit 1..32. result.baseline_history
contains task_id, revision, total, next_after (null at end), entries[{sequence,reference}]. A changed
Task returns REVISION_CONFLICT; an out-of-range cursor is INVALID_ARGUMENT. Task baselines.history is
a bounded first page of the complete array, always accompanied by history_total, history_next_after
and history_revision. Storage is never truncated to page size or archived and has no configured count
limit; resources and revision-number ranges still apply. A page has at most 64 KiB of actual JSON and
the complete response at most 1 MiB, so fewer than limit entries may be returned. Continue only with
returned next_after, never by adding limit. An empty first page with nonzero total uses cursor 0 for
explicit pagination. Oversized non-history responses remain rejected.

`taskbelay_resolve_blocker` accepts rename_id and rename_choice=complete|cancel only for its prepared
branch-rename blocker. Complete verifies disappearance of the source ref and unchanged Task facts;
cancel requires unchanged original facts. Both return the Task directly. A saved unrecorded decision
uses recover_action with its action_id, not a replacement payload.

`taskbelay-codex host-launch supersede` reads launch_id, repository_key, expected_receipt_digest,
reason and replacement (the complete prepare input with a distinct explicit launch_id). status returns
receipt_digest. Supersede returns receipt_path, receipt, receipt_digest, predecessor and prepare_input.
Use the returned prepare_input for ordinary preparation. `supersede-resume` reads launch_id,
repository_key and seed_digest from the predecessor mark and returns the same shape. Neither command
executes target Git/Host operations. Refer to operation --help for the closed input schema.

Codex/Claude/ZCode cleanup-worktree and cleanup-branch additionally require core_task: the actual
complete terminal Core Task. The helper matches its repository key, provisioning origin, root and
effective branch/HEAD and preserves separate authorization. Claude host-launch relocate additionally
requires core_preparation, the actual complete result of prepare_task_relocation; ZCode already uses
that input. Creation receipts retain their original target branch. Missing effective facts never select
that old name. DeepSeek cleanup reads its terminal Core Task and uses its current_branch.
