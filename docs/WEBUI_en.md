# Local WebUI

[中文](WEBUI.md) | [English](WEBUI_en.md)

After configuring an adapter, run taskbelay webui start; status inspects the instance, open opens it, and stop ends the UI service. Lists support request/task-ID search, Host and lifecycle filters, archived visibility and pagination. Filtering precedes paging. The browser retains language selection; /tasks/{id} opens a task directly.

Desktop uses a compact task list beside task details. The list shows a request excerpt, repository names, Host and state; full requests and paths remain in details. Search and filters survive task changes. Narrow screens show the list and details separately, preserving filters and list position on return.

Details have Overview, Check records, and Requirements & usage tabs. Overview prioritizes pending user decisions, uncertain executions, current results, goal progress and remaining problems; empty problem sections are omitted. Checks show actual results and applicability, with output expanded directly below the selected check. Requirements, approach, workspace, explicit constraints, check definitions, cumulative usage and recent changes have their own sections.

“Update records” reads saved content; “Check current code” re-observes code and execution conditions. The page distinguishes saved update time, record read time and the latest explicit inspection in this page session. Active tasks periodically read saved records while the page is visible and no form is being edited; uncertain answers or execution outcomes require an explicit record update. Saved reads do not write metering or inspect process identity. An active task does not indicate that an agent is executing right now.

When a background read finds the same task revision and authorization, content and plan digests, the page retains the last Core inspection response and its time; this is not another code inspection. Changed records replace that view. An explicit Update records action also switches to saved content.

Users can answer the current question directly. The answer binds issue_digest and enters recent decision history without automatically changing authorization. Budget decisions show exact changes and retain usage. Human checks require an actual completed inspection. Accepting an existing regression failure requires its current failed run, a passed comparison, named failures and a reason; acceptance for the requested outcome must still pass.

Recovery reads the original run and can restore its saved result. It refuses while the original process is running or cannot be inspected. With a missing result, the user records inspected effects and remaining uncertainty; the result remains unknown and the command is not repeated. The browser cannot start check processes; native Host tools perform execution.

Answers, human checks, budget changes, recovery and cancellation use contextual forms with saving feedback and specific errors; failed submissions retain their input. The UI never retries mutations automatically. After response loss, update saved records first. A saved mutation followed by display failure is reported explicitly; late responses cannot replace another selected task. Cancellation retains files and requires unresolved execution to be handled first. Terminal tasks support archive/restore. Recent adjustments and user answers can be expanded.

HTTP binds only loopback and checks Host, Origin and CSRF for mutations. The UI owns no second task state. Source checks, browser interaction and real Host sessions are verified separately. See [Commands](COMMANDS_en.md) and [Product](PRODUCT_en.md).
