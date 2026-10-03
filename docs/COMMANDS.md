# TaskBelay 命令参考

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

> 普通用户通常只需要安装统一入口、运行 `taskbelay`，并在 Host 中使用对应 selector。其余命令
> 主要用于诊断、恢复和集成开发。

本文件列出当前命令入口。统一 lifecycle 由 `packages/taskbelay/bin/taskbelay.mjs` 和 `lib/cli.mjs` 定义，Codex、Claude 与 ZCode 分别由各自 package 的 bin 定义，DeepSeek 命令由 DSH lifecycle 测试核对。Core 命令来自 `cmd/taskbelay/main.go`，MCP 工具来自 `internal/mcp/`。

公开安装示例使用 npm 的 `latest` dist-tag，以便安装当前最新稳定包；支持矩阵、Release 链接和
安装包验证结果仍使用精确版本号，不应替换为 `latest`。

当前源码的 launcher 和 bundled Core 只接受两个精确运行时对：`darwin-arm64` 与
`win32-x64`。下方 `@latest` 命令仍描述当前 npm 稳定通道；Windows 10/11 桌面 x64 的源码能力
要通过本仓库构建的 package 验证，直到一次明确确认的发布把对应安装包发布到稳定通道。

## 多数用户需要的推荐入口

```bash
npm install -g taskbelay@latest
taskbelay
```

安装后，Codex 使用 `$taskbelay-codex:taskbelay <任务描述>`，DeepSeek Harness 使用
`/taskbelay <任务描述>`，Claude Code 使用 `/taskbelay-claude:taskbelay <任务描述>`。这些是 Host 对话 selector，不是 shell 命令。ZCode 在输入框的 `/` → Skills 中选择 `taskbelay` 后描述任务，见 [ZCode 指南](ZCODE.md)。

## 统一 Adapter 生命周期

`taskbelay` 提供 Host 无关的生命周期和 Control Center 入口：

```bash
npm install -g taskbelay@latest
taskbelay
```

支持的子命令为 `status`、`doctor`、`install`、`upgrade`、`repair`、`reinstall`、`uninstall` 和
`factory-reset`。Host 选择为 `codex|deepseek|claude|zcode|all`；DeepSeek Profile 默认 `web`。普通卸载、升级、
修复和重装保留用户配置与 Task 数据；`factory-reset` 要求绑定当前计划的 token，`--yes` 不能单独
授权数据清理。默认清理在 macOS 移动到用户 Trash，在 Windows 移动到
`%LOCALAPPDATA%\taskbelay\trash` 的可恢复隔离目录；Windows 目标不是系统回收站。永久删除还需
独立确认。
Codex 全局 package 与 receipt、Plugin 注册分别判断；即使注册已缺失，`uninstall` 和
`factory-reset` 仍会卸载已安装的全局 package。
升级留下的 package、Plugin 与 receipt 版本差异不会阻止移除同一归属的 Codex 注册，路径和来源仍须匹配。
若整个 Codex Adapter package 已缺失，统一入口会读取残留 receipt，通过 Codex 原生命令移除对应注册并回读确认；不要求重新安装 Adapter。
Core 缺失时，仅在没有 WebUI runtime receipt 的情况下继续清理；仍有运行记录、记录不可读或停止失败时保留现场。
交互界面读取当前 locale：`zh*` 使用简体中文，其余 locale 统一使用英文；JSON 输出保持语言无关。
文本模式会在安装、升级、修复和重装执行期间逐项显示 Host 动作及已完成的 package、注册、构建产物和就绪检查步骤；`--json` 不输出这些进度行。

| 入口 | 作用 |
| --- | --- |
| `npm install -g taskbelay@latest` | 全局安装公共 `taskbelay` 命令。 |
| `taskbelay` | 打开交互式 lifecycle 菜单。 |
| `taskbelay status\|doctor --host codex\|deepseek\|claude\|zcode\|all` | 只读检查或诊断。 |
| `taskbelay install\|upgrade\|repair\|reinstall --host ... [--profile web] [--version latest] --yes` | 执行普通维护并保留配置与 Task 数据。 |
| `taskbelay install\|repair --host deepseek\|all --adopt ...` | 接管已经存在且身份可验证的 DeepSeek Profile contribution；其他操作和纯 Codex 目标不接受 `--adopt`。 |
| `taskbelay install\|upgrade\|repair\|reinstall ... --confirm-downgrade <token>` | 当目标版本低于已安装版本时，使用当前计划给出的 token 明确确认降级。 |
| `taskbelay uninstall --host ... [--all-known-profiles] --yes` | 移除选定 Adapter并保留配置与 Task 数据；Codex 会先安全停止对应 WebUI，失败时不移除注册或 package。 |
| `taskbelay factory-reset --host all --all-known-profiles` | 生成绑定当前状态的 reset plan/token；`--yes` 不授权清理。 |
| `taskbelay factory-reset ... --confirm-reset <token> [--reinstall]` | 将已确认数据移动到 Trash，可随后全新重装。 |
| `taskbelay factory-reset ... --confirm-explicit-data <absolute-path>` | 确认计划中列出的一个显式 `TASKBELAY_DATA_DIR`；多个目录时可重复传入该参数。 |
| `taskbelay factory-reset ... --permanent --confirm-reset <token> --confirm-permanent <token>` | 永久删除计划中的精确目标；需要 reset token 和独立的永久删除 token。 |
| `taskbelay webui start\|open\|status\|stop` | 从任一已安装 Adapter 选择并校验 Core，管理共享本机 Control Center；`start` 可创建缺失的默认数据目录：macOS 使用 `0700`，Windows 继承用户 profile/LocalAppData ACL。其余命令不创建目录。 |
| `--json` / `--plain` | 分别选择单一 JSON 对象或无 ANSI 的纯文本结果。 |

### 生命周期命令行为

菜单读取安装状态后提供 Adapter 安装、维护、Control Center 和宠物入口；支持输入重试、返回和退出，完成操作后回到菜单。无终端交互环境时，裸 `taskbelay` 显示帮助。`taskbelay <生命周期命令> --help` 显示参数与示例。

| 命令 | 目标版本与重复执行 |
| --- | --- |
| `install` | 默认保留已有 Adapter 版本，缺失项使用 `latest`；已就绪且版本相同则不替换 Adapter。 |
| `upgrade` | 默认选择 `latest`；已是目标版本且就绪则不替换 Adapter。 |
| `repair` | 默认修复当前 Adapter 版本；恢复损坏文件与同版本受管注册，健康 Adapter 无需修复。 |
| `reinstall` | 默认重新安装当前版本，每次都执行替换，保留配置与 Task 数据。 |
| `uninstall` | 已移除的 Adapter 无需再次操作，保留配置与 Task 数据。 |
| `factory-reset` | 完成清理后再次执行为空操作；有实际清理目标时仍须确认当前计划。 |

显式 `--version` 选择目标版本；所有版本替换命令的降级均须 `--confirm-downgrade`，普通 `--yes` 不代替降级确认。本地开发分发包始终使用包内校验过的版本和制品，维护时替换包内内容。

执行前展示操作、当前/目标版本、资源路径和数据处理方式。JSON 模式从不询问；需要确认时返回 `confirmation` 与可复制的 `next_step`。显式数据目录在任何 Adapter 移除前完成确认。清理目录按 canonical 路径、文件系统身份和权限绑定，允许关闭受管服务时移除运行记录；单文件清理还核对大小和修改时间。

安装、升级、修复和重装维护 Adapter；当管理器包内包含当前平台的桌面宠物应用时，还会更新用户目录中的应用副本，保留设置与形象素材。即使 Adapter 已健康且无需替换，应用更新仍会执行。公共入口及其包内应用通过 `npm install -g taskbelay@latest` 更新。

`status` 保留未安装目标，返回 Host 可用性、Adapter/Core 版本及问题；`doctor` 另外列出安装与配置检查，检查失败返回非零退出码。选择全部 Host 时，已有健康 Adapter 的情况下，未安装的可选 Adapter 仅列为未安装，不算故障。Codex 自检失败时仍读取 npm 安装信息以支持修复；DeepSeek 同时检查 Profile contribution、受管记录与实际 Core。未受管的现有 DeepSeek contribution 必须通过 `--adopt` 明确接管。

`doctor` 将已有用户配置交给选定 Core 的 `config validate` 校验；没有可用 Core 时报告无法完成配置语义校验，不按通过处理。配置文件缺失表示采用 Core 默认值。`factory-reset` 初始化配置时只写入 `{}`；普通维护保留已有合法配置。

确认 reset 后，管理器先停止已管理 Adapter 的 WebUI、可识别的 STDIO Core 和桌面宠物，卸载后再检查
是否仍有匹配进程，确认退出后才清理数据。无法确认退出或 Host 自动重连时，操作在清理前失败；关闭
对应 Host 会话后按返回说明重试。显式 `TASKBELAY_DATA_DIR` 与默认目录相同只清理一次，显式路径确认仍必需。
卸载后失败的重试仍会检查先前确认的 Core 位置，直到关联进程退出才允许清理。

