# Core 响应规范

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

CLI、MCP 与 HTTP 使用 internal/protocol/handler.go 的同一响应生产者。成功包含 ok=true、request_id、tool、result；失败包含 ok=false、request_id、tool、error、recovery，二者互斥。结果中的 refreshed=false 表示保存状态，不能解释为已核对当前证据。check.run/recover_run 返回 view 与 run；其他任务操作返回任务 view。

error.code 是类别，message 说明实际要求，details 给出具体 path/rule/message。参数、过期观察、范围违规、未满足验收、成本额度、未确定执行与存储不兼容各有类别。数量额度包含 budget.used/requested/limit/actual/reserved/uncertain/remaining；权限和范围不以数量不足伪装。公共错误不回显敏感参数或底层堆栈。未知内部失败要求先查保存状态，不假称零写入。

recovery.action 指定 correct_request、read_task、self_correct、recover_run、request_authorization 或 repair_storage。纠正保留真实授权与操作身份，仅依据已有事实修改返回字段；不能补造用户决定、语义通过或执行结果。已提交后清理失败仍可能返回错误，需查保存状态。成本计量写入不等于语义提交；不得从错误反推任务或进程必然没有执行。

操作结果不明先读原 Task/run。执行已持久预留后不可盲目重放。恢复匹配签名收据只保存原结果；Core 已退出但命令进程组仍存活时返回 EXECUTION_RUNNING 并保留 pending；无法判断进程状态时返回 EXECUTION_UNCERTAIN。全部停止但缺收据只产生 interrupted_unknown。旧 Schema 保留原数据且拒绝打开。完整 JSON 示例及同工具具体错误在共享 Skill 的 protocol.md，测试比较完整编码并校验输出 Schema；in-memory SDK 测试核对实际传输。见[命令](COMMANDS.md)和[架构](ARCHITECTURE.md)。

JSON 未知成员、重复成员及类型错误定位到具体 arguments 路径，包括数组下标，例如 arguments.plan.checks[0].argv[0]；邻接示例使用真实生产者的完整编码。

AUTHORIZATION_REQUIRED 表示用户目标、明确限制或已设上限发生变化，需要实际新用户引用；AUTHORIZATION_PENDING 表示已有问题尚未得到决定。AUTHORIZATION_CHANGED 要求读取当前任务后再提交预算决定。CHECK_ALREADY_SATISFIED 表示复用已有结果；RETRY_BASIS_REQUIRED 要求读取原运行并补充新诊断信息。FAILURE_ACCEPTANCE_NOT_ALLOWED 禁止用例外豁免验收。

REMOTE_OBSERVATION_STALE 表示 HEAD、对应上游或追踪提交与宿主观察不匹配，需重新观察。server_info.startup_diagnostics 说明当前 Core 已到达；不能据 CLI 成功断言当前会话已暴露 MCP。调用计量初始化失败不会执行请求。error.details 的 control_meter.<阶段> 保存原始原因：PROCESS_IDENTITY_DENIED/PROCESS_IDENTITY_UNAVAILABLE 要求检查进程查询与宿主权限；拒绝不能作为进程已结束的证明。STORAGE_UNAVAILABLE 仅表示真实存储读取/写入失败；已有 STORAGE_CORRUPT 等原因保留，修复时不删除数据。CONTROL_INITIALIZATION_FAILED 指向其他初始化步骤，CONTROL_INITIALIZATION_CANCELLED 表示取消或超时。恢复提示按实际责任分类，不据此声称 MCP 未连接。

WebUI 启动若子进程提前退出，明确返回进程退出和本次 webui-runtime.log 位置；未就绪超时也保留日志位置。先读原启动记录，不将退出或存储失败统称为连接问题。

默认任务返回当前摘要，不包含完整文件指纹和尝试历史。taskbelay_get_task 的 detail:true 将完整视图放入 result.details；check.run/recover_run 的 run 是状态摘要，通过 get_run 读取真实输出。未设成本上限时 budget.limits[].limited=false；remaining=null 表示不限额或无法计算，结合 limited 与 coverage 判断。

ISSUE_CHANGED 表示答复对应的问题已改变，应重新读取；resolve_issue 不修改授权。保存状态查询不进入计量初始化，相关计量错误只阻止变更。check.run 是原生 CLI 响应，不是 MCP 工具。

## 当前状态与具体失败处理

普通变更返回 identity/digests、当前 decision、progress、检查摘要、concerns、limitations、issue、usage、budget。open/get 增加 context；detail:true 增加明确记录，包括 facts 和 evidence_applicability。输入不接受内部 Task 快照或调用方自填的审查摘要。get_run 返回实际 cwd、原始 operation_id、执行指纹及 output.stdout/output.stderr。

Decision.gaps 包含 code/path/subject/message/action/related_ids。finish 重新计算同一组条件，在失败时返回这些具体缺口；消费方无需翻译泛化的 review 提示。过期审查和修订冲突在可以取得时附带当前身份。CHECK_ALREADY_SATISFIED 的 existing_result 提供实际结果和运行身份。

失败的 outcome 为 not_applied/applied/unknown，描述语义提交，与计量无关；原生命令在可以确定时返回 execution 阶段。recovery.operation 为 reuse/replace/read_before_retry/none。已知提交后的读取或清理失败返回 applied；结果未知时先读原记录。任何字段都不授权盲目重复副作用。

Host 用量允许 null，并带 unreported/partial/reported；未知不等于实测零值。明确 Host 上限缺少足够报告时，新命令返回 HOST_USAGE_UNAVAILABLE，保存状态读取和恢复仍可执行。未设上限的 budget.remaining 为 null。

方法说明、字段规则及相邻的完整实际样例可通过 server_info.contracts 和原生方法 --help --json 按需读取。auxiliary_contracts 列出原生/HTTP 名称，不增加工具。MCP 始终发送完整 structuredContent；仅 ZCode 请求简短文本，主要展示文本的 Host 保留完整响应。见[接口使用说明](INTERFACES.md)。
