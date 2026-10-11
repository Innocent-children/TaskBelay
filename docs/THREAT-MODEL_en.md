# TaskBelay trust boundaries

[中文](THREAT-MODEL.md) | [English](THREAT-MODEL_en.md)

TaskBelay is a local engineering-control system. The agent retains native permissions; Core grants no network/file/Git publication authority. Core starts controlled local checks, but commands may have side effects and must obey actual authorization. It is not a sandbox and cannot isolate hostile programs able to alter code, local data or processes as the same OS user.

Deterministic facts come from Git, file fingerprints, process results and SQLite. Hosts supply semantic judgments, user-message references, dependency scopes and human observations with explicit provenance and uncertainty. Programs cannot authenticate conversation or prove model conclusions true. record_evidence cannot submit Core execution provenance. Signed receipts protect against damage/mistaken submissions, not same-account attackers.

Structured hooks check parseable paths. Shell/external edits, databases and remote services are not automatically controlled; disclose observation timing and external-condition coverage. Default inputs are Git-visible files; ignored dependencies require explicit inclusion. Endpoint fingerprints cannot prove no transient changes during execution or fully stable external conditions. Frontend caches are not task authority.

HTTP binds loopback with exact Host/Origin/CSRF; never expose it publicly. Local evidence/output can contain test logs and follows project data rules. Core itself observes Git read-only; authorized Hosts own preparation/cleanup. Old data is not migrated/reset. See [architecture](ARCHITECTURE_en.md).