失败结果包含 `error.code/message/detail`、`operation_id`、`failed_action`、`completed_actions` 和处理命令；文本模式展示同样的原因与完成步骤。再次执行会重新观察当前安装，不回放旧操作。处理命令在适用时固定本次目标版本；reset 再次生成当前状态的计划。安装成功结果保留 hook 审核/信任及 Profile 重启提示。

生命周期退出码：`0` 成功或无需变更，`1` 检查/执行失败，`2` 参数错误，`3` 等待确认或取消确认，`4` 计划或清理授权不满足，`5` 部分执行或最终检查失败。菜单主动退出返回 `0`。WebUI 参数错误返回 `2`，launcher 在 `--json` 模式下的错误也返回 JSON。

设置 `TASKBELAY_DATA_DIR` 时，公共 launcher 只接受已存在、canonical、非符号链接的绝对目录，任何命令都
不会自动创建显式目录。

默认本机路径按平台固定：

| 路径 | macOS arm64 | Windows 10/11 x64 |
| --- | --- | --- |
| Task 数据 | `$HOME/.taskbelay/data` | `%LOCALAPPDATA%\taskbelay\data` |
| 用户配置 | `$HOME/.taskbelay/config.json` | `%USERPROFILE%\.taskbelay\config.json` |
| 生命周期管理状态 | `$HOME/.taskbelay` | `%LOCALAPPDATA%\taskbelay` |
| 桌面宠物与注册状态 | `$HOME/.taskbelay/pet`, `$HOME/.taskbelay/registrations` | `%LOCALAPPDATA%\taskbelay\pet`, `%LOCALAPPDATA%\taskbelay\registrations` |

PowerShell 中设置显式数据目录的形式为：

```powershell
$env:TASKBELAY_DATA_DIR = 'C:\absolute\existing\taskbelay-data'
taskbelay status --host all
```

下方 Host 原生命令保留为诊断恢复入口。

## 桌面宠物（macOS arm64 与 Windows x64）

安装 `taskbelay@latest` 获取包内 macOS arm64 与 Windows 10/11 x64 应用，并配置至少一个 Codex、DeepSeek、Claude 或 ZCode Adapter 提供 Core；Claude 的安装渠道见 [Host 指南](CLAUDE.md)。`install`、`upgrade`、`repair`、`reinstall` 更新应用副本并保留设置和形象，即使 Adapter 已是目标版本也执行。详见[桌面宠物指南](DESKTOP-PETS.md)。

| 命令 | 行为 |
| --- | --- |
| `taskbelay pet start` | 启动或恢复宠物；核对 Core 与数据目录，必要时启动 WebUI。已有用户目录应用优先于包内应用。 |
| `taskbelay pet stop` | 正常退出宠物，保留 WebUI、Task、设置和形象。 |

仅接受这两种参数形式，输出为纯文本；退出码为成功 `0`、运行失败 `1`、参数错误 `2`。
`pet status` 和 `pet start --json` 不是公开入口。

```bash
taskbelay pet start
taskbelay pet stop
```

菜单提供任务和形象选择、导入、“动画”、“待机活动”、隐藏与退出。任务选择、九类动作的适用范围、触发规则和排查见
[桌面宠物指南](DESKTOP-PETS.md)。更新或移除提供当前 Core 的 Adapter 或统一入口前先停止宠物；停止失败时中止维护。
确认的 factory-reset 会清理 `productRoot/pet`，普通退出和卸载保留用户素材与设置。

## Codex

### 安装

```bash
npm install -g taskbelay-codex@latest
taskbelay-codex setup
taskbelay-codex --version
```

npm 全局安装只把 `taskbelay-codex` launcher 放到 `PATH`。`setup` 是独立步骤，它验证平台、
package、bundled Core 和 Codex 版本，然后注册本地 marketplace、Plugin 与 MCP 配置，并回读
注册结果。`setup` 保留配置路径、文件类型和权限检查，并使用包内 Core 校验已有配置，合法内容不改写。配置缺失时，在 macOS 创建 `$HOME/.taskbelay/config.json`，在 Windows 创建
`%USERPROFILE%\.taskbelay\config.json`，初始内容为 `{}`，默认偏好由 Core 解释；成功后显示配置/receipt 的
实际文件变化和一个下一步。`--version` 同时报告 Host package 与 bundled Core 版本。

在启动 Codex 前，将 `TASKBELAY_DATA_DIR` 设为已存在的规范化绝对目录，MCP、hook 和文件准备命令使用同一数据目录。`taskbelay-codex artifacts <collect|prepare> --help` 返回 JSON 示例、字段说明、输出和下一步，查询时不启动 Core。 Plugin 通过 `env_vars` 只显式转发此变量；启动环境变化在新 Codex 会话中生效。

### 支持的 Codex 命令

| 命令 | 作用 |
| --- | --- |
| `npm install -g taskbelay-codex@latest` | 从 npm 安装 `latest` 指向的 Codex package，并把 `taskbelay-codex` 全局加入 `PATH`。它不会自动注册 Codex Plugin。 |
| `taskbelay-codex setup` | 创建或验证固定用户配置，验证安装内容和 Codex 兼容版本，注册 marketplace、Plugin、MCP 与 packaged hook，并在成功后提示先用 Codex `/hooks` 审核并信任当前 hook。重复执行时会读取并校验现有注册。 |
| `taskbelay-codex setup --json` | 执行与 `setup` 相同的操作，但只输出一行机器可读 JSON，保留 operation、status、changed、receipt_path，并增加 configuration_path、file_changes 与 next_step。 |
| `taskbelay-codex status` | 只读显示当前 package/Core 与注册状态。 |
| `taskbelay-codex status --json` | 只读回读 package、Core、receipt、marketplace 与 Plugin 状态，不创建配置、注册或数据。 |
| `taskbelay-codex --version` | 输出 `taskbelay-codex <package-version> (core <core-version>)`，用于确认实际安装的 package 与 bundled Core 身份。 |
| `taskbelay-codex remove` | 先按 runtime receipt 停止对应 WebUI，再删除由该 package 拥有的 Codex Plugin、marketplace 注册与 receipt。停止失败时不注销；Task data 和目标 Git 仓库保持不变。 |
| `taskbelay-codex remove --json` | 执行与 `remove` 相同的操作，并输出机器可读 JSON；返回的 `next_step` 指向单独的全局 npm 卸载。 |
| `npm uninstall -g taskbelay-codex` | 在完成 `remove` 后卸载全局 npm package。单独运行它不会先清理 Codex 注册。 |
| `taskbelay-codex artifacts <collect\|prepare>` | 从 stdin 读取 closed JSON 对象，由 packaged Core 收集或准备当前 Action 文件，见[文件收集与提交](ARTIFACTS.md)。 |
| `taskbelay-codex artifacts --help` | 列出文件收集准备操作及帮助入口，不读取 stdin 或启动 Core。 |
| `taskbelay-codex artifacts <collect\|prepare> --help` | 返回 JSON 示例、字段来源、输出字段和下一步。帮助不解析安装或数据路径，实际输入由 Core 校验。 |
| `taskbelay-codex mcp` | **内部 Host 命令。** 由 Plugin 的 MCP 配置调用；它设置数据目录和 Codex admission instructions，然后启动 packaged Core 的 `mcp --stdio`。正常用户不应手工启动它。 |
| `taskbelay-codex hook pre-tool-use` | **内部 Host 命令。** Codex packaged hook 通过 `PATH` 中 package-owned launcher 调用它；该命令读取一个 Hook 事件，提取 `apply_patch` 目标并执行写前检查。正常用户不应手工启动它。 |
| `taskbelay-codex host-check pre-file-write` | **内部 Host 命令。** `hook pre-tool-use` 的实现调用它；launcher 定位 package-local Core，并原样转发 stdin/stdout 与精确的 `host-check pre-file-write` 参数。正常用户不应手工启动它。 |
| `taskbelay-codex host-check workspace-available` | **内部 Host 命令。** 原样调用 Core 的只读目录占用检查；由本地分支启动助手使用。 |
| `taskbelay-codex host-launch <operation>` | **内部 Host 命令。** 从 stdin 接收一个 closed JSON 对象，并输出一个 JSON 对象。`operation` 只允许 `inspect|prepare|supersede|supersede-resume|local-provision|status|dispatch-start|dispatch-call|dispatch-recover|dispatch-reconcile|dispatch-result|bootstrap|cli-provision|scope|handoff-start|handoff-result|handoff-status|cleanup-decision|cleanup-worktree|cleanup-branch`；它执行或记录当前用户已经确认的 assessment、provisioning、relaunch、handoff 与 cleanup 步骤，不是通用 Git CLI。 |

工作位置选择：`workspace_mode=new_branch` 为默认值，另可选 `current_branch` 或 `dedicated_worktree`。本地模式要求 `source_type=local`、空 remote、当前分支作为 base、起始 HEAD，`worktree_path=repository_path`；`carry_changes` 表示是否接受原目录的初始修改。两个本地模式保持原会话，不使用需求交接文件；`new_branch` 需要新目标分支，`current_branch` 的 target 等于 base。

