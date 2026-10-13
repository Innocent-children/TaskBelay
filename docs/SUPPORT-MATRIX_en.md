# TaskBelay support matrix

[中文](SUPPORT-MATRIX.md) | [English](SUPPORT-MATRIX_en.md)

This record concerns the current engineering-control source and does not change public-version support commitments. Host guides describe public installation; machine-readable records under release/ own published identities.

## Current source

| Component | Implementation scope | Verification in this change |
| --- | --- | --- |
| Go Core, CLI, MCP, HTTP | Existing user authority, scope/verification control, Git observation, actual argv execution, cross-repository evidence and recovery | Earlier Windows x64 record; native macOS arm64 regressions here, Windows launch fix cross-compiled only |
| Codex | Current tools, Skill, workspace operations and apply_patch path preflight | Actual Core calls from an independent package and adapter tests; no model chat |
| Claude Code | Current tools, Skill, workspaces, Write/Edit/NotebookEdit preflight | Actual Core calls from an independent package and adapter tests; no model chat |
| ZCode | Current tools, Skill, workspaces and registered structured write hooks | Actual Core calls from an independent package and adapter tests; no UI installation or model chat |
| DeepSeek/DSH | Current catalog, Skill, workspaces, write/edit/str_replace_editor preflight | Actual Core calls from an independent package, catalog and adapter tests; no DSH model chat |
| WebUI | Local results, evidence, gaps, cumulative cost and user observations | Frontend build, actual HTTP checks and local browser display inspection |
| Desktop pet | Displays Core activity, blockers, completion and cancellation | Windows display-rule unit checks; macOS Swift checks unavailable here |

Adapter packages require Node.js >=24. Tool and permission coverage must be verified in the actual Host session. Installation or an independent Core handshake does not establish that the model used the plugin. ZCode action_required still requires user UI operations.

## Platforms

| Exact runtime pair | Scope in this change |
| --- | --- |
| win32-x64 | Windows 10/11 desktop x64 implementation; earlier verification retained, changed suspended launch still needs native checks |
| darwin-arm64 | macOS arm64 implementation; targeted Core tests, actual process recovery and independent Host-package checks, no model sessions |

Runtime selection rejects win32-ia32, win32-arm64 and darwin-x64. Linux, Windows Server, Rosetta and remote MCP have no current support commitment. Compilation or simulated platform branches do not expand support. Formal desktop signing and notarization were not checked here.

## Control boundary

Supported structured write hooks can preflight forbidden paths. Shell, external edits and operations outside hook coverage are observed at checkpoints; they are not continuously intercepted. Semantic scope needs Host reasoning with cited basis. Go exported declarations and dependency changes are program observations, not general semantic proofs. Services, files and conditions outside declared inputs and environment are not treated as verified.

Current internal protocols are incompatible with historical data; Core preserves and rejects it. Use an independent data directory for source evaluation. See [Project status](PROJECT-STATUS_en.md) and [Commands](COMMANDS_en.md) for records, installation and permissions.
