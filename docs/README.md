# 文档说明

本目录描述 TaskBelay 的产品目标、判定算法及实现设计。各份文档围绕同一条原则：用户照常开发，TaskBelay 记录事实，帮助 Agent 完成当前要求。

## 设计与实现状态

算法文档定义语言无关的目标设计：统一词法与版面结构、从仓库统计学到的约定、变更簇、需求相关区（局部图聚类）、对外新增项与锚定、间接层的变化度、近似重复与没人使用的代码、撤回试验的分组检验、逐个运行的变异、验证输入记录，以及合并信号、反馈选择、校准和按事实拦截。目标设计尚未在产品代码中实现，不能据此认为新算法已经可用。

当前代码仍使用语言适配、D1 至 D8 / E1、T1 至 T6 / V1 至 V3 等规则及原有测试流程。它们的固定数量阈值和语言支持分级不是目标设计的约束。历史报告按其规则与程序版本解释，不重新标成新算法结论。

安装命令、CLI、八个 MCP 工具、现有设置项、schema 11 和用户数据保留规则继续按现有实现说明使用。目标设计的输入通过现有工具的字段提供：验收标准写在 `requirement_save` 的 `acceptance`，对反馈的处理、保留决定和重跑理由用 `note_add` 记录。目标设计需要的新记录在 [任务账本](requirements-ledger.md#目标算法的逻辑记录) 中单列为逻辑记录，系统架构和账本明确区分当前实现与目标职责。

## 文档的层次

1. **产品需求**：[product.md](product.md) 定义产品解决什么问题、正常使用方式和成功标准。
2. **核心设计**：[requirements.md](requirements.md) 定义任务、需求、工作区更新和改动归属；[algorithm.md](algorithm.md) 定义共享的算法：变更簇、需求相关区、验收依据、分组检验、合并信号与校准；[over-design.md](over-design.md) 与 [over-testing.md](over-testing.md) 定义两类判定；[languages.md](languages.md) 定义统一词法、仓库统计、命令记录与副本运行；[handoff.md](handoff.md) 与 [semi-auto.md](semi-auto.md) 定义接续和方案流程。
3. **衍生设计**：账本、Agent 接入、系统架构和评估依据前两层展开。核心行为改变时，同步修改数据、事件、执行流程和验收场景。

每件事在负责它的文档中完整定义，其余文档引用，不另立一套规则。

## 每份文档负责什么

| 文档 | 内容 |
| --- | --- |
| [product.md](product.md) | 目标用户、正常开发场景、功能、模式、默认行为和成功标准 |
| [requirements.md](requirements.md) | 任务开始、暂停、恢复、交付；累计 diff；用户修改；需求变更、位置与归属 |
| [algorithm.md](algorithm.md) | 设计原则、变更簇、需求相关区、验收依据、分组检验、合并信号与反馈选择、拦截条件、校准、问题身份、计算时机和复杂度 |
| [over-design.md](over-design.md) | 与需求无关的改动、对外新增项与锚定、间接层的变化度、重复实现、没人使用的代码和撤回试验 |
| [over-testing.md](over-testing.md) | 用例与表格行、等价类与边界值、变异与保留集合、测试缺口、验证输入记录、范围包含和通过规律 |
| [languages.md](languages.md) | 统一词法、版面树与单元、从起点学习的约定、结构置信度、命令与运行记录、副本中的运行 |
| [handoff.md](handoff.md) | 接续时机、摘要内容和预算、用户修改与多 session 协作 |
| [semi-auto.md](semi-auto.md) | 方案范围、确认、增补、模式切换、S1 和 S2 |
| [requirements-ledger.md](requirements-ledger.md) | 记录、字段、修订、MCP 工具和保留期限 |
| [product-agent-integration.md](product-agent-integration.md) | hook、MCP、Agent 与平台差异、安装和故障处理 |
| [product-architecture.md](product-architecture.md) | 进程、模块、快照、观察与编辑核对、检查执行、存储和性能 |
| [product-evaluation.md](product-evaluation.md) | 当前测试维护范围，以及目标算法的正确性、效果、普适性和成本评估口径 |

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
| 单元、首行 | 版面树的节点，通常对应函数、类型或配置段；以及单元的第一行 | [输入与执行协议](languages.md) |
| 新词 | 起点词表中没有、本次改动引入的标识符 | [算法核心](algorithm.md) |
| 变更簇 | 必须一起保留或撤回的一组 hunk | [算法核心](algorithm.md) |
| 需求相关区 | 以一条需求的原话、方案和需求内容为种子，用局部图聚类得到的单元和变更簇 | [算法核心](algorithm.md) |
| 验收依据 | 起点已有测试中能通过的部分、实现前登记的验收和用户点名的用例 | [算法核心](algorithm.md) |
| 对外入口 | 参数、子命令、环境变量、HTTP 路由和配置字段 | [过度设计判定](over-design.md) |
| 对外新增项、锚定 | 外部看得见的新增内容；以及它的名字或取值能在原话或方案中找到，或已登记为需求的实现位置 | [过度设计判定](over-design.md) |
| 撤回试验 | 把变更簇恢复成起点内容后运行验收依据 | [过度设计判定](over-design.md) |
| 可信度 | 按本项目的反馈记录，同类建议成立的概率；只用于给建议排序 | [算法核心](algorithm.md) |
| 测试用例、验证 | 一条可单独运行的测试，以及运行测试、构建、检查等动作 | [过度测试判定](over-testing.md) |
| 验证输入 | 决定一次验证结果的内容、目录、参数、环境变量和工具身份，以其哈希比较 | [过度测试判定](over-testing.md) |
| 检查问题 | 带稳定身份、事实、建议和处理状态的一处检查结果 | [算法核心](algorithm.md) |
| 检查结论 | 放行、提示或拦截；处理状态、依据范围与结论分别记录 | [算法核心](algorithm.md) |
| 工作模式、拦截强度 | 全自动或半自动；拦截一次或只提示 | [产品需求](product.md) |
| 有效方案、待确认增补 | 已确认并仍有效的范围，以及新提出的范围变化 | [半自动模式](semi-auto.md) |
| 接续摘要 | 根据账本和当前工作区生成的任务摘要 | [任务接续](handoff.md) |
