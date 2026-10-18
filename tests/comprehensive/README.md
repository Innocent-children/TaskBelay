# Cross-layer engineering contracts

The current suite checks the shared Core lifecycle/tool/evidence boundary and native command policy.
Package-local tests own deterministic scope, applicability, execution, persistence and recovery.
The actual CLI journey in tests/journeys/engineering.test.mjs executes checks and kills a real owner;
its semantic judgments are explicit fixture input. tests/host-engineering.test.mjs uses independently
installed source packages and real Core entrypoints, not real Host chat sessions.

Run the coherent final check with `node scripts/validate-engineering.mjs`. This builds Core, tests
current contracts, checks installed references and runs native journeys. Platform builds and native
platform/Host verification remain separate. Retired phase/topology tests are absent.
