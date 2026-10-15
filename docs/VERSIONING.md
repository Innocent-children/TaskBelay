# Product Versions

TaskBelay maintains independent product versions:

```text
Core      -> CORE_VERSION
Codex     -> packages/codex/package.json.version
DeepSeek  -> packages/deepseek/package.json.version
Claude    -> packages/claude/package.json.version
ZCode     -> packages/zcode/package.json.version
CLI       -> packages/taskbelay/package.json.version
```

Each Codex/Claude/ZCode plugin manifest and the ZCode marketplace entry copy their corresponding package version; they do not define separate versions. Root
`package.json` is private monorepo tooling and has no version.

Products evolve independently. Releasing one product changes only its version file and
the copies that must match it. Host Adapters may package a different Core version; build and release checks
read it from the actual Core executable.

SQLite additionally has one Core-owned database Schema version, currently `2.5.0`. It identifies
the one supported persisted layout and changes with that current layout.

Internal protocols, limits, snapshots, process definitions, payload contracts,
receipts, build reports, release manifests, and publication records do not have maintained version
numbers. Current capabilities, allowed fields and tools, content digests, artifact digests, and
runtime behavior define the supported interfaces and rules.

Product Tag prefixes are `core-vX.Y.Z`, `codex-vX.Y.Z`, `deepseek-vX.Y.Z`, `claude-vX.Y.Z`,
`zcode-vX.Y.Z` and `taskbelay-vX.Y.Z`. The release entrypoints cover all four Host Adapters and the CLI. Historical unprefixed
Tags remain frozen and are not used to determine current product versions.

Host `stable` releases add or update the selected product in `release/public-versions.json`; `beta`
releases leave those stable identities unchanged. A first stable release can use the current source
package version and add its missing public identity without requiring a previous Tag.

Local package metadata and release tooling do not establish a published release or verified Host
support. Claude and ZCode remain outside the stable release metadata until their own confirmed
release runs. See [Release Ownership](../release/README.md) for npm setup and publication requirements.


Before 1.0, incompatible Core contract changes increment MINOR. The current source engineering upgrade changes the executable and data contract; stable public identities remain under the separate release flow.
