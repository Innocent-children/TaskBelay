# Artifact collection and submission

[中文](ARTIFACTS.md) | [English](ARTIFACTS_en.md)

## Responsibilities

Core observes Git and computes the complete current Action delta. Codex classifies each file and
describes its purpose. Read-only preparation generates artifact references from those observed
facts. A successful preparation is permission to construct the submission; Core still validates the
node result, transition, workspace identity, history, allowed effects and file scope when submitting.

The process definition, transitions, Task revision and persisted Schema are unchanged by collection
or preparation. These commands write no Task, Event, Claim, Action operation or Git state. Temporary
JSON files belong outside Task worktrees. Ignored files are outside Core's Git-visible scope.

## Commands

Read the installed command help before building an input object:

```bash
taskbelay-codex artifacts --help
taskbelay-codex artifacts collect --help
taskbelay-codex artifacts prepare --help
```

The group help lists both operations. Operation help returns a JSON object containing `operation`, `description`, `transport`, `input_example`, `input_fields`, `output_fields` and `next_step`. Replace example identities and observations with the complete values from the current Action or collection. Help reads no stdin, installation path or Task data and starts no Core process. Core validates actual command inputs.

For a custom data directory, set `TASKBELAY_DATA_DIR` before starting Codex. The Plugin explicitly forwards it to MCP; the hook and artifact commands inherit the same Host environment. The directory must already exist and have a canonical absolute path. Omit the variable to use the default data directory. Launch environment changes require a new Codex session.


`taskbelay-codex artifacts collect` forwards to the packaged `taskbelay artifacts collect`.
`taskbelay-codex artifacts prepare` forwards to the packaged `taskbelay artifacts prepare`.
Both read one closed UTF-8 JSON object from stdin, bounded to 1 MiB. Unknown or duplicate members,
trailing JSON and malformed UTF-8 are rejected. The commands use the existing Task database, return
JSON on stdout, and exit with 0 on success or 1 on failure. They do not initialize storage.

Collect input:

```json
{"host":"codex","task_id":"task-example","action_id":"action-example"}
```

The successful response is `{ "ok": true, "result": <collection> }`. The collection contains
`task_id`, `action_id`, `revision`, `observation_digest`, and `files`. Each file has `path`,
`change_type`, `digest`, `slot`, and `summary`. Core fills the facts and leaves the last two strings
empty. Preserve the entire collection and fill only `slot` and `summary`.

`path` is repository-relative for a single repository, or `<repository-key>::<relative-path>` for
an existing multi-repository Task. The delta includes hidden files, staged and unstaged changes,
untracked files and changes committed during this Action. `change_type` describes the current Task
surface relative to its frozen base; `restored` means a previously retained changed path returned
to the base state. Digests describe observed path state, including deletion; they are not copied
from model-generated text. File enumeration is complete within Core's existing repository limits.

Prepare input is `{ "host": "codex", "collection": <classified collection> }`:

- `slot="current"`: current-node process document, only where the node exposes this artifact slot.
- `slot="other_process"`: related method document or support file.
- `slot="product"`: product change, permitted only at IMPLEMENT and REFACTOR; omitted from artifact
  arrays and still checked against the Task Plan by the ordinary submission.
- Every entry needs a non-empty bounded `summary`. Unknown purposes must be resolved before preparing.

Preparation reobserves Git and rejects changed observations, missing or duplicate paths, altered
file facts, unclassified entries and roles unavailable at the current node. The successful `result`
is the generated `artifacts` object with `current` where allowed and `other_process`. Use it directly
in the ordinary submission. Separately verified unchanged document references may be appended to an
allowed slot. Further repository writes require recollection and preparation. Preparation does not
replace the ordinary submission's live-schema validation or authorization checks.

For stale Action, history-conflict or repository-drift errors, read the next Core Action and follow
its workspace decision. A changed observation requires recollection; missing or unclassified entries
are completed before preparing again. Workspace unavailability follows the existing restore or
abandonment rules.

## OpenSpec files

Classify the proposal and specifications as current-node documents where appropriate. Inspect and
classify initialization configuration, change metadata and generated README files as well. File
location alone does not authorize a change; no `openspec/**` allowlist or automatic classification
of remaining files is introduced. OpenSpec validation checks content and structure; the collection
and preparation commands check file coverage.

## Rejection and correction

An ordinary process-artifact submission that omits changed files returns `INVALID_ARGUMENT` with
the closed rule `artifact_manifest_incomplete`. `error.repository_paths` carries validated omitted
file paths, separately from JSON field paths. After a proven zero-write failure, MCP returns
`correct_current_action`, `retry_safe=true`, and only the applicable artifact fields in
`allowed_paths`. The Host may collect, classify and prepare again, correct only those artifact
fields through the same current submission tool once, and must stop if that submission also fails.
Semantic results, transitions and method conclusions keep the same meaning. HTTP exposes omitted
paths alongside `field_paths`; WebUI displays both. Real workspace/history failures and uncertain
operations keep their existing recovery routes. Correction never bypasses current Core checks.

## Verification

The targeted application regression covers five changed files with only two declared, exact omitted
paths, zero writes on rejection, the unchanged Action on reread, and successful corrected submission
to DESIGN. The native Git/CLI test covers hidden and Unicode paths, committed/staged/untracked
changes, preparation, unchanged database/Git bytes, post-collection content changes and a branch
switch. Application and MCP tests cover forbidden product classification, stale Action/worktree
identity, bounded correction and unsafe public path rejection. These are local automated tests,
not a real Codex session running the OpenSpec executable or a Windows validation claim.

## Process files after final verification

`collectArtifacts` compares current content with the retained Implementation/Test content digest in TEST, COMPREHENSION_REVIEW and DELIVERY; process files are included. Codex therefore completes process-file updates before final verification and reconciles them read-only afterward. If another update is needed, it uses a current legal return path and re-establishes verification. Classifying a file as `other_process` does not bypass the content check. Implementation: `internal/application/artifacts.go` and `internal/application/workspace.go`.

The DeepSeek Skill packages `scripts/artifacts.mjs`. Invoke the same read-only Core preparation commands with `node <actual Skill directory>/scripts/artifacts.mjs collect` or `prepare`. Inputs and results use the shapes in this document with `host="deepseek"`. The script reuses the Adapter runtime/data-directory resolution and creates no store. Resolve its path from the actual DSH Skill resourceBase. `--help` reads no stdin and resolves no runtime. It is not a standalone taskbelay-deepseek CLI or an additional workspace_coordinator operation.

## Baseline references and Action identity

All older requirements, design and task-plan references remain in the Task in saved order, without
archiving or a configured count limit. Resource and revision-number ranges still apply. The response
first page and complete-history pages preserve each revision, digest, summary and timestamp, with an
explicit total and next cursor. Page size never controls retention. Missing full documents are not
reconstructed, and the current artifact set and content checks remain unchanged. File submissions
must still cover the paths required by Core.

Branch-rename preparation issues a new BLOCKED Action. After resolution, read the current Action
and collect or prepare its artifacts against the saved effective branch. Never reuse the old Action
or treat the creation branch as the current binding. Core verifies repository instance, HEAD, index
and content; an artifact list cannot stand in for those checks.
