# Contributing to TaskBelay

[中文](CONTRIBUTING_zh-CN.md) | [English](CONTRIBUTING.md)

TaskBelay accepts reproducible defects, documentation corrections, platform support backed by
final-package test results, and clearly scoped product improvements that solve real development problems.

## Change classification

| Change | Requirement |
| --- | --- |
| Spelling, links, translation, or correction of existing behavior documentation | Open a bounded pull request directly and synchronize the maintained languages of the affected document family according to the [I18n policy](docs/I18N_en.md) |
| Template or documentation-maintenance rule change | Explain the affected surface; do not change product versions or perform a release |
| Implementation defect that does not change public semantics | Identify the gap between the approved contract and actual behavior, then fix only that gap |
| Business functionality, user workflow, Core/MCP contract, persistence, control decision, or host-adapter contract change | Explain the user problem, scope, acceptance criteria, and approach, synchronize implementation/tests/docs/i18n, and update `CORE_VERSION` when shipped Core changes |
| npm package version, npm publication, Tag, or GitHub Release | Do not make this an ordinary pull-request deliverable; maintainers run the separate release flow after product work is merged |

When classification is unclear, open an Issue first and describe the user problem, current behavior,
and expected result. Do not implement a large solution and then ask the specification to accept the
completed code.

## Opening an Issue

A useful bug report includes:

- the product and version, such as Core, `taskbelay-codex`, `taskbelay-deepseek`, `taskbelay-claude`, or `taskbelay-zcode`;
- operating system, CPU, Node.js version, and host version;
- minimal reproduction steps;
- expected and actual results;
- logs or errors with secrets, private paths, and personal data removed;
- whether the issue concerns installation, explicit activation, Task transitions, Recovery, the data
  directory, or removal.

A product proposal should first explain the concrete user problem, why the current workflow cannot
solve it, and how success would be measured. An implementation approach may be discussed, but it does
not replace requirement definition.

### Product feature proposal template

```markdown
## Problem

What actually happened?

## Current approach

How does the user handle it without TaskBelay?

## Available data

What can the Task, authorization, repository, and saved results tell us?

## Behavior rules

Should the system continue, review, retry, block, or ask the user to decide?

## Expected result

What change will the user ultimately see?

## Risks and impact

What are the consequences of a false allow and a false block?

## Acceptance checks

How will we test this, in which environment, and what result should we see?

## Non-goals

Which capabilities will this change not expand?
```

### Assessment before implementation

Before implementation, a proposal must answer clearly:

1. Does it help a long-running task resume from the correct state?
2. Is it based on Task, authorization, repository observation, or retained records rather than only the
   agent's narrative?
3. Does it reduce the user's effort to judge current state and next step?
4. Can we repeat the full workflow in an actual Codex, DeepSeek, Claude Code or ZCode session?
5. Does it retain one Core Task state?
6. Does it add unnecessary process steps?
7. Does it solve a task problem, or only add another platform, Host, or interface?

A proposal that cannot explain the user problem, visible result, and acceptance method should not
move directly into implementation.

## Documentation style

Write for the document's reader and purpose. Root READMEs introduce the project and explain use in
ordinary developer language: purpose, installation, operations, and necessary limitations. Technical
references explain component responsibilities, interfaces, implementation, and verification. Use
concise headings and explain unfamiliar terms when needed. See the [documentation index](MANIFEST_en.md)
for reading paths; repository AI working rules are maintained in [AGENTS.md](AGENTS.md).

- Use “Acceptance checks” to list steps or tests, the environment, and expected results.
- Describe actual test results, run records, and saved results, including checks that were not run.
- State allowed fields, commands, and operations directly; name the component responsible for each behavior.
- Write natural prose. Keep code identifiers, field names, commands, and paths unchanged, explaining them on first use.
- Retain established technical terms such as code baseline, interface specification, state machine, and idempotency. Decide whether to revise wording from its context and accuracy, not a word blacklist.
- Rewrite sentences in context while preserving behavior, permission requirements, failure handling, and support scope.

## Local environment

