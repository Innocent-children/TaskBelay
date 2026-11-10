# Agent 接入

TaskBelay 通过 hook 记录开发过程，通过 MCP 让 Agent 保存需求、方案和决定。平台和 Agent 的格式差异放在适配器里，任务和判定逻辑共用。

本文说明安装、事件和 MCP 接口。Agent 接入未能提供某项信息时，TaskBelay 将相应结果记为未知，不因此划分语言支持等级。具体分析流程见 [算法核心](algorithm.md)。

## 接入方式

每次 hook 启动 `taskbelay hook <agent> <事件>`，JSON 通过标准输入输出传递。输出仅包含该 Agent 支持的字段，中文用 ASCII 转义输出。MCP 使用 stdio，每个 session 一个 `taskbelay mcp` 进程，提供 [任务账本](requirements-ledger.md) 的八个工具。

| 时机 | hook | 处理 |
| --- | --- | --- |
| 开始、恢复、清空、压缩或分叉 session | SessionStart | 核对任务上下文和当前文件，注入提示与接续摘要 |
| 用户消息到达 | UserPromptSubmit | 保存原文和快照，更新当前决定与任务上下文；同时匹配种子，提示可复用的已有单元 |
| shell 命令前 | PreToolUse | 记录命令，计算验证输入，核对上次结果和有效的重跑决定 |
| 编辑工具前 | PreToolUse | 读取目标文件，发现工作区更新，核对过期编辑和方案范围；同时提示新文件、对外新增项、近似重复及相近测试的断言异同 |
| Claude Code 调用 TaskBelay MCP 工具前 | PreToolUse | 记录本次调用的 session、轮次和工具调用编号，供 MCP 确定调用所属的会话和轮次 |
| 命令或编辑后 | PostToolUse；Claude Code 另接 PostToolUseFailure | 核对实际结果，记录编辑及工作区更新，补充验证结果 |
| 本轮准备结束 | Stop | 处理待交付任务，或保存普通停顿的进度与提示 |

| 工具 | Claude Code | Codex |
| --- | --- | --- |
| shell | Bash、PowerShell | Bash，包括 exec_command |
| 编辑 | Edit、Write、NotebookEdit | apply_patch |

后置 hook 同时匹配 shell 和编辑工具。用户直接在 IDE 或终端修改，不需要触发 Agent 工具；下次观察文件时将变化记为工作区更新。

## SessionStart

TaskBelay 先记录 session 和事件，再核对当前文件、分支情况、账本及有效设置，按 [任务接续](handoff.md) 生成摘要。恢复和分叉时也会核对工作区，不能只因为账本没变就沿用旧上下文。

新开、清空或压缩 session 后，向 Agent 提供事前提示；恢复时，如果工作模式或规则发生变化，则更新提示中的对应部分。摘要与提示共用长度预算。另一个 session 最近有活动时，附一条共享工作区的提示。

## UserPromptSubmit

TaskBelay 保存完整消息、轮次和快照，用于确定任务起点及引用来源。Agent 根据消息更新需求和用户决定。工作区有更新时，提示当前内容发生了哪些变化；上一轮已经输出相同提示，而且没有新情况时，不重复提供。

此时，TaskBelay 用用户原话匹配种子，在任务起点的图上做局部推送，找出与需求最相关、使用最广的已有单元，并提示 Agent 可以复用哪些实现。这部分与摘要共用长度预算，见 [算法核心](algorithm.md)。

Codex 可以在同一个 `turn_id` 中追加用户消息。消息事件用轮次和完整原文的 SHA-256 共同标识，不把轮次当作唯一消息编号；轮次本身保持不变。同轮不同原文分别保存，相同原文按重复投递处理。Codex 未提供独立消息编号时，无法区分同轮同文重发与再次提交。

## PreToolUse：shell

适配器给出命令、实际目录、shell 写法和工具调用编号。检查流程计算这次命令的验证输入，查询验证输入记录、范围包含与通过规律；命令是否属于验证，按 [输入与执行协议](languages.md) 由行为判断。具体判定见 [过度测试判定](over-testing.md)，不以命令名称或累计次数直接决定。