`taskbelay-codex host-launch <operation>` 从 stdin 流读取最多 1 MiB 的 UTF-8 JSON 对象，支持分块输入及跨块中文字符。读取失败、非法 UTF-8、重复成员、非法 JSON、数组或 null 均在执行操作前拒绝；错误写入 stderr，成功结果以 JSON 写入 stdout。

Codex 新会话启动使用原会话保存的完整需求交接材料。以下内部操作的输入为 closed JSON 对象：

| 操作 | 输入字段与材料处理 |
| --- | --- |
| `prepare` | 必填 `request`、`assessment`、`user_choice`、`repository_key`、`repository_path`、`workspace_mode`、`source_type`、`carry_changes`、`remote_name`、`base_branch`、`target_branch`、`surface`、`worktree_path`、`handoff_file`；可选 `launch_id`。默认模式 `new_branch`，另有 `current_branch` 和 `dedicated_worktree`。本地模式使用 `current_session`、原目录路径和 `handoff_file=null`；独立工作树的 handoff_file 为仓库外的完整需求 JSON，保存后通过 `handoff_digest` 关联。 |
| `local-provision` | 只接收 `launch_id`、`repository_key`。对 `current_session` 启动记录检查 Core 占用，创建本地新分支或保留当前分支，返回已准备记录及 `workspace_origin`；全部仓库完成后在原会话调用 `scope` 和 Core。 |
| `dispatch-start` | 只接收 `launch_id`、`repository_key`、`project_id`。保存完整 `host_request`；由 `dispatch-call` 登记调用许可后原样交给桌面任务创建。 |
| `cli-provision` | 只接收 `launch_id`、`repository_key`、`additional_worktree_paths`、`source_repository_path`。从同一份保存材料生成 relaunch 参数，调用方原样使用。 |

材料包含目标、关联原始消息的确定要求、术语、范围限制、代码调查、工作要求、未采纳建议、假设、
待确定问题和按顺序保存的原始讨论。格式见 [Codex 发送端交接格式](../packages/codex/plugin/skills/taskbelay/references/task-handoff.md)。

交接中的工作要求仅包含会话专属指示和授权。
全局及仓库 `AGENTS.md` 由目标 Codex 会话正常加载，交接材料不重复其正文或摘要，也不将自动注入的规则块保存为原始需求讨论。
用户在对话中提出的规则修改仍保留为真实需求。
目标无法自动加载的适用规则注明来源路径、适用范围和具体原因，优先引用可读取的源文件；源文件不可读取时仅补充本次需要的规则文本并排除凭据。
可用性尚未核实时记入 `open_questions`，不预防性复制整份文件。

材料从文件读取，不受 stdin 1 MiB 限制。完整结构化 Prompt 超过 24 KiB UTF-8 时使用完整文件路径及读取说明，
不截断材料。两个启动入口都不再接收新写的 `request` 摘要。材料缺失或被修改时，发送操作在记录桌面
dispatch 或创建 CLI 工作树之前失败。任务标题继续使用确定的 launch/repository 标识。

`taskbelay-codex` 不支持其他子命令，也不提供隐式 `help`、`update` 或 `uninstall` 子命令。Host 原生更新到
当前 `latest` 时重新运行全局安装和 `setup`：

```bash
npm install -g taskbelay-codex@latest
taskbelay-codex setup
taskbelay-codex --version
```

保留 Task 数据的卸载顺序是 `taskbelay-codex remove`，然后
`npm uninstall -g taskbelay-codex`。只有在 Codex、DeepSeek、Claude 和 ZCode Adapter 都已移除且不再需要任何
Task 时，才删除共享默认产品目录：macOS 为 `$HOME/.taskbelay`，Windows
为 `%LOCALAPPDATA%\taskbelay`。

### Codex 智能启用与显式 selector

```text
$taskbelay-codex:taskbelay <任务描述>
```

Codex 沿用当前请求和评估下仍有效的明确选择与授权；没有待决定事项或必需输入时直接继续，不为进度说明或技能规则解释增加“确认后继续”的暂停。需要输入时，在同一回复中提出具体问题；开发方式、工作树参数、理解确认、独立操作授权及阻塞处理仍按各自规则执行。

这不是 shell 命令，而是 Codex 用户消息中的精确 Skill selector。边界明确的开发请求也可以由 Host
隐式选择 Skill；裸 `$taskbelay` 和错误 namespace 不是显式 selector。无论隐式还是显式，新请求都先
做只读 assessment，输出改动级别、候选影响面、未知项和建议，然后停止等待用户选择。确认前不调用
Core、不创建 Task/receipt/child，也不写 Git；request、root、HEAD 或 status 变化会使评估失效。

选择 TaskBelay 后，默认在当前目录新建分支；明确选择当前分支或独立工作树时采用对应模式。
本地启动通过 `prepare`、`local-provision`、`scope` 在原会话继续；独立工作树才使用来源选择、
复制、dispatch/bootstrap 或 CLI relaunch。一个目录只允许一个活动 Task；独立并行任务需要不同
目录，或按顺序执行。`ACTIVE_TASK_CONFLICT` 返回后停止创建并处理已有 Task。

明确 resume 是唯一跳过 assessment 的路径，它回到原 worktree instance，不选择替代 branch/worktree。

## DeepSeek Harness

`taskbelay-deepseek` 的 `package.json` 没有 `bin` 字段，因此它不提供名为
`taskbelay-deepseek` 的独立 CLI。安装、检查和移除都由 DSH profile 生命周期完成。

### 安装

先安装 DSH，再在可写目录中把 TaskBelay 安装到一个真实 profile。下面使用 `web`；需要其他
profile 时修改 `PROFILE`，不要把 `<profile>` 原样输入 shell：

```bash
npm install -g @deepseek-ai/dsh@latest
dsh --version
PROFILE=web
TARBALL="$(npm pack taskbelay-deepseek@latest --silent)"
dsh plugin --profile "$PROFILE" add "$PWD/$TARBALL"
rm -f "$PWD/$TARBALL"
dsh --profile "$PROFILE" --dump-config
```

Windows PowerShell 使用同一个 DSH profile lifecycle，但必须把 `npm pack` 结果解析为绝对路径：

```powershell
npm install -g @deepseek-ai/dsh@latest
dsh --version
$ProfileName = 'web'
$Tarball = (npm pack taskbelay-deepseek@latest --silent | Select-Object -Last 1).Trim()
$TarballPath = (Resolve-Path -LiteralPath $Tarball).Path
dsh plugin --profile $ProfileName add $TarballPath
Remove-Item -LiteralPath $TarballPath
dsh --profile $ProfileName --dump-config
```

`npm pack` 下载 `latest` 指向的官方 package，并把 tarball 写入当前目录；命令替换保存实际文件名。
DSH `plugin add` 接收该 tarball 的绝对路径，将 package、bundle layer、Skill、guard 与 MCP child
加入指定 profile。安装后按 DSH 的 profile lifecycle 停止并重启该 profile。

### TaskBelay 相关的 DSH 命令

| 命令 | 作用 |
| --- | --- |
| `dsh --version` | 输出当前 DSH 版本。TaskBelay 的公开支持范围要求 DSH 满足 Support Matrix 中的最低版本。 |
| `TARBALL="$(npm pack taskbelay-deepseek@latest --silent)"` | 从 npm 获取当前 `latest` package，并把生成的 tarball 文件名保存到 shell 变量。 |
| `dsh plugin --profile "$PROFILE" add "$PWD/$TARBALL"` | 把绝对 tarball 路径安装到 `PROFILE` 指定的 DSH profile。DSH lifecycle tests 使用这一命令形态。 |
| `dsh --profile "$PROFILE" --dump-config` | 输出该 profile 的有效配置，可用于确认 `taskbelay-deepseek` bundle contribution 已存在或已移除。它是 DSH 的检查入口，不修改 TaskBelay Task。 |
| `dsh plugin --profile "$PROFILE" remove taskbelay-deepseek` | 从指定 profile 移除 bundle contribution 与 package。Task data、目标 Git 仓库和 Codex 状态保持不变。 |

更新或重新安装时，先停止 profile，再执行 remove、重新获取 `@latest` tarball、add、删除临时
tarball 并重启 profile。对每个安装过 TaskBelay 的 profile 分别执行 remove。不再使用 DSH 时，
可另行执行 `npm uninstall -g @deepseek-ai/dsh`；macOS 的 `$HOME/.dsh` 或 Windows 的
`%USERPROFILE%\.dsh` 中的 profile 数据会保留。

彻底清除 Task 数据时，先移除全部已安装的 Host Adapter，再删除
macOS 的 `$HOME/.taskbelay` 或 Windows 的 `%LOCALAPPDATA%\taskbelay`。
若设置过 `TASKBELAY_DATA_DIR`，还需核对并单独删除该变量对应的绝对目录。删除 `.dsh` 用户目录会
同时删除所有 DSH profile、会话和其他插件。

### DeepSeek 显式 selector

```text
/taskbelay <任务描述>
```

普通新请求先完成零 TaskBelay 调用的只读 assessment。选择 TaskBelay 后默认 `new_branch`，在
当前目录从 HEAD 创建任务分支；也可明确选择 `current_branch` 或 `dedicated_worktree`。当前直接
用户消息须包含 `/taskbelay` 和 Skill 展示的 `confirm-workspace` 确认，逐仓包含 mode/source/base/target/carry。
全本地选择返回 `status:"ready"` 与 `open_task`，直接在原会话调用 Core；独立工作树仍返回
`{command,arguments,cwd}` relaunch descriptor，目标会话 consume 后创建。混合模式检查新会话的全部目录权限。

