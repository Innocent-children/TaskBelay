# Working directories, branches and local changes

[中文](WORKTREE-SOURCES.md) | [English](WORKTREE-SOURCES_en.md)

## Use case

Developers can reuse the dependencies, local configuration and build outputs in an existing directory
while TaskBelay retains development progress. The default creates a task branch from the current HEAD
in the current directory. Explicit alternatives retain the current branch or create a dedicated worktree.
The Host shows the actual directories, branches and initial changes, and retains valid user choices.

## Workspace choices and defaults

`workspace_mode` is required. The Host uses `new_branch` unless the user selects another mode:

| Mode | Host preparation | Retained `workspace_origin.mode` |
| --- | --- | --- |
| `new_branch` (default) | Create and switch to the confirmed branch from current HEAD in the original directory | `new_branch` |
| `current_branch` | Retain the directory and current named branch | `current_branch` |
| `dedicated_worktree` | Prepare a separate directory using the source rules below | `dedicated_worktree` |

Local modes require `source_type=local`, `remote_name=""`, the starting current branch as `base_branch`,
and its HEAD as `base_commit`. `current_branch` keeps that branch; `new_branch` requires an unused target.
Previously committed contents are starting code. Confirmed uncommitted contents and later changes
belong to the Task surface. Here `carry_changes` accepts initial local changes while keeping the index,
files and ignored contents in place; the snapshot is not applied to the original directory. Unaccepted
initial changes stop preparation. The Host does not automatically stash, clean or ignore other work.

Before local branch operations, the Host calls Core's read-only `host-check workspace-available` with
`{"repository_path":"<canonical root>"}` and reads `available`, `repository_path` and optional `task_id`.
An active claim or an unavailable check stops preparation. This check does not reserve the directory;
Core acquires all unique directory claims in the creation transaction. Users and external tools must
avoid starting another writer during preparation.

When the current session has every required directory permission, local modes continue there. Codex
uses the `current_session` surface and `local-provision`, with `prepare.handoff_file=null` and retained
`handoff_digest=null`. Requirements handoff files are needed only for a session handoff. An all-local
DeepSeek launch returns `ready` and the full `open_task` descriptor without relaunch or consume.
Repositories may choose different modes, but the actual execution session must cover every root.
DeepSeek mixed launches restart from their common parent; the running session does not gain broader
permissions. Any repository failure prevents a partial Core Task. Retain local branches and changes
for inspection.

Local resume uses the original directory instance. Branch/history conflicts, content invalidation,
file scope and verification rules remain in effect. Local modes support neither workspace relocation
nor assisted deletion of the directory or branch. DONE/CANCELLED releases claims and retains work.
Uncommitted contents remaining after one Task become initial changes for the next Task.
The UI presents the lifecycle operations supported by each mode.

The `standard-development` definition digest, nodes, full outgoing edges, guards, Action allowed effects
and verification requirements are unchanged. This changes workspace creation and lifecycle contracts,
without adding a process node or another Task state.

## Available data and responsibilities

The Host reads local branches, remotes, source HEAD, staged, unstaged and untracked paths and presents
the choices. The Host creates Git resources and copies content. Core checks worktree identity, source,
branch, HEAD and actual changes read-only, then creates the Task.

`source_type` is required and accepts `local` or `remote`; `carry_changes` is a required boolean.
Local sources use an empty `remote_name` and a local `base_branch`. Remote sources require a valid remote
name and `carry_changes=false`. `target_branch` names the new task branch. Missing choices have no defaults.

## Dedicated-worktree source rules

1. Preparation starts only after the choices are explicit. Remote sources fetch the exact ref; local
   sources resolve `refs/heads/<base>` without network access.
2. The Host retains `base_commit` and creates the dedicated worktree from that commit. Codex managed
   dispatch also uses this frozen commit.
3. When carrying local changes, the Host creates Git snapshot objects and retains `snapshot_commit`.
   The snapshot preserves staged and unstaged layers and includes non-ignored untracked files.
   The source checkout, source index, HEAD and `refs/stash` remain unchanged.
4. The Host applies the snapshot on the new branch with staging state restored. When another local
   branch is selected, Git applies the changes to it. Conflicts or file collisions stop Core Task
   creation and retain the destination worktree and failed receipt for inspection.
5. Core checks the dedicated worktree, named branch, frozen commit and source ref. Initial changes
   are allowed only for local `carry_changes=true`; other choices require a clean worktree. Carried
   content belongs to the Task surface and remains subject to scope and verification rules.
6. Each repository retains its own choices; all repositories must be prepared before one multi-repository
   Task opens. Recovery reads the original receipt. Uncertain results never repeat creation or snapshot
   application. Later source edits do not alter captured content.

A dedicated worktree does not require a clean or unclaimed source checkout. When changes are not
carried, the Host creates a clean destination from the frozen commit and leaves the source directory
and its claim intact. The local-mode checks for accepting initial edits and source claims do not apply
to this read-only source.

Snapshot preparation rejects unresolved conflicts and submodule changes. Ignored files are excluded.
DeepSeek assisted cleanup retains local-source branches for separate user inspection and handling;
local creation does not gain a remote dependency.

All four Hosts use the same shared snapshot implementation. Before returning a snapshot, it reads
HEAD, the index tree, the working-file tree, and the untracked tree twice in succession. A difference
stops preparation without returning a snapshot or retrying automatically. Further edits to already
modified files are compared through their content trees even when Git status stays the same. Failure
retains source files, staged state, and the launch record; inspect the source before deciding how to
continue. Preparation still requires one writer. This check does not provide an atomic snapshot under
arbitrary external concurrency or guarantee detection of content that changes and then changes back.

