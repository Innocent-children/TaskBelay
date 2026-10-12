# TaskBelay project status

[中文](PROJECT-STATUS.md) | [English](PROJECT-STATUS_en.md)

Current source implements pre-development understanding/plan confirmation, explicit scope, verification necessity, cross-repository applicability, cumulative cost, known-failure exceptions and recovery. Core owns one Task; the retired node workflow and historical-data compatibility are not restored. Source changes do not update public npm/Tag/GitHub Release or native Host support claims.

## Verification entrypoints and scope

| Check | Coverage |
| --- | --- |
| go test -count=1 -timeout=90s ./... | Current domain, approval, storage, execution, CLI/MCP/HTTP and retained repository contracts |
| tests/journeys/engineering.test.mjs | Actual processes in isolated Git/SQLite: approval gates, weakened acceptance, cross-repository invalidation, repetition, exceptions and real interrupted execution |
| tests/host-engineering.test.mjs | Independently staged packages for four Hosts using current Core, tools and complete references |
| Four Host Node suites | Direct consumers of launch, workspace, hooks, Skills and installation management; unavailable platforms/capabilities are explicitly skipped |
| scripts/build-webui.mjs | TypeScript and generated assets; browser interactions are checked separately |
| scripts/sync-skill-references.mjs --check and scripts/check-versions.mjs | Generated contract consistency and version ownership |

The original engineering-upgrade record used Windows x64, Go 1.27.0 and Node.js 24.18.0. The 2026-10-12 fixes were checked on macOS arm64, Go 1.27.0 and Node.js 24.21.0 for result reuse, branch/history write preflight, Claude/ZCode cleanup and process-group recovery after forced Core termination. The changed Windows suspended-launch implementation was cross-compiled but has not run natively. Checks use temporary repositories/data and preserve user installations and historical data. The complete entrypoint is node scripts/validate-engineering.mjs; choose individual checks according to affected scope. Actual passes, failures and skips come from the current execution output. Older tests/journeys/evidence/ records apply only to their recorded artifacts and cannot substitute for current-source verification.

## Unproven outcomes

Real Core execution is distinct from real user/model conversations. Journey answers and semantic judgments are fixture inputs; they cannot establish that AI understands every requirement or eliminates overdesign. Native Codex/Claude Code/ZCode/DSH model sessions, final registry installation artifacts and long-term cost comparisons have not been established here. No support or benefit claim is expanded from these checks.

Core can reject unconfirmed plans, explicit scope violations, stale evidence and unjustified repetition. Semantic necessity, comparison adequacy and authentic Host user testimony still need real review. Bypassed Host writes cannot be claimed preemptively intercepted. See [engineering acceptance](ENGINEERING-CONTROL_en.md) and the [support matrix](SUPPORT-MATRIX_en.md).