DSH bundle 还提供内部 `workspace_coordinator` 工具，operation 只允许
`provision|consume|prepare_cleanup|cleanup_worktree|cleanup_branch`。它不是 shell 命令。
本地模式保留目录和分支，三个 cleanup 操作均不适用。独立工作树的 `prepare_cleanup` 先读取终态 Core Task，并返回从仍存在的源 checkout 重新启动的 descriptor；随后
worktree 与 branch cleanup 分别要求新的 direct-user confirmation，核对 repository group、HEAD、
clean 和远端 task branch 后才使用非 force Git 命令。

## Claude Code

这些命令由 `taskbelay-claude` 包提供。安装方式和 Claude Code 插件启用步骤见 [Host 指南](CLAUDE.md)。

实现入口：`packages/claude/bin/taskbelay-claude.mjs`、`lib/lifecycle.mjs`、`lib/workspace.mjs`。

除前五项外，命令通过已关闭的标准输入接收一个 UTF-8 JSON 对象，最多 1 MiB。表格列出命令名和输入字段，不表示启动交互式输入会话。

| 命令 | 输入或用途 |
| --- | --- |
| `taskbelay-claude status --json` | 检查包版本、注册和缓存状态；无标准输入。 |
| `taskbelay-claude setup --json` | 注册用户范围插件；无标准输入。 |
| `taskbelay-claude remove --json` | 移除归属已核验的注册，保留任务数据；无标准输入。 |
| `taskbelay-claude --version` | 输出包内 Core 的版本。 |
| `taskbelay-claude mcp` | 启动 stdio MCP 服务。 |
| `taskbelay-claude artifacts collect` | Core 文件收集请求。 |
| `taskbelay-claude artifacts prepare` | Core 文件分类结果。 |
| `taskbelay-claude host-check workspace-available` | repository_path。 |
| `taskbelay-claude host-check pre-file-write` | host、repository_path、tool_name、paths、intent_digest、path_parse_complete。 |
| `taskbelay-claude hook pre-tool-use` | Claude 原始 PreToolUse 事件。 |
| `taskbelay-claude host-launch inspect` | request、repositories[{key,repository_path}]；返回评估锚点。 |
| `taskbelay-claude host-launch prepare` | request、assessment、user_choice、repositories、handoff；返回准备记录。 |
| `taskbelay-claude host-launch provision` | launch_id；准备全部选定仓库。 |
| `taskbelay-claude host-launch status` | launch_id；读取保留的启动记录。 |
| `taskbelay-claude host-launch scope` | launch_id；核验工作区并返回 Core 创建范围，单仓库也包含 primary_repository_key。 |
| `taskbelay-claude host-launch bind-task` | launch_id、成功 Core 响应中的 task_id。 |
| `taskbelay-claude host-launch launch` | launch_id；返回会话启动描述，不启动交互进程。 |
| `taskbelay-claude host-launch resume` | launch_id；返回已有会话的恢复描述。 |
| `taskbelay-claude host-launch record-session` | launch_id、实际 session_id。 |
| `taskbelay-claude host-launch retry-launch` | launch_id、previous_caller_stopped、session_not_started、reason；两个事实字段须为 true。 |
| `taskbelay-claude host-launch relocate` | 输入 launch_id、relocation_id、destinations[{repository_key,repository_path}]、authorized；返回可直接提交 Core 的 relocation_id 和 relocation_destinations[{key,repository_path}]。 |
| `taskbelay-claude host-launch cleanup-worktree` | launch_id、repository_key、terminal、authorized。 |
| `taskbelay-claude host-launch cleanup-branch` | launch_id、repository_key、terminal、authorized、core_task；独立于工作树清理授权。 |

`prepare.repositories` 的每项均需提供 key、repository_path、workspace_mode、source_type、remote_name、base_branch、target_branch、carry_changes、worktree_path。仓库 key 遵循 Core 的 `^[a-z0-9][a-z0-9._-]{0,127}$` 规则，在 Git 修改前校验，创建与迁移使用同一标识。mode 为 `new_branch`、`current_branch` 或 `dedicated_worktree`。assessment 保存原始 `inspect` 锚点、影响面、验证安排和已解决的未知项；user_choice 记录实际用户决定。Host 操作的具体前提和值来源见 [admission 引用](../packages/claude/plugin/skills/taskbelay/references/admission.md)及[生命周期引用](../packages/claude/plugin/skills/taskbelay/references/host-lifecycle.md)。

`status`、`setup`、`remove` 无论是否带 `--json` 都输出 JSON。Host-launch 成功时直接输出操作结果；artifacts 保留 Core 的 `ok/result` 或 `ok/error` 结构；host-check 返回检查结果。`mcp` 使用 MCP 协议，不是一次性结果输出。普通 CLI 或输入错误退出 1，Hook 检查失败退出 2，转发的 Core 命令保留其错误输出与退出码。不能将所有结果都按 MCP envelope 读取。

对话触发方式为 `/taskbelay-claude:taskbelay <任务描述>`。`CLAUDE_CONFIG_DIR` 指定 Claude 设置目录；`TASKBELAY_DATA_DIR` 指定已有的规范绝对任务数据目录，在 Host、MCP 和助手中须一致。

## ZCode

本节对应已发布的 `taskbelay-zcode` 包。可用 `taskbelay install --host zcode --yes` 安装；CLI 只准备本地插件来源，还需按 [ZCode 指南](ZCODE.md) 在界面中安装并启用插件。实现来自 `packages/zcode/bin/taskbelay-zcode.mjs`、`lib/lifecycle.mjs`、`lib/workspace.mjs` 和 `hooks/pre-tool-use.mjs`。

| 命令 | 输入与结果 |
| --- | --- |
| `taskbelay-zcode status [--json]` | 无 stdin；检查本地包、Core 和准备记录，不检测 ZCode 插件加载。 |
| `taskbelay-zcode setup [--json]` | 无 stdin；校验并保存本地来源，返回 ZCode UI 安装/启用步骤。 |
| `taskbelay-zcode remove [--json]` | 无 stdin；保留待 UI 移除记录，不删除 Task 数据。 |
| `taskbelay-zcode remove --confirm-host-removed [--json]` | 用户已在 UI 移除插件和 marketplace 并关闭相关会话后，清除归属明确的记录；人工确认不等于自动检测。 |
| `taskbelay-zcode --version` | 输出包内 Core 版本。 |
| `taskbelay-zcode mcp` | 启动 stdio MCP。 |
| `taskbelay-zcode artifacts collect\|prepare` | stdin 为对应 Core 文件收集或准备请求。 |
| `taskbelay-zcode host-check workspace-available` | stdin 为 repository_path。 |
| `taskbelay-zcode host-check pre-file-write` | stdin 为 host、repository_path、tool_name、paths、intent_digest、path_parse_complete。 |
| `taskbelay-zcode hook pre-tool-use` | stdin 为原始 ZCode PreToolUse 事件；Write/Edit 读取 tool_input.file_path。 |
| `taskbelay-zcode host-launch inspect` | request、repositories[{key,repository_path}]；返回评估锚点。 |
| `taskbelay-zcode host-launch prepare` | request、assessment、user_choice、repositories、handoff；保存准备记录。 |
| `taskbelay-zcode host-launch provision` | launch_id；准备全部仓库。 |
| `taskbelay-zcode host-launch status` | launch_id；读取保留的启动记录。 |
| `taskbelay-zcode host-launch scope` | launch_id；核验并返回 Core 创建范围。 |
| `taskbelay-zcode host-launch bind-task` | launch_id、task_id；绑定实际 Core 成功结果。 |
| `taskbelay-zcode host-launch open\|resume` | launch_id；返回 UI 操作说明、工作目录和完整接续提示，不自动启动会话。 |
| `taskbelay-zcode host-launch relocate` | launch_id、relocation_id、destinations[{repository_key,repository_path}]、authorized=true、core_preparation；core_preparation 必须为实际 `taskbelay_prepare_task_relocation` 成功响应的完整 result，含 relocation_id/task；返回供 Core 核验的目标。 |
| `taskbelay-zcode host-launch cleanup-worktree\|cleanup-branch` | launch_id、repository_key、terminal、authorized、core_task；工作树和分支分别授权。 |

Host-launch 接受 stdin 中的一个封闭 JSON 对象，最多 1 MiB；`prepare.repositories` 各项必需 key、repository_path、workspace_mode、source_type、remote_name、base_branch、target_branch、carry_changes、worktree_path。模式为 `new_branch`、`current_branch`、`dedicated_worktree`；key 使用 Core 的 `^[a-z0-9][a-z0-9._-]{0,127}$` 规则。迁移核对准备结果中的已绑定 ZCode Task、当前迁移 blocker 和全部源工作区；不能只凭迁移 ID 移动目录。同一 ID 已移动时，只核验目标身份并回读；结果不确定时拒绝重复移动。Core 完成前次迁移后，新的准备结果和 ID 可以开始下一次迁移。评估、授权和迁移前提见包内 [admission](../packages/zcode/skills/taskbelay/references/admission.md) 和 [生命周期说明](../packages/zcode/skills/taskbelay/references/host-lifecycle.md)。

