# TaskBelay 接口使用说明

[中文](INTERFACES.md) | [English](INTERFACES_en.md)

适配器从方法说明读取输入、输出、授权要求和失败处理；不需要查看 Go/JavaScript 源码或 SQLite。公开接口保留现有入口，完整说明按方法读取，避免每次调用重复传输全部契约。

## 查找与调用

Core 的 `taskbelay_server_info` 返回 15 个实际工具名、原生命令说明和 `auxiliary_contracts`。请求中的可选 `contracts` 数组按名称展开 `name`、`description`、`input_schema`、`output_schema`、`read_only`、完整 `examples`，原生/HTTP 入口还说明 `transport`。它只查询说明，不执行被查询的方法。CLI 的 `call <tool> --help --json`、`check run --help --json` 与对应的 `host-check` 帮助不依赖数据库。

Codex、Claude、ZCode 的 `host-launch <operation> --help --json` 返回相同用途的随包说明。`hook pre-tool-use --help --json`、安装管理及宠物命令也提供方法帮助。DeepSeek 的 `contracts/interfaces.json` 包含 workspace_coordinator 的六个分支；可直接读取或调用包内 `lib/interface-reference.mjs` 的 `describeInterface(packageRoot, name)`，传入安装目录和完整条目名，返回对应契约；省略 name 返回名称/描述列表，不执行工作区操作。

所有条目的集中说明在 `contracts/interfaces.json`。每个条目含至少一个完整成功样例和同方法的具体失败样例，包含所需前提。路径、身份、时间和耗时采用示例值；实际调用必须使用本机路径和上次实际返回的身份。`implementation` 只供维护者定位，不是调用前提。安装管理样例的外部 Host 驱动、用户决定和桌面动作注明为模拟；它们证明返回格式，不证明已安装或打开真实会话。

| 入口 | 条目 | 随包/原生读取方式 |
| --- | ---: | --- |
| Core MCP 方法 | 15 | server_info.contracts / call 方法帮助 |
| 原生检查、Host 预检 | 3 | check run / host-check 方法帮助 |
| Codex / Claude / ZCode 工作区操作 | 16 / 13 / 11 | host-launch 方法帮助 |
| DeepSeek 协调器分支 | 6 | 包内 contracts/interfaces.json |
| 本机 HTTP | 6 | server_info.contracts 中的 GET/POST 名称 |
| 原生 hook | 3 | hook pre-tool-use 方法帮助 |
| Core 配置、服务、帮助、版本、MCP 启动 | 8 | 对应方法帮助 |
| 统一安装管理、宠物命令 | 8 / 2 | 对应方法帮助 |
| 三个 Host 安装管理 | 9 | status/setup/remove 方法帮助 |

100 个条目是现有入口的说明范围，不是新增 100 个工具。Core 方法的 MCP/CLI/HTTP 转发不重复计数。HTTP `/api/core/{tool}` 的 arguments 与输出由所选 Core 方法决定；应同时读取该方法说明。

## 怎样使用返回值

普通 Task 变更返回 `identity`、`digests`、`decision.gaps`、`progress`、`checks`、`concerns`、`limitations`、`issue`、`usage`、`budget`。下一次变更直接复制实际 `identity.task_id/revision`；不要自行加一。open/get 的 `context` 保存请求、完整计划和仓库摘要。`get_task detail:true` 的 `details` 包含逐项审查、观察、结果适用性、仓库绑定与历史；没有可写回的内部 Task 快照。

原生 `check run` 实际执行一次命令，返回 `view` 和 `run`。`ok:true` 只代表请求完成处理，必须检查 `run.status`。stdout/stderr 在 `get_run.output` 读取。已有适用结果会由 `CHECK_ALREADY_SATISFIED` 返回其身份，直接复用，不为登记再运行一次。

`refreshed:false` 表示保存状态，检查是否仍适用于当前文件需要刷新。已保存的通过结果仍显示 passed，但不能把 applicable=false 忽略。未设上限时 `limited:false`、`remaining:null`；Host 未上报的数值为 null，实际测得零值才是 0。partial 的数字仅为已知部分，不能认为全部用量。

省略 assessment.concerns 或空数组保留原问题。提交同一问题但省略 resolution 表示重新打开；明确处理使用 verified、not_blocking 或 scope_changed。not_blocking 的理由进入交付限制，不能豁免必需检查或确定性违规。输入与输出的内部摘要字段分开，调用方不能自行填写 scope_digest 等只读字段。

## 失败后的动作

Core 错误包含具体字段、当前缺口，以及可用时的当前身份或已存在结果。`outcome` 是语义提交结果：not_applied、applied 或 unknown；计量写入另外记录。原生命令失败可附带 execution，说明已知的启动状态。`recovery.operation` 指出原 operation_id 应复用、替换、先读再决定或无需操作。非零退出或失败响应都不能单独证明未执行。

Host CLI 的 stdout、stderr、退出码分别解释；不能拼接后当 JSON。MCP 的 structuredContent 始终保留完整结果；ZCode 采用简短文本，其他主要展示文本的 Host 保留完整文本。HTTP 使用实际状态码及共享错误体；变更需要精确 loopback Host、当前 Origin 和页面会话 token。工作区创建、会话派发和删除仍受真实权限约束。

响应丢失先查原 Task/run/工作区收据。不确定执行不能通过重建请求重复调用。返回 action_required 的安装或会话操作需要完成给出的实际动作，不能当成 ready 或已打开会话。

## 维护与确认

Core DTO 和运行时 Schema 由 `internal/protocol` 维护。示例生成器执行隔离 Core、Git、HTTP 和公开序列化方法，分别保存到 contracts 下的 Core、Host、安装管理及运行时文件。`scripts/package-interface-contracts.mjs` 合并并复制到安装包，`--check` 确认没有漏同步；帮助只读取包内资源。

`internal/protocol/examples_test.go` 执行真实生产者，校验实际成功/失败输出并比较完整错误。`tests/contract/interfaces_test.go` 校验 100 个条目的完整返回样例。`scripts/interface-contracts.test.mjs` 检查入口覆盖、字段说明和成功/失败样例，并在没有 Core/Git/数据库的独立包目录中调用方法帮助。真实 Host 模型会话和最终发布包安装另行验证，不能由这些检查替代。
