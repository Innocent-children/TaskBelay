<p align="center">
  <img src="https://raw.githubusercontent.com/Innocent-children/taskbelay/main/packages/webui/src/assets/taskbelay-app-icon-light.svg" width="112" height="112" alt="TaskBelay" />
</p>

# taskbelay-deepseek

[中文](https://github.com/Innocent-children/taskbelay/blob/main/packages/deepseek/README.md) |
[English](https://github.com/Innocent-children/taskbelay/blob/main/docs/DEEPSEEK_en.md)

`taskbelay-deepseek` 让 DeepSeek Harness（DSH）运行一个持久 Core Task。普通开发请求先只读评估；
选择 TaskBelay 后，默认在当前目录新建任务分支，也可明确选择当前分支或独立工作树。全本地选择
在原会话继续；Core 只读观察 Git，DSH 负责获授权的分支、文件和命令操作。

## 支持范围

| 项目 | 当前支持 |
| --- | --- |
| Package | [`taskbelay-deepseek`](https://www.npmjs.com/package/taskbelay-deepseek) |
| 稳定 Platform | macOS arm64 |
| 当前源码 Platform | macOS arm64（`darwin-arm64`）；Windows 10/11 桌面 x64（`win32-x64`） |
| Node.js | `>=24` |
| DSH | `>=0.1.2-rc.1` |
| Releases | [GitHub Releases](https://github.com/Innocent-children/taskbelay/releases) |

稳定支持以[支持矩阵](../../docs/SUPPORT-MATRIX.md)为准。`main` 中存在的能力不一定已经进入 npm
`@latest`。Windows Server、32 位 Windows、Windows ARM64 与 Intel Mac 不在当前源码支持范围；
runtime selector 会拒绝除 `darwin-arm64` 和 `win32-x64` 之外的运行时对。

## 开发前确认方案

先查看需求与验收条件、设计及影响，再讨论包含任务、预计文件和验证安排的完整计划。明确认可后才开始实现；选择 TaskBelay 和工作树参数不代替方案确认。等待答复时保留在任务拆分阶段。修改方案或扩大文件范围后，需要重新确认修订计划；恢复同一待确认计划无需重新保存。优先列具体文件，目录范围需说明理由。

## 安装

DSH 是前置 Host。推荐使用统一 lifecycle 入口，并选择真实 Profile；默认是 `web`：

```bash
npm install -g taskbelay@latest
taskbelay
```

`taskbelay-deepseek` 没有独立 `bin`，不会安装同名 CLI。诊断恢复时，可以通过 npm tarball 和 DSH
profile lifecycle 执行原生安装：

```bash
npm install -g @deepseek-ai/dsh@latest
PROFILE=web
TARBALL="$(npm pack taskbelay-deepseek@latest --silent)"
dsh plugin --profile "$PROFILE" add "$PWD/$TARBALL"
rm -f "$PWD/$TARBALL"
dsh --profile "$PROFILE" --dump-config
```

Windows PowerShell 使用：

```powershell
npm install -g @deepseek-ai/dsh@latest
$ProfileName = 'web'
$Tarball = (npm pack taskbelay-deepseek@latest --silent | Select-Object -Last 1).Trim()
$TarballPath = (Resolve-Path -LiteralPath $Tarball).Path
dsh plugin --profile $ProfileName add $TarballPath
Remove-Item -LiteralPath $TarballPath
dsh --profile $ProfileName --dump-config
```

安装后按 DSH profile lifecycle 重启该 Profile。完整命令和更新顺序见
[命令参考](../../docs/COMMANDS.md#deepseek-harness)。

默认 Task 数据目录在 macOS 为 `$HOME/.taskbelay/data`，在 Windows 为
`%LOCALAPPDATA%\taskbelay\data`；显式 `TASKBELAY_DATA_DIR` 必须已经存在且通过 canonical、非链接目录检查。

## 启动一个 Task

仓库调查遵循当前用户指令和适用的 `AGENTS.md`。需要项目索引时，DeepSeek 在当前 Workspace Root
和权限内只读检查索引、候选项目说明及相关代码与配置，形成完整候选范围，再逐仓确认并准备工作树；
Task 创建后范围固定。选择代码检索工具时，这些指令优先于 `host_preferences.deepseek.codebase_memory`
默认偏好。

先把开发请求正常发给 DSH。Adapter 只读检查候选实现、调用关系、测试、配置和 Git 状态，返回
`small|standard|large|uncertain`、已经找到的影响面、未知项和建议；这一轮不调用 Core、不修改 Git、
不运行测试，也不创建 Task。即使第一条消息已经包含 `/taskbelay`，新请求也不能跳过评估和确认。

选择 TaskBelay 后，默认在当前目录从当前 HEAD 新建任务分支，或明确选择当前分支、独立工作树。
本地模式确认新目标分支和初始修改选择，保留文件、暂存、ignored 配置及已有依赖。确认消息使用
Adapter 显示的精确形式，例如：

```text
/taskbelay confirm-workspace
repository=primary;mode=new_branch;source=local;carry=true;remote=;base=main;target=feature/payment-callback-signature
```

本地分支操作前由 Core 只读检查目录占用。全部选择本地模式时，WorkspaceCoordinator 返回 `ready`
和完整 `open_task`，原会话检查 Core 后直接创建并继续任务，无需 relaunch/consume。`current_branch`
保留当前分支，其 base 和 target 相同。有未提交内容但未接受时停止，不自动清理或忽略其他工作。

独立工作树额外选择本地或远端来源及起始分支，固定 commit 后在 sibling 目录创建工作树，并按选择
复制初始内容。DSH 的 Workspace Root 在启动时固定，独立工作树返回 `{command,arguments,cwd}`
重启描述；混合模式从包含原目录和工作树的共同父目录启动，新会话验证全部目录权限后 consume。
任一仓库准备失败都不创建部分 Task，本地分支和修改保留供检查。新 Task 保存原请求、范围、验收和
method profile，不在分析前冻结最终验证预算。完整规则见[工作位置与分支](../../docs/WORKTREE-SOURCES.md)。

## 恢复已有 Task

回到 Task 原来绑定的同一工作目录实例，并在当前直接用户消息中再次使用 `/taskbelay`。Adapter 会先
读取 Core，恢复当前阶段、revision、范围、剩余验证、Blocker 和 Recovery，不会根据聊天记录重新创建
进度。原 worktree 丢失或被替换时进入 `WORKSPACE_UNAVAILABLE`；同路径重建目录或同名 branch 不能冒充
原实例。此时只能恢复原实例，或明确 abandon Task。

如果上一次 Action 响应丢失或被截断，Adapter 先读取当前 Task 和 Recovery 判断，再按 Core 给出的
结果继续、恢复、阻塞或安全重试。它不会自行重复原提交。

同一失败、同一测试结果，或相同修改路径与失败组成的测试循环连续出现三次时，Core 会保存第三次
结果并暂停 Task。Adapter 不会自动解除；用户明确选择换方案或再试一次后，才解除 blocker，并从
Core 保存的原目标阶段继续。下一次仍然完全重复时会再次暂停。

## 验证投入和修改后复核

DeepSeek 在读完需求、设计、影响面和现有测试后规划验证，记录必要检查、理由、初始命令额度、完整套件预期及测试代码预期。优先选择与改动最接近的定向检查。

额度不足时，先记录具体的新影响、风险、失败或缺口，只增加当前需要的部分；已有检查的补做或重跑也可以补充额度，增加本身不产生通过结果。每次完整套件都需重新说明必要性，不能仅凭剩余额度或上次已执行来决定。

测试代码应保护稳定行为、公开接口、重要失败路径或真实回归。修改后复核只覆盖 diff、因果影响与验收所需内容；修复后只做相关复查。显式审查保持只读，交付问题后等待单独修复授权；普通修改后复核不报告无关历史问题。

提交字段见[命令参考](../../docs/COMMANDS.md)。

## 范围外文件先询问

Adapter 在 DSH `tools/pre-execute` 中检查 `write`、`edit` 和变更型 `str_replace_editor`。当前 direct
user turn 使用 `/taskbelay` 时，这些工具在写入前把目标文件交给 packaged Core。Core 使用当前 Task
Plan 全部 WorkItem 的 `ExpectedPaths` 合集；多仓库路径带 repository key。B、C 等仓库已在 Task
Repository Scope、位于 Workspace Root 且文件属于计划范围时，不因为当前目录位于 A 而询问。

计划外文件会在工具执行前暂停 Task。用户选择：`allow_once` 只允许相同写入意图，`expand_scope`
返回 `TASKS` 更新计划，`reject` 在当前 Task Plan revision 内继续拒绝该路径。选择与原因由 Core
保存；Core 根据 frozen base、commit、index、worktree 和 untracked 内容重新计算当前 Task surface。

该 gate 不解析 Bash、外部进程或其他工具路径；这些写入可能只能在 Core 最终检查时发现。gate
不可用时，支持的结构化写入保守停止。

## 查看状态

查看统一 lifecycle 与 DSH Profile 状态：

```bash
taskbelay status --host deepseek --profile web
dsh --profile web --dump-config
```

查看 Task、当前阶段、时间线、Recovery 和 Blocker：

```bash
taskbelay webui start
```

WebUI 只监听本机 loopback。完整用法见 [WebUI](../../docs/WEBUI.md)。

## 移除

推荐从统一入口选择 DeepSeek 卸载。Host 原生移除为：

```bash
PROFILE=web
dsh plugin --profile "$PROFILE" remove taskbelay-deepseek
dsh --profile "$PROFILE" --dump-config
```

对每个安装过 TaskBelay 的 Profile 分别执行。移除 package 或 bundle contribution 会保留 Task 数据、
目标仓库和 Codex 状态。重新安装兼容 package 并重启 Profile 后可以继续已有 Task。

彻底清理数据属于独立的 `taskbelay factory-reset` 流程，需要当前计划给出的强确认。

## DeepSeek 权限与边界

- DSH 启动时的 canonical Workspace Root 是权限边界；仓库和 symlink 解析结果必须位于其中；
- TaskBelay 保持当前 Workspace Root 权限边界，索引结果不改变已创建 Task 的 Scope；需要 sibling worktree 时重启到
  Coordinator 返回的新 Root；
- Core 只读观察 Git，不执行 fetch、branch、worktree、commit、push、merge、rebase、tag 或 publish；
- DeepSeek 负责用户确认后的 fetch、branch、worktree、文件修改和命令执行；Host gate 检查列出的
  结构化工具，Core 计算当前 Task surface，但不会拦截每一次操作；
- `/taskbelay` 不绕过当前 Action、Workspace 权限、Git 写入授权或发布确认。

`DONE` 和 `CANCELLED` 只结束 Core Task并释放 claim，不会 commit、push、创建 PR、handoff 或删除
worktree/branch。终态会显示 source/base/frozen commit、task branch/HEAD、路径、clean 状态、当前改动和
验证结果。本地模式保留目录和分支，不支持工作区迁移或辅助清理；独立 worktree 删除与 branch 删除需要两次独立授权；active、dirty、未推送或状态不确定的资源不
自动清理。

DeepSeek 的清理仍由 WorkspaceCoordinator 执行。它先在不删除资源的 `prepare_cleanup` 操作中核对
receipt、终态 Task 和同 Git group 的源 checkout，并返回从该源 checkout 重启 DSH 的 descriptor，避免
原地删除当前 Workspace Root。重启后，第一次独立确认只删除 receipt 拥有、Core 已终态、clean 且
terminal HEAD 已精确推送的 worktree，并保留 task branch。第二次独立确认使用非 force 的
`git branch -d` 删除 branch；未合并、仍被 worktree 使用、HEAD/remote 不一致或 Core 读取失败都会
保留资源。源 checkout 路径只在调用中使用，不写入 receipt。

## 高级多仓库

当前源码支持一个主仓库和最多七个显式附加仓库。逐仓保存工作位置、分支和初始修改选择；全部
仓库准备并验证成功后才创建一个 Core Task。全本地模式沿用原 Workspace Root，独立或混合模式使用
返回的重启描述。所有目录和 symlink 解析结果必须位于实际会话授权的 Root 内。失败时不创建部分
Core claim，Scope 创建后固定，不自动扩大仓库范围。

使用前请阅读[项目状态](../../docs/PROJECT-STATUS.md)确认多仓库属于稳定还是源码范围。精确
Repository Scope、路径格式和协议规则见[架构](../../docs/ARCHITECTURE.md)与
[命令参考](../../docs/COMMANDS.md)。

## 相关文档

- [产品定义](../../docs/PRODUCT.md)
- [中断后继续的演示](../../docs/DEMO.md)
- [命令参考](../../docs/COMMANDS.md)
- [架构](../../docs/ARCHITECTURE.md)
- [项目状态](../../docs/PROJECT-STATUS.md)
- [WebUI](../../docs/WEBUI.md)

## 桌面任务入口

正式 `taskbelay` npm 包提供 macOS arm64 与 Windows 10/11 x64 桌面宠物应用，运行时由已配置的 Adapter 提供 Core。宠物读取 Task 保存状态并打开对应 WebUI，不代表 Host 实时活动或完成百分比。统一入口的安装、升级、修复和重装会更新用户目录中的应用副本，并保留设置与形象素材。安装、操作、更新和形象使用见[桌面宠物指南](../../docs/DESKTOP-PETS.md)。

## 完成与恢复

进入测试前须完成当前计划全部工作项。交付时逐条关联验收条件、对应的已完成工作和当前通过的检查，开发者理解确认单独进行。提交结果不确定时，Adapter 先读取 Core 保存的操作，再恢复或重试。集成细节见[命令参考](../../docs/COMMANDS.md)。

## 文件漏报

Adapter 报告漏报路径，并按 Core 允许的范围纠正一次。工作树和历史异常继续使用对应恢复规则。本地来源的任务分支由辅助清理流程保留，供用户单独检查。详见[文件收集与提交](../../docs/ARTIFACTS.md)。

## Skill 交互参考

Codex 与 DeepSeek 的 Core 交互说明和完整示例统一维护于 `skills/taskbelay/core/`，由构建脚本生成各包内的引用文件。各 Host 的授权、工作树准备和工具调用分别说明；实际执行使用当前 Action、已安装接口和真实用户决定。节点提交、返回处理、阻塞恢复与验证规则使用相同内容，并对两边生成的示例运行同一套 Core 校验。

[DeepSeek Skill](skills/taskbelay/SKILL.md)

DeepSeek Skill 随包提供 `scripts/artifacts.mjs`，以 `node <实际 Skill 目录>/scripts/artifacts.mjs collect` 或 `prepare` 调用同一套 Core 只读文件准备命令。输入与返回结构与本文相同，`host` 使用 `deepseek`；脚本复用 Adapter 的运行时和数据目录解析，不创建存储。通过实际 DSH Skill 的 `resourceBase` 取得脚本路径。`--help` 不读取 stdin 或解析运行时。该脚本不是独立的 `taskbelay-deepseek` CLI，也不增加 `workspace_coordinator` 操作。

## 持续修订与任务分支改名

同一 Task 完整保留多轮需求、设计和计划修订的全部已保存基线引用，不归档、无配置性条数上限，
仍受资源与修订号表示范围约束。响应给出有界首页与总数，可分页读取全部引用，不恢复未保存的旧正文。活动 Task 需要改分支名称时，先由 Core 准备一个仓库的改名，
再让获授权的 Host 执行一次非 force 的 `git branch -m`，最后由 Core 核对并确认。工作树、HEAD、
index、内容和其他仓库必须不变；改名前可以取消准备，已经改名但未确认时先回读，不重复执行。

后续搬迁和终态清理使用 Core 已确认的有效分支，创建时分支仍作为来源保留。DeepSeek 清理直接回读 Core。
精确条件和恢复步骤见[工作树来源与恢复](../../docs/WORKTREE-SOURCES.md)。