生命周期命令始终输出 JSON。校验完整且保存准备记录时，`status=action_required`、`registration.host=unverified`，`next_steps` 指示 UI 安装、启用或缓存更新；准备不完整为 `partial`。普通 `remove` 保存 `phase=removal_required`；明确确认已移除后返回 `absent`、`registration.host=user_confirmed_removed`。这些字段描述本地准备与实际用户声明，不是自动 Host 就绪检查。统一管理器同样保留 `action_required`，不将准备成功显示为 `ready`。首轮统一 `uninstall` 保留包供确认命令使用；完成 UI 移除、关闭会话并运行确认命令后，再次统一 `uninstall` 才移除包。ZCode 包或记录仍存在时，`factory-reset` 拒绝清理共享数据。

统一安装、维护或移除的本地步骤成功时可以退出 0 并返回 `action_required`，须继续查看 `next_steps`。统一 `doctor` 在未达到 `ready` 时退出 1；ZCode 的 `action_required` 表示 Host UI 状态尚未核验，不等于本地准备失败。

Host-launch 成功直接输出操作结果；artifacts 保留 Core 成功或错误结构；host-check 输出检查结果；MCP 使用协议传输。普通输入或 CLI 错误退出 1，Core 转发保留其输出及退出码。Hook 收到 Core 的明确拒绝时输出 `permissionDecision=deny` 并退出 0；事件解析或检查调用异常时退出 2。不能仅凭退出 0 判定写入获准，也不能将拒绝当作无 Task 放行；Shell 或外部写入仍由后续观察检查。

插件通过 `ZCODE_PLUGIN_ROOT` 定位包内 MCP 和 Hook 入口。`TASKBELAY_DATA_DIR` 指定已有的规范绝对数据目录，须在 Host、MCP 和助手间一致。ZCode 的 `open` 与 `resume` 只给出 UI 接续描述，不声明存在未核实的会话 CLI 或会话 ID。

## Packaged Core

Host package 内含的 Go Core 不作为普通用户的全局 CLI 安装。以下是 Core executable 实际接受的
完整命令面，主要用于 Host 集成、开发和诊断：

| 命令 | 作用 |
| --- | --- |
| `taskbelay` | 不带参数时打印帮助文本。 |
| `taskbelay help` | 打印帮助文本。 |
| `taskbelay -h` | `help` 的短选项形式。 |
| `taskbelay --help` | `help` 的长选项形式。 |
| `taskbelay version` | 输出 `taskbelay <core-version>`。 |
| `taskbelay config validate` | 从 stdin 读取原始 UTF-8 配置 JSON，最多 16 KiB，输出配置校验结果。 |
| `taskbelay config validate --help` | 显示配置校验帮助；不读取 stdin，不访问配置文件、Task 数据或 Git。 |
| `TASKBELAY_DATA_DIR=/absolute/path taskbelay mcp --stdio` | 使用现有可用数据目录启动 local STDIO MCP。目录不存在或不是目录时启动失败。 |
| `$env:TASKBELAY_DATA_DIR = 'C:\absolute\existing\data'; taskbelay.exe mcp --stdio` | Windows PowerShell 中使用现有可用数据目录启动 local STDIO MCP。 |
| `taskbelay host-check pre-file-write` | **Host 受管命令。** 从 stdin 读取规范化的结构化写入目标，检查活动 Task 的跨仓库 ExpectedPaths，并输出 `allow` 或在写入前持久化 file-scope blocker 后输出 `deny`。Codex、DeepSeek、Claude 和 ZCode Adapter 调用，普通用户不手工运行。 |
| `taskbelay host-check workspace-available` | **内部 Host 命令。** stdin 接收 `{"repository_path":"<absolute root>"}`，只读检查同目录活动 Task；stdout 返回 `available`、规范化 `repository_path` 和可选 `task_id`。失败以非零退出，不创建数据库或预占目录。 |
| `taskbelay webui start [--no-open] [--plain\|--json]` | 启动或复用共享 loopback WebUI；默认打开浏览器。 |
| `taskbelay webui open [--plain\|--json]` | 验证 receipt、进程身份和实时 Core 状态后打开同一 URL。 |
| `taskbelay webui status [--plain\|--json]` | 返回 `ready`、`read_only`、`incompatible` 或 `unavailable`。 |
| `taskbelay webui stop [--plain\|--json]` | 核对 PID 与进程启动身份后停止共享实例。 |

`taskbelay host-check pre-file-write` 与 `taskbelay webui serve` 都是 Adapter/lifecycle 内部入口，不是 Host 用户命令。Core 不支持 remote
transport、通用 HTTP/SSE transport、通用 shell 或 Git mutation 命令。Codex 用户应通过
`taskbelay-codex mcp` 的受管入口启动 Core；DeepSeek 用户由 DSH integration process 启动 Core；Claude 插件通过 `taskbelay-claude mcp` 启动 Core，ZCode 插件通过 `taskbelay-zcode mcp` 启动。

写前检查的 `repository_path` 用于定位已有仓库，`paths` 保留完整写入目标，目标父目录可以尚未创建。
DeepSeek Adapter 会从目标最近的现存父目录定位；Core 的观察超时或输出超限等失败以非零退出，
不能当作无活动 Task 放行。普通无 Task 仓库和非 Git 目录的写入仍允许。

### 配置校验

此处 `taskbelay` 指 Host 包内的 Go Core executable，不是全局生命周期管理器。`config validate` 接收一个 UTF-8 JSON 对象，必须关闭 stdin；不接受文件路径参数，不读取用户配置、Task 存储或 Git，也不写入任何数据。配置规则统一由 `internal/userconfig.Decode` 实现：拒绝重复字段、未知字段、非法 UTF-8、超出 16 KiB 的输入及不符合当前字段类型的内容。空对象 `{}` 使用全部 Host 的默认值。

成功退出码为 `0`，stdout 返回四个 Host 的实际偏好；例如输入 `{}`：

```json
{"ok":true,"result":{"codex":{"codebase_memory":false},"deepseek":{"codebase_memory":false},"claude":{"codebase_memory":false},"zcode":{"codebase_memory":false}}}
```

无效配置退出码为 `1`，stdout 返回具体原因；例如输入 `{"other":true}`：

```json
{"ok":false,"error":{"code":"INVALID_CONFIGURATION","message":"unknown top-level field \"other\""}}
```

参数错误退出码为 `2`。`taskbelay config validate --help` 退出 `0`，仅显示帮助，不消费 stdin。此命令不需要 `TASKBELAY_DATA_DIR`。

## MCP 工具

以下十八个工具是当前全部公开 MCP 工具。它们由 Host Adapter 调用，不是终端 shell
命令。

| 工具 | 类型 | 作用 |
| --- | --- | --- |
| `taskbelay_server_info` | 只读 | 读取 Core 产品版本、transport、健康状态、支持的 process、Host、method profile、工具目录和有效 Host 代码索引偏好。每次有效 Host admission 后必须首先调用。 |
| `taskbelay_open_task` | 读取或创建 | 在全部 `workspace_origin` 按所选工作区模式通过核验后创建 Task；`new_task` 为空时从原 worktree instance 恢复并先检查 workspace。 |
| `taskbelay_get_task` | 只读 | 按 Task ID 读取持久化 Task，包括 verification plan、当前预算/消耗、调整原因和最多三条近期测试尝试；存在 Core 保存的 Action 提交时自动返回 Recovery assessment。 |
| `taskbelay_get_next_action` | 观察/可能 mutation | 先观察 workspace；必要时幂等创建 workspace blocker，否则返回当前 Action、`submission_tool` 和全部合法 transition。 |
| `taskbelay_submit_requirements` | mutation | 提交 REQUIREMENTS 节点结果。 |
| `taskbelay_submit_design` | mutation | 提交 DESIGN 节点结果。 |
| `taskbelay_submit_tasks` | mutation | `tasks_plan_saved` 保存含 `verification_plan` 的完整 baseline 并停留 TASKS；`tasks_ready` 确认已保存计划后进入开发。 |
| `taskbelay_submit_implementation` | mutation | 提交 IMPLEMENT 节点结果。 |
| `taskbelay_submit_test` | mutation | 提交 TEST 节点结果；`verification_budget_increased` 用具体原因增加预算并留在 TEST，普通结果发送 `budget_adjustment=null`；第三次精确重复时暂停。 |
| `taskbelay_submit_comprehension` | mutation | 提交 COMPREHENSION_REVIEW 节点结果。 |
| `taskbelay_submit_refactor` | mutation | 提交 REFACTOR 节点结果。 |
| `taskbelay_submit_delivery` | mutation | 提交 DELIVERY 判断、明确的 acceptance 关联、风险和发现；每条验收包含 work_item_ids 与当前 Test 的 evidence_ids。汇总验证记录 ID 与 Test/Comprehension record ID 由 Core 补齐，调用方提交这些汇总字段会被拒绝。 |
| `taskbelay_resolve_blocker` | mutation | 在 Core 确认当前 blocker 条件后解除阻塞；文件范围使用 `choice` 与 `reason`，history 使用 `history_resolution:{choice:"accept_current_history",reason}`，relocation 使用 `relocation_id` 与全部 `relocation_destinations[{key,repository_path}]`，验证/Recovery blocker 使用当前身份字段。 未执行写前请求按保存绑定到当前观察的完整层增量核对；未变继承路径不自动获准修改。`expand_scope` 后必须重新保存并确认精确计划。 |
| `taskbelay_recover_action` | mutation | 使用 Core 在独立 Action 操作记录中保存的规范化提交恢复不确定 Action；不接收原始 payload。 |
| `taskbelay_cancel_task` | destructive mutation | 使用当前 revision 和非空 reason 将非终态 Task 转为 `CANCELLED`。 |
| `taskbelay_prepare_task_branch_rename` | mutation | host、task_id、revision、repository_key、target_branch、reason；返回 rename_id/task，进入 BLOCKED 准备单仓纯改名。 |
| `taskbelay_prepare_task_relocation` | mutation | 保存 relocation ID、源 workspace/content/surface 和 resume node；Host handoff 期间保留原 claims。 |
| `taskbelay_abandon_task` | destructive mutation | 原 worktree 确实不可用时，用精确 host/task/revision 和非空 reason 进入 `CANCELLED` 并释放 claims；先尝试观察仓库，以确认原 worktree 不可用。 |

