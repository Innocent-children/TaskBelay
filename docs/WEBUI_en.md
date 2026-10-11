# Local WebUI

[中文](WEBUI.md) | [English](WEBUI_en.md)

After configuring an adapter run taskbelay webui start. status reads the current local instance, open opens it and stop ends the UI process. See README for source installation. WebUI owns no second task state.

Lists show saved summaries. Details show authorized goals, progress, evidence provenance, gaps, unresolved execution, necessary decisions and cumulative cost. Refresh actual state observes the workspace and applicability; saved state does not assert continued validity. Actual output is readable with explicit truncation labels. Human-check buttons require the user to have performed a human check and describe what was observed; they cannot represent pending or agent checks.

Users do not fill node payloads, select transitions or repeatedly confirm phases. Important authorization/tradeoff decisions still go to the agent as real user instructions. Cancellation retains files and first requires resolving uncertain execution. HTTP purge only applies to an explicitly matched terminal task, not source or worktrees. The pet reads saved summaries rather than live activity or completion percentage.

The UI is loopback-only with Host/Origin/CSRF mutation checks. See [commands](COMMANDS_en.md) and [product](PRODUCT_en.md).
