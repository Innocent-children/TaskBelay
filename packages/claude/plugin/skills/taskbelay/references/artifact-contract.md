<!-- Generated from skills/taskbelay/core/artifact-contract.md; edit the shared source and run node scripts/sync-skill-references.mjs. -->

# Core artifact preparation

Use the [Host artifact entry](artifacts.md) for the executable and current authorization.
The existing Core collect/prepare commands accept one closed UTF-8 JSON object (at most 1 MiB),
return {ok:true,result} or {ok:false,error} and exit 0/1. They read the existing Task store/Git,
and create no Task, operation, blocker or cursor.

Implementation: `cmd/taskbelay/artifacts.go` — `runArtifacts`, `decodeArtifactInput`.

## Collect and classify

Implementation: `internal/application/artifacts.go` — `collectArtifacts, PrepareArtifacts`.

Use the Task/Action identity from the same fresh Action:

Request types: the input is an object; `host`, `task_id` and `action_id` are strings.

<!-- example:artifact collect current -->
```json
{
  "host": "claude",
  "task_id": "task-example",
  "action_id": "action-example"
}
```

Complete output shape example for a requirements document:

Response types: `ok` is a boolean and `result` is an object. Within `result`, `revision` is an
integer; `files` is an array of objects; all other fields and each file entry's fields are strings.

<!-- example:artifact-output collect one-file -->
```json
{
  "ok": true,
  "result": {
    "task_id": "task-example",
    "action_id": "action-example",
    "revision": 1,
    "observation_digest": "2222222222222222222222222222222222222222222222222222222222222222",
    "files": [
      {
        "path": "docs/requirements.md",
        "change_type": "added",
        "digest": "4444444444444444444444444444444444444444444444444444444444444444",
        "slot": "",
        "summary": ""
      }
    ]
  }
}
```

The collection includes every observed Action delta: hidden, staged, unstaged, non-ignored untracked,
committed and restored paths. `restored` means the earlier Task path returned to base content.
An empty collection means no current Action delta, not no total Task changes.
Keep every entry and all identity/digest/change fields. Edit only `slot` and `summary`:

| slot | Purpose |
| --- | --- |
| `current` | Current node process document, only when that node exposes the slot. |
| `other_process` | A related process/method document. |
| `product` | Product changes at IMPLEMENT/REFACTOR; excluded from generated artifact arrays. |

Classify by the file's actual purpose, including method initialization files and metadata. A directory
name is not blanket approval. Unexplained/forbidden changes stop for the applicable scope decision.

## Prepare and submit

Keep the complete classified collection and call prepare:

Request types: `host` is a string; `collection` is an object with the same field types as the
collect response's `result`. Keep `files` as an array of objects, not a JSON-encoded string.

<!-- example:artifact prepare one-file -->
```json
{
  "host": "claude",
  "collection": {
    "task_id": "task-example",
    "action_id": "action-example",
    "revision": 1,
    "observation_digest": "2222222222222222222222222222222222222222222222222222222222222222",
    "files": [
      {
        "path": "docs/requirements.md",
        "change_type": "added",
        "digest": "4444444444444444444444444444444444444444444444444444444444444444",
        "slot": "current",
        "summary": "Record the current endpoint requirements."
      }
    ]
  }
}
```

Complete success shape:

Response types: `ok` is a boolean; `result` is an object; `result.current` and
`result.other_process` are arrays of objects with string `path`, `digest` and `summary`.

<!-- example:artifact-output prepare one-file -->
```json
{
  "ok": true,
  "result": {
    "current": [
      {
        "path": "docs/requirements.md",
        "digest": "4444444444444444444444444444444444444444444444444444444444444444",
        "summary": "Record the current endpoint requirements."
      }
    ],
    "other_process": []
  }
}
```

Pass the full `result` directly as the node submission's `artifacts`. Separately verified unchanged
references may be added in allowed slots; preserve all generated entries. Do not write again before
submission. A changed observation requires a new collection/classification. Missing, duplicate,
altered or unclassified entries receive correction before prepare is repeated. `ACTION_STALE`,
`WORKSPACE_HISTORY_CONFLICT` or `REPOSITORY_DRIFT` requires a guarded next-Action read and its decision;
`WORKSPACE_UNAVAILABLE` follows restoration/abandonment. This helper error has no MCP recovery envelope.

## Content after implementation

Implementation: `internal/application/workspace.go` — `contentDiffersFromCurrentAuthority, invalidateContentEvidence`.
Implementation: `internal/repository/fingerprint.go` — `digestWorkspaceContent`.

Core content digests include Git-visible process files. At TEST, COMPREHENSION_REVIEW and DELIVERY,
collect rejects content different from the current implementation/Test record. A guarded next read
then invalidates current Implementation/Test/Comprehension and returns to IMPLEMENT. Thus finalize
repository process-file updates before the final implementation submission; record test/understanding/
delivery results in Core and reconcile files read-only afterward. If a later file update is required,
follow a returned remediation edge, make the change in an allowed node, then re-establish verification.

Example: archiving `openspec/changes/endpoint/` during DELIVERY changes Git-visible paths. Treating
those paths as other_process does not bypass the content guard; perform any appropriate archival
write before final verification. A linear commit preserving the same content is distinct from a
content change; the existing workspace guard handles both.

## Planning files and confirmation

Finalize requirements, design and plan documents before saving the TASKS draft. Show exact expected files and the reasons for directory ranges. After tasks_plan_saved, retain the complete saved plan and its returned content identifiers. Any revision requires another plan save and current user confirmation; submitting a verdict is not an opportunity to edit the plan being approved.

When the Task declares additional repositories, qualify every submitted repository path, including
the primary repository's, with its key. A plan for a Task whose primary repository is `core` and
which also declares `docs` writes both sides of the change:

```json
{"work_item_id":"work-endpoint","summary":"Return the field and update the shared guide.","expected_paths":["core::src/endpoint.js","docs::guides/endpoint.md"],"acceptance_indexes":[0],"verification_steps":["Run endpoint-check."],"dependencies":[]}
```

The same qualification applies to `artifacts.current` and `artifacts.other_process` entries. A path
without its key, and a key the Task does not declare, both fail with `repository_path_invalid` on the
exact submitted member. A single-repository Task writes plain paths and refuses a keyed one.

## Confirmed carried content


Implementation: `internal/application/apply_action_results.go` — `applyTaskPlanResult`;
`internal/domain/file_scope.go` — `UnexplainedChangedPaths`.

For local carry, record an actual requirement to preserve the confirmed content and its acceptance
check. In TASKS reconcile full `current_changed_paths` with expected_paths and retained process
artifacts. Assign each carried path to development or bounded preservation work. Compare unaffected
carried contents with the saved launch snapshot; report preservation separately from new behavior.
An empty Action collection does not erase carried scope. If replanning requires a missing preservation
requirement, use the current TASKS requirement-return edge before rebuilding the plan.

Example work item (within the full TASKS baseline):

```json
{"work_item_id":"preserve-notes","summary":"Preserve the confirmed local notes.","expected_paths":["notes/change.md"],"acceptance_indexes":[1],"verification_steps":["Compare notes/change.md with the retained launch snapshot."],"dependencies":[]}
```

The sample index 1 requires a real second preservation criterion. Carry authorization preserves
contents; it does not declare existing business behavior tested or authorize unexplained new paths.
