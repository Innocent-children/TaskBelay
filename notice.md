# 平台差异带来的问题

本文记录 Windows、macOS、Linux 三个平台的差异可能给 TaskBelay 带来的问题。每条写明涉及哪些平台、问题是什么、会造成什么影响、怎样处理、在哪一组验证。验证过的条目，把状态改成“已验证”，并写上验证结论；验证发现处理办法要改的，同时改掉对应的文档。

| 编号 | 问题 | 涉及的平台 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| P1 | Claude Code 的 hook 经过 PowerShell 运行 | Windows | 第 2 组 | 未验证 |
| P2 | Codex 用什么 shell 运行 hook 命令 | Windows | 第 2 组 | 未验证 |
| P3 | 程序路径里有空格、中文或引号 | 三个平台，Windows 上最常见 | 第 2 组 | 未验证 |
| P4 | 同一个路径有几种写法 | 三个平台 | 第 2、9 组 | 未验证 |
| P5 | WSL 和 Windows 两侧各有一本账本 | Windows 的 WSL | 第 8 组 | 未验证 |
| P6 | 启动进程慢 | Windows | 第 2、7 组 | 未验证 |
| P7 | 换行符 | Windows | 第 4 组 | 未验证 |
| P8 | Codex 的沙箱 | 三个平台 | 第 2、3 组 | 未验证 |
| P9 | Claude Code 的 PowerShell 工具 | macOS、Linux | 第 7 组 | 未验证 |
| P10 | 系统自带的 git 版本 | Linux 为主 | 第 2 组 | 未验证 |

## P1 Claude Code 的 hook 经过 PowerShell 运行

- **平台**：Windows，Claude Code。
- **问题**：hook 条目指定了 `"shell": "powershell"`，由 PowerShell 启动 `taskbelay.exe`。有三点要实测：PowerShell 会不会把自己收到的标准输入转交给 `taskbelay.exe`；PowerShell 5.1 转发原生程序的输出时，会不会按系统代码页重新编码，中文 Windows 的代码页是 GBK；每启动一次 PowerShell 要多久。
- **影响**：标准输入传不过去，所有 hook 都失效；输出被转码，说明里的中文变成乱码，JSON 也可能被破坏；启动慢，Agent 每运行一条命令都要多等这段时间。
- **处理**：hook 输出的 JSON 只用 ASCII 字符，中文写成 `\u` 转义，转码问题就不存在了，这一条已经写进接入文档。实测发现标准输入传不过去或者启动太慢时，hook 条目改为不指定 shell，让 Claude Code 用 Git Bash 运行：TaskBelay 本来就要求装 Git for Windows，Git Bash 会原样传递输入输出。
- **验证**：第 2 组。在 Windows 上安装后发一条中文消息，`taskbelay status` 能看到原文；Agent 收到的中文说明显示正常；测量一次 PreToolUse 从 Agent 发起到收到结果的总耗时。

## P2 Codex 用什么 shell 运行 hook 命令

- **平台**：Windows，Codex。
- **问题**：Codex 的文档只说 `commandWindows` 是 Windows 上专用的命令写法，没写由什么 shell 运行，是 cmd 还是 PowerShell。标准输入怎样传递、输出会不会被转码、路径的引号怎样写，都取决于这一点。
- **影响**：和 P1 相同。
- **处理**：JSON 输出只用 ASCII，和 P1 相同。按实测结果确定 `commandWindows` 里路径的引号写法，写回接入文档。
- **验证**：第 2 组，验证内容和 P1 相同。

## P3 程序路径里有空格、中文或引号

- **平台**：三个平台。Windows 上最常见，用户名里带空格或中文时，`go install` 装出来的程序路径里就有，比如 `C:\Users\张 三\go\bin\taskbelay.exe`。
- **问题**：hook 命令里写的是 taskbelay 的绝对路径，引号写错，shell 就找不到程序。
- **影响**：所有 hook 失效。
- **处理**：路径一律加单引号。POSIX 写法里，路径中的单引号写成 `'\''`；PowerShell 写法里，路径中的单引号写成两个单引号。这一条已经写进接入文档。
- **验证**：第 2 组。在用户名带空格和中文的系统账户下安装并使用。

## P4 同一个路径有几种写法

- **平台**：三个平台。Windows 和 macOS 默认不区分大小写，Linux 区分；macOS 上 `/tmp` 是 `/private/tmp` 的符号链接；Windows 上盘符可能是大写或小写，分隔符可能是 `\` 也可能是 `/`。
- **问题**：hook 输入里的 `cwd`、编辑工具给出的 `file_path`、git 给出的根目录，写法不一定一致。
- **影响**：同一个工作区被记成两个，任务和接续摘要对不上；半自动模式判断“文件在不在工作区里”时出错，该拦的编辑没拦，或者拦错了。
- **处理**：按架构文档“平台”一节的路径比较规则处理：工作区一律取 git 给出的根目录并解析符号链接；比较前统一成正斜杠；Windows 和 macOS 上不区分大小写，Linux 上区分。macOS 也可以把磁盘格式化成区分大小写的，这种情况很少见；按不区分处理时，两个只差大小写的目录会被当成同一个。
- **验证**：第 2 组验证工作区的识别，用大小写不同、经过符号链接的路径进入同一个仓库；第 9 组验证编辑拦截时的路径判断。

## P5 WSL 和 Windows 两侧各有一本账本

- **平台**：Windows 的 WSL。
- **问题**：WSL 里的 Agent 用 Linux 版 TaskBelay，数据在 WSL 的用户目录下；Windows 上的 Agent 用 Windows 版，数据在 Windows 的用户目录下。同一个仓库在两边的路径也不同，比如 `/mnt/c/repo` 和 `C:\repo`。另外，WSL 里访问 `/mnt/c` 下的仓库很慢，算工作区状态时的 `git add -A` 会明显变慢。
- **影响**：WSL 里的 Claude Code 和 Windows 上的 Codex 处理同一个仓库时，彼此看不到对方的任务，接续摘要接不上；仓库放在 `/mnt/c` 下时，每次 hook 都更慢。
- **处理**：产品文档已经写明，两个 Agent 都在 WSL 里，或者都在 Windows 上，才共用同一本任务账本。在 WSL 里使用时，仓库放在 WSL 自己的文件系统里。
- **验证**：第 8 组。在 WSL 里装好两个 Agent，做一次接续；分别测量仓库在 WSL 文件系统和 `/mnt/c` 下时算工作区状态的耗时。

## P6 启动进程慢

- **平台**：Windows。
- **问题**：Windows 创建进程比 macOS 和 Linux 慢，杀毒软件还会扫描新启动的程序；hook 命令外面那层 shell 也要启动时间。
- **影响**：架构文档里“不含验证命令的 PreToolUse 在 50 毫秒内返回”的预算可能达不到，Agent 每运行一条命令都会变慢。
- **处理**：快速路径上只做必要的事：判断命令里有没有验证命令之前，不打开账本，不运行 git。实测达不到预算时，按实测结果调整 Windows 上的预算，写回架构文档。
- **验证**：第 2 组测量三个平台上一次空的 hook 调用的耗时；第 7 组测量不含验证命令的 PreToolUse 的耗时。

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
- **处理**：确定 TaskBelay 支持的最低 git 版本，写进架构文档的运行时要求；安装时检查 git 版本，太老就报错并说明要求。
- **验证**：第 2 组。在自带较老 git 的 Linux 发行版上安装。
