# 文档说明

本目录的文档描述 TaskBelay 这个产品。本文说明文档分哪几层、怎样命名、每份文档负责什么，以及所有文档共用的术语。

## 文档的层次

文档分三层，每一层依据上一层来写：

1. **产品需求**：[product.md](product.md) 是源头，写用户要的功能。
2. **算法**：[requirements.md](requirements.md)、[over-design.md](over-design.md)、[over-testing.md](over-testing.md)、[handoff.md](handoff.md)、[semi-auto.md](semi-auto.md) 写怎样做到这些功能，是 TaskBelay 的核心。这几份会随讨论和评估反复修改。
3. **衍生文档**：[requirements-ledger.md](requirements-ledger.md)、[product-agent-integration.md](product-agent-integration.md)、[product-architecture.md](product-architecture.md)、[product-evaluation.md](product-evaluation.md) 在算法确定之后才能写：算法要用什么数据，账本就记什么；算法要在什么时机介入，就接哪些 hook；架构把算法实现出来；评估衡量算法的效果。

前两层是元文档，决定其他文档写什么；第三层由元文档推出。上一层改了，下一层跟着改。

## 文档怎样命名

元文档按内容直接命名，比如 `over-design.md`。衍生文档的名字是“所依据的元文档的名字-自己的名字”：任务账本记录的是 [requirements.md](requirements.md) 定义的任务、需求和实现位置，所以叫 `requirements-ledger.md`；Agent 接入、系统架构、算法评估依据的是整个产品，所以名字以 `product-` 开头。

## 每份文档负责什么

| 文档 | 负责 |
| --- | --- |
| [product.md](product.md) | 要解决的问题、目标用户、两种工作模式、功能、支持范围、默认行为、成功标准 |
| [requirements.md](requirements.md) | 任务的开始、结束和范围；需求和原话引用；实现位置；改动怎样归属到需求；凭用户原话放行 |
| [over-design.md](over-design.md) | 过度设计的判定规则、参数、事前提示、给 Agent 的说明、会在哪里判错、例子 |
| [over-testing.md](over-testing.md) | 过度测试的判定规则（测试代码和验证过程）、参数、事前提示、给 Agent 的说明、会在哪里判错、例子 |
| [handoff.md](handoff.md) | 什么时候给接续摘要、摘要写什么、摘要的长度上限、Agent 要记下什么、怎样判断新消息是不是另一件事、两个 session 同时在一个工作区时怎样处理 |
| [semi-auto.md](semi-auto.md) | 半自动模式的流程：方案怎样写、写什么、怎样提交和确认，待确认期间怎样保持工作区不变，确认之后方案起什么作用；这个模式下的规则、参数、事前提示、例子 |
| [requirements-ledger.md](requirements-ledger.md) | 账本里有哪些记录和字段、谁在什么时候写、保留多久；Agent 读写账本的 MCP 工具 |
| [product-agent-integration.md](product-agent-integration.md) | 接了 Claude Code 和 Codex 的哪些 hook、每个 hook 做什么、拦截和提示怎样生效、两个 Agent 和两个平台的差异、安装和卸载、TaskBelay 出错时怎样处理 |
| [product-architecture.md](product-architecture.md) | 进程怎样组织、命令、项目结构和扩展方式、内部流程、快照和代码分析和运行测试的实现、存储、性能预算、依赖、平台和安装 |
| [product-evaluation.md](product-evaluation.md) | 怎样衡量算法的效果、一次算法修改怎样决定采不采用、参数怎样调整、两个平台上怎样验证 |

每件事只在负责它的那份文档里写清楚，其他文档用到时，写一句话加链接。

## 术语

所有文档按下表用词，一个词只指一样东西。“详见”一列是这个词的完整定义所在的文档。