验证输入与上次通过时完全相同、满足重复验证的全部条件，且本次尚未拦截过时，返回 deny，并在原因中附上次输出和重跑方式。用户要求重跑，或存在有效的重跑理由时，照常运行。上次结果未知、命令会写入、命令不稳定，或两次运行之间修改过环境变量时，也照常运行。

非只读命令使用一秒的预算，先采集并保存命令开始时的工作区，将已有改动记为工作区更新。只读命令使用五十毫秒。判定重复时先查同一命令的记录和两次运行之间的近期操作。拒绝原因说明命令没有运行，并给出上次输出和重跑方式，过长的输出保留开头和结尾。

根据范围包含关系、通过规律或命令对注释不敏感的记录得出的判断，只返回提示。多段命令输入重复、两次验证之间可能发生过其他操作，或近期记录不足以覆盖整个间隔时也只提示。提示通过 additionalContext 返回，保留 Agent 原有的权限流程。

命令前的验证输入使用已发布的索引和已保存的状态摘要。已识别为验证且需要判断覆盖范围或通过规律时，再读取其他命令的运行记录。

POSIX 与 PowerShell 按实际工具确定；外层为 `bash -lc`、`pwsh -Command` 等形式时解析内部命令。未识别的命令仍照常执行，记录为其他命令。

## PreToolUse：Claude Code 的 MCP 调用身份

Claude Code 的 hook 提供 `session_id`、`prompt_id` 和 `tool_use_id`，但 MCP 请求不保证携带轮次。为使写入对应到准确的用户消息，单独注册匹配 `^mcp__taskbelay__.*$` 的 PreToolUse 条目：只保存工作区、Agent、session、轮次、工具调用编号和工具名，不检查业务内容、不改写工具参数，也不返回 allow 或 deny。该事件沿用快速事件预算。

Claude Code 启动 MCP 子进程时提供 `CLAUDE_CODE_SESSION_ID`，工具请求的 `_meta["claudecode/toolUseId"]` 提供 Claude Code 自己的工具调用编号。MCP 根据这两项和当前工作区读取前置记录，核对工具名后取得所属轮次，不根据最近活动或 JSON-RPC 请求编号猜测。相同调用重发时可以复用记录；同一调用编号对应了不同轮次或工具时，返回冲突，不覆盖原记录。

前置 hook 缺失、未加载、超时或调用身份字段不全时，hook 仍正常退出；MCP 写入工具返回调用身份无法核对的说明，不写账本。`task_get` 仍可查询已有任务。已完成调用重试时，继续按 Agent 提供的工具调用编号返回原结果；旧轮次的新写入仍按账本规则拒绝。

Codex 适配器从 MCP 请求的 `callId` 和 `x-codex-turn-metadata` 中读取工具调用编号、`session_id`、`turn_id`，并核对同时出现的 `sessionId`；不能用 `threadId` 代替 hook 中的 session。Codex 不需要增加记录调用身份的 hook。两种接入方式都只适配已有的传输字段，不向 MCP 工具参数增加身份字段。

## PreToolUse：编辑

TaskBelay 先读取目标文件，将当前内容与最近观察的内容及请求中的编辑前内容核对。未变化的片段正常编辑；发现更新时，附上变化位置。请求将覆盖已变化的片段时，退回这次编辑，让 Agent 重新读取当前版本；只提示模式仅补充当前内容和冲突位置。

尚未建立任务时，也会读取当前生效的工作模式；半自动模式下，按首次方案尚未确认处理。随后按 [半自动模式](semi-auto.md) 检查目标是否已获准编辑。首次确认前拦下工作区编辑；增补内容待确认时，只拦新增范围。只提示模式下，将方案问题交给 Agent 处理，不拒绝调用。

保存工具请求、编辑前标识及可推导的预期改动，用于后置核对。不为每次编辑扫描整个仓库。原文核对、冲突决定和这些记录先保存，再生成设计提示；提示来不及生成时只略过本次提示。全自动模式不按目标文件类别判断编辑授权；半自动模式下类别核对不了的目标不按辅助文件放行，已明确授权的范围照常生效。

