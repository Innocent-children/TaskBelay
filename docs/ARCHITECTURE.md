# TaskBelay 架构

[中文](ARCHITECTURE.md) | [English](ARCHITECTURE_en.md)

## 组件责任

| 组件 | 唯一责任 |
| --- | --- |
| internal/domain | 授权、目标、约束、适用性关系、生命周期和纯控制决定 |
| internal/application | 组合观察、语义提交、实际执行与原子持久化；无操作系统选择 |
| internal/repository | 保留的只读 Git 观察、工作区身份与历史、index/worktree 内容、文件内事实 |
| internal/execution | 输入/环境/执行文件指纹、真实进程、输出、平台进程寿命和结果收据 |
| internal/store | 当前 SQLite 布局、CAS、目录占用、操作去重、成本及记录压缩 |
| internal/protocol | 共享 DTO、Schema、结果与错误；不保存任务状态 |
| internal/mcp 与 cmd/taskbelay | MCP 仅提供任务操作；原生命令由 CLI check run 执行 |
| internal/webui 与 packages/webui | 受限本机 HTTP、结果展示和用户明确检查 |
| Host 包 | 原生权限、授权消息来源、结构化路径钩子、可选工作区准备；不保存第二任务游标 |
| scripts 与 release | 构建、完整安装产物、独立发布；不决定产品验收 |

Host 工作区操作使用私有原子收据记录固定请求、绑定工作树与逐仓整合结果；它不是第二个 Task。修改前保存意图，响应丢失按原身份读取；只读恢复核对当前 Git，保留未知命令结果，不回退已应用仓库或继续剩余动作。Core 仍只观察 Git，并核对不可变来源选择。调用计量按存储、进程查询与其他初始化责任保留原始错误；查询拒绝不证明进程已停止。

## 当前契约与控制

Task 保存 authorization、plan、budget_decision、exceptions、archived、changes、repositories、assessment、progress、issue、evidence、attempts、facts、usage、acceptance、revision 和时间。没有固定开发节点。生命周期出边只有 active→done（完整验收）、active→cancelled（明确取消且无未确定执行）和 active→active（检查点、修订、执行、观察、恢复）；终态没有回边。控制建议由当前事实推导为 continue/review/self_correct/verify/stop/ask_user，不是第二状态机。

authorization 保存用户目标、非目标/接口/数据/依赖/设计约束、允许/禁止路径、SHA-256 内容 pin、可变依赖名及累计 limits。user_ref 由宿主按真实用户消息提供。确定性限制由程序检查；预测路径、公开 Go 声明、数据文件和变动量促使绑定当前 content_digest 的语义审查。每条语义结论保留 basis、verdict 和 uncertainty。程序不证明这些判断真实。

check 保存 argv、repository_key、cwd、inputs、input_basis、environment、conditions、constraint_ids、timeout_ms、retry 和 rationale。CheckDigest 包含检查语义、关联目标和相关约束，排除 timeout_ms、retry、full_suite、rationale 和 input_basis；这些运行设置与说明仍由 PlanDigest 完整绑定。只有当前检查确实有适用通过结果时才拒绝重复执行；输入包括选定文件的内容与 mode、缺失文件和目录成员变化；环境包括当前平台、所选变量的哈希、cwd、conditions 及实际执行文件哈希。缺省全范围输入覆盖全部绑定仓库的 Git 跟踪及非 ignored 新文件；显式输入可包含 ignored 文件/目录。符号链接及无法掌握的输入不会被静默当成有效。缩小依赖范围是宿主判断，缺省保守使用全仓。

Core 实际运行产生 core_execution；claim 永不满足检查；human 只用于明确完成的用户检查。结果新鲜度按上述摘要独立计算。观察无法完成时不能交付。Task 表保存唯一当前状态；runs 保存真实尝试，operations 保存请求身份；结果不由前端或宿主本地缓存改写。

## 持久化与恢复

当前 Schema 只支持一个布局。已存在数据库先只读验证 Schema，再允许可写打开；不兼容不迁移或删除。任务更新在一个事务内 CAS revision、保存操作身份、费用、运行及 claims。执行前保存 running、实际调用身份及 timeout 预留。命令激活前以保留运行快照为条件更新 runs.snapshot，写入 process.pid/identity，不改变 Task revision；身份保存失败不放行命令。macOS 启动管道在 Core 退出时关闭，未收到放行信号的进程不会执行命令；Windows 在挂入 Job 并保存身份后恢复挂起进程。输出结果先写带完整性签名的本地收据，再原子保存。签名用于防止意外损坏/伪造提交，不隔离同一操作系统账户的恶意写入者。

响应丢失按原 operation_id/run_id 回读。签名收据恢复不会再次启动进程。无收据时，恢复同时核对 Core 和已记录命令；macOS 检查整个进程组，包括组长退出后的非僵尸后代。仍在运行或无法观察时保留 pending，不允许新执行、完成或取消。两者均停止后只能分类为 interrupted_unknown，不能产生通过结果；按 retry 策略核对副作用和必要用户决定。进程身份包括启动身份，避免 PID 重用。Windows 使用隐藏进程和 Job，macOS 使用进程组；这些属于平台边界，不构成安全沙箱。