| 术语 | 含义 | 详见 |
| --- | --- | --- |
| Agent | Claude Code 或 Codex | |
| session | Agent 的一次会话。Codex 里叫 thread，本项目统一叫 session | |
| 一轮 | 从用户发出一条消息，到 Agent 停下来等用户 | |
| 工作区 | 一个 git 工作区的根目录。同一个仓库的不同 worktree 是不同的工作区 | [requirements.md](requirements.md) |
| 任务 | 用户在一个工作区里要 Agent 完成的一件事，可以跨多个 session 和 Agent | [requirements.md](requirements.md) |
| 用户消息 | 用户发给 Agent 的消息原文，由 TaskBelay 记录 | [requirements.md](requirements.md) |
| 原话引用 | 从本任务的用户消息里原样摘出的一段文字 | [requirements.md](requirements.md) |
| 需求 | 用户要的一项结果，编号 R1、R2…… | [requirements.md](requirements.md) |
| 原话需求、推断需求 | 有原话引用的需求是原话需求；没有的是推断需求 | [requirements.md](requirements.md) |
| 实现位置 | Agent 为每条需求列出的位置：Go 声明、非 Go 文件、测试用例、对外入口、依赖 | [requirements.md](requirements.md) |
| 归属 | 一处改动为哪条需求服务 | [requirements.md](requirements.md) |
| 需求代码 | 归属一条需求的改动声明，加上它的实现位置里列出的 Go 声明 | [requirements.md](requirements.md) |
| 工作区状态 | 代表工作区里文件内容的一个标识，包括没提交的改动和新文件，不包括被 `.gitignore` 忽略的文件。内容相同，状态就相同 | 计算方法见 [product-architecture.md](product-architecture.md) |
| 起点快照 | 任务的起点消息到达时的工作区状态 | [requirements.md](requirements.md) |
| diff | 当前工作区和起点快照之间的差别，包括没提交的改动 | [requirements.md](requirements.md) |
| 声明 | Go 的顶层声明：函数、方法、类型、变量、常量 | |
| 改动声明 | diff 里新增或修改的声明。只改了空白和格式的不算 | [requirements.md](requirements.md) |
| 对外入口 | 用户或其他程序直接接触的东西：命令行参数、子命令、环境变量、HTTP 路由、配置字段 | [over-design.md](over-design.md) |
| 测试 | 写出来的测试代码 | |
| 测试用例 | 一条可以单独运行的测试 | [over-testing.md](over-testing.md) |
| 验证 | 运行测试、构建、静态检查这类命令的动作 | |
| 验证命令 | 用来验证的命令，例如 `go test`、`go build`、`golangci-lint run` | [over-testing.md](over-testing.md) |
| 检查 | TaskBelay 按算法文档的规则检查改动、测试和命令 | |
| 命中 | 一处改动、一条用例或一条命令符合某条规则 | |
| 结论 | 一次检查的结果：放行（没有命中）、提示（只有提示档的命中）、拦截（至少有一处拦截档的命中）。按项目惯例或用户原话放行的地方不算命中 | |
| 拦截强度 | 用户的一项设置：拦截一次（默认），或者只提示 | [product.md](product.md) |
| 工作模式 | 用户的一项设置：全自动（默认）或半自动 | [product.md](product.md) |
| 全自动模式 | 用户给出需求后，Agent 直接开发 | [product.md](product.md) |
| 半自动模式 | Agent 先交出方案，用户确认后才开始开发 | [semi-auto.md](semi-auto.md) |
| 方案 | 半自动模式下 Agent 提交的技术方案，带“涉及的改动”一节，按版本编号 v1、v2…… | [semi-auto.md](semi-auto.md) |
| 已确认方案 | 用户用原话确认过的最新一版方案 | [semi-auto.md](semi-auto.md) |
| 待确认期间 | 从任务开始，或者从提交新一版方案，到这一版被确认之前的这段时间 | [semi-auto.md](semi-auto.md) |
| 任务账本 | TaskBelay 对一个任务的全部记录 | [requirements-ledger.md](requirements-ledger.md) |
| 接续摘要 | 新 session 开始时 TaskBelay 交给 Agent 的任务摘要 | [handoff.md](handoff.md) |
| 事前提示 | session 开始时 TaskBelay 告诉 Agent 的做事规矩 | 内容见两份判定文档和 [semi-auto.md](semi-auto.md) |