编辑前对目标文件的当前与拟写入内容做增量词法分析，使用已经发布的分析索引；索引尚未就绪时不提示。发现新建文件、对外新增项，或新单元与已有单元近似重复时，通过 additionalContext 立即提示，写明可以复用的已有单元，或原话中找不到对应要求的新增项。拟改动中的抽象展示声明和成员，提示核对隔离用途与类型、调用约定，不统计实现数量，见[编辑前核对抽象用途](over-design.md#编辑前核对抽象用途)。拟改动中的转发包装展示被调名字，提示核对行为、职责、入口与兼容要求，见[编辑前核对转发包装](over-design.md#编辑前核对转发包装)。拟改动涉及相邻调用赋值和条件分支中的调用时，展示具体位置，提示核对恢复用途，不能据此判定恢复多余，见[防御处理的核对](over-design.md#防御处理的核对)。相近测试补齐索引中的直接参照，核对参照文件仍未变化后，展示等价类和完整断言异同，范围与略过条件见 [编辑前提醒](over-testing.md#编辑前提醒)。这些提示不拒绝编辑。

新增入口的提醒展示已识别的参数、环境变量、路由字面量和注册式调用候选，核对实际入口、使用者、当前需求、已有入口和使用维护成本；代码形状不证明它确为对外入口或有必要，见[编辑前核对新增入口](over-design.md#编辑前核对新增入口)。新增依赖的提醒展示清单中识别的名字和版本片段，核对当前需求、已有实现和引入成本，见[编辑前核对新增依赖](over-design.md#编辑前核对新增依赖)。两者的已有登记沿用用途和引用，原话或有效方案直接对应的项仍略过。这些信息不代表已安装依赖或已确认版本可用。

新增配置的提醒展示识别到的键、取值片段和需求对应关系，核对实际配置用途、使用者、切换场景、固定行为会失去的功能及使用维护成本。内部常量也可能符合键值形状，依据不足时保留必要配置，不因此补测试；已有登记沿用用途和引用，原话或有效方案直接对应的项仍略过。预览不补读使用者或解析配置加载过程，普通编辑继续放行，见[编辑前核对新增配置](over-design.md#编辑前核对新增配置)。

重复实现的提醒只采用仍有效的索引参照。请求外的参照文件按当前内容及模式核对，同批目标中的参照声明则同时核对当前与拟写入内容；已改变、删除或移位的参照略过。只读取实际候选文件并按路径复用核对结果，不扫描全仓库；失败或来不及时沿用预览略过规则，编辑记录和过期保护先行处理。完整范围见[重复实现与没人使用的代码](over-design.md#5-重复实现与没人使用的代码)。

请求按规范化后实际指向的文件分组，保留每个文件对应的原始操作下标。单项请求中的多个 hunk 分别按其定位条件核对。如果同一文件出现多个独立操作，且无法确定这些操作最终会怎样执行，就将该文件标为无法完整核对，其他文件继续处理。工作区外的目标不使用本工作区的历史；无法确定路径指向或读取失败时，单独保存原因。工作区内能够确定的目标仍需检查编辑是否过期、是否在方案范围内。需要可核对的编辑记录时使用编辑工具，shell 写入按实际取得的信息记录。

## PostToolUse 与失败事件

TaskBelay 根据工具调用编号查找前置记录。编辑请求与实际内容一致时，保存为 Agent 编辑。非只读命令结束后比较前后观察。两次观察完整且 HEAD 和分支一致时，期间发现的变化可记为 Agent 命令写入，只用于建议。用户恰在命令运行期间保存的内容仍无法区分。缺少前置观察、HEAD 更新或切换了分支时，本次差异全部作为工作区更新。

验证结果按可取得的信息保存。运行结果、执行后的工作区状态和输出先写入验证记录，再在已发布的索引里识别输出中的用例名；识别未完成不影响这条记录。只读命令只记录结果。Claude Code 的成功和失败事件可核对退出状态；Codex 的结构化结果读取退出码或运行中的 session。

Codex 的部分版本只在后置 hook 中提供 stdout。输入带有 `transcript_path` 时，字符串结果按 stdout 处理，从该文件末尾最多读取 4 MiB 的完整记录，只接受已知的 `CommandExecution` 完成事件。事件的 session、turn、工具调用编号及成功或失败状态必须一致，hook 收到的输出须与完成事件的输出相同或结尾一致，才采用退出码；命令先让出控制权或输出被宿主截去中段时，hook 只拿到后面的部分。会话格式不是稳定协议；文件不可读、记录尚未写入、超出读取范围、格式变化或内容不匹配时仍为未知，不把 stdout 当作 Codex 的结果头部。

未提供会话路径时，识别包含耗时、退出状态和输出分隔的完整 Codex 结果头部；JSON 字符串中的结构化结果须同时具有耗时和输出字段。普通命令输出里的退出码或成功字样不作为结果依据。长命令只有可核对的启动结果、还在后台运行时记为正在运行；同一工具调用后续收到结束事件时补齐结果，拿不到结束信息就保留未知，不能作为成功结果复用。

算法将结果写入验证输入记录，更新命令的不稳定标记、注释敏感性和通过规律，规则见 [过度测试判定](over-testing.md)。退出码为零只说明命令成功，不能据此认定业务验收已经通过。

## Stop 与交付

```mermaid
flowchart TB
    A[收到 Stop 事件] --> B{本轮或同一会话是否有可继续处理的交付请求}
    B -->|没有| C[保存进度和已有提示，结束]
    B -->|有| D[读取请求中的任务和有效输入]
    D --> E[复用有效结果，或运行交付检查]
    E --> F{检查期间输入是否有变化}
    F -->|有| G[保存历史结果，任务继续进行]
    F -->|没有| H{是否有尚未处理的拦截问题}
    H -->|是且未拦过| I[反馈给 Agent，保留本轮交付请求]
    H -->|否或已反馈| J[保存完成状态和交付报告]
```

- `task_finish` 只提交交付请求，Stop 使用请求中的任务编号，不因待交付状态走“没有任务”的分支。
- Stop 失败后，同一 session 的当前轮次可以接续当前任务尚未完成的交付请求，并重新核对输入版本；不接管其他 session、其他任务或迟到轮次的请求。
- 普通分析、问问题或等待确认时，不启动完整交付检查和动态测试，保留这轮观察。没有记录需求却已有 Agent 编辑时，在下一次任务接续摘要提示补记，不使用 `BlockStop` 强制继续。
- 交付先更新变更簇、需求相关区和静态信号；项目开启或本次请求了动态检查时，读取 MCP 进程中撤回试验和变异的结果，尚未完成时在预算内继续。结果不完整时使用已完成部分，不自动创建下一轮运行。逻辑记录与报告字段见 [任务账本](requirements-ledger.md)。
- 同一问题按稳定编号对每个 session 最多拦一次，每轮最多三次。问题位置或其他文件发生变化，不会改变问题标识。
- 首次反馈后保留本轮交付请求；输入未变就再次 Stop 时，展示未处理问题并完成。Agent 继续修改或更新需求、授权时，请求过期，任务回到进行中，之后重新申请交付并复用有效分析。
- 交付请求过期，或检查期间用户、其他 session 修改了输入时，通过 `systemMessage` 立即告诉用户任务仍在进行中，需要按当前内容重新申请交付；旧结果保留为历史，本轮不使用旧结果拦截。TaskBelay 根据任务记录整理尚未解决的事项，随下一次接续摘要提供给 Agent。
- 交付完成后显示需求、位置、用户决定、工作区更新及检查完成情况；同时列出“未经要求的新增”和“由 Agent 说明对应需求的新增”，让用户看到原话之外的对外新增项。普通提示去重后最多展示五条，其余可用 `taskbelay status` 查看。

Stop 超时设为 300 秒，动态检查自己的预算是 120 秒。只提示模式下同样生成报告，不拒绝工具调用或阻止结束。

以上为当前预算上限，不是必须用满的执行时间。算法分别报告检查是否完成、判断依据适用的范围、检查结论，以及实际采取了什么动作。交付流程结束时仍可能有未验证的事项，不能把任务结束等同于全部检查通过。

## Agent 与平台能力

适配器按已支持的输入字段读写，缺失的退出码或结果保留未知。Codex 的 write_stdin 不重复触发原命令的前置事件，命令完成时可以补发原调用的后置事件；交付检查仍以观察到的实际文件为准。

| 项目 | Claude Code | Codex |
| --- | --- | --- |
| hook 配置 | `~/.claude/settings.json` 的 hooks | `~/.codex/hooks.json` |
| MCP 注册 | `claude mcp add --scope user` | `codex mcp add` |
| hook 生效 | 安装后新 session | 用户用 `/hooks` 确认信任后 |
| 轮次 | prompt_id | turn_id |
| 验证结果 | 按成功、失败或中断事件记录 | 没有可核对退出状态时记未知 |

## 三个平台的差异

macOS 和 Linux 的做法相同，Windows 不同：

| 项目 | macOS、Linux | Windows |
| --- | --- | --- |
| 用户目录 | `~` | `%USERPROFILE%` |
| TaskBelay 程序 | `taskbelay` | `taskbelay.exe` |
| Agent 运行命令用的 shell | Claude Code、Codex 都是用户的 bash 或 zsh | Claude Code 是 Git Bash 或 PowerShell，Codex 是 PowerShell |
| Codex 运行 hook 命令用的 shell | bash 或 zsh，用 `-c` 运行 | PowerShell，用 `-NoProfile -Command` 运行 |
| Codex 的 hook 条目 | `command` 写成 `'/Users/dev/go/bin/taskbelay' hook codex stop` | `command` 写成 `& 'C:\Users\dev\go\bin\taskbelay.exe' hook codex stop` |

Claude Code 的 hook 条目在三个平台上写法相同，用 exec 形式：`command` 写 taskbelay 的绝对路径，`args` 写参数。Claude Code 直接启动这个程序，不经过 shell，所以路径里有空格、中文或引号都原样写，也省掉了每次启动 shell 的时间。exec 形式从 Claude Code 2.1.139 开始支持。

Codex 用自己运行命令的那个 shell 运行 hook 命令，所以 Codex 的 hook 条目按这个 shell 的写法来写。程序路径里可能有空格、中文或引号，比如用户名带空格，所以路径一律加单引号：POSIX 写法里，路径中的单引号写成 `'\''`；PowerShell 写法里，路径中的单引号写成两个单引号，PowerShell 当作单引号的几个弯引号也一样。

在 Windows 的 WSL 里运行的 Agent 是 Linux 程序，按 Linux 接入：TaskBelay 装在 WSL 里，配置和数据都在 WSL 的用户目录下。

## 安装和卸载

### 安装程序

macOS、Linux 在项目目录执行 `bash install.sh --source`；Windows 在 PowerShell 中执行 `& .\install.ps1 -Source`。源码安装需要 Go 1.26 或更高，构建成功后才替换已安装程序。

发布版本后，可直接下载脚本安装预编译程序，无需 Go：

```sh
curl -fsSL https://github.com/Innocent-children/TaskBelay/releases/latest/download/install.sh | bash
```

```powershell
irm https://github.com/Innocent-children/TaskBelay/releases/latest/download/install.ps1 | iex
```

脚本默认选择最新发布版本；`install.sh --version vX.Y.Z` 或 `install.ps1 -Version vX.Y.Z` 可指定正式版本。`release-manifest.json` 保存 `version`、`revision` 字符串和 `assets` 文件名数组。安装器先核对元数据与目标平台的发布文件，再从该标签的固定下载地址取得 `SHA256SUMS` 和程序，避免发布版本切换时混用文件。下载、校验或程序版本核对失败时保留旧版；缺少元数据的旧发布包不按新格式安装，可改用源码安装。

发布文件包括 `taskbelay-<系统>-<架构>` 六种程序、`install.sh`、`install.ps1`、`SHA256SUMS` 和 `release-manifest.json`。Windows 程序增加 `.exe` 后缀，系统为 darwin、linux、windows，架构为 amd64、arm64。校验清单包括程序和安装脚本，程序版本与元数据使用同一个标签。源码安装中的 `@new` 指向开发分支；`@latest` 按 Go 发布版本选择，不保证包含 `new` 的改动。

macOS、Linux 安装到 `~/.local/bin/taskbelay`，将该目录加入 bash、zsh 的用户启动文件；Windows 安装到 `%LOCALAPPDATA%\Programs\TaskBelay\taskbelay.exe`，加入用户 PATH。重新打开终端后运行 `taskbelay`；安装结束也会打印当前终端可直接运行的完整路径。脚本只安装程序，Agent 接入由用户在菜单中选择。

### 交互管理

`taskbelay` 在交互终端中打开菜单，`taskbelay menu` 显式打开。上下方向键选择，右方向键或回车进入，左方向键、Esc 或 `q` 返回，空格勾选，Ctrl+C 退出菜单。Agent 列表可多选，回车执行；未勾选时不修改接入。菜单包括：

- 安装或修复 Claude Code、Codex 接入，以及移除接入。
- 查看本机版本、程序和数据位置、Agent 命令、TaskBelay hook 和 MCP 注册。发现配置不完整时说明具体原因；本机诊断不证明 Agent 已信任或实际执行 hook。
- 在用户、项目或当前任务范围设置工作模式、拦截强度和动态检查。显示有效设置及来源；项目、任务范围沿用现有命令的工作区要求。
- 查看当前任务和最近检查，主动运行静态检查；一次动态检查需要单独确认。
- 更新程序，或移除两个 Agent 的接入后卸载程序。卸载默认取消，确认后才执行；接入移除失败时保留程序并说明未完成部分。

无交互终端时不读取按键；无参数显示帮助，显式 `menu` 返回说明。子命令始终可单独调用。`taskbelay update` 更新程序，完成后重新打开菜单。`taskbelay uninstall --all --yes` 用于明确要求无交互卸载的脚本。

更新器使用 Go 语义版本比较正式版本与伪版本，例如 `v0.5.0 < v0.5.1-0.20261027023236-bee390c559a8 < v0.5.1`。目标更高才下载替换，相同或更低时返回无需更新。当前为 `dev`、`(devel)` 或无效版本时显示版本与构建信息，结束自动更新；用户可用安装脚本明确选择发布包或当前源码。工作区修改标记另行显示，不改变合法版本的排序。

每个 MCP 在启动时记录程序路径和文件身份，每五秒检查一次。该路径指向另一份常规文件时，MCP 关闭自己的协议连接，等待已接收的请求返回后退出。源码安装、发布安装、内置更新和 `go install` 替换同一路径的程序时都适用。安装失败而原文件仍在、路径暂时不存在或无法核对文件身份时，原 MCP 继续服务。其他安装位置的进程独立运行。Agent 下次连接会启动新版；宿主若未自行重连，用户可以重连 MCP 或重新打开会话。首次从没有这项能力的版本升级时，原进程需要重连一次。

程序使用 schema 14。打开账本时核对版本，在同一事务中更新结构。失败时保留完整记录，下次打开可重试。旧分析索引在转换期间仍可查询。版本不兼容的进程停止读写账本，hook 自身失败时仍允许 Agent 的原操作继续。安装和更新保留账本及 Agent 配置。

安装脚本在程序旁写入 `.taskbelay-install` 标记。自更新、自卸载会核对用户安装位置和标记，不删除通过 Go 安装或由其他包管理器管理的程序。macOS、Linux 的旧入口若是符号链接，安装脚本通过本机 Go 的构建信息确认它指向 TaskBelay 后，可用新程序替换链接本身，原 Go 程序保留；其他链接和未标记的普通文件不覆盖。

Windows 上，运行中的旧程序先重命名，再由新程序替换原路径。若旧文件暂时无法删除，程序会显示文件路径，待使用它的进程退出后可手动删除。任务账本、日志、用户原有配置及用户 PATH 中的目录条目保留；卸载不递归删除用户目录。

### 接入和移除 Agent

**安装 Claude Code 的接入：`taskbelay install claude`。**

1. 在 Claude Code 的 `settings.json` 的 `hooks` 里加入 8 个条目：SessionStart、UserPromptSubmit、Stop 各一个；PreToolUse 三个，分别匹配 `Bash|PowerShell`、`Edit|Write|NotebookEdit`、`^mcp__taskbelay__.*$`，第三个只记录 MCP 调用身份；PostToolUse、PostToolUseFailure 各一个，匹配 `Bash|PowerShell|Edit|Write|NotebookEdit`。每个条目用 exec 形式写 taskbelay 的绝对路径和参数，绝对路径保证 Agent 运行时找得到它。Stop 的超时设为 300 秒，其他设为 30 秒。用户原有的配置保持不动。
2. 运行 `claude mcp add --scope user taskbelay -- <taskbelay 的绝对路径> mcp`。

写入的 hook 条目是这个样子（只列出两个，路径以 macOS 为例，Windows 上是 taskbelay.exe 的路径）：

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|PowerShell",
        "hooks": [
          { "type": "command", "command": "/Users/dev/go/bin/taskbelay", "args": ["hook", "claude", "pre-tool-use"], "timeout": 30 }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "/Users/dev/go/bin/taskbelay", "args": ["hook", "claude", "stop"], "timeout": 300 }
        ]
      }
    ]
  }
}
```

**安装 Codex 的接入：`taskbelay install codex`。**

1. 在 Codex 的 `hooks.json` 里加入 6 个条目：SessionStart、UserPromptSubmit、Stop 各一个；PreToolUse 两个，一个匹配 `Bash`，一个匹配 `apply_patch`；PostToolUse 一个，匹配 `Bash|apply_patch`。命令的写法见上一节“三个平台的差异”，超时和 Claude Code 相同。
2. 运行 `codex mcp add taskbelay -- <taskbelay 的绝对路径> mcp`。
3. 提示用户打开 Codex，用 `/hooks` 确认信任 TaskBelay 的 hook。不确认的话，hook 不会运行。

两条安装命令都可以重复运行，重复运行只更新 TaskBelay 自己的条目。安装写的是用户级配置，对这台电脑上所有的 git 工作区生效；配置文件是符号链接时，修改它指向的文件。安装前先检查版本：git 要 2.5 或更高，安装 Claude Code 的接入时 Claude Code 要 2.1.139 或更高，不满足时报错并说明要求，不写入任何配置。安装时找不到 `claude` 或 `codex` 命令，TaskBelay 会报错，并打印需要手动写入的配置。

Codex 允许在 `hooks.json` 或 `config.toml` 中声明 hook。同一个配置层同时包含两种事件定义时，两边都会加载，但启动时会提示 `prefer a single representation for this layer`。TaskBelay 安装器只维护 `hooks.json`，不会自动迁移用户在 `config.toml` 中定义的 hook。遇到该提示时，先备份两个文件，将原有事件组完整合并到 `hooks.json`，再从 `config.toml` 移除对应的事件定义；保留其他配置以及 `[hooks.state]` 中由 Codex 管理的启用和信任记录。迁移不得改变命令、匹配条件或超时，也不手工补写信任记录；来源路径变化后，按 `/hooks` 提示重新核对需要信任的条目。配置来源与信任机制见 [Codex hooks 文档](https://developers.openai.com/codex/hooks)。

**卸载：`taskbelay uninstall claude`、`taskbelay uninstall codex`。** 删除命令里带 taskbelay 路径和 `hook` 子命令的条目，运行 `claude mcp remove --scope user taskbelay` 或 `codex mcp remove taskbelay`。任务账本里的数据保留。

## TaskBelay 出错时

TaskBelay 自己出了问题，比如数据库打不开、git 命令失败、代码加载失败、检查超时，hook 正常退出，Agent 的原操作照常进行。

错误写进日志（位置见 [系统架构](product-architecture.md)）。Stop 时检查没能完成的，用 `systemMessage` 告诉用户“TaskBelay 这次没有完成检查”和原因，`taskbelay status` 里也能看到。

当前目录不在 git 工作区里时，所有 hook 直接退出，MCP 工具返回“当前目录不在 git 工作区里”。

## 参考资料

- [Claude Code hooks 文档](https://code.claude.com/docs/en/hooks)
- [Claude Code 工具说明（含 PowerShell 工具）](https://code.claude.com/docs/en/tools-reference)
- [Codex hooks 文档](https://developers.openai.com/codex/hooks)
- [Codex MCP 文档](https://developers.openai.com/codex/mcp)

## 会话动作记录

hook 成功输出提示或拒绝结果后，才记录为已提示或已拦截。如果只是生成了处理决定，或者在生成协议输出、写入 stdout 时失败，都不能记作已经提示或拦截。MCP 的需求、笔记、任务与方案数据实际保存成功后，才记录相应动作；准备好 MCP 返回摘要，不等于结果已经通过 stdio 送达。

检查记录区分新执行、复用已有结果和结果未知。验证结果只使用正常后置事件已经提供的状态和退出结果，不为补充记录增加轮询或重跑。

用户自行打开 `~/.taskbelay/session-records/`（Windows 为 `%USERPROFILE%\.taskbelay\session-records\`）阅读，无需让 Agent 查询，也不在对话中自动播报。会话动作记录与 debug 独立；无法核对会话标识时，不猜测应写入哪个文件。写入失败时使用现有错误日志记录，原业务结果和拦截决定保持不变。
