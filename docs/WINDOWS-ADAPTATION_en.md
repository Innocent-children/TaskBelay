# Windows adaptation and current verification

[中文](WINDOWS-ADAPTATION.md) | [English](WINDOWS-ADAPTATION_en.md)

The engineering-control source was verified on Windows x64, Go 1.27.0 and Node.js 24.18.0. Platform implementations own paths, file identity, permissions and process lifetime; Core semantics do not select an OS. Execution uses hidden processes and Windows Jobs. Recovery checks the original owner's startup identity and real receipt; this is not a sandbox.

Full Go checks, native development/revision/recovery, four independent Host packages and Windows desktop display rules passed. The final local ZCode artifact exercised isolated launchers, MCP, real checks and path hooks without operating its actual UI. Unavailable native file-symlink conditions remain explicit skips rather than passing results. Historical adaptation records remain in Git history; [Project status](PROJECT-STATUS_en.md) owns current evidence.

macOS arm64 was cross-built with metadata inspection only. Native Swift tests, model sessions, signing, notarization and publication were not performed. User installations, Task data and original main were unchanged. See [Support](SUPPORT-MATRIX_en.md), [Commands](COMMANDS_en.md) and the [desktop guide](DESKTOP-PETS_en.md).