Repository development requires:

- Go `>=1.26`;
- Node.js `>=24`;
- pnpm `>=11 <12`.

Fork the repository on GitHub, then create a branch from your fork:

```bash
git clone https://github.com/<your-account>/taskbelay.git
cd taskbelay
git remote add upstream https://github.com/Innocent-children/taskbelay.git
git fetch upstream
git checkout -b <type>/<short-description> upstream/main
pnpm install --frozen-lockfile
```

Before editing, read the [I18n policy](docs/I18N_en.md), the
[Command Reference](docs/COMMANDS_en.md), and the documents directly related to the change.

## Implementation principles

- Solve only the problem stated by the pull request; do not add future capabilities, generic
  frameworks, or unrelated refactoring.
- Keep Go Core responsible for Task authorization, observations, evidence applicability, cumulative
  cost, recovery and terminal acceptance.
- Keep Core's Git observation read-only. Authorized local checks use the execution boundary and native
  Host permissions; they grant no Git mutation or publication authority.
- Run only validation directly connected to the changed surface, acceptance criteria, or known risk.
- Update affected explanations by document responsibility: README covers the project and everyday
  use, `docs/PRODUCT*` covers product scope and behavior rules, and technical references cover
  interfaces and implementation details.
- Update root READMEs when business functionality or user workflows change the project purpose,
  capabilities, installation steps, common operations, or necessary usage limits; synchronize all
  nine languages. Update implementation and interface details in their owning technical references.
- A documentation correction synchronizes the maintained languages of its affected document family;
  it does not copy change summaries into other documents.
- When adding or changing a command, verify it against the package manifest, CLI parser, DSH lifecycle,
  Core parser, or MCP catalog and synchronize `docs/COMMANDS*`.
- Public npm installation examples use `@latest`; human-readable documentation contains no exact
  product release versions.
- An ordinary feature pull request that changes shipped Core behavior or contracts updates the
  machine-readable `CORE_VERSION`; it does not change npm release versions or perform Tag, npm, or GitHub Release operations.

## Validation

For documentation changes, check the affected scope:

- Markdown, tables, code fences, and Mermaid render correctly on GitHub;
- every file in the language navigation exists and links back to the other locales;
- section structure, commands, platforms, and support claims are aligned in paired Chinese/English
  document families;
- when README changes, all nine languages keep the introduction, installation, operations, necessary
  limits, and documentation links aligned;
- every ordinary installation example uses `@latest`, while exact product versions remain in
  machine-readable files and release records;
- changed command documentation matches the executable command and tool catalog;
- non-English files contain no placeholder translation or whole-section English fallback;
- the change does not broaden claims in the current
  [Support Matrix](docs/SUPPORT-MATRIX_en.md).

For code changes, prefer targeted checks for the affected package, node, contract, or user story. Run
repository-wide validation only at the final checkpoint required by the change contract:

```bash
pnpm run validate
```

Each complete-suite run needs a reason tied to the change. Report simulations, static checks, and
user-performed checks separately from automated tests of the final package in actual Codex, DeepSeek, Claude Code or ZCode.

## Pull request requirements

Create the branch from current `main` and explain:

1. the current problem;
2. what the change actually modifies;
3. the explicit non-goals;
4. validation performed and its result;
5. acceptance criteria and their corresponding tests or contracts;
6. the document families changed and locales synchronized;
7. the implementation source for each installation or command claim.

Concise Conventional Commit-style messages are recommended, for example:

```text
docs: synchronize README locales
fix(store): reject invalid snapshot before writable open
```

A pull request should remain independently reviewable. Split documentation rewrites, product
behavior changes, unrelated refactors, and version publication into separate changes.

## Release boundary

Merging product work does not publish it immediately. Core and the Host Adapters have independent
versions. After changes are merged, maintainers select the product, channel, and exact version, then
run the fixed checks, version alignment, build, read-back, Tag, npm, and GitHub Release operations.

By submitting a pull request, you agree that your contribution is provided under this repository's
[Apache License 2.0](LICENSE).
