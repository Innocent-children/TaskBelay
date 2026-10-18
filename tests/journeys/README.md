# Native engineering journeys

Build current Core, then run node --test tests/journeys/engineering.test.mjs with ENGINEERING_CORE pointing at its absolute executable. ENGINEERING_REPORT optionally selects an output JSON. scripts/validate-engineering.mjs builds Core and runs this journey plus tests/host-engineering.test.mjs.

The journey actually executes Node assertions in temporary Git repositories and an isolated current-schema database on the running platform. It changes code, records an actual failure and fix, revises requirements, preserves independent evidence, invalidates relevant evidence, rejects an unapproved dependency, retries an identical operation without rerunning and kills the real Core owner before recovering an unknown run. Semantic judgments and user references are scripted inputs, not an AI-agent or human-time experiment.

The Host test invokes actual Core through independent package directories outside the checkout. It verifies complete resources and current tools; it does not operate authenticated Host UI or model sessions. Saved evidence states its environment and limits. Historical stage/fixture suites are removed because they describe superseded contracts.
