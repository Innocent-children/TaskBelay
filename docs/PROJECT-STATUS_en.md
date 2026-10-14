# TaskBelay project status

[中文](PROJECT-STATUS.md) | [English](PROJECT-STATUS_en.md)

Current source reuses existing user authority, supports focused method/check revisions, explicit scope restrictions, selective result invalidation, optional cost caps with continuous metering, known-failure exceptions and recovery. Routine tool responses are summaries with full details on demand. Core owns one Task; the retired node workflow and historical-data compatibility are not restored. Source changes do not update public npm/Tag/GitHub Release or native Host support claims.

## Verification entrypoints and scope

| Check | Coverage |
| --- | --- |
| go test -count=1 -timeout=90s ./... | Current domain, authorization, storage, execution, CLI/MCP/HTTP and retained repository contracts |
| tests/journeys/engineering.test.mjs | Actual processes in isolated Git/SQLite: immediate authorized start, independent result retention after new checks, acceptance gaps, cross-repository invalidation, repetition, exceptions and real interrupted execution |
| tests/host-engineering.test.mjs | Independently staged packages for four Hosts using current Core, tools and complete references |
| Four Host Node suites | Direct consumers of launch, workspace, hooks, Skills and installation management; unavailable platforms/capabilities are explicitly skipped |
| scripts/build-webui.mjs | TypeScript and generated assets; browser interactions are checked separately |
| scripts/sync-skill-references.mjs --check and scripts/check-versions.mjs | Generated contract consistency and version ownership |

The original engineering-upgrade record used Windows x64, Go 1.27.0 and Node.js 24.18.0. The 2026-10-12 fixes were checked on macOS arm64, Go 1.27.0 and Node.js 24.21.0 for result reuse, branch/history write preflight, Claude/ZCode cleanup and process-group recovery after forced Core termination. The changed Windows suspended-launch implementation was cross-compiled but has not run natively. Checks use temporary repositories/data and preserve user installations and historical data. The complete entrypoint is node scripts/validate-engineering.mjs; choose individual checks according to affected scope. Actual passes, failures and skips come from the current execution output. Older tests/journeys/evidence/ records apply only to their recorded artifacts and cannot substitute for current-source verification.

On 2026-10-13, this change passed node scripts/validate-engineering.mjs on macOS arm64, Go 1.27.0 and Node.js 24.21.0: Go vet and every Go package passed, independently staged packages for four Hosts passed, and the Node suite passed 487 of 499 tests with 12 Windows-native or exact-DSH-environment skips. After adding the requirement-change scenario, all 10 tests in tests/journeys/engineering.test.mjs passed, and affected Go checks passed again. Windows x64 Core cross-compilation passed without native execution. A real local browser verified refresh, expandable details, unset-limit display and budget-save readback without console errors. Operations used temporary repositories and isolated data, without installing or changing the user's plugins.

For the same initial MCP task fixture, the summary was 2737 bytes versus 4712 bytes for the full application view. This measures that fixture's response size only; it does not establish model-task time or token savings.

## Unproven outcomes

Real Core execution is distinct from real user/model conversations. Journey answers and semantic judgments are fixture inputs; they cannot establish that AI understands every requirement or eliminates overdesign. Native Codex/Claude Code/ZCode/DSH model sessions, final registry installation artifacts and long-term cost comparisons have not been established here. No support or benefit claim is expanded from these checks.

Core can reject boundary changes without a new user reference, explicit scope violations, stale results and unjustified repetition. Semantic necessity, comparison adequacy and authentic Host user testimony still need real review. Bypassed Host writes cannot be claimed preemptively intercepted. See [engineering acceptance](ENGINEERING-CONTROL_en.md) and the [support matrix](SUPPORT-MATRIX_en.md).

## 2026-10-14 authorization, execution and acceptance adjustment

Current source places checks, forecasts and review hints in Plan and uses goal_ids as the single outcome association. Every goal needs non-waivable acceptance; delivery cites passed acceptance separately from regression exceptions. internal/protocol owns shared requests, Schemas and errors; MCP is transport only, while native Host tools invoke check run. Saved reads neither write metering nor query process identity. WebUI adds search, filters, language selection, actual answers, exceptions and recovery.

Full validation ran on macOS arm64, Go 1.27.0, Node.js 24.21.0 and pnpm 11.27.1: Go vet and all Go packages passed; real journeys and four independent Host packages passed 15 checks; the Node suite passed 639 of 652, with 13 skipped for native Windows or exact DSH environments. After correcting the recovery response, four frontend response-loss, stale-selection and recovery-refresh checks passed, as did TypeScript and the rebuilt WebUI. All Go packages passed again after the final responsibility changes, followed by the added actual saved-task read test. Windows x64 Core cross-compilation passed without native execution.

The local browser used temporary repositories and isolated storage to check search/Host filtering, language, user answers, budget updates, regression exceptions retaining acceptance requirements and unknown-run recovery. The UI now handles recovery view/run responses correctly; rechecking retained interrupted_unknown, execution count and uncertain duration with no console errors. User and semantic inputs were fixtures, not real model sessions or product-benefit comparisons. User plugins, historical databases and public release records were untouched.
