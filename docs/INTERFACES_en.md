# Using TaskBelay interfaces

[中文](INTERFACES.md) | [English](INTERFACES_en.md)

Adapters obtain inputs, outputs, authority requirements and failure handling from method descriptions, without Go/JavaScript source or SQLite access. Existing entrypoints remain. Complete contracts are requested per method so routine calls do not repeat the whole catalog.

## Discover and invoke

Core `taskbelay_server_info` returns 15 actual tool names, native check information and `auxiliary_contracts`. Its optional `contracts` array selects complete `name`, `description`, `input_schema`, `output_schema`, `read_only` and `examples`; native/HTTP entries also explain `transport`. Discovery never executes the selected method. CLI `call <tool> --help --json`, `check run --help --json` and matching `host-check` help work without a database.

Codex, Claude and ZCode provide packaged contracts through `host-launch <operation> --help --json`. Hooks, lifecycle operations and pet commands also provide method help. DeepSeek's `contracts/interfaces.json` contains all six workspace_coordinator branches. Read it directly or call the installed `lib/interface-reference.mjs` function `describeInterface(packageRoot, name)` with the installation directory and exact entry name; it returns the contract. Omitting name returns names/descriptions without executing workspace operations.

The complete reference is `contracts/interfaces.json`. Each entry has a complete success example and an adjacent concrete failure for the same method, with preconditions. Paths, identities, timestamps and timings are illustrative; actual calls must retain real returned identities. `implementation` references are for maintainers, not prerequisites. External Host drivers, user decisions and desktop actions in management examples are explicitly simulated; they establish serialization, not a real installation or chat.

| Entrypoints | Entries | Packaged/native discovery |
| --- | ---: | --- |
| Core MCP methods | 15 | server_info.contracts / call method help |
| Native check and Host preflights | 3 | check run / host-check help |
| Codex / Claude / ZCode workspace operations | 16 / 13 / 11 | host-launch help |
| DeepSeek coordinator branches | 6 | Installed contracts/interfaces.json |
| Loopback HTTP | 6 | GET/POST names in server_info.contracts |
| Native hooks | 3 | hook pre-tool-use help |
| Core configuration, service, help, version, MCP startup | 8 | Corresponding method help |
| Unified lifecycle and pet operations | 8 / 2 | Corresponding method help |
| Three Host lifecycle adapters | 9 | status/setup/remove help |

These 100 entries document existing operation branches; they do not add 100 tools. Core MCP/CLI/HTTP aliases are counted once. HTTP `/api/core/{tool}` arguments and outputs depend on the selected Core method, whose contract must also be read.

## Consume results

Task mutations return `identity`, `digests`, `decision.gaps`, `progress`, `checks`, `concerns`, `limitations`, `issue`, `usage` and `budget`. Copy actual `identity.task_id/revision` into the next mutation; never increment revisions yourself. open/get add `context` with the request, full plan and repository summaries. `get_task detail:true` adds explicit `details` for assessment, facts, applicability, bindings and history, never a writable internal Task snapshot.

Native `check run` executes the command once and returns `view` plus `run`. `ok:true` means the request was processed; inspect `run.status`. Read stdout/stderr under `get_run.output`. `CHECK_ALREADY_SATISFIED` identifies the applicable existing result; reuse it rather than executing again to register it.

`refreshed:false` describes saved state. Refresh to establish applicability to current files. A saved passed result remains passed, but applicable=false cannot be ignored. Unset caps have `limited:false` and `remaining:null`. Unreported Host metrics are null; only measured zero is 0. Partial numeric totals represent the known portion, not total usage.

Omitted assessment.concerns and empty arrays preserve original questions. Submitting the same question without a resolution reopens it. Explicit resolutions use verified, not_blocking or scope_changed. not_blocking derives a delivery limitation and cannot waive required checks or deterministic violations. Input DTOs exclude read-only Core stamps such as scope_digest.

## Handle failures

Core errors contain specific fields/gaps and, when available, current identities or existing results. `outcome` is not_applied, applied or unknown for semantic commit; metering is separate. Native failures may include execution with the known startup phase. `recovery.operation` says whether to reuse, replace, read before deciding, or take no operation action. Neither a nonzero exit nor an error response proves that effects did not happen.

Interpret Host CLI stdout, stderr and exit status separately; never concatenate them into JSON. MCP always retains complete structuredContent. ZCode uses concise text, while Hosts that primarily display text retain the full envelope. HTTP uses real status codes and the shared error body; mutations require exact loopback Host, current Origin and page session token. Workspace creation, session dispatch and deletion continue to require actual authority.

After response loss, inspect the original Task/run/workspace receipt. Do not reconstruct a payload to repeat an uncertain action. Installation/session action_required means the returned actual actions remain; it is not ready or proof of an opened chat.

## Maintain and verify

Core DTOs and runtime Schemas are owned by `internal/protocol`. Exporters execute isolated Core/Git/HTTP and public serializers, recording Core, Host, management and runtime examples under contracts. `scripts/package-interface-contracts.mjs` combines and packages them; `--check` detects missing synchronization. Help reads only package-local resources.

`internal/protocol/examples_test.go` executes actual producers, validates successful/failed outputs and compares complete errors. `tests/contract/interfaces_test.go` validates complete response examples for all 100 entries. `scripts/interface-contracts.test.mjs` checks registered coverage, field notes, success/error examples and installed-only help without Core/Git/storage. Actual model chats and final published-package installation remain separate verification.
