# 文档导航

本目录分别维护产品设计、迭代流程、评估方法、两类索引和历史记录。每项规则只在负责它的文档中定义，其余文档引用该定义。开发约束见项目根目录的 [AGENTS.md](../AGENTS.md)。

## 迭代与评估

| 文档 | 职责 |
| --- | --- |
| [迭代流程](evolution/README.md) | 唯一流程定义：任务准备、隔离开发、独立复盘、问题形成、有限修复、采用与继续；授权、停止和记录规则 |
| [评估与验证方法](evolution/evaluation.md) | 证据、误报漏报、必要行为、成本及版本或开关对照的方法 |
| [业务样本池](evolution/samples.md) | 脱敏真实业务需求索引，沿用已有样本编号，记录阶段与关联结论 |
| [产品问题池](evolution/backlog.md) | TaskBelay 调查、修复和验证问题的状态、关联记录及下一步 |
| `evolution/records/` | 原编号迭代记录，以及关联的选题资料和业务阶段摘要；保存详细契约、证据与结果 |

## 产品与设计

产品设计维持现有位置，描述已采用的行为；候选变化随候选一起复盘。

| 文档 | 职责 |
| --- | --- |
| [product.md](product.md) | 目标用户、保持产品简单的原则、功能、模式、默认行为和成功标准 |
| [requirements.md](requirements.md) | 任务与需求生命周期、累计 diff、用户修改、位置与归属 |
| [algorithm.md](algorithm.md) | 共享算法、分析信号、反馈选择、拦截条件、校准与复杂度 |
| [over-design.md](over-design.md) | 过度设计的判断依据与撤回试验 |
| [over-testing.md](over-testing.md) | 测试约简、验证输入与重复验证判断 |
| [languages.md](languages.md) | 统一词法、版面树、仓库统计、命令记录与副本运行 |
| [handoff.md](handoff.md) | 接续时机、摘要、用户修改与多 session 协作 |
| [semi-auto.md](semi-auto.md) | 方案范围、确认、增补、模式切换、S1 和 S2 |
| [requirements-ledger.md](requirements-ledger.md) | 账本记录、字段、修订、MCP 工具和保留期限 |
| [product-agent-integration.md](product-agent-integration.md) | 安装、hook、MCP、平台差异与故障处理 |
| [product-architecture.md](product-architecture.md) | 进程、模块、快照、观察与编辑核对、检查执行、存储和调试记录 |

## 共同术语

| 术语 | 含义 | 定义 |
| --- | --- | --- |
| Agent | Claude Code 或 Codex | |
| session | Agent 的一次会话，Codex 的 thread 在本文也称 session | |
| 一轮（Agent 轮次） | 用户消息到达至 Agent 停下等待用户 | |
| 真实需求池 | 来自已授权业务历史、可从实现前基线重新开发的任务样本 | [迭代流程](evolution/README.md) |
| 改进问题池 | 从重演或日常使用证据形成的 TaskBelay 调查、修复和验证问题 | [迭代流程](evolution/README.md) |
| 迭代 | 一次重演、调查或候选改进，从定界至独立复盘、决策和收尾，可包含多个新 session | [迭代流程](evolution/README.md) |
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
