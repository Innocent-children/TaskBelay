# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Engineering control for real AI coding tasks.**

TaskBelay works alongside Codex, DeepSeek Harness, Claude Code and ZCode. It keeps the authorized outcome, actual code changes and verification evidence together so an agent can continue across sessions with fewer repeated decisions. The agent remains responsible for coding and technical judgment.

## What you can do

- Keep goals, explicit non-goals and interface, data, dependency and design constraints active while developing. Inspect changes within files, not just their paths. Correct scope drift within existing authority; ask the user only when authority or an important choice must change.
- Execute approved local checks and retain actual exit codes, inputs, environment conditions and results. Claims, process execution and acceptance conclusions remain distinct. Relevant changes expire evidence; justified independent checks can remain valid. Replanning never resets cumulative cost.
- Alternate implementation and verification, revise requirements and resume from current work, failed attempts and unresolved executions. Recent history is bounded while live results and cumulative costs remain.

## Start

Use Node.js >=24 and an installed coding Host. Source development additionally needs Go >=1.26 and pnpm >=11 <12. This checkout contains a breaking engineering upgrade; published package versions and stable support records are unchanged. Build this source to use the new contract. Preserve old data and select a separate explicit data directory; no migration or automatic reset is provided.

Public installation:

```sh
npm install -g taskbelay@latest
taskbelay
```

Build and install the current source locally:

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

Enable the adapter using its [Host guide](packages/codex/README.md), then request a bounded outcome in the agent:

```text
Use TaskBelay to add login failure throttling. Preserve the public authentication API and stored user data; do not redesign account management. Run the relevant existing tests and report actual results.
```

State the desired result and necessary limits. Existing authorization remains valid; you do not need to approve internal phases or fill workflow forms. The local WebUI shows results, remaining gaps and necessary decisions. Resume by providing the saved Task ID in its original workspace.

```sh
taskbelay webui start
```

## Boundaries

Structured hooks cover supported write paths; shell and external edits are observed at checkpoints. Only checks actually run by Core have Core execution provenance. Declared inputs and environment conditions do not automatically include ignored dependencies, databases or remote services. Semantic reasoning is explicit but does not prove correctness. TaskBelay is not a sandbox or another coding agent.

Core overhead is measured; Host analysis, waiting and tokens are reported and may be incomplete. Small mechanical tasks are often simpler with the native agent alone. The source tests demonstrate capabilities, not a measured reduction in defects or human supervision. Completion retains files and grants no Git, publication or deletion authority.

## Further help

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
