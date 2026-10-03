# Core 响应规范

[中文](CORE-RESPONSES.md) | [English](CORE-RESPONSES_en.md)

本规范约束 Core 的 MCP 响应，由 `internal/mcp/results.go` 和 `output_schemas.go` 实现。
Core 负责描述实际结果、具体失败条件和允许的下一步；Host 保存完整响应并按结构化字段处理。
工具请求见 [命令参考](COMMANDS.md)，Host 执行说明见共享 Skill 的 `tool-results.md`。

## 成功和失败

每个响应包含 `ok`、`request_id`、`tool`。`request_id` 关联本次调用，`tool` 标识实际工具。
`ok=true` 必须包含 `result`，且不含 `error`、`recovery`；`ok=false` 必须包含
`error`、`recovery`，且不含 `result`。输出 Schema 必须校验这两种互斥结构。

成功表示该工具完成了所报告的操作。节点提交仍须读取保存的 Task：结果可能是 BLOCKED、DONE
或 CANCELLED。普通提交的 Task 在 `result`；open/get 在 `result.task`；next-action 在
`result.action`；relocation prepare 返回 `result.relocation_id` 和 `result.task`。
恢复查询另带 `recovery_assessment`，先处理其 `next_advice`，再执行当前 Action。

## 错误字段

18 个工具共用以下规则。`error.code` 是稳定的错误类别，`message` 直接说明本次失败条件。
参数和转换条件错误的 `message` 汇总字段路径与具体要求；详情较多时只显示有界摘要，完整原因保留在
`details[]` 或 `guard.failures[]`。不把已知原因改写为“参数无效”或“操作失败”。
已识别的参数问题使用 `details[]`，每项包含请求字段 `path`、固定规则 `rule` 和具体要求
`message`。类型错误说明所需类型；文本、标识符、重复成员、字段依赖和工作项依赖错误说明实际限制。
缺少字段与 JSON 语法错误、无效 UTF-8、重复对象成员分别报告。非法成员名无法安全回传时，路径指向其
所在对象，不回显该成员名或值。数组位置使用 `[index]`。预算错误中的 `verification.*` 指向当前 Task 的预算或用量；`arguments` 指整个 MCP 参数对象，其余参数错误路径指向本次请求字段。状态转换条件使用 `guard.guard_id` 和
`guard.failures[]`。Core 返回能确定的全部相关字段，不让 Host 从通用文案猜字段。

`VERIFICATION_BUDGET_EXCEEDED` 只表示数量上限，`error.budget` 给出 `used`、`requested`、
`limit`；`details[].rule` 区分自动命令和保存记录数量。`VERIFICATION_NOT_ALLOWED` 表示
完整套件或待办人工检查的权限限制，具体字段由 `details[]` 指明。已完成的用户检查和已给出的
验收决定不消耗自动命令额度，也不受待办人工检查开关限制。

公开错误不包含提交的正文、配置值、密钥、内部目录或堆栈。`repository_paths` 仅用于已有的
文件清单遗漏反馈。计数只包含上述非敏感数量。

存储和仓库观察失败保留检查位置及已知原因，例如保存的快照无法解码、工作树实例不符、Git 超时，
或 SQLite 已知的锁、只读、容量和 I/O 错误。更外层的调用不得把已有说明替换为错误类别的通用文案。
无法确定底层原因时说明已知的失败操作和诊断范围，不编造磁盘、权限或用户输入问题。

成功结果为空、JSON 编码失败、响应超过字节上限和请求 ID 生成失败分别返回说明。
编码失败仍保留实际工具和有效的 `request_id`；请求 ID 无法生成时使用 `request-unavailable`，并说明
工具尚未执行。错误码和恢复决定继续由原检查负责；新增说明不会授予重试权限，也不会证明操作零写入。

## 恢复指引

