# 文档说明

本目录描述 TaskBelay 的产品目标、判定算法及实现设计。各份文档围绕同一条原则：用户照常开发，TaskBelay 记录事实，帮助 Agent 完成当前要求。

## 文档的层次

1. **产品需求**：[product.md](product.md) 定义产品解决什么问题、正常使用方式和成功标准。
2. **核心设计**：[requirements.md](requirements.md) 定义任务、需求、工作区更新和改动归属；[over-design.md](over-design.md) 与 [over-testing.md](over-testing.md) 定义检查；[languages.md](languages.md) 把通用概念对应到各语言；[handoff.md](handoff.md) 与 [semi-auto.md](semi-auto.md) 定义接续和方案流程。
3. **衍生设计**：账本、Agent 接入、系统架构和评估依据前两层展开。核心行为改变时，同步修改数据、事件、执行流程和验收场景。

每件事在负责它的文档中完整定义，其余文档引用，不另立一套规则。

## 每份文档负责什么

| 文档 | 内容 |
| --- | --- |
| [product.md](product.md) | 目标用户、正常开发场景、功能、模式、默认行为和成功标准 |
| [requirements.md](requirements.md) | 任务开始、暂停、恢复、交付；累计 diff；用户修改；需求变更、位置与归属 |
| [over-design.md](over-design.md) | D1 至 D8、项目惯例、用户决定、问题身份和反馈 |
| [over-testing.md](over-testing.md) | T1 至 T6、V1 至 V3、风险建议、动态检查和运行预算 |
| [languages.md](languages.md) | 分级能力、各语言语法、测试工具、依赖和运行方式 |
| [handoff.md](handoff.md) | 接续时机、摘要内容和预算、用户修改与多 session 协作 |
| [semi-auto.md](semi-auto.md) | 方案范围、确认、增补、模式切换、S1 和 S2 |
| [requirements-ledger.md](requirements-ledger.md) | 记录、字段、修订、MCP 工具和保留期限 |
| [product-agent-integration.md](product-agent-integration.md) | hook、MCP、Agent 与平台差异、安装和故障处理 |
| [product-architecture.md](product-architecture.md) | 进程、模块、快照、观察与编辑核对、检查执行、存储和性能 |
| [product-evaluation.md](product-evaluation.md) | 当前测试维护范围，以及历史规则例子、开发场景、效果与成本评估口径 |

## 共同术语

| 术语 | 含义 | 定义 |
| --- | --- | --- |
| Agent | Claude Code 或 Codex | |
| session | Agent 的一次会话，Codex 的 thread 在本文也称 session | |
| 一轮 | 用户消息到达至 Agent 停下等待用户 | |
| 工作区 | 一个 git 工作区，worktree 分别管理 | [任务和需求](requirements.md) |
| 当前任务 | 该工作区当前接续和检查的一件事，可以跨提交与 session | [任务和需求](requirements.md) |
| 暂停任务 | 保存进度、等待恢复的任务 | [任务和需求](requirements.md) |
| 用户消息、原话引用 | 保存的消息原文，以及有明确来源的引用 | [任务和需求](requirements.md) |
| 需求 | 用户要的一项结果，编号 R1、R2…… | [任务和需求](requirements.md) |
| 原话需求、推断需求 | 有可核对来源的需求，以及 Agent 提议的需求 | [任务和需求](requirements.md) |
| 实现判断 | 完成需求所需的接线、处理和内部组织 | [任务和需求](requirements.md) |
| 实现位置、归属 | 需求落到哪里，以及改动与需求的联系 | [任务和需求](requirements.md) |
| 工作区状态 | git 管理的内容与未忽略新文件组成的 tree 标识 | [系统架构](product-architecture.md) |
| 起点快照、累计 diff | 任务起点状态，以及当前状态与它的差别 | [任务和需求](requirements.md) |
| 观察点 | 某时刻的完整状态或目标文件内容标识 | [任务账本](requirements-ledger.md) |
| Agent 编辑 | 工具请求与实际前后内容能够对应的改动 | [任务和需求](requirements.md) |
| 工作区更新 | 观察到、但不能归入已核对 Agent 编辑的变化 | [任务和需求](requirements.md) |
| 需求代码 | 与需求关联的改动声明及列出的声明 | [任务和需求](requirements.md) |
| 声明 | 函数、方法、类、接口、类型、顶层常量和变量等具名定义 | [语言支持](languages.md) |
| 其他文件 | 按文件分析的文档、配置和未解析语言文件 | [任务和需求](requirements.md) |
| 对外入口 | 参数、子命令、环境变量、HTTP 路由和配置字段 | [过度设计判定](over-design.md) |
| 测试用例、验证 | 一条可单独运行的测试，以及运行测试、构建、检查等动作 | [过度测试判定](over-testing.md) |
| 检查问题 | 带稳定身份、事实、建议和处理状态的一处检查结果 | [过度设计判定](over-design.md) |
| 检查结论 | 放行、提示或拦截，完成情况另外记录 | [过度设计判定](over-design.md) |
| 工作模式、拦截强度 | 全自动或半自动；拦截一次或只提示 | [产品需求](product.md) |
| 有效方案、待确认增补 | 已确认并仍有效的范围，以及新提出的范围变化 | [半自动模式](semi-auto.md) |
| 接续摘要 | 根据账本和当前工作区生成的任务摘要 | [任务接续](handoff.md) |
