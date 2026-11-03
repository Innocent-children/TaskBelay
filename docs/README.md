# 文档说明

本目录描述 TaskBelay 的产品目标、判定算法及实现设计。各份文档围绕同一条原则：用户照常开发，TaskBelay 记录事实，帮助 Agent 完成当前要求。

## 设计与实现状态

本目录描述的新算法尚未实现。它用证据超图管理事实及其依赖，通过约束优化寻找精简方案，通过集合覆盖选择验证操作，并在输入变化后更新受影响的结果。这些方法使用统一的数据表示，不按开发语言划分。

当前代码仍使用语言适配、D1 至 D8 / E1、T1 至 T6 / V1 至 V3 等规则及原有测试流程。新设计不再采用这些固定数量阈值和语言支持分级。历史报告按其规则与程序版本解释，不重新标成新算法结论。

安装命令、CLI、八个 MCP 工具、现有设置项、schema 11 和用户数据保留规则仍按当前实现说明使用。新算法如何接收和复核分析结果、怎样存储数据，以及如何迁移现有代码，需在实施时确定。架构和账本文档分别标明了当前实现与计划调整的部分。

## 文档的层次

1. **产品需求**：[product.md](product.md) 定义产品解决什么问题、正常使用方式和成功标准。
2. **核心设计**：[requirements.md](requirements.md) 定义任务、需求、工作区更新和改动归属；[algorithm.md](algorithm.md) 定义事实、证据及其增量更新方法；[over-design.md](over-design.md) 与 [over-testing.md](over-testing.md) 定义两类决策；[languages.md](languages.md) 定义分析所需的输入和验证操作的执行要求；[handoff.md](handoff.md) 与 [semi-auto.md](semi-auto.md) 定义接续和方案流程。
3. **衍生设计**：账本、Agent 接入、系统架构和评估依据前两层展开。核心行为改变时，同步修改数据、事件、执行流程和验收场景。

每件事在负责它的文档中完整定义，其余文档引用，不另立一套规则。

## 每份文档负责什么

| 文档 | 内容 |
| --- | --- |
| [product.md](product.md) | 目标用户、正常开发场景、功能、模式、默认行为和成功标准 |
| [requirements.md](requirements.md) | 任务开始、暂停、恢复、交付；累计 diff；用户修改；需求变更、位置与归属 |
| [algorithm.md](algorithm.md) | 输入信息、证据关系、增量更新、检查结论、运行成本与适用条件 |
| [over-design.md](over-design.md) | 有限域约束、成本、树分解、分支定界和精简方案及依据 |
| [over-testing.md](over-testing.md) | 待验证问题、验证选择、证据复用、停止条件和测试精简 |
| [languages.md](languages.md) | 文件与改动信息、分析结果、输入清单和验证执行要求 |
| [handoff.md](handoff.md) | 接续时机、摘要内容和预算、用户修改与多 session 协作 |
| [semi-auto.md](semi-auto.md) | 方案范围、确认、增补、模式切换、S1 和 S2 |
| [requirements-ledger.md](requirements-ledger.md) | 记录、字段、修订、MCP 工具和保留期限 |
| [product-agent-integration.md](product-agent-integration.md) | hook、MCP、Agent 与平台差异、安装和故障处理 |
| [product-architecture.md](product-architecture.md) | 进程、模块、快照、观察与编辑核对、检查执行、存储和性能 |
| [product-evaluation.md](product-evaluation.md) | 当前测试维护范围，以及新算法的正确性、效果、普适性和成本评估口径 |

## 术语

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
| 改动单元 | 带内容版本、范围和来源的变化，不要求对应语法声明 | [分析输入与验证执行](languages.md) |
| 实现机制 | 一组共同完成行为的代码、配置、依赖和状态 | [过度设计判定](over-design.md) |
| 支持关系、可能依赖 | 已核对的结论依据，以及用于保守失效的影响范围 | [算法设计](algorithm.md) |
| 待验证问题 | 有来源且尚待回答的行为问题，包括条件和可观察结果 | [过度测试判定](over-testing.md) |
| 验证操作、证据 | 能回答问题的具体操作，以及实际结果提供的有效依据 | [过度测试判定](over-testing.md) |
| 对外入口 | 参数、子命令、环境变量、HTTP 路由和配置字段 | [过度设计判定](over-design.md) |
| 测试用例、验证 | 一条可单独运行的测试，以及运行测试、构建、检查等动作 | [过度测试判定](over-testing.md) |
| 检查问题 | 带稳定身份、事实、建议和处理状态的一处检查结果 | [算法设计](algorithm.md) |
| 检查结论 | 放行、提示或拦截；处理状态、证据范围与求解结果分别记录 | [算法设计](algorithm.md) |
| 工作模式、拦截强度 | 全自动或半自动；拦截一次或只提示 | [产品需求](product.md) |
| 有效方案、待确认增补 | 已确认并仍有效的范围，以及新提出的范围变化 | [半自动模式](semi-auto.md) |
| 接续摘要 | 根据账本和当前工作区生成的任务摘要 | [任务接续](handoff.md) |
