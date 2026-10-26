# TaskBelay

TaskBelay 是一个在本机运行的 Go 命令行工具，通过 hook 和 MCP 接入 Claude Code 与 Codex。它把 Agent 的改动与当前需求对应起来，帮助减少多余的功能、抽象、测试和重复验证，并保存跨 Agent、跨会话继续工作所需的上下文。

你照常提需求、编辑代码、提交和切换分支。TaskBelay 记录开发过程，向 Agent 提供有依据的检查反馈；你手动修改代码后，Agent 从当前文件继续工作。

## 能做什么

- **约束改动范围**：围绕任务起点查看累计改动，检查与需求缺少联系的修改、没有说明用途的入口、未使用的代码和不必要的抽象。
- **减少无效验证**：检查测试代码与验证过程，提示值得合并的用例、过大的验证范围和满足条件的重复验证。
- **接续任务**：保存需求、决定、进度和检查结果，帮助 Agent 在会话重开、上下文压缩、任务暂停恢复或切换 Agent 后继续工作。
- **保留正常开发习惯**：提交不会结束任务；用户手改、文件移动和分支变化会作为新的工作区事实记录，后续工作以当前内容为准。

例如，你要求“给 export 命令增加 CSV 导出”，TaskBelay 会帮助 Agent 核对新增代码是否服务于这个需求，并提示缺少用途依据的额外选项。涉及工程取舍的建议仍由 Agent 结合需求和项目惯例复核。

## 支持范围

| 项目 | 范围 |
| --- | --- |
| Agent | Claude Code、Codex |
| 系统与架构 | macOS、Linux、Windows；amd64、arm64。WSL 按 Linux 安装和运行 |
| 工作区 | Git 仓库及其 worktree，各工作区分别管理任务 |
| 文件与代码 | 所有文件支持文件级记录与检查；内置语法和 tags 查询的语言支持代码级分析 |
| 测试分析 | Go、Python、JavaScript、TypeScript、Java、Rust 的已支持框架；实际能力取决于框架和本机工具，见[语言支持](docs/languages.md) |

TaskBelay 没有常驻后台服务。hook 在 Agent 事件发生时启动短进程，MCP 进程随 Agent 会话运行，二者通过本机 SQLite 账本共享记录。动态分析使用项目自己的测试工具。

## 快速开始

### 1. 准备环境

- Git 2.5 或更高版本。
- Go 1.26 或更高版本，用于下面的源码安装。
- 已安装要接入的 Agent，终端中能够运行 `claude` 或 `codex`。Claude Code 需要 2.1.139 或更高版本。

### 2. 安装程序

以下示例从 `new` 分支安装 Go 实现。已有本分支源码时，直接进入项目根目录执行安装脚本。

```sh
git clone --branch new https://github.com/Innocent-children/TaskBelay.git
cd TaskBelay
```

macOS、Linux 和 WSL：

```sh
bash install.sh --source
```

Windows PowerShell：

```powershell
& .\install.ps1 -Source
```

安装无需管理员权限。macOS、Linux 的程序位于 `~/.local/bin/taskbelay`，Windows 的程序位于 `%LOCALAPPDATA%\Programs\TaskBelay\taskbelay.exe`。安装脚本配置用户 PATH；重新打开终端后运行：

```sh
taskbelay --version
taskbelay
```

`taskbelay` 打开终端管理菜单。上下方向键选择，右方向键或回车进入，左方向键返回，空格勾选 Agent，Ctrl+C 退出。菜单提供 Agent 接入、设置、状态查看、检查、更新和卸载入口。

