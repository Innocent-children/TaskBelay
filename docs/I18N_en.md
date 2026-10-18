# TaskBelay Documentation Internationalization

[中文](I18N.md) | [English](I18N_en.md)

## Scope

This policy governs human-readable repository documentation; it does not define runtime product
locales. Source code, machine-readable schemas, package manifests, CLI parsers, and executable tests
still define runtime behavior.

## Maintained languages

When root README content changes, all nine languages are updated together. Detailed documentation
keeps paired English and Simplified Chinese files, including:

- `docs/PRODUCT*`, `docs/DEMO*`, `docs/ROADMAP*`, and `docs/PROJECT-STATUS*`;
- `docs/CORE-RESPONSES*`, `docs/ARCHITECTURE*`, `docs/COMMANDS*`, `docs/WEBUI*`, and `docs/SUPPORT-MATRIX*`;
- `MANIFEST*`, `CONTRIBUTING*`, and this I18n policy;
- the Chinese and English Codex, DeepSeek, Claude Code and ZCode Host guides; Claude uses `docs/CLAUDE.md` and `docs/CLAUDE_en.md`, and ZCode uses `docs/ZCODE.md` and `docs/ZCODE_en.md`.

Paired English and Simplified Chinese documents preserve the same meaning within each document
family's subject matter.

Desktop usage and toolchain policy also maintain paired Chinese/English files. Synchronization preserves product meaning within each page’s responsibility; technical-reference links supply detailed fields.

## Root README locales

The root README family retains these nine locales:

| Locale | Language | File | Maintenance role |
| --- | --- | --- | --- |
| `en` | English | `README.md` | Continuously synchronized default product entry |
| `zh-CN` | Simplified Chinese | `README_zh-CN.md` | Continuously synchronized Simplified Chinese product entry |
| `zh-TW` | Traditional Chinese | `README_zh-TW.md` | Continuously synchronized root README |
| `ja` | Japanese | `README_ja.md` | Continuously synchronized root README |
| `ko` | Korean | `README_ko.md` | Continuously synchronized root README |
| `es` | Spanish | `README_es.md` | Continuously synchronized root README |
| `fr` | French | `README_fr.md` | Continuously synchronized root README |
| `de` | German | `README_de.md` | Continuously synchronized root README |
| `pt-BR` | Brazilian Portuguese | `README_pt-BR.md` | Continuously synchronized root README |

## Synchronization scope

Select the document family by its responsibility before translating. Root READMEs introduce the
project and explain everyday use; PRODUCT describes product scope and behavior rules; technical
references explain interfaces and implementation. See the [documentation index](../MANIFEST_en.md).
Repository AI instructions and the rules for selecting documentation updates live in
[AGENTS.md](../AGENTS.md).

1. Update root READMEs when project purpose, main capabilities, prerequisites, installation, common
   operations, or necessary usage limits change. Then synchronize the affected content in all nine
   languages.
2. Internal implementation, protocol, test, build, or agent-rule changes do not by themselves
   require README edits. Translate changes in the document family that owns those details.
3. A changed PRODUCT or technical-reference statement is synchronized with its maintained
   Chinese/English counterpart. Do not copy it into other families simply to synchronize documents.
4. List the document paths and languages actually updated in the pull-request validation summary.

Translations preserve capabilities, commands, selectors, package names, paths, version identities,
and support claims. They do not add capabilities or platform promises absent from the corresponding
English or Simplified Chinese content.

## Installation commands and version identities

Public installation examples use npm's stable channel:

```text
taskbelay@latest
taskbelay-codex@latest
taskbelay-deepseek@latest
```

Exact Core, Codex, DeepSeek, Claude, ZCode, and TaskBelay CLI product versions remain only in machine-readable version
files, package metadata, Release Tags, artifact digests, and release records. Human-readable documents
do not contain exact product versions.

Command documentation must be checked against implementation:

- package names, `bin` entries, and platform constraints come from the relevant `package.json`;
- Codex commands come from `packages/codex/bin/taskbelay-codex.mjs`;
- ZCode commands come from `packages/zcode/bin/taskbelay-zcode.mjs`;
- unified lifecycle commands come from `packages/taskbelay/lib/cli.mjs`;
- DeepSeek installation and removal forms come from DSH lifecycle tests;
- packaged Core commands come from `cmd/taskbelay/main.go`;
- MCP tools come from the fixed tool list under `internal/mcp/`.

## Translation invariants

Every locale preserves:

- TaskBelay's primary position and failure scenario;
- the distinction between current capability and future direction;
- commands, selectors, tool names, environment variables, paths, and filenames;
- package, bundled Core, platform, and Host compatibility facts;
- the meaning of capabilities, non-goals, security boundaries, and support claims;
- links to current Simplified Chinese or English technical references.

Narrative prose may be natural for the target language. Identifiers without a stable translation stay
in English; translations do not invent additional product terminology or commitments.

## Review requirements

Check the affected document family:

- every file in the language navigation exists;
- when README changes, all nine languages align on the introduction, installation, operations,
  necessary limits, and documentation links;
- paired Chinese and English technical-document families express the same product facts;
- ordinary installation examples use `@latest`;
- non-English files contain no placeholder translation or whole-section English fallback;
- changed command documentation matches current parsers, lifecycle tests, and the MCP catalog.

The engineering design and acceptance family also maintains docs/ENGINEERING-CONTROL.md and docs/ENGINEERING-CONTROL_en.md with matching behavior and verification boundaries.
