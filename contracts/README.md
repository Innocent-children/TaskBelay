# Public interface reference

The current machine-readable reference is interfaces.json. Usage and transport rules are in docs/INTERFACES.md and docs/INTERFACES_en.md. Every method carries input/output field descriptions, complete producer examples, preconditions and failure handling. Maintainer paths are informational; installed consumers need no repository source.

Sources are the live Core DTO/catalog, explicit host-inputs.mjs, Codex's existing host-launch-contract.mjs and field-notes.json. Example scripts execute isolated Git/Core/HTTP and public serializers; external Host installation/session/desktop drivers are labelled simulations. Output sample schemas are expanded with live Core schemas where those contracts are reused. Retained Host receipts, handoffs and external Host result objects are opaque and must be preserved unchanged.

Regenerate after changing an owning producer:

1. Build the current Core at .evaluation/taskbelay (or set ENGINEERING_CORE).
2. Run `TASKBELAY_UPDATE_EXAMPLES=1 go test ./internal/protocol -run '^TestPublishedMethodExamplesUseActualProducers$' -count=1`.
3. Run `node scripts/export-host-contracts.mjs`, `node scripts/export-management-contracts.mjs` and `node scripts/export-runtime-contracts.mjs`.
4. Run `node scripts/package-interface-contracts.mjs` and `node scripts/sync-skill-references.mjs`.
5. Rebuild Core after updating its embedded examples. Run the affected producer/transport tests, `go test ./tests/contract`, `node --test scripts/interface-contracts.test.mjs` and the packaging `--check`.

All mutations made by exporters belong to temporary fixture directories. No user adapter installation, actual Host chat, remote publication or user database is required. Production wrappers read only the copied JSON/reference reader; no generator or repository shared file is a runtime dependency.
