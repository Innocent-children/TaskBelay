# TaskBelay project status

[中文](PROJECT-STATUS.md) | [English](PROJECT-STATUS_en.md)

This page records verification of the engineering-control source on October 11, 2026. CORE_VERSION owns the source Core version. This change publishes no npm package, Tag or GitHub Release; existing public versions do not contain this upgrade.

## Current delivery

One durable Core Task connects user authorization, actual changes, semantic reasoning, real executions, evidence applicability and cumulative cost. Implementation and verification can alternate; requirement revisions preserve unaffected results. CLI, MCP and WebUI share the application service; all four Hosts use the same generated Core instructions. Reliable read-only Git observation, platform implementations and installation management are retained. The old stages, protocols, data readers and generated rules are removed. See [Architecture](ARCHITECTURE_en.md).

## Executed checks

Environment: Windows x64, Go 1.27.0 and Node.js 24.18.0; temporary Git repositories and isolated SQLite. User installations and historical Tasks were untouched.

| Check | Actual result and boundary |
| --- | --- |
| Go validation | go vet ./... and go test ./... passed, covering current Core, storage, execution, platform, HTTP/MCP contracts and the retained repository observer |
| Native development journey | A real failing test passed after a fix; claims did not satisfy execution; independent revisions retained evidence and relevant code changes expired it; scripted reasoning identified same-path semantic drift; unauthorized dependency changes blocked execution; replay did not restart a process; an actual killed Core left an unknown run that was recovered, rechecked and ultimately delivered |
| Four Host packages | Independent directories outside the checkout called actual Core, checked the current tool catalog, opened Tasks, ran checks and contained complete Skill references. This is not a native Host chat or authenticated model invocation |
| WebUI | TypeScript and bundling passed; Go HTTP tests cover shared Core projection, loopback Host/Origin and CSRF. Browser inspection is recorded separately |
| Node checks | Current installation management, workspace, structured write, packaging and resource checks passed. Platform or local-permission skips remain explicit in test output |
| Platform builds | Windows x64 ran natively. macOS arm64 was cross-built and its Go metadata inspected, without native execution |

Repeatable entry: node scripts/validate-engineering.mjs. If Go is not on PATH, set GO_EXECUTABLE and add its directory to PATH. See the [raw journey and cost JSON](../tests/journeys/evidence/engineering-windows.json) and [test method](../tests/journeys/README.md).

## Comparison and unproven benefits

Executed tests on old main show that its budget counts only the current plan revision and counts Host-supplied automatic-check records. The new native journey establishes execution provenance, content-dependent evidence preservation, cumulative cost and interrupted-run recovery. See [Engineering acceptance](ENGINEERING-CONTROL_en.md) for comparison methods and results.

These facts do not establish improved AI output quality, human supervision time or total fees. Semantic judgments are scripted inputs; no randomized native-agent experiment was performed. Core observation time substantially exceeds command time in the small native fixture. Control is not free: all queries and uncertain overhead are recorded, so command counts alone cannot establish savings.

## Current limitations

Structured hooks intercept only writes actually supported by each Host; shell and external edits are examined at the next observation. Executions outside Core, remote services and hidden dependencies do not automatically become trustworthy execution evidence. Semantic conclusions retain basis and uncertainty but remain Host judgments; user references are not cryptographic authorization. TaskBelay is not a sandbox and cannot resist malicious same-user data modification. Historical internal data is rejected and preserved, without migration.

Native Codex/Claude Code/ZCode/DSH model sessions, macOS operation, long-term user cost benefits and final registry artifacts were not checked. See the [Support matrix](SUPPORT-MATRIX_en.md).

Final Node checks: 469 total, 446 passed, 23 explicit skips, zero failures. Native journeys and four-Host checks: 8 passed. The extracted ZCode artifact exercised npm launchers, stdio MCP, command execution, forbidden-path hooks, cancellation and data retention. Actual old-Core stdio comparison accepted a Host automatic-pass claim and advanced while the local assertion exited 1 both before and after; omitting plan confirmation returned TRANSITION_NOT_ALLOWED. See [raw old-Core evidence](../tests/journeys/evidence/old-core-comparison-windows.json). This did not exercise all old delivery stages and does not establish that its final delivery must be wrong.

Default Go test-result-cache execution stalled, and an earlier full source check recorded a real timeout; neither is counted as passing. Final checks use go test -count=1 -timeout=90s ./..., which actually passed. Core also ran full Go and journey checks on the actual upgraded repository, retaining exit codes, logs and measured durations. Unknown control cost conservatively uses the interval from durable start to first detection of a stopped owner, potentially including offline time, separately from measured time.