热任务保留当前证据、最多 64 个近期尝试；近期操作保留 256 个修订内记录，open 与未确定运行身份保留。旧身份被压缩后，旧 revision 仍不能启动重放。运行记录保留近期 128 项及有效引用、未确定执行。压缩后的自有收据清理，累计费用不减。当前快照最多 8 MiB，观察和输入集合有明确资源上限。明确清除仅适用于匹配修订的终态任务。

Core 计量变更调用、墙钟、观察和执行，宿主另报分析、等待与 token。类别有重叠；未确定耗时保守记录，不伪称精确值。Core 不能测量不经过它的宿主活动。历史压缩、需求修订和检查重建均不能重置用量。

## 接口与信任

MCP 15 个工具，CLI call 和 HTTP /api/core/{tool} 使用同一 DTO/Schema/服务。对象拒绝未知成员和重复 JSON 成员。成功和失败互斥。HTTP 限于随机端口 loopback、精确 Host、Origin 和会话 CSRF。桌面只读取保存摘要；当前适用性需要明确刷新。宿主路径钩子不涵盖任意 shell、外部服务或未调用 Core 的操作。详细入口见[命令](COMMANDS.md)，错误见[响应规范](CORE-RESPONSES.md)。

变更调用先保留调用身份，再结算墙钟时间。保存状态查询不写计量、不检查进程身份；Service.SavedControlCost 仅返回已保存总额和未结算记录。control_cost 保存跨任务 calls、wall_ms、unknown_ms 和 pending；崩溃失去精确结束时间时单列保守未知估算，不冒充实测。运行记录与热调用压缩不改变总额。Task 的 Core 墙钟包含执行和等待，不能与这些分类直接相加。

新提交的 assessment 绑定当前全局摘要，但已有判断按 Core 计算的 scope_digest 分别保留：路径关联绑定相关目标和实际内容；约束绑定自身定义及声明路径；观察事实绑定相关内容。checkpoint 可只补充受影响的判断。需求独立新增不清空已有审查，也不自动解决未回答的用户决定。证据按检查、来源、输入与环境上下文保留；同一上下文的最新失败替代旧通过，不同上下文的失败不会删除另一上下文的通过。最多 192 项热证据，达到上限先保留全部当前适用结果，再保留近期历史。

## 用户边界与验收

实际请求、目标和明确限制与计划的 approach、verification_basis 分开。Authorization.BoundaryDigest 只排除引用和另行比较的成本字段；检查映射、预测和复核提示已经归入 Plan。用户边界变化或已设正数额度扩大/取消时，Revise 要求实际新 user_ref；普通方法调整沿用授权，实际操作仍遵守宿主权限。PlanApproval 和确认工具已移除；PlanDigest 用于技术复核当前安排，不表示用户批准。

Goal.RequirementDigest 只关联用户目标；新增检查不会重置已完成实现或独立结果。CheckDigest 仍绑定具体方法、相关目标和约束。路径判断关联目标与实际内容。最终 DesignReview 使用 summary 和 uncertainty，由 Core 绑定当前计划及内容。既有失败例外依赖实际失败/比较结果及当前适用性，不因无关计划文字变化要求重复接受。

未设额度为零；正数额度在预留和执行时检查。BudgetDecision 单独记录实际预算变化；未设上限也持续记录消耗，不通过默认额度制造审批。

internal/protocol/view.go 将应用结果转换为当前摘要，完整 Task 仍由应用和存储保存。get_task detail:true 增加 details，包含完整仓库指纹、复核与历史；run/recover 返回运行摘要，get_run 读取保存的输出。转换层不保存第二份状态。列表保留有界分页，归档仅改变终态可见性。

## 实施计划、原生执行与验收

Plan 保存 approach、verification_basis、expected_paths、review_after_lines 和 checks。Check.GoalIDs 是唯一目标关联，所有目标至少一个 acceptance 检查。acceptance 和 regression 均需解决，diagnostic 不作为交付要求。Decide 可以认可 regression 例外，但 Finish 的 acceptance 只允许引用实际通过的 acceptance 检查。changes 保存最多 64 条近期授权、计划、预算、用户答复及失败例外的原因；它不是状态机或完整快照历史。Progress.NextStep 保存恢复所需的下一步。

internal/protocol.Handler 提供共享解码、结果摘要和错误。MCP/HTTP 只调用 Catalog；NativeCheck 仅通过 CLI check run 进入 Service.Execute。宿主原生工具负责权限、环境和等待，Core 的执行实现继续保存真实输入、进程身份和运行收据。长命令使用原生会话，不延长 MCP 请求或创建调度服务。Service.ResolveIssue 使用 issue_digest 防止过期答复，记录决定但不自动扩大授权。

完成时只保留实际用于本次交付且仍适用的回归例外；读取终态返回这些交付时记录，既不丢失已接受失败，也不重新认证当前工作区。
