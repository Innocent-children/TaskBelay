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
| internal/mcp 与 cmd/taskbelay | 同一 DTO 和应用服务的 MCP/CLI 投影 |
| internal/webui 与 packages/webui | 受限本机 HTTP、结果展示和用户明确检查 |
| Host 包 | 原生权限、授权消息来源、结构化路径钩子、可选工作区准备；不保存第二任务游标 |
| scripts 与 release | 构建、完整安装产物、独立发布；不决定产品验收 |

## 当前契约与控制

Task 保存 authorization、checks、repositories、assessment、progress、issue、evidence、attempts、facts、usage、acceptance、revision 和时间。没有固定开发节点。生命周期出边只有 active→done（完整验收）、active→cancelled（明确取消且无未确定执行）和 active→active（检查点、修订、执行、观察、恢复）；终态没有回边。控制建议由当前事实推导为 continue/self_correct/verify/stop/ask_user，不是第二状态机。

authorization 明确目标、非目标/接口/数据/依赖/设计约束、预测路径、禁止路径、SHA-256 内容 pin、可变依赖名及累计 limits。user_ref 由宿主按真实用户消息提供。确定性限制由程序检查；预测路径、公开 Go 声明、数据文件和变动量促使绑定当前 content_digest 的语义审查。每条语义结论保留 basis、verdict 和 uncertainty。程序不证明这些判断真实。

check 保存 argv、repository_key、cwd、inputs、input_basis、environment、conditions、constraint_ids、timeout_ms、retry 和 rationale。CheckDigest 包含定义、关联目标和相关约束；输入包括选定文件的内容与 mode、缺失文件和目录成员变化；环境包括当前平台、所选变量的哈希、cwd、conditions 及实际执行文件哈希。全仓输入采用 Git 跟踪及非 ignored 新文件；显式输入可包含 ignored 文件/目录。符号链接及无法掌握的输入不会被静默当成有效。缩小依赖范围是宿主判断，缺省保守使用全仓。

Core 实际运行产生 core_execution；claim 永不满足检查；human 只用于明确完成的用户检查。结果新鲜度按上述摘要独立计算。观察无法完成时不能交付。Task 表保存唯一当前状态；runs 保存真实尝试，operations 保存请求身份；结果不由前端或宿主本地缓存改写。

## 持久化与恢复

当前 Schema 只支持一个布局。已存在数据库先只读验证 Schema，再允许可写打开；不兼容不迁移或删除。任务更新在一个事务内 CAS revision、保存操作身份、费用、运行及 claims。执行前保存 running、实际调用身份及 timeout 预留；输出结果先写带完整性签名的本地收据，再原子保存。签名用于防止意外损坏/伪造提交，不隔离同一操作系统账户的恶意写入者。

响应丢失按原 operation_id/run_id 回读。签名收据恢复不会再次启动进程。无收据的停止执行者只能分类为 interrupted_unknown，不能产生通过结果；按 retry 策略核对副作用和必要用户决定。进程身份包括启动身份，避免 PID 重用。Windows 使用隐藏进程和 Job，macOS 使用进程组；这些属于平台边界，不构成安全沙箱。

热任务保留当前证据、最多 64 个近期尝试；近期操作保留 256 个修订内记录，open 与未确定运行身份保留。旧身份被压缩后，旧 revision 仍不能启动重放。运行记录保留近期 128 项及有效引用、未确定执行。压缩后的自有收据清理，累计费用不减。当前快照最多 8 MiB，观察和输入集合有明确资源上限。明确清除仅适用于匹配修订的终态任务。

Core 计量调用、墙钟、观察和执行，宿主另报分析、等待与 token。类别有重叠；未确定耗时保守记录，不伪称精确值。Core 不能测量不经过它的宿主活动。历史压缩、需求修订和检查重建均不能重置用量。

## 接口与信任

MCP 12 个工具，CLI call 和 HTTP /api/core/{tool} 使用同一 DTO/Schema/服务。对象拒绝未知成员和重复 JSON 成员。成功和失败互斥。HTTP 限于随机端口 loopback、精确 Host、Origin 和会话 CSRF。桌面只读取保存摘要；当前适用性需要明确刷新。宿主路径钩子不涵盖任意 shell、外部服务或未调用 Core 的操作。详细入口见[命令](COMMANDS.md)，错误见[响应规范](CORE-RESPONSES.md)。

每个本地 Core 调用（包括未关联 Task 的查询）均先保留调用身份，再结算墙钟时间。control_cost 保存跨任务 calls、wall_ms、unknown_ms 和 pending；崩溃失去精确结束时间时单列保守未知估算，不冒充实测。运行记录与热调用压缩不改变总额。Task 的 Core 墙钟包含执行和等待，不能与这些分类直接相加。

新提交的 assessment 绑定当前全局摘要，但已有判断按 Core 计算的 scope_digest 分别保留：路径关联绑定相关目标和实际内容；约束绑定自身定义及声明路径；观察事实绑定相关内容。checkpoint 可只补充受影响的判断。需求独立新增不清空已有审查，也不自动解决未回答的用户决定。证据按检查、来源、输入与环境上下文保留；同一上下文的最新失败替代旧通过，不同上下文的失败不会删除另一上下文的通过。最多 192 项热证据，达到上限先保留全部当前适用结果，再保留近期历史。