八个普通节点提交工具都只接收 `host`、`task_id`、`action_id`、`transition_id`、`summary`、
`reason`、`artifacts`、`method_results` 和只含语义事实的节点专属 `node_result`；其中没有
`changed_paths` 或 `no_file_changes`。Core 从 Git 计算 Action delta/current surface，并从当前 Action 补齐
revision、Action kind、process identity、source cursor、repository binding、artifact role、method
step identity/order/status 与内部 payload envelope。`get_next_action` 的 `submission_tool` 指出当前
唯一可用的提交工具。

`method_results` 以当前 `method_steps[].step_id` 为键，每个值只含 `capability` 和 `summary`。
外部工具完成步骤时填写实际 capability ID，普通等价工作完成后填写空字符串；Core 生成内部
`MethodEvidence` 的步骤、顺序和状态。artifact 按当前 Schema 放入 `artifacts.current` 或
`artifacts.other_process`，每项只含 `path`、`digest` 和 `summary`，`role` 由 Core 根据槽位和节点赋值。

`taskbelay_submit_design` 的 `node_result.baseline.requirements_revision`、`taskbelay_submit_tasks` 的
`node_result.baseline.design_revision` 与 `taskbelay_submit_implementation` 的
`node_result.task_plan_revision` 均不属于 Host 可提交的字段。Core 确认当前 Action 身份后，从同一 Task
快照填充这些字段；提交任一字段会返回准确路径的 `unknown_member`。节点提交缺少
其他必填字段时返回准确的 `required_member_missing` 路径；只有已证明零写入且修正内容来自当前节点
既有事实时，Host 才能按 `recovery.allowed_paths` 通过同一提交工具修正一次。

`taskbelay_submit_tasks` 的 `node_result` 固定包含 `problem_class`、`baseline`、`findings` 和 `user_confirmation`。保存/修订计划使用 `tasks_plan_saved`：完整 baseline、problem_class=none、空 findings、user_confirmation=null，返回的 Task 仍在 TASKS。确认使用 `tasks_ready`：baseline=null，确认对象为 `{source:"user",status:"passed",summary,requirements_digest,design_digest,task_plan_digest,task_plan_revision}`；四个引用值来自当前 `baselines.requirements.digest`、`baselines.design.digest`、`baselines.task_plan.digest` 和 `baselines.task_plan.revision`。只有用户明确认可这些内容后才能提交。返回 `task_plan.confirmation` 和 Core 记录的 `confirmed_at`。缺失或不匹配的确认拒绝进入开发，等待仍在 TASKS。上游返回边保留原有 findings/reason 要求，并提交 null baseline 和 null confirmation。

`host-launch prepare` 的 `assessment` 包含 `change_level`（small/standard/large/uncertain）、observed_repositories、candidate_components、candidate_paths、public_contract_flags、persistence_or_state_flags、host_or_platform_flags、verification_shape、unknowns、recommendation、reasons 和 anchor。`user_choice` 为 `{source:"user",mode:"taskbelay",summary}`，记录展示评估后的真实选择。缺失输入、未解决未知项、根集合不一致、失效 anchor 或非 TaskBelay 选择在准备前拒绝。回执的 `admission` 保存完整评估和选择；已确认接续读取原回执，不重复选择。

新 Task 的 `new_task` 不包含 `verification_budget`。TASKS 的 `baseline.verification_plan` 包含
`checks[{name,rationale}]`、`initial_budget`、`full_suite_expected` 和
`test_code_changes_expected`。TEST 容量不足时可以选择同一 Action 返回的
`verification_budget_increased`，提交 `budget_adjustment`：`basis`、`additional_checks`、
`additional_automatic_commands`、`allow_full_suite`、`allow_manual_handoff`；transition `reason`
说明具体的新影响、风险、失败或验证缺口。没有实际增加、没有需要补充额度的检查说明或没有具体原因会被拒绝。

每个 TEST check 还必须提交 `full_suite_reason`。`full_suite=false` 时它是空字符串；完整套件则记录本次
运行补足的具体风险。Core 保存该结果，但 Host 仍需在命令执行前判断本次完整套件是否必要。

未知 CLI 参数、未列出的 MCP 工具或未满足隐式/显式统一 admission 的调用不属于受支持入口。

### Repository Scope 与 Host 偏好字段

创建 Task 前，Host 按当前用户指令和适用的 `AGENTS.md` 只读调查仓库。需要项目索引时，结合索引、
候选项目说明及代码与配置确定完整候选范围，再逐仓确认并准备工作树。所有读取遵守现有 Host 权限，
Core 保存经过确认的固定 Scope。

创建多仓库 Task 时，`repository_path` 是主仓库；调用可以增加一个主 key 和最多七个显式附加仓库：

```json
{
  "host": "codex",
  "repository_path": "/workspace/core",
  "workspace_origin": {
    "mode": "dedicated_worktree",
    "source_type": "remote",
    "carry_changes": false,
    "remote_name": "origin",
    "base_branch": "main",
    "base_commit": "<frozen-commit>",
    "task_branch": "feature/core-docs",
    "provisioning_receipt_id": "launch-core-docs"
  },
  "primary_repository_key": "core",
  "additional_repositories": [
    {
      "key": "docs",
      "repository_path": "/workspace/docs",
      "workspace_origin": {
        "mode": "dedicated_worktree",
        "source_type": "remote",
        "carry_changes": false,
        "remote_name": "origin",
        "base_branch": "main",
        "base_commit": "<frozen-commit>",
        "task_branch": "feature/docs",
        "provisioning_receipt_id": "launch-core-docs"
      }
    }
  ],
  "new_task": {
    "request": "同步 Core 与文档仓库中的接口说明",
    "initial_scope": [],
    "initial_out_of_scope": [],
    "known_acceptance_criteria": [],
    "method_profile": "plain"
  }
}
```

该示例只说明 closed MCP 输入形状，不是 shell 命令。`<frozen-commit>` 必须替换为实际 object ID。
创建时每个 repository 都必须带 receipt 证明的 `workspace_origin` 和非空 `new_task`；Core 从本地 Git
核对并补齐 source group、canonical root 与 worktree Git-dir。恢复时省略或设 `new_task=null`，
`repository_path` 指向原参与 worktree，并省略全部 Scope/origin 创建字段。总仓库数为一至八；附加
仓库按 key 排序，Scope 创建后不可变。多仓库 payload 路径使用
`<repository-key>::<repository-relative-path>`。

Task result 保留主 `repository`，增加 `primary_repository_key` 与 sorted
`additional_repositories`。当前 Action 中唯一的 `repository_binding_digest` 在单仓库 Task 中仍是
主 binding digest，在多仓库 Task 中是完整 Scope aggregate。活动 Task 的全部
`repository_claims` 与 snapshot/event 在同一 SQLite transaction 中 Acquire、Retain 或 Release。

`repository_claims` 使用可直接观察的 worktree-instance identity，不是整个 Git common directory。
linked worktree 共享逻辑仓库组标识，但 canonical root/worktree Git-dir 不同，因此可以分别持有活动
Task；同一实例只能持有一个活动 Task。Control Center 的 Task summary 公开只读 `repository_group_id` 和
`worktree_path`，详情中的每个 repository 也公开自己的 `repository_group_id`。

Task result 的 `verification` 同时返回 `plan`、`current_budget`、当前 Task Plan revision 的 `usage` 和
`adjustments`；在首次保存 TASKS 计划前，`plan` 与 `current_budget` 为 `null`。

`taskbelay_server_info({})` 的结果包含：

```json
{
  "host_preferences": {
    "codex": { "codebase_memory": false },
    "deepseek": { "codebase_memory": false },
    "claude": { "codebase_memory": false },
    "zcode": { "codebase_memory": false }
  }
}
```

