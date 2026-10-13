# Local WebUI

[中文](WEBUI.md) | [English](WEBUI_en.md)

After configuring an adapter, run taskbelay webui start. status reads the local instance, open opens it and stop ends the UI process. See README for source installation. The interface reads the Core Task without creating another task state.

Details lead with goals, progress and remaining problems. Expand approach, workspace, restrictions and check definitions as needed. Executions, human observations and agent claims are labeled separately; get_run reads output. Applicability is assessed after refreshing actual conditions; saved state is not a live check. There is no repeat plan-confirmation step.

Usage remains visible. Unset caps show Not set. Users may explicitly change budgets; zero means no cap and existing usage remains. Time categories may overlap and must not be added. Human-check buttons require a completed user observation that can be described. Important requirement decisions are given to the coding agent as real instructions.

Lists support paging, archived visibility and /tasks/{id} links, with browser history retaining selection. Terminal tasks can be archived/restored. Accepted regression failures remain failures. Cancellation retains files and requires resolving uncertain execution first; purge removes only an explicitly matched terminal record, not source or worktrees.

HTTP is loopback-only with Host/Origin/CSRF mutation protection. The interface explicitly requests full details, while ordinary tool responses remain summaries. The desktop pet reads saved summaries, not live activity or completion percentage. See [commands](COMMANDS_en.md) and [product](PRODUCT_en.md).
