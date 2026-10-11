# Core source

Domain defines current authorization/evidence rules; application composes repository, execution and store. MCP, CLI and WebUI project the same results. Fixed workflow phases are removed. Platform-specific behavior lives in repository/execution/webui platform files.

Use [architecture](../docs/ARCHITECTURE_en.md) for ownership and package-local checks. Full engineering validation uses node scripts/validate-engineering.mjs; native execution journeys live in tests/journeys/engineering.test.mjs.
