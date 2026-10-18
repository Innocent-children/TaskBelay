# Engineering-task walkthrough

[中文](DEMO.md) | [English](DEMO_en.md)

Ask a configured Host for a concrete outcome, such as login-failure throttling while preserving public interfaces and stored data. The agent explains the cause, approach and relevant checks, then proceeds under the original request. Method changes do not require repeat approval; changed user outcomes or explicit restrictions require an actual new decision.

Method changes and additional checks reuse the request; new user outcomes record the actual new decision. Unaffected goals and checks remain valid; relevant code changes require only missing verification. Unpredicted paths need an explanation, while explicit path/dependency violations block. Correct within existing authority first.

After interruption provide the Task ID and return to the original workspace. Read current requirements, results, failures and unresolved execution. Read an original run after response loss without rerunning. Deliver per-goal evidence and conclusions. The repeatable native script is tests/journeys/engineering.test.mjs. Scripted semantic inputs and real Host judgments are distinct; see [verification](PROJECT-STATUS_en.md).
