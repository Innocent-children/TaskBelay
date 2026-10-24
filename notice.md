# 平台差异带来的问题

本文记录 Windows、macOS、Linux 三个平台的差异可能给 TaskBelay 带来的问题。每条写明涉及哪些平台、问题是什么、会造成什么影响、怎样处理、在哪一组验证。验证过的条目，把状态改成“已验证”，并写上验证结论；验证发现处理办法要改的，同时改掉对应的文档。

| 编号 | 问题 | 涉及的平台 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| P1 | Codex 的 hook 经过 PowerShell 运行 | Windows | 第 2 组 | 未验证 |
| P2 | Codex 用什么 shell 运行 hook 命令 | 三个平台 | 第 2 组 | 已验证 |
| P3 | 程序路径里有空格、中文或引号 | 三个平台，Windows 上最常见 | 第 2 组 | macOS 已验证 |
| P4 | 同一个路径有几种写法 | 三个平台 | 第 2、9 组 | 第 2 组的部分 macOS 已验证 |
| P5 | WSL 和 Windows 两侧各有一本账本 | Windows 的 WSL | 第 8 组 | 未验证 |
| P6 | 启动进程慢 | Windows | 第 2、7 组 | macOS 已测 |
| P7 | 换行符 | Windows | 第 4 组 | 未验证 |
| P8 | Codex 的沙箱 | 三个平台 | 第 2、3 组 | 未验证 |
| P9 | Claude Code 的 PowerShell 工具 | macOS、Linux | 第 7 组 | 未验证 |
| P10 | 系统自带的 git 版本 | Linux 为主 | 第 2 组 | 已处理，未在老版本上验证 |

## P1 Codex 的 hook 经过 PowerShell 运行

- **平台**：Windows，Codex。Claude Code 的 hook 用 exec 形式，不经过 shell，没有这个问题。
- **问题**：Windows 上 Codex 用 `powershell -NoProfile -Command` 运行 hook 命令（见 P2）。有三点要实测：PowerShell 会不会把自己收到的标准输入转交给 `taskbelay.exe`；PowerShell 5.1 转发原生程序的输出时，会不会按系统代码页重新编码，中文 Windows 的代码页是 GBK；每启动一次 PowerShell 要多久。
- **影响**：标准输入传不过去，所有 hook 都失效；输出被转码，说明里的中文变成乱码，JSON 也可能被破坏；启动慢，Agent 每运行一条命令都要多等这段时间。
- **处理**：hook 输出的 JSON 只用 ASCII 字符，中文写成 `\u` 转义，转码问题就不存在了，这一条已经写进接入文档。PowerShell 调用原生程序、又没有给它管道输入时，原生程序继承 PowerShell 自己的标准输入，预计能传过去，要在 Windows 上实测确认。
- **验证**：第 2 组。在 Windows 上安装后，在 Codex 里发一条中文消息，`taskbelay status` 能看到原文；Agent 收到的中文说明显示正常；测量一次 PreToolUse 从 Codex 发起到收到结果的总耗时。

## P2 Codex 用什么 shell 运行 hook 命令

- **平台**：三个平台，Codex。
- **问题**：Codex 的文档只说 `commandWindows` 是 Windows 上专用的命令写法，没写 hook 命令由什么 shell 运行。标准输入怎样传递、输出会不会被转码、路径的引号怎样写，都取决于这一点。
- **影响**：引号写法和 shell 对不上，hook 找不到程序，全部失效。
- **处理**：按 Codex 源码确定写法。Windows 上的 `command` 写成 PowerShell 写法，不另写 `commandWindows`：Windows 上 Codex 只读 `commandWindows`，没有它时读 `command`，写一份就够了。
- **验证**：第 2 组，已验证，依据是 Codex 源码（`codex-rs/core/src/session/mod.rs` 的 `build_hooks_config`、`codex-rs/core/src/shell.rs` 的 `derive_exec_args`、`codex-rs/hooks/src/engine/command_runner.rs` 的 `build_command`）。结论：Codex 用 session 自己运行命令的 shell 运行 hook 命令，不是登录 shell；bash、zsh 用 `-c`，PowerShell 用 `-NoProfile -Command`。拿不到 session 的 shell 时，Windows 上退回 `%COMSPEC% /C`，也就是 cmd，macOS 和 Linux 上退回 `$SHELL -lc`。cmd 运行不了 PowerShell 写法的命令；登录 shell 会读用户的 profile，profile 往标准输出打印内容时 JSON 会被破坏。源码里没看出什么时候会拿不到 session 的 shell，按正常情况写。在 macOS 上用 zsh 的 `-c` 运行安装写入的 6 条命令，输入按 Codex 源码里的格式构造，都正确记录、正常退出、没有输出。用 `codex exec` 端到端运行需要跳过 hook 信任，这次没有运行。