这些值来自只读用户配置的进程启动快照：macOS 为 `$HOME/.taskbelay/config.json`，Windows 为
`%USERPROFILE%\.taskbelay\config.json`。它们仅表示偏好，不表示索引能力已经安装或可用。文件不存在时
四个 Host 都为 false；Core 仅解释配置，不创建或修改配置文件。Codex setup 和管理器初始化缺失配置时写入 `{}`，已有合法配置保持原样。

Host 选择检索工具时，当前用户指令和适用的 `AGENTS.md` 优先于这些默认偏好。没有相应指令时，
false 选择普通文件和文本搜索，true 可优先使用当前可用的代码索引。索引不可用或结果不完整时，
Host 在当前会话中至多提示一次并回到普通搜索；索引结果不改变已创建 Task 的 Scope。

## 文件收集与准备命令

`taskbelay-codex artifacts collect` 和 `taskbelay-codex artifacts prepare` 分别转发到包内 Core 的 `taskbelay artifacts collect` 和 `taskbelay artifacts prepare`。两个命令从 stdin 读取最多 1 MiB 的单个 UTF-8 JSON 对象，通过 stdout 返回 `{ok:true,result:...}` 或 `{ok:false,error:...}`，成功退出码为 0，失败为 1。collect 输入为 `{host,task_id,action_id}`；prepare 输入为 `{host,collection}`。前者输出完整文件信息，后者检查逐项分类、观察是否变化并生成 artifact 数组。只读取已有 Task 和 Git，不创建存储或推进流程。完整字段及使用步骤见[文件收集与提交](ARTIFACTS.md)。

## Codex Host 操作帮助

```bash
taskbelay-codex --help
taskbelay-codex host-launch --help
taskbelay-codex host-launch prepare --help
taskbelay-codex host-launch scope --help
```

所有帮助查询均在读取 stdin、解析安装路径或执行 Core/Git 操作前返回。单个操作帮助是 JSON，包含 `input_schema`、`output_fields` 和 `next_step`；字段说明交代值来自用户确认、前一步结果还是 Host 查询。帮助查询不创建配置、工作区或记录。

`inspect` 返回 anchor，放入完整 `assessment.anchor`。`prepare` 接收原样保留的 request、完整评估、`user_choice`、工作树参数和 `handoff_file`；桌面工作区显式传 `worktree_path: null`。首个结果的 `receipt.launch_id` 用于同一 Task 的其余仓库。依次完成受管派发和 `bootstrap`，或 CLI provisioning 后，调用只读汇总命令：

```text
taskbelay-codex host-launch scope
stdin: {"launch_id":"<saved launch ID>","repository_keys":["api","web"],"primary_repository_key":"api"}
```

`repository_keys` 必须列出全部已确认仓库。命令拒绝缺失、尚未准备完成、重复或属于不同请求的记录，输出 `repository_path`、`workspace_origin`，多仓库时还包含 `primary_repository_key` 和 `additional_repositories`。将完整输出作为 `taskbelay_open_task` 的仓库字段，再添加 `host` 与已确认需求对应的 `new_task`。

## MCP 结果读取

每个工具提供输入和结果 Schema。成功结果的 `ok=true`，数据在 `result`；失败结果通过 `error` 和 `recovery` 描述原因及允许的处理方式。`structuredContent` 与文本内容中的 JSON 相同，读取完整结果一次即可。

Codex 在提取 `result` 前保留完整响应并检查 `ok`。`ok=false` 响应没有成功结果，不能将其
`result` 传给会话 `store`：`undefined` 会导致本地序列化异常并遮住原始错误。完整拒绝仍按
`error` 和 `recovery` 处理；只有原始响应无法完整取得时才进入不确定操作恢复。

TEST 选择 `tests_failed_implementation` 时，`problem_class="implementation_failure"` 且
`findings` 必须非空。`failed_items` 列出失败检查或项目，`findings` 说明需要退回实现的具体缺陷；
其他字段中的失败描述不能替代它。暴露 `findings` 的节点结果在 `problem_class="none"` 时使用空数组。

| 工具 | Task / Action 位置 |
| --- | --- |
| `taskbelay_open_task`、`taskbelay_get_task` | `result.task`；先处理同层的 `result.recovery_assessment` |
| `taskbelay_get_next_action` | `result.action`；先处理 `result.recovery_assessment`、`result.blocker`、`result.outcome` |
| 八个 `taskbelay_submit_*`、`taskbelay_resolve_blocker`、`taskbelay_recover_action` | `result` 本身是 Task，下一步是 `result.current_action` |
| `taskbelay_cancel_task`、`taskbelay_abandon_task` | `result` 本身是终态 Task |
| `taskbelay_prepare_task_relocation` | `result.task` 与 `result.relocation_id` |

新会话恢复时，已有 `recovery_assessment` 优先于源 Action；使用 `operation.action_id` 恢复保存的提交。创建响应不确定时在原工作树调用省略 `new_task` 的 `open_task` 回读，并核对来源、范围和需求。取消使用预先保留的 `request_id` 对照 `task.last_operation.operation_id`、kind 和 outcome；放弃及迁移准备对照原 Task、预期 revision、操作类型和保存结果。无法确认时停止，保留原资源；生命周期操作不套用普通 Action 恢复规则。

`read_next_action` 可以直接使用 open/next-action 已返回的完整 Action；来自只读快照 `get_task` 时查询一次新 Action。已经完成的恢复评估持续存在时不重复查询。

Task Plan 的 `expected_paths` 支持精确路径及目录后缀 `/**`，不支持一般 glob；`src` 不代表目录下的全部文件。声明了附加仓库的 Task 中，每条路径都必须写成 `key::relative-path`，主仓库的路径同样带 `primary_repository_key` 前缀；缺少前缀或使用未声明的 key 会以 `repository_path_invalid` 指向具体成员。单仓库 Task 使用不带前缀的相对路径。`acceptance_indexes` 从 0 开始，对应当前 Requirements 的 `acceptance_criteria` 数组；`dependencies` 引用同一计划中的 work-item ID。

`host-launch prepare` 省略 `launch_id` 时自动生成 ID，并使用该 ID 核对启动记录。重试时传入返回的 `receipt.launch_id`，继续同一次启动；记录已为 `prepared` 时跳过 fetch。显式传入的 ID 必须与保存记录一致。

`dispatch-result` 的 `host_result` 使用 Codex 原始完整返回值：支持直接结果对象、`result`、`structuredContent`、`structuredContent.result`，以及没有结构化结果时单个 `content` 文本块中的 JSON。结构化结果优先；文本 JSON 必须合法且无重复成员。`isError: true`、缺少标识、无法解析或多个文本块均记录为 `uncertain`。

有效 `clientThreadId` 保存到 `operation_status.host_client_thread_id`，阶段为 `queued`；有效 `threadId` 保存到 `host_thread_id`，阶段为 `dispatched`。对相同 `launch_id` 和 `repository_key` 补交保留的原始结果，允许 `uncertain → queued`；后续就绪结果沿 `queued → dispatched` 保存，并保留排队 ID 和原派发标识。`dispatch-start` 在这些阶段均返回 `should_dispatch: false`。Host 继续检查同一次创建；`clientThreadId` 不是可传给要求 `threadId` 的工具的任务 ID。

Codex 启动先由 `dispatch-start` 将完整 `host_request` 保存到 `receipt.operation_status.host_request`，进入 `dispatch_prepared`；重复调用和 `status` 均可回读。`dispatch-call` 使用当前 `dispatch_attempt_id` 将阶段改为 `dispatching`，仅首次返回 `should_dispatch=true` 时允许调用一次创建工具。调用方将命令完整 stdout 写入私有文件，检查退出码并从文件解析 JSON，再原样转发请求，避免显示长度限制截断内容。

确认原调用方已停止且创建工具尚未调用时，`dispatch-recover` 接收当前派发 ID、`host_call_not_made=true`、`previous_caller_stopped=true` 和具体 `reason`，保留原请求并换发调用许可 ID；随后执行 `dispatch-call`。空任务 ID 本身不能证明未调用。已经调用但结果未知时，Host 按保存的启动标题、启动 ID 和仓库标识查找任务及归档任务，读取候选任务完整初始消息，将 `candidates`（`thread_id`、`initial_prompt`）交给 `dispatch-reconcile`。唯一完整消息匹配才保存任务 ID；零匹配、多个匹配或查询不可用均不允许重新创建。Core Task 状态保持由 Core 管理。

默认在当前目录新建分支；本地模式检查当前具名分支及初始修改选择。明确选择独立工作树时，再确认本地或远端来源、起始分支、目标分支，并询问本地内容是否携带。`source_type` 和
`carry_changes` 为必填字段，本地 `remote_name=""`，远端 `carry_changes=false`。详见[工作树来源与本地改动](WORKTREE-SOURCES.md)。

## 已有检查的验证额度

增加验证额度时，`additional_checks` 可以引用原计划或此前增加记录中的检查名称，使用 `rationale` 说明本次补做或重跑。单次提交内名称仍需唯一，具体原因、实际增加量和上限继续校验；追加额度本身不生成通过结果。