The assessment anchor has a different responsibility: it retains observations such as the request,
directory, HEAD, and status without comparing working-file contents. Equal status digests do not prove
that an already modified file is unchanged. Hosts reassess known requirement or code changes while
waiting for a choice.

## Launch records and Task state

Host receipts retain `workspace_mode`, `source_type`, `carry_changes` and `snapshot_commit`, and retain the starting point as
`base_commit`. Preparation follows `confirmed -> resolving -> prepared`, then the existing dispatch and
worktree provisioning phases. Failures and uncertain results retain `failed` and `uncertain`. These
are Host operation records, not additional Core process nodes.

`WorkspaceOrigin` and CLI/MCP/WebUI projections include `source_type` and `carry_changes`. Core continues
to own identity, recovery and process rules. The current `standard-development` definition digest, nodes,
outgoing edges, guards, Actions and verification requirements are unchanged. `tasks.snapshot` retains the selected workspace mode and origin fields; the SQL table layout is unchanged.

## Expected result and impact

Local branches reuse the existing environment offline. Dedicated worktrees copy uncommitted content only by user choice. An
application failure preserves the source checkout and does not report a successful Task. Carried content
counts toward the Task surface; it is not completed work or a passed verification result.

## Acceptance checks

- Codex `task-launch.test.mjs`: temporary Git repositories cover remote creation, both offline local
  content choices, staged separation, untracked files, ignored-file exclusion, source preservation,
  frozen snapshots, different base branches, managed bootstrap and conflict retention.
- DeepSeek `workspace-coordinator.test.mjs`: explicit choices, offline creation, binary untracked content
  and consume after relaunch.
- Targeted snapshot regressions for all four Hosts edit already dirty tracked/untracked files after
  the first tracked-content capture. They check that unchanged status still stops preparation and
  preserves source files, the index tree, HEAD, and stash. Generated-copy checks separately compare
  packaged files with the shared source byte for byte.
- ZCode `workspace.test.mjs` uses temporary Git repositories for all three workspace modes, carried contents, partial preparation failures, replaced instances, relocation and separately authorized cleanup; `open`/`resume` only return UI continuation guidance.
- Core `workspace_observer_test.go`: local admission without a remote, required carry authorization for
  initial changes, rejection of remote carry, and retained actual Task surface.
- Core `workspace_check_test.go`: real Git and SQLite cover local creation, accepted initial changes, unique claims, same-content commits, resume, branch-switch blocking, relocation rejection and cancellation without deleting local files. The availability check creates no database when none exists.
- Local Host tests cover both branch modes, index/ignored-file retention, ready-in-session output, active-Task refusal and provisioned retries without Git mutation.
- Package, CLI/MCP input and storage checks cover the new fields and packaged snapshot modules.

These checks use native temporary repositories and Host helpers. Managed dispatch uses simulated Host
creation results; they do not establish an actual Codex, DSH, Claude Code or ZCode session end-to-end run and do
not extend support claims for other platforms.

## Non-goals

This change adds no Core Git writes, automatic commits or publication, historical-data compatibility,
cross-machine copying, ignored-file copying or submodule-content copying.

## Codex planning and preservation checks

When Codex carries local changes, it records their preservation in REQUIREMENTS and reconciles the complete `current_changed_paths` with `expected_paths` and retained process artifacts in TASKS. New development and preservation receive separate work and checks; an empty current-Action file collection cannot replace the complete Task path comparison. Preservation checks compare the launch snapshot and do not certify existing behavior as tested. Existing file-scope blockers continue through the current Core choices and transitions.

## Correct an unexecuted Codex launch or active branch name

For an unexecuted Codex launch, read host-launch status and retain its receipt_digest. Explicit
supersede takes that digest, a reason and complete newly confirmed prepare input with a distinct launch_id
and the same repository key/source. Superseded receipts permanently reject execution and keep one
complete successor seed. supersede-resume takes the predecessor identity and seed_digest; it recreates
only that successor, without overwriting another receipt or inventing a new launch. Preparation,
supersession, dispatch claim and target Git calls share the receipt lock. A claim or possible target call
prevents supersession; a failed/resolving record needs positive not_invoked evidence. Missing directories,
empty Host IDs and failure alone are insufficient. Unknown or partial operations require reconciliation.

For an active Task, prepare one branch rename through Core before the Host runs `git branch -m` once.
Complete only after Core checks the exact refs and unchanged workspace/index/content; cancel only before
the source facts change. After interruption read the same Task, inspect the actual refs, and recover its
saved resolution if present. A same-HEAD branch switch, recreated source ref, existing target, changed
index or another repository's changes cannot pass. Preparation does not authorize commit, push or force.

The origin branch stays in the receipt and WorkspaceOrigin. Resume reads the same Core Task; its
repository.current_branch is authoritative for later history, relocation and cleanup. Codex, Claude and
ZCode cleanup commands require the actual terminal core_task. Claude and ZCode relocation consume the
actual core_preparation result. DeepSeek reads its terminal Task directly. Missing or mismatched facts
stop the helper; no old-name fallback is used. Codex receipts without a saved worktree instance proof
require manual inspection before cleanup. These operations do not introduce automatic cleanup.