## P3 程序路径里有空格、中文或引号

- **平台**：三个平台。Windows 上最常见，用户名里带空格或中文时，`go install` 装出来的程序路径里就有，比如 `C:\Users\张 三\go\bin\taskbelay.exe`。
- **问题**：hook 条目里写的是 taskbelay 的绝对路径，引号写错，shell 就找不到程序。
- **影响**：所有 hook 失效。
- **处理**：Claude Code 的 hook 用 exec 形式，路径原样写，不经过 shell。Codex 的 hook 命令里路径一律加单引号：POSIX 写法里，路径中的单引号写成 `'\''`；PowerShell 写法里，路径中的单引号和 PowerShell 当作单引号的弯引号都写两遍。这一条已经写进接入文档。
- **验证**：第 2 组。macOS 上已验证：把程序放在 `张 三's/bin/` 下安装，用 `claude -p` 真实运行一次，session 和消息原文都记下了；Codex 的命令用 bash 和 zsh 的 `-c` 都能运行。Windows 和 Linux 上还要在用户名带空格和中文的系统账户下安装并使用。

## P4 同一个路径有几种写法

- **平台**：三个平台。Windows 和 macOS 默认不区分大小写，Linux 区分；macOS 上 `/tmp` 是 `/private/tmp` 的符号链接；Windows 上盘符可能是大写或小写，分隔符可能是 `\` 也可能是 `/`。
- **问题**：hook 输入里的 `cwd`、编辑工具给出的 `file_path`、git 给出的根目录，写法不一定一致。
- **影响**：同一个工作区被记成两个，任务和接续摘要对不上；半自动模式判断“文件在不在工作区里”时出错，该拦的编辑没拦，或者拦错了。
- **处理**：按架构文档“平台”一节的路径比较规则处理：工作区一律取 git 给出的根目录并解析符号链接；比较前统一成正斜杠；Windows 和 macOS 上不区分大小写，Linux 上区分。macOS 也可以把磁盘格式化成区分大小写的，这种情况很少见；按不区分处理时，两个只差大小写的目录会被当成同一个。
- **验证**：第 2 组验证工作区的识别，用大小写不同、经过符号链接的路径进入同一个仓库；第 9 组验证编辑拦截时的路径判断。macOS 上第 2 组的部分已验证：经过 `/tmp` 符号链接、把路径改成大写进入同一个仓库，都记成同一个工作区。Linux 和 Windows 上还要验证。

## P5 WSL 和 Windows 两侧各有一本账本

- **平台**：Windows 的 WSL。
- **问题**：WSL 里的 Agent 用 Linux 版 TaskBelay，数据在 WSL 的用户目录下；Windows 上的 Agent 用 Windows 版，数据在 Windows 的用户目录下。同一个仓库在两边的路径也不同，比如 `/mnt/c/repo` 和 `C:\repo`。另外，WSL 里访问 `/mnt/c` 下的仓库很慢，算工作区状态时的 `git add -A` 会明显变慢。
- **影响**：WSL 里的 Claude Code 和 Windows 上的 Codex 处理同一个仓库时，彼此看不到对方的任务，接续摘要接不上；仓库放在 `/mnt/c` 下时，每次 hook 都更慢。
- **处理**：产品文档已经写明，两个 Agent 都在 WSL 里，或者都在 Windows 上，才共用同一本任务账本。在 WSL 里使用时，仓库放在 WSL 自己的文件系统里。
- **验证**：第 8 组。在 WSL 里装好两个 Agent，做一次接续；分别测量仓库在 WSL 文件系统和 `/mnt/c` 下时算工作区状态的耗时。

## P6 启动进程慢

- **平台**：Windows。
- **问题**：Windows 创建进程比 macOS 和 Linux 慢，杀毒软件还会扫描新启动的程序。Codex 的 hook 命令外面还有一层 PowerShell，也要启动时间；Claude Code 的 hook 不经过 shell，没有这一层。
- **影响**：架构文档里“不含验证命令的 PreToolUse 在 50 毫秒内返回”的预算可能达不到，Agent 每运行一条命令都会变慢。
- **处理**：快速路径上只做必要的事：找到工作区根目录要运行一次 `git rev-parse`，更新 session 记录要写一次账本，判断命令里有没有验证命令之前不再做别的。实测达不到预算时，按实测结果调整 Windows 上的预算，写回架构文档。
- **验证**：第 2 组测量三个平台上一次 hook 调用的耗时；第 7 组测量不含验证命令的 PreToolUse 的耗时。macOS 上已测：直接启动 taskbelay 处理一次 PreToolUse，20 次的中位数约 12 毫秒，最慢约 14 毫秒。Linux 和 Windows 上还要测。

## P7 换行符

- **平台**：Windows。
- **问题**：Git for Windows 默认 `core.autocrlf=true`，工作区里的文件是 CRLF，对象库里是 LF。工作区状态和 diff 按对象库里的内容算，Go 代码分析和覆盖率读的是工作区里的文件。
- **影响**：CRLF 和 LF 只差行尾，不改变行数，两边的行号一一对应，预计没有影响。仓库用 `.gitattributes` 对某些文件做了特殊的换行转换时，diff 的行号可能和工作区文件对不上。
- **处理**：例子集里加一个 `core.autocrlf=true` 的仓库，确认改动声明和行号正确。
- **验证**：第 4 组。

## P8 Codex 的沙箱

- **平台**：三个平台，Codex 在 macOS、Linux、Windows 上的沙箱实现各不相同。
- **问题**：Codex 在沙箱里运行 Agent 的命令。hook 命令和 MCP server 是不是也在沙箱里运行，能不能写用户目录下的 `.taskbelay`，能不能在工作区的 git 对象库里写入快照，官方文档没写。
- **影响**：写不进账本或 git 对象库时，TaskBelay 按“出错时照常放行”处理，Agent 不会被卡住，但 TaskBelay 的功能全部失效。
- **处理**：实测。受限时，按 Codex 的配置让 hook 和 MCP server 能写这两个地方，把配置方法写进接入文档的安装步骤。
- **验证**：第 2 组验证 hook，第 3 组验证 MCP server，三个平台都要验证。

## P9 Claude Code 的 PowerShell 工具

- **平台**：macOS、Linux。
- **问题**：用户装了 PowerShell 7 并打开 Claude Code 的 PowerShell 工具后，在 macOS 和 Linux 上也会出现 PowerShell 写法的命令。
- **影响**：如果按平台猜命令写法，这些命令会按 POSIX 解析，验证命令识别错误。
- **处理**：命令写法按工具名判断，不按平台判断，这一条已经写进接入文档。
- **验证**：第 7 组。在 macOS 或 Linux 上打开 PowerShell 工具，运行一条 PowerShell 写法的验证命令，确认被识别和记录。

## P10 系统自带的 git 版本

- **平台**：Linux 为主。部分 Linux 发行版自带的 git 版本较老。
- **问题**：TaskBelay 用到的 `git rev-parse --git-path`、`git commit-tree --no-gpg-sign` 等命令和参数，需要较新的 git 才支持。
- **影响**：git 太老时，算工作区状态或保存快照会失败，TaskBelay 的功能失效。
- **处理**：已处理。TaskBelay 用到的参数里，`git rev-parse --git-path` 最晚出现，从 git 2.5 开始有；`git commit-tree --no-gpg-sign` 从 2.0 开始有。最低版本定为 2.5，写进了架构文档的运行时要求；安装时检查 git 版本，太老就报错并说明要求，不写入配置。
- **验证**：第 2 组。在自带较老 git 的 Linux 发行版上安装，确认安装时报错。
