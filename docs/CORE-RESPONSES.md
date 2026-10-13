# Core 响应规范

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

CLI、MCP 与 HTTP 使用 internal/mcp/server.go 的同一响应生产者。成功包含 ok=true、request_id、tool、result；失败包含 ok=false、request_id、tool、error、recovery，二者互斥。结果中的 refreshed=false 表示保存状态，不能解释为已核对当前证据。run_check/recover_run 返回 view 与 run；其他任务操作返回任务 view。

error.code 是类别，message 说明实际要求，details 给出具体 path/rule/message。参数、过期观察、范围违规、未满足验收、成本额度、未确定执行与存储不兼容各有类别。数量额度包含 budget.used/requested/limit/actual/reserved/uncertain/remaining；权限和范围不以数量不足伪装。公共错误不回显敏感参数或底层堆栈。未知内部失败要求先查保存状态，不假称零写入。

recovery.action 指定 correct_request、read_task、self_correct、recover_run、request_authorization 或 repair_storage。纠正保留真实授权与操作身份，仅依据已有事实修改返回字段；不能补造用户决定、语义通过或执行结果。已提交后清理失败仍可能返回错误，需查保存状态。成本计量写入不等于语义提交；不得从错误反推任务或进程必然没有执行。

操作结果不明先读原 Task/run。执行已持久预留后不可盲目重放。恢复匹配签名收据只保存原结果；Core 已退出但命令进程组仍存活时返回 EXECUTION_RUNNING 并保留 pending；无法判断进程状态时返回 EXECUTION_UNCERTAIN。全部停止但缺收据只产生 interrupted_unknown。旧 Schema 保留原数据且拒绝打开。完整 JSON 示例及同工具具体错误在共享 Skill 的 protocol.md，测试比较完整编码并校验输出 Schema；in-memory SDK 测试核对实际传输。见[命令](COMMANDS.md)和[架构](ARCHITECTURE.md)。

JSON 未知成员、重复成员及类型错误定位到具体 arguments 路径，包括数组下标，例如 arguments.checks[0].argv[0]；邻接示例使用真实生产者的完整编码。

AUTHORIZATION_REQUIRED 表示用户目标、明确限制或已设上限发生变化，需要实际新用户引用；AUTHORIZATION_PENDING 表示已有问题尚未得到决定。AUTHORIZATION_CHANGED 要求读取当前任务后再提交预算决定。CHECK_ALREADY_SATISFIED 表示复用已有结果；RETRY_BASIS_REQUIRED 要求读取原运行并补充新诊断信息。FAILURE_ACCEPTANCE_NOT_ALLOWED 禁止用例外豁免验收。

REMOTE_OBSERVATION_STALE 表示 HEAD、对应上游或追踪提交与宿主观察不匹配，需重新观察。server_info.startup_diagnostics 说明当前 Core 已到达；不能据 CLI 成功断言当前会话已暴露 MCP。调用计量初始化失败不会执行请求。error.details 的 control_meter.<阶段> 保存原始原因：PROCESS_IDENTITY_DENIED/PROCESS_IDENTITY_UNAVAILABLE 要求检查进程查询与宿主权限；拒绝不能作为进程已结束的证明。STORAGE_UNAVAILABLE 仅表示真实存储读取/写入失败；已有 STORAGE_CORRUPT 等原因保留，修复时不删除数据。CONTROL_INITIALIZATION_FAILED 指向其他初始化步骤，CONTROL_INITIALIZATION_CANCELLED 表示取消或超时。恢复提示按实际责任分类，不据此声称 MCP 未连接。

WebUI 启动若子进程提前退出，明确返回进程退出和本次 webui-runtime.log 位置；未就绪超时也保留日志位置。先读原启动记录，不将退出或存储失败统称为连接问题。

默认任务返回当前摘要，不包含完整文件指纹和尝试历史。taskbelay_get_task 的 detail:true 将完整视图放入 result.details；run_check/recover_run 的 run 是状态摘要，通过 get_run 读取真实输出。未设成本上限时 budget.limits[].limited=false；remaining=0 不能解释为额度耗尽。