## 包制备与发布

### Host Adapter

四个 Host 使用相同的独立发布参数，各自维护 package、Tag 和公开版本记录：

| Host | 发布命令 | 精确确认格式 |
| --- | --- | --- |
| Codex | `pnpm run release:codex -- ...` | `codex-v<VERSION>` |
| DeepSeek | `pnpm run release:deepseek -- ...` | `deepseek-v<VERSION>` |
| Claude Code | `pnpm run release:claude -- ...` | `claude-v<VERSION>` |
| ZCode | `pnpm run release:zcode -- ...` | `zcode-v<VERSION>` |

```bash
pnpm run release:claude -- --channel stable --version "<VERSION>" --output "<ABSOLUTE_DIRECTORY>" --confirm "claude-v<VERSION>"
node scripts/build-host-release.mjs --product <codex|deepseek|claude|zcode> --output "<ABSOLUTE_DIRECTORY>"
```

发布命令的 `--channel` 默认为 `stable`，接受 `MAJOR.MINOR.PATCH`，要求干净且与 `origin/main` 同步的 `main`；`beta` 接受 `MAJOR.MINOR.PATCH-beta.N`，使用干净的命名分支。`--version` 和 `--confirm` 必填，确认内容必须对应所选 Host 与版本。`--output` 可省略，默认使用用户目录下的 `taskbelay-releases/<host>-v<VERSION>`；显式目录须为仓库外绝对路径，父目录必须已经存在。发布前执行固定检查，随后对齐所选 package 与 plugin/marketplace 版本副本。有版本文件变化时先提交并推送，再核对推送后的远端提交；之后才构建产物并交给 publisher 核对与发布。只有 stable 更新对应公开版本条目。

`build-host-release.mjs` 只制备和核对包含两个平台 Core 的五个产物文件，不执行发布；`--product` 和 `--output` 必填，输出须为已经存在的仓库外空绝对目录。源码入口来自 `scripts/release-<host>.mjs`、`release/host-command.mjs` 和 `scripts/build-host-release.mjs`。每次发布前均需核对所选包的 npm 所有权、Trusted Publisher 和精确版本。完整要求与产物说明见[发布说明](../release/README.md)。

### 正式桌面包制备

```bash
node release/taskbelay/prepare.mjs --output "/absolute/pet-release"
```

使用仓库工具链与 Swift >=6.0，在 macOS arm64 执行。此命令装配两个平台应用并验证最终 tarball，不执行发布；输出目录必须在仓库外。

## Host 调用示例

Codex、DeepSeek、Claude Code 与 ZCode 的 Core 交互说明和完整示例统一维护于 `skills/taskbelay/core/`，由构建脚本生成各包内的引用文件。各 Host 的授权、工作树准备和工具调用分别说明；实际执行使用当前 Action、已安装接口和真实用户决定。节点提交、返回处理、阻塞恢复与验证规则使用相同内容，并对四个 Host 生成的示例运行同一套 Core 校验。

[Codex Skill](../packages/codex/plugin/skills/taskbelay/SKILL.md) · [DeepSeek Skill](../packages/deepseek/skills/taskbelay/SKILL.md) · [Claude Skill](../packages/claude/plugin/skills/taskbelay/SKILL.md) · [ZCode Skill](../packages/zcode/skills/taskbelay/SKILL.md)

DeepSeek Skill 随包提供 `scripts/artifacts.mjs`，以 `node <实际 Skill 目录>/scripts/artifacts.mjs collect` 或 `prepare` 调用同一套 Core 只读文件准备命令。输入与返回结构与本文相同，`host` 使用 `deepseek`；脚本复用 Adapter 的运行时和数据目录解析，不创建存储。通过实际 DSH Skill 的 `resourceBase` 取得脚本路径。`--help` 不读取 stdin 或解析运行时。该脚本不是独立的 `taskbelay-deepseek` CLI，也不增加 `workspace_coordinator` 操作。

## Core 响应和既有失败验收

所有工具的成功/失败结构、错误字段和下一步操作遵守 [Core 响应规范](CORE-RESPONSES.md)。`ok=true` 只包含 result，`ok=false` 只包含 error/recovery；二者均有 request_id/tool。数量超限返回 VERIFICATION_BUDGET_EXCEEDED 和 error.budget 的 used/requested/limit；权限限制返回 VERIFICATION_NOT_ALLOWED 和具体字段。空的 budget_adjustment.additional_checks 返回字段详情；Core 确认零写入后允许在同一 Action 内按 allowed_paths 纠正一次。

`taskbelay_submit_test` 新增 `tests_accepted_with_known_failures` → COMPREHENSION_REVIEW，要求具体 reason，原始 failed 检查和单独 passed 的自动比较检查，以及 `node_result.known_failure_acceptance`：source=user、summary、failed_checks、comparison_check、task_plan_revision、content_digest。用户确认绑定其所见内容；失败集合必须完整，其余检查通过，无待办或未执行检查。其他转换省略该字段或传 null。普通 tests_passed 仍只接受通过检查。具体保存和交付规则见 [架构说明](ARCHITECTURE.md#既有失败验收)。

`allow_manual_handoff` 仅限制待办人工检查；已完成用户检查和独立验收可如实记录。仅调整权限时 additional_automatic_commands 可以为 0，additional_checks 仍需说明涉及的检查。`taskbelay_get_task` 和 `taskbelay_get_next_action` 的恢复探针复制保存的完整操作。公开输入 Schema 是 Host 传输提示：保留顶层和探针的字段、类型与封闭结构，但省略探针内重复的必填声明及深层检查来源枚举；`verification_plan`、`budget_adjustment`、`known_failure_acceptance` 三个深层对象只以带类型的开放对象投影，以容纳完整的已保存操作。实际运行校验由 Core 执行：私有输入校验仍要求 `operation_id`、`action_id`、`payload`，并核对全部身份、检查来源及完整 payload 的必填和封闭规则；省略或添加字段不能改变保存的操作。普通节点提交工具的 Schema 与检查来源枚举保持不变。

MCP 参数纠错分为 `correct_current_action`（普通节点提交）和 `correct_request`（握手、读取、创建及生命周期请求）。二者均要求 Core 确认零写入，并用 allowed_paths 限定本次纠正。`correct_request` 保留原请求身份和已有授权，不要求先取得一个尚不存在的 Action。四个 Host Skill 为每个请求提供完整成功响应的链接，以及经过代码比对的具体错误响应与实现位置。历史恢复分别在 `history_resolution.choice` 和 `history_resolution.reason` 上报告枚举错误和文本错误。

所有 18 个工具的 `error.message` 直接说明具体失败条件；字段错误同时返回路径、规则和要求，多个原因保留在 `details` 或 `guard.failures`。JSON 语法、类型、重复成员、状态限制、仓库观察、存储和响应编码失败分别说明。编码失败保留实际工具和已有请求 ID，未知底层原因会明确说明，具体规则见 [Core 响应规范](CORE-RESPONSES.md)。

## Task 接续输入

`taskbelay_get_task` 可带 baseline_history={revision,after,limit}；首屏用 revision=0/after=0，后续
保留返回的 revision/next_after，limit 为 1..32。result.baseline_history 包含 task_id、revision、total、
next_after（结束为 null）及 entries[{sequence,reference}]。Task 变化返回 REVISION_CONFLICT；越界游标
返回 INVALID_ARGUMENT。Task baselines.history 是完整数组的有界首页，并始终带 history_total、
history_next_after、history_revision。存储不按页截断、不归档、无配置性条数上限；资源和修订号范围仍适用。
每页最多 64 KiB 实际 JSON 字节，整个响应最多 1 MiB，实际返回条数可小于 limit。只用返回的 next_after
继续，不自行加 limit；首页为空但 total 非零时从返回的 0 开始显式分页。非历史响应超限仍拒绝。

`taskbelay_resolve_blocker` 只在已准备改名 blocker 中接受 rename_id 和 rename_choice=complete|cancel。
complete 核对源 ref 消失及 Task 事实未变，cancel 要求原事实未变。两者直接返回 Task。已保存未提交的决定
使用原 action_id 调用 recover_action，不替换载荷。

`taskbelay-codex host-launch supersede` 读取 launch_id、repository_key、expected_receipt_digest、reason
和 replacement（包含不同显式 launch_id 的完整 prepare 输入）；status 返回 receipt_digest。替代结果为
receipt_path、receipt、receipt_digest、predecessor、prepare_input；普通准备使用返回的 prepare_input。
`supersede-resume` 读取前驱标记的 launch_id、repository_key、seed_digest，返回同样结构。两命令均不执行
目标 Git/Host 操作；封闭输入 Schema 见对应 operation --help。

Codex/Claude/ZCode 的 cleanup-worktree、cleanup-branch 额外要求 core_task：实际完整的终态 Core Task。
助手核对仓库 key、provisioning origin、root 及有效分支/HEAD，清理仍各自授权。Claude host-launch relocate
额外要求 core_preparation，即 prepare_task_relocation 的实际完整 result；ZCode 已有此输入。创建回执的
target 分支不改；缺少有效事实不选用旧名。DeepSeek 清理直接读取终态 Core Task 的 current_branch。
