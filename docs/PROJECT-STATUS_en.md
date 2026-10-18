# TaskBelay project status

[中文](PROJECT-STATUS.md) | [English](PROJECT-STATUS_en.md)

Current source supports development and verification under an existing user request, retaining one Core Task, actual results, unresolved questions and cumulative usage. Ordinary methods/checks can change within that request; changed outcomes, explicit restrictions or increased existing caps still require a real decision. Public npm versions, Tags, release records and platform support promises have not changed with this source work.

## Current implementation

- Stable assessment.concerns retain engineering questions until explicitly resolved. Delivery limitations derive from applicable not_blocking records; omitted prose, empty arrays or a completion summary cannot erase the original question.
- Decision.gaps describes unmet goals, checks, reviews, concerns and runs. Failures expose current identities, existing results and known commit/execution phases. Recovery never reruns the original command.
- Existing goal_ids, rationale, conditions and verification_basis express coverage and retain independent applicable results. A passing command executes once; unavailable usage or a reached cap cannot make completed work require another execution.
- Meaningful new development/recovery discovers the actual Task, including every selected root's claim. Hosts preserve real permissions and message references without a second Task cursor.
- Mutations return compact state. Context, full records, actual output and method contracts are read on demand. All 100 existing method/operation branches have field descriptions and success/failure examples; packaged help works without source or storage. ZCode uses concise MCP text; text-rendering Hosts keep the full envelope.
- New branches explicitly use --no-track, separating source selection from Task upstream. Cumulative Host samples use non-overlapping sources and commit deduplication, totals and operation identity in the same CAS transaction. Unknown values remain distinct from zero.

Only the current database Schema is accepted. Older data is preserved and refused, without migration/reset; source trials need a separate data directory. CORE_VERSION and internal/store define the current Core/Schema versions.

## Checks performed

On 2026-10-14, macOS arm64 with Go 1.27.2, Node.js 24.21.0 and pnpm 11.28.2 ran:

| Check | Result and scope |
| --- | --- |
| pnpm validate | Passed Go vet/all Go packages, WebUI build/four request tests, native execution journeys, four independent Host packages, Node and existing build/release tests |
| Native execution journeys and independent Host packages | 15 passed, including real child-process interruption, original-run recovery, result reuse and isolated packaged Core invocation |
| Final Node suite | 655 tests: 642 passed, 13 skipped for Windows-native or specified DSH environments, no failures |
| Focused Core/interface checks | Passed concern retention/invalidation, claim rejection, completion gaps, source deduplication/CAS rollback, result reuse after reaching a cap, post-commit read failure and MCP concise-text preservation |
| 100 interface descriptions | Complete output examples validated against Schema; field notes, registered coverage, installed-only help and generated-file synchronization passed |
| Windows x64 Core | Current source cross-compiled; no native Windows execution |
| macOS desktop response models | All 14 WebUIModelTests passed |
| Local browser | Isolated Git/data verified saved-versus-refreshed state, concrete gaps, actual passed results, retained concerns, limitations, unknown Host usage and layout; no browser warnings/errors |

Simulated installation/session/user-semantic/desktop drivers are labelled separately and do not establish real Host actions. Checks did not change user installations, historical databases or public release records.

## Known verification limits

The full macOS Swift suite ran 192 tests, with two skipped for missing specified artwork packs. Nine tests produced 20 failed assertions in native animation/layout timing and existing phase text. Running those nine tests against a separate unchanged HEAD copy reproduced the same 20 assertions. The 14 affected response-model tests passed. The complete Swift suite is not reported as passing, and existing animation/text issues were not expanded into this change.

No real Codex, Claude Code, ZCode or DSH model development session was run. ZCode UI installation and new session events were not verified; no long-term token/time benefit was measured. ZCode recovery relies on Skill guidance and saved identity without assuming unavailable on-turn/on-resume/token callbacks. Other-platform native behavior, final public installation artifacts and desktop signing/notarization remain separate checks.

Core verifies actual results and applicability, not the truth of Host semantic testimony, user references or declared dependency scopes. Writes outside integrated boundaries cannot be claimed as preemptively intercepted. See [product](PRODUCT_en.md), [engineering control](ENGINEERING-CONTROL_en.md), [interfaces](INTERFACES_en.md) and [support](SUPPORT-MATRIX_en.md).
