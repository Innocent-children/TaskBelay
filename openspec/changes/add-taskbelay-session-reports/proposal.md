# Proposal

## Why

TaskBelay 的需求记录、接续提示和检查干预主要交给 Agent，用户日常不容易看见它实际做过的事。将这些动作在发生时直接写入用户目录，用户打开会话记录文件即可了解过程。

## What Changes

- 在日常运行过程中自动记录 TaskBelay 实际完成的需求与决定保存、接续提示、检查反馈、验证拦截和过期编辑处理。
- 将简短、可直接阅读的中文 Markdown 追加到用户数据目录的 `session-records/`，按工作区、Agent 和 session 区分文件；同一会话持续追加。
- 每条写清发生时间、对象、实际动作、当时已知结果和必要的证据编号。后续有新事件时另写一条，不反推或重写此前结果。
- 用户自行查看对应文件夹；不新增查询工具、查询命令、统计汇总、报告生成入口或自动对话播报。
- 只记录功能运行后新发生的动作，不读取、兼容、转换或回填历史数据。
- 规划阶段仅交付 OpenSpec 文档；实际实施状态见 `tasks.md`。

## Capabilities

### New Capabilities

- `taskbelay-session-reports`: 在动作发生时持续保存可直接阅读的会话记录，覆盖记录内容、文件归属、追加写入和行为边界。

### Modified Capabilities

无。当前 OpenSpec 根目录尚无主规格；本变更以 TaskBelay 现有产品文档和实现为依据。

## Impact

- 目标项目：`/Users/innocent-children/GoLandProjects/TaskBelay`。规划文件保存在项目内的 `openspec/changes/add-taskbelay-session-reports/`。
- `internal/hook/` 和现有 MCP 处理路径：在已有动作及结果可确认的位置传入记录信息；不增加新的 MCP 工具或改变既有协议输出。
- `internal/ledger/`：增加简短会话记录的文件追加写入，复用现有用户数据目录及本地文件写入方式，不增加报告查询表或汇总任务。
- 产品文档：同步 `docs/product.md`、`docs/requirements-ledger.md`、`docs/product-agent-integration.md` 和 `docs/product-architecture.md` 中的记录范围、路径及实际写入时机。
- 运行范围继续覆盖 Claude Code、Codex，以及 Windows、macOS、Linux；记录不依赖 debug，不启动额外检查、测试或工作区扫描。
