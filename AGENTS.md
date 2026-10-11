# TaskBelay repository instructions

## Authority and scope

Read the current user request, CONTRIBUTING_zh-CN.md or CONTRIBUTING.md, docs/PRODUCT.md or its English counterpart, related technical contracts, then current source/Schema/tests. Explicit user instructions take precedence. Existing code is evidence, not an immutable design requirement. Identify root cause and ownership before selecting the smallest coherent change. Do not preserve an incorrect design because it already exists; do not add speculative frameworks or unrelated cleanup.

Before delivery ask: starting from the actual current need, would I choose this resulting structure? Acceptance must establish resolved causes, clear ownership and proportionate complexity; passing tests or complete checklists alone is insufficient.

## Code discovery

Prefer codebase-memory-mcp search_graph, trace_path, get_code_snippet, query_graph and get_architecture. Index an unindexed repository first. Use rg for literals/configuration/non-code files and incomplete/unavailable graph coverage. External indexes are optional product dependencies; never install one automatically. Report a limitation once and continue with available search.

## Product responsibilities

- Go Core owns the single Task, authorization contracts, goals/constraints, current progress, evidence applicability, control decisions, lifetime cost, recovery and terminal acceptance.
- Host adapters preserve real user authority and native permissions, provide semantic reasoning and user references, translate interfaces and perform authorized workspace operations. No second Task cursor or inferred completion belongs in a Host, method tool or frontend.
- Repository observation owns read-only Git identity/history/content and within-file facts.
- Execution owns actual argv processes, input/environment/executable fingerprints, output and receipts. OS paths, process lifetime, file identity and permissions remain in their platform implementations.
- Persistence owns the one current Schema, atomic CAS, claims, operation identity, cost and bounded history. Build/release owns compilation, staging, package completeness and publication.

Core semantic code contains no OS selection. Keep interfaces small and consumer-specific; direct functions/modules usually suffice. Core itself must not mutate Git or grant permission through a verification command. Commands still obey actual Host/user authority. TaskBelay is not a sandbox, coding agent or general workflow engine.

## Engineering contract maintenance

The lifecycle is active/done/cancelled. Control advice is derived from observations and required evidence; it is not another workflow. Do not introduce mandatory phases, repeated plan confirmations or management forms. Existing authorization remains valid until an actual material change requires a user decision.

When behavior changes, update the owning contract and DTO/Schema before consumers. Define deterministic observations, semantic testimony/uncertainty, evidence sources, applicability dependencies, cumulative cost, recovery of uncertain effects and forbidden outcomes. Programs check facts; model scores never establish correctness. Claims cannot manufacture execution. Limits cannot waive acceptance. Replanning/compaction cannot reset cost or erase unresolved operations.

Current data/protocols only: remove superseded implementations, compatibility branches, old readers/migrations, fallback chains, tests and instructions in the affected surface. Do not reset or delete old user data. Receipt and transport results remain authoritative after response loss; never repeat uncertain side effects from reconstructed payloads.

## Documentation and Skills

Choose the document by reader and purpose. Root READMEs explain purpose, installation, normal operations and necessary limits. PRODUCT owns user outcomes and boundaries; ARCHITECTURE/THREAT-MODEL own responsibilities/data/trust; COMMANDS owns exact entrypoints; CORE-RESPONSES owns envelopes/errors/recovery; SUPPORT-MATRIX and PROJECT-STATUS own actual support/evidence. AGENTS owns repository engineering instructions, CONTRIBUTING human contribution guidance, directory READMEs their maintainer procedures. Do not append implementation logs to product guides.

Update inaccurate statements and owning references in the same change. Root README changes synchronize all nine locales. Other maintained families synchronize affected Chinese/English counterparts without expanding their purpose. No placeholder translations or whole-section English fallback. Keep exact commands/identifiers and support claims consistent. Installation examples use @latest; exact versions belong in machine-readable metadata and release records. Distinguish source capability from published support.

Shared Core Skill source is skills/taskbelay/core/. Host entrypoints live in packages/codex/plugin/skills/taskbelay/, packages/deepseek/skills/taskbelay/, packages/claude/plugin/skills/taskbelay/ and packages/zcode/skills/taskbelay/. Generated copies are never edited by hand. Run node scripts/sync-skill-references.mjs and --check. Remove retired generated references, update package manifests/staging and prove installed packages are complete without repository shared files. Shared Host command/workspace copies likewise change at packages/host-command/ and packages/host-workspace/.

Every complete shipped MCP request example has an adjacent same-tool error with a concrete trigger and implementation references. Tests reproduce actual responses, compare complete encoded errors and validate actual success/failure against published output Schema.

## Validation and delivery

Use the product proposal/assessment in CONTRIBUTING before implementation. Checks trace to current acceptance, affected contracts or relevant regressions. Prefer package/storage/transport/user-story checks. Do not run whole matrices after each edit or retain superseded tests. Full final validation is justified for a system-wide change. Native Core execution, simulated semantic input, adapter tests and real Host sessions are distinct; never claim one proves another. Report unavailable checks and unverified benefits honestly.

Review overall behavior and structure, then report exact changed documentation, actual checks and remaining limits. Stop only at the user's requested checkpoint. Do not replace a complete implementation request with a roadmap or isolated prototype.

## Version and release boundaries

CORE_VERSION is the single Core version authority. Ordinary changes to shipped executable behavior or Core-owned contracts update it according to docs/VERSIONING.md; pre-1.0 incompatible changes require a minor increment. Tests/docs/build-only changes need no Core increment. Ordinary work leaves public stable identities and Host release versions unchanged.

Core only observes Git. Repository Git mutations require actual user authorization. Do not merge, publish, create/change Tags, upload release assets or expand public stable support from ordinary product work. Release requires product/channel/exact target version/exact confirmation and the standalone release contracts. Read release/README.md, selected product README, prepare/publisher contracts and metadata before version-only release work.
