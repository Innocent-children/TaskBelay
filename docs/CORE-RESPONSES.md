# Core 响应规范

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

CLI、MCP 与 HTTP 使用 internal/mcp/server.go 的同一响应生产者。成功包含 ok=true、request_id、tool、result；失败包含 ok=false、request_id、tool、error、recovery，二者互斥。结果中的 refreshed=false 表示保存状态，不能解释为已核对当前证据。run_check/recover_run 返回 view 与 run；其他任务操作返回任务 view。

error.code 是类别，message 说明实际要求，details 给出具体 path/rule/message。参数、过期观察、范围违规、未满足验收、成本额度、未确定执行与存储不兼容各有类别。数量额度包含 budget.used/requested/limit/actual/reserved/uncertain/remaining；权限和范围不以数量不足伪装。公共错误不回显敏感参数或底层堆栈。未知内部失败要求先查保存状态，不假称零写入。

recovery.action 指定 correct_request、read_task、self_correct、recover_run、request_authorization 或 repair_storage。纠正保留真实授权与操作身份，仅依据已有事实修改返回字段；不能补造用户决定、语义通过或执行结果。已提交后清理失败仍可能返回错误，需查保存状态。成本计量写入不等于语义提交；不得从错误反推任务或进程必然没有执行。

操作结果不明先读原 Task/run。执行已持久预留后不可盲目重放。恢复匹配签名收据只保存原结果；Core 已退出但命令进程组仍存活时返回 EXECUTION_RUNNING 并保留 pending；无法判断进程状态时返回 EXECUTION_UNCERTAIN。全部停止但缺收据只产生 interrupted_unknown。旧 Schema 保留原数据且拒绝打开。完整 JSON 示例及同工具具体错误在共享 Skill 的 protocol.md，测试比较完整编码并校验输出 Schema；in-memory SDK 测试核对实际传输。见[命令](COMMANDS.md)和[架构](ARCHITECTURE.md)。

JSON 未知成员、重复成员及类型错误定位到具体 arguments 路径，包括数组下标，例如 arguments.checks[0].argv[0]；邻接示例使用真实生产者的完整编码。

PLAN_CONFIRMATION_REQUIRED、PLAN_CHANGED、AUTHORIZATION_PENDING 要求真实用户决定；UNAPPROVED_CHANGES 要求恢复提案时内容或明确披露后修订。CHECK_ALREADY_SATISFIED 表示复用已有结果，RETRY_BASIS_REQUIRED 要求读取原运行并提供新诊断信息。FAILURE_ACCEPTANCE_NOT_ALLOWED 不允许以例外绕过目标验收。拒绝未启动进程，但调用成本仍记录。

PLAN_BUDGET_INFEASIBLE 要求根据预算报告修正不可预留的配置，不能据此取消验收。REMOTE_OBSERVATION_STALE 表示 HEAD、对应上游或追踪提交与宿主观察不匹配，需重新观察。server_info.startup_diagnostics 说明当前 Core 已到达；不能据 CLI 成功断言当前会话已暴露 MCP。STORAGE_UNAVAILABLE 的恢复信息区分已到达 Core 与调用计量写入失败，要求检查目录权限、Schema 和宿主访问并保留数据。

WebUI 启动若子进程提前退出，明确返回进程退出和本次 webui-runtime.log 位置；未就绪超时也保留日志位置。先读原启动记录，不将退出或存储失败统称为连接问题。