安装脚本省略 `--source` 或 `-Source` 时会下载发布包。使用这种方式前，所选 Release 需要提供对应系统和架构的 Go 程序及 `SHA256SUMS` 校验清单；源码安装不依赖这些发布资产。发布安装方式见[安装和卸载](docs/product-agent-integration.md#安装和卸载)。

### 3. 接入 Agent

安装脚本只安装 TaskBelay 程序。接着在菜单中选择要接入的 Agent，或按需执行以下命令；同时使用两个 Agent 时可分别执行：

```sh
taskbelay install claude
taskbelay install codex
```

接入命令会配置 hook 并注册 MCP。安装使用用户级配置，对本机该 Agent 的 Git 工作区生效，用户原有的其他配置会保留。重复执行安装命令可更新或修复 TaskBelay 自己的接入配置。

- **Claude Code**：新建会话以加载接入配置。
- **Codex**：重新打开会话，再用 `/hooks` 确认信任 TaskBelay 的 hook；完成信任后 hook 才会运行。

可用下面的命令查看程序位置、Agent 命令、hook 和 MCP 注册情况：

```sh
taskbelay doctor
```

`doctor` 检查本机配置，实际 hook 是否被信任和执行，还需在 Agent 中确认。

### 4. 开始开发

在目标 Git 工作区中打开 Agent，像平时一样提出需求。Agent 通过 MCP 保存需求和进度，TaskBelay 在消息到达、编辑、运行命令和准备交付等时机记录与检查。

你可以中途手改代码、分多次提交，或让 Agent 暂停当前任务去处理另一件事。恢复工作时，Agent 会拿到任务记录和当前工作区的变化。查看任务时，在对应工作区运行：

```sh
taskbelay status
```

## 工作模式与设置

| 设置项 | 默认值 | 含义 |
| --- | --- | --- |
| `workflow` | `auto` | 全自动开发；`semi` 表示先确认方案，再按方案开发 |
| `enforcement` | `block` | 对满足拦截条件的问题有限拦截；`hint` 表示只提示 |
| `analysis.tests` | `off` | 关闭自动动态测试检查；`on` 表示开启 |
| `debug` | `off` | 关闭详细过程记录；`on` 表示开启 |

默认的交付检查使用静态分析和已有验证记录，不额外启动项目测试。对满足条件的同一问题，每个会话最多拦截一次，每轮合计最多三次；依赖工程判断的建议交给 Agent 复核。TaskBelay 自身发生故障时，Agent 的原操作继续执行，检查未完成的情况会保留在状态和日志中。

设置默认保存到用户范围；`--project` 限定到当前项目，`--task` 限定到当前任务。优先级为当前任务、项目、用户、默认值。项目和任务设置需要在 Git 工作区内执行，任务设置还需要已有当前任务。

```sh
# 当前项目采用先确认方案、再开发的模式
taskbelay config set workflow semi --project

# 当前任务只提示检查问题
taskbelay config set enforcement hint --task

# 查看当前实际生效的值及来源
taskbelay config get workflow
```

## 常用命令

在目标 Git 工作区中使用任务与检查命令。

| 命令 | 用途 |
| --- | --- |
| `taskbelay` 或 `taskbelay menu` | 打开终端管理菜单 |
| `taskbelay status` | 查看当前任务、可恢复任务、接续摘要和最近检查 |
| `taskbelay check` | 主动运行一次静态检查 |
| `taskbelay check --tests` | 请求一次包含动态测试分析的检查，在固定快照的临时副本中运行 |
| `taskbelay doctor` | 查看本机程序和 Agent 接入状态 |
| `taskbelay --help` | 查看完整命令与设置项 |

动态检查需要项目的测试环境和工具齐备；不能完成的部分会单独记录，不算作检查通过。非交互环境中，无参数运行 `taskbelay` 会显示帮助。

## 数据与调试记录

TaskBelay 数据保存在本机：macOS、Linux 为 `~/.taskbelay`，Windows 为 `%USERPROFILE%\.taskbelay`。

| 相对数据目录的路径 | 内容 |
| --- | --- |
| `taskbelay.db` | 任务账本、需求与决定、设置、检查和验证记录 |
| `plans/` | 保存的方案版本 |
| `logs/` | 按天记录的错误日志，保留七天 |
| `debug/<YYYY-MM-DD>/<任务唯一标识>.jsonl` | 开启 debug 后的详细过程记录 |

任务快照及必要的内容证据保存在对应仓库的 Git 对象库中，由 TaskBelay 的引用保留。

需要排查接入、检查判断或任务接续问题时，可以为当前项目开启 debug：

```sh
taskbelay config set debug on --project
taskbelay config get debug
```

记录包含任务事件、MCP 调用、处理阶段、检查依据、结果、错误和耗时，也可能包含需求原文、编辑内容和工具参数。文件按本机日期分组，同一任务跨天时分别写入各日期目录。记录只保存在本机，不自动分析或上传。

关闭后停止记录后续调用，已有文件继续保留：

```sh
taskbelay config set debug off --project
```

debug 文件不会随任务结束或七天错误日志清理而自动删除，复盘完成后可自行清理。记录范围和格式见[调试记录](docs/product-architecture.md#调试记录)。

## 更新与卸载

从源码安装后，更新源码并重新执行对应系统的源码安装命令。使用发布版时，可通过菜单或 `taskbelay update` 下载并校验最新发布程序。程序更新后重新打开 Agent 会话，使 MCP 加载新程序；如果程序路径发生变化，重新执行对应的 `taskbelay install` 命令以更新接入路径。

只移除某个 Agent 的接入：

```sh
taskbelay uninstall claude
taskbelay uninstall codex
```

移除两个 Agent 的接入并卸载由安装脚本管理的程序：

```sh
taskbelay uninstall --all
```

完整卸载需要交互确认。更新和卸载保留任务账本、日志与用户原有配置；卸载只移除 TaskBelay 管理的接入和资源。

## 进一步阅读

完整文档从[文档说明](docs/README.md)进入。

| 文档 | 内容 |
| --- | --- |
| [产品需求](docs/product.md) | 产品目标、使用场景、模式和默认行为 |
| [任务和需求](docs/requirements.md) | 任务生命周期、累计改动、用户修改与需求变更 |
| [过度设计判定](docs/over-design.md)、[过度测试判定](docs/over-testing.md) | 检查依据、反馈和验证规则 |
| [Agent 接入](docs/product-agent-integration.md) | 安装、hook、MCP、平台差异与故障处理 |
| [系统架构](docs/product-architecture.md) | 进程、模块职责、存储和调试记录 |

参与开发前先阅读 `AGENTS.md` 和 `docs/README.md`。开发范围以 `docs/` 定义的需求、行为和支持范围为准；代码的模块职责见 `docs/product-architecture.md`。