`recovery` 必须包含 `action`、`retry_safe`、`message`。机器处理以 `action` 为准，文案必须
说明同一个动作。`retry_safe=false` 不代表发生过写入，也不要求对允许的只读查询再次征求许可。

| action | Host 行为 |
| --- | --- |
| `correct_request` | Core 已确认握手、读取、创建或生命周期请求在参数检查时零写入。保留同一工具、未列入纠正范围的已有请求身份字段和已有授权，仅按已有事实纠正 allowed_paths 并提交一次 |
| `correct_current_action` | Core 已确认零写入。核对同一个 Action 和当前 Schema，仅根据已有事实修改 `allowed_paths` 内字段，纠正提交一次 |
| `read_next_action`、`read_task`、`retry_read` | 执行指定的只读查询，处理返回的当前状态；不原样重放被拒绝的提交 |
| `resolve_blocker`、`restore_or_abandon`、`use_origin_host`、`provision_worktree` | 根据当前状态执行对应操作，仅对缺失的用户决定提问 |
| `repair_storage`、`report_internal_error`、`none` | 停止自动提交，报告具体问题和需要的操作；文案不能同时要求自动重试 |

只有 `correct_current_action` 和 `correct_request` 返回 `retry_safe=true` 和非空 `allowed_paths`，且路径必须来自
本次错误详情。确定的检查说明遗漏可以纠正；用户确认、验收结论和未知测试结果不能通过纠错补造。
提交前先核对必填字段，纠正后仍失败则报告新的具体问题。不得把响应里的 request_id 加入不接受该参数的工具请求。

普通 Action 的纠正资格及允许字段由 `internal/recovery/correction.go` 统一判断。一次失败中的全部规则
都必须允许纠正；混入未知规则、不安全字段路径、用户决定或未知结果时，不提供自动纠正。
适配器隐藏不适合公开的错误详情，不会让原本不允许纠正的失败变成可重试。

响应缺失、损坏、超时或写入情况不确定时，Host 先读取原 Task/Action 的保存操作，再使用 Core
允许的恢复路径。不得把失败响应解释为成功，也不得用上一次成功提交的结果替代本次响应。

## 验证要求

MCP 测试对真实成功/失败响应执行输出 Schema 校验；`error_reasons_test.go` 遍历全部 18 个工具，检查 JSON、类型和编码失败，并实际触发嵌套字段、工作项依赖和损坏快照错误。请求 ID 失败通过内存 MCP 传输检查实际工具身份。现有测试还覆盖结果路径、非法混合结构、错误字段、
数量与权限区分、零写入纠错及结果不确定的停止条件。共享 Skill 示例必须对同一接口执行校验，
Codex、DeepSeek、Claude Code 和 ZCode 使用相同语义。新增或修改错误时同步更新实现、Schema、本规范及受影响示例。

## WebUI HTTP 映射

WebUI 使用独立的 HTTP DTO：失败仍使用相同的 Core code、details、budget，并提供 field_paths
用于表单定位。workflow_write_state 区分 not_committed 与 unknown；前者说明当前边界确认未提交，
后者要求先查询保存操作。HTTP 的恢复提示保持相同的纠错前提，不能把结果不确定当作参数错误直接提交。

HTTP 的 `correct_current_action` 同样返回非空 `recovery.allowed_paths`，只允许按已知事实纠正一次。
字段路径保留 HTTP 请求中的 `payload.` 前缀；MCP 普通节点工具直接接收这些语义字段，因此去掉该前缀。
两端共用纠正判断，响应结构、状态码和表单定位仍由各自适配器负责。

已保存的 blocker 决定在恢复时使用与普通提交相同的仓库校验。决定仍适用时重放原保存载荷；仓库又发生
变化时保留原 blocker 和待恢复操作，返回 `stop_for_repository_drift`。恢复到保存决定所对应的仓库状态后，
再恢复同一个 Action；不能通过重新提交另一份决定绕过已有操作。

