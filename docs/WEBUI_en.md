# Local WebUI

[中文](WEBUI.md) | [English](WEBUI_en.md)

After configuring an adapter, run taskbelay webui start; status inspects the instance, open opens it, and stop ends the UI service. Lists support request/task-ID search, Host and lifecycle filters, archived visibility and pagination. Filtering precedes paging. The browser retains language selection; /tasks/{id} opens a task directly.

Details show outcomes, completed work, next actions, remaining problems, check results and cumulative usage. Expand the plan, workspace, explicit restrictions and check definitions as needed. Saved state does not claim live verification; Refresh actual state observes code and execution conditions again. Saved-state reads do not write metering data or query process identity.

Users can answer the current question directly. The answer binds issue_digest and enters recent decision history without automatically changing authorization. Budget decisions show exact changes and retain usage. Human checks require an actual completed inspection. Accepting an existing regression failure requires its current failed run, a passed comparison, named failures and a reason; acceptance for the requested outcome must still pass.

Recovery reads the original run and can restore its saved result. It refuses while the original process is running or cannot be inspected. With a missing result, the user records inspected effects and remaining uncertainty; the result remains unknown and the command is not repeated. The browser cannot start check processes; native Host tools perform execution.

The UI never retries mutations automatically. After response loss, refresh saved state. A saved mutation followed by display failure is reported explicitly; late responses cannot replace another selected task. Cancellation retains files and requires unresolved execution to be handled first. Terminal tasks support archive/restore. Recent adjustments and user answers can be expanded.

HTTP binds only loopback and checks Host, Origin and CSRF for mutations. The UI owns no second task state. Source checks, browser interaction and real Host sessions are verified separately. See [Commands](COMMANDS_en.md) and [Product](PRODUCT_en.md).
