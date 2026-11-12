# 文档说明

本目录描述 TaskBelay 的产品目标、判定算法及实现设计。各份文档围绕同一条原则：用户照常开发，TaskBelay 记录事实，帮助 Agent 完成当前要求。

## 产品与算法

TaskBelay 用语言无关的词法、版面树和仓库统计分析内容，再计算变更簇、需求相关区，以及过度设计和过度测试的分析信号。静态信号用于给候选问题排序。动态检查根据副本中的撤回试验、逐个变异和整组确认结果，判断哪些改动或测试可以精简。TaskBelay 按可核对的事实决定是否拦截；信息不足、执行出错和检查未完成分别记录。

Agent 通过 `requirement_save.acceptance` 保存验收标准，通过 `implemented_by` 登记实现需求的文件、入口和依赖。处理少量可疑设计、测试或防御时，可用 `note_add.review` 引用具体代码并说明用途。TaskBelay 核对引用、当前需求和编辑来源，给出有条件的建议。保留决定和重跑理由也通过 `note_add` 记录。数据记录见 [任务账本](requirements-ledger.md#算法的逻辑记录)，模块关系见 [系统架构](product-architecture.md)。

## 文档的层次

1. **长期路线图**：[roadmap.md](roadmap.md) 定义长期方向、各阶段目标和完成条件，并在“已完成记录”中保存实际开发进展。常规开发按路线图推进；本次按 [自动化迭代方法](iteration-method.md) 进行的产品进化按实际问题和评估结果选题，不受阶段顺序约束，具体范围见项目根目录的 `AGENTS.md`。
2. **产品需求**：[product.md](product.md) 定义产品解决什么问题、保持产品简单的原则、正常使用方式和成功标准。
3. **核心设计**：[requirements.md](requirements.md) 定义任务、需求、工作区更新和改动归属；[algorithm.md](algorithm.md) 定义共享的算法：变更簇、需求相关区、验收依据、分组检验、合并信号与校准；[over-design.md](over-design.md) 与 [over-testing.md](over-testing.md) 定义两类判定；[languages.md](languages.md) 定义统一词法、仓库统计、命令记录与副本运行；[handoff.md](handoff.md) 与 [semi-auto.md](semi-auto.md) 定义接续和方案流程。
4. **衍生设计**：账本、Agent 接入、系统架构和评估文档依据产品需求与核心设计展开。核心行为改变时，同步修改数据、事件、执行流程和验收场景。

每件事在负责它的文档中完整定义，其余文档引用，不另立一套规则。

## 每份文档负责什么

| 文档 | 内容 |
| --- | --- |
| [roadmap.md](roadmap.md) | 长期方向、各阶段目标和完成条件、保持产品简单的原则、已完成记录 |
| [product.md](product.md) | 目标用户、保持产品简单的原则、正常开发场景、功能、模式、默认行为和成功标准 |
| [requirements.md](requirements.md) | 任务开始、暂停、恢复、交付；累计 diff；用户修改；需求变更、位置与归属 |
| [algorithm.md](algorithm.md) | 设计原则、变更簇、需求相关区、验收依据、分组检验、合并信号与反馈选择、拦截条件、校准、问题标识、计算时机和复杂度 |
| [over-design.md](over-design.md) | 与需求无关的改动、对外新增项与需求的对应关系、间接层的变化度、重复实现、没人使用的代码和撤回试验 |
| [over-testing.md](over-testing.md) | 用例与表格行、等价类与边界值、变异与保留集合、测试缺口、验证输入记录、范围包含和通过规律 |
| [languages.md](languages.md) | 统一词法、版面树与单元、从起点学习的约定、结构置信度、命令与运行记录、副本中的运行 |
| [handoff.md](handoff.md) | 接续时机、摘要内容和预算、用户修改与多 session 协作 |
| [semi-auto.md](semi-auto.md) | 方案范围、确认、增补、模式切换、S1 和 S2 |
| [requirements-ledger.md](requirements-ledger.md) | 记录、字段、修订、MCP 工具和保留期限 |
| [product-agent-integration.md](product-agent-integration.md) | hook、MCP、Agent 与平台差异、安装和故障处理 |
| [product-architecture.md](product-architecture.md) | 进程、模块、快照、观察与编辑核对、检查执行、存储和性能 |
| [product-evaluation.md](product-evaluation.md) | 当前测试维护范围，以及算法的正确性、效果、普适性和成本评估口径 |
| [iteration-method.md](iteration-method.md) | 助手自主准备、评估、采用和汇报的产品进化流程；问题池、已定结论、批次记录和 Prompt 在 `iteration/` 目录 |

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
| 实现判断 | 为完成需求而作出的调用衔接、处理方式和内部结构安排 | [任务和需求](requirements.md) |
| 实现位置、归属 | 需求由哪些代码或文件实现，以及改动与需求的联系 | [任务和需求](requirements.md) |
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
| 对外新增项、需求对应关系 | 外部看得见的新增内容；它的名字或取值是否能在原话或方案中找到，或是否已登记为需求的实现位置 | [过度设计判定](over-design.md) |
| 撤回试验 | 把变更簇恢复成起点内容后，运行验收依据中规定的用例和命令 | [过度设计判定](over-design.md) |
| 可信度 | 按本项目的反馈记录，同类建议成立的概率；只用于给建议排序 | [算法核心](algorithm.md) |
| 测试用例、验证 | 一条可单独运行的测试，以及运行测试、构建、检查等动作 | [过度测试判定](over-testing.md) |
| 验证输入 | 决定一次验证结果的内容、目录、参数、环境变量，以及工具的实际路径、内容哈希或版本输出；按这些信息的哈希比较 | [过度测试判定](over-testing.md) |
| 检查问题 | 带稳定标识、事实、建议和处理状态的一处检查结果 | [算法核心](algorithm.md) |
| 检查结论 | 放行、提示或拦截；处理状态、依据范围与结论分别记录 | [算法核心](algorithm.md) |
| 工作模式、拦截强度 | 全自动或半自动；拦截一次或只提示 | [产品需求](product.md) |
| 有效方案、待确认增补 | 已确认并仍有效的范围，以及新提出的范围变化 | [半自动模式](semi-auto.md) |
| 接续摘要 | 根据账本和当前工作区生成的任务摘要 | [任务接续](handoff.md) |