未执行写前请求的范围决定使用保存绑定到当前观察的完整层增量；未变的继承内容不造成该次
决定的范围冲突，也不因此获准修改。新增、删除或任一文件层的变化超出 pending 和已计划范围时，
决定返回 `REPOSITORY_DRIFT`，不保存新的 Action operation。已暂存决定仍经同一校验恢复，
不得把一个新的范围决定当作既存待恢复操作的替代。


## 请求与完整响应示例

四个 Host Skill 的每个完整 MCP 请求都链接对应的完整成功响应，并紧跟错误响应，逐项说明触发条件、校验函数和响应编码函数。成功示例包含用当前 Task 值替换占位符后的完整请求；测试实际执行请求并比较完整返回。仓库观察使用固定测试数据，生成的身份、时间和操作摘要使用稳定示例值。Codex Host helper 与 DeepSeek workspace 请求同样配对完整返回，由临时 Git 仓库中的实际适配器操作校验；Host 会话和 Core 终态读取使用标注的模拟结果。
测试从前一个正确请求构造说明中的失败输入，对完整响应进行比较，包括 message、details、guard
和 recovery。示例说明确定的失败情况，不穷举所有运行条件；已有公共错误示例也与当前编码结果一致。

历史恢复分别报告 `history_resolution.choice` 的枚举错误和 `history_resolution.reason` 的文本错误；多个字段同时错误时一并返回。

## 历史分页与分支改名

`taskbelay_get_task` 可返回 `result.baseline_history`，含 `task_id`、Task `revision`、真实累计
`total`、`entries`（`sequence`、`reference`）及 `next_after`（仅结束为 null）。每个 Task 响应的
`baselines.history` 都是完整数组的有界首页，旁边必有 `history_total`、`history_next_after`、
`history_revision`；不能把首页当作全部历史。页最多 32 条、64 KiB 实际 JSON 字节，完整响应仍限制为
1 MiB，必要时继续缩页且不跳项。首页可缩为空，此时总数非零、下一游标为 0，Host 从 0 显式分页；
请求的非空历史页至少返回一条。非历史部分本身超限仍返回既有编码错误，不放宽响应保护。
分页保留真实引用，不归档、不重建正文。Task revision 变化为 `REVISION_CONFLICT`，须按新 revision
从头读取；非法边界为 `INVALID_ARGUMENT`。快照引用缺号、重复或格式损坏为 `STORAGE_UNAVAILABLE`，
不会成功返回空历史或删除记录。只读操作不升级数据库。

`taskbelay_prepare_task_branch_rename` 返回 `result.rename_id` 与 `result.task`，准备时换发 BLOCKED
Action，并保留 `task.branch_rename`。`taskbelay_resolve_blocker` 接收当前 `action_id`、原 `rename_id`
和 `rename_choice:"complete"` 或 `"cancel"`，返回完整 Task；不得混入其他类型的阻塞决定。complete
核对源 ref 消失、目标 ref 正确，以及全部绑定仓库的实例、HEAD、index、内容均未变化；cancel 核对
原事实全部未变。ref/历史不符为 `WORKSPACE_HISTORY_CONFLICT`，index/内容变化为 `REPOSITORY_DRIFT`，
过期 revision/Action 为 `REVISION_CONFLICT`/`ACTION_STALE`，未恢复的普通操作为 `RECOVERY_UNAVAILABLE`。
决定提交结果不明时保留精确 operation，用 `taskbelay_recover_action` 重放，不因响应丢失重复 Git。
准备结果通过 Task 保存的 rename ID 回读。普通 Task 取消也须核对原事实未变或完整改名事实，才释放 claims。

完整响应与同工具具体失败例子维护在 `skills/taskbelay/core/core-lifecycle-examples.md`，执行检查覆盖
MCP dispatch 与公开输出 Schema。Host launch 替代失败属于 Adapter 结果，不表示 Core 转换；回执与
后继恢复见[工作树来源](WORKTREE-SOURCES.md)。
