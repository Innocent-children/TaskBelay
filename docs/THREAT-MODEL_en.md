# TaskBelay trust boundaries

[中文](THREAT-MODEL.md) | [English](THREAT-MODEL_en.md)

TaskBelay is a local engineering-control system. The agent retains native permissions; Core grants no network/file/Git publication authority. Core starts controlled local checks, but commands may have side effects and must obey actual authorization. It is not a sandbox and cannot isolate hostile programs able to alter code, local data or processes as the same OS user.

Deterministic facts come from Git, file fingerprints, process results and SQLite. Hosts supply semantic judgments, user-message references, dependency scopes and human observations with explicit provenance and uncertainty. Programs cannot authenticate conversation or prove model conclusions true. record_evidence cannot submit Core execution provenance. Signed receipts protect against damage/mistaken submissions, not same-account attackers.

Structured hooks check parseable paths. Shell/external edits, databases and remote services are not automatically controlled; disclose observation timing and external-condition coverage. Default inputs are Git-visible files; ignored dependencies require explicit inclusion. Endpoint fingerprints cannot prove no transient changes during execution or fully stable external conditions. Frontend caches are not task authority.

HTTP binds loopback with exact Host/Origin/CSRF; never expose it publicly. Local evidence/output can contain test logs and follows project data rules. Core itself observes Git read-only; authorized Hosts own preparation/cleanup. Old data is not migrated/reset. See [architecture](ARCHITECTURE_en.md).

Core retains the actual request and explicit restrictions. Routine plan and check-method revisions do not create another user approval. Changed user boundaries or expanded caps need an actual new decision. A Host-provided user_ref cannot authenticate conversation; Core cannot determine whether a new reference reflects the user's intent. Necessity, test adequacy and comparison sufficiency remain technical judgments. Programs check explicit restrictions, actual execution and applicability. WebUI budget decisions and human observations are Origin/CSRF-protected.

Commands enter Core through check run invoked by the Host native process tool. MCP and HTTP expose no process-start entrypoint, preserving native permission and environment. Saved-state reads require neither process queries nor metering writes; execution cannot bypass metering. User answers bind the displayed question and do not themselves expand authorization.

Manager data deletion requires confirmation bound to the exact plan, not ordinary --yes. Extra directories and permanent deletion have separate confirmations. Keeping plugins also keeps registrations and directory bindings; cleanup directories cannot contain retained installation resources. A stopped Core does not prove its commands ended, so pending task executions are read before deletion. ZCode private-cache shutdown requires actual user confirmation and is not claimed as automatically verified; reconnection and unfinished plugin removal still block cleanup. Launcher self update pins the version, preserves the npm/local channel and verifies local distribution digests before installation.
