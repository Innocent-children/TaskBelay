# 验收运行与证据

本目录按 `docs/product-evaluation.md` 验证规则、开发流程和实际使用效果。内部测试、真实 Agent 接入、目标平台运行及产品收益分别记录；任何一层通过都不能代替其他层。

## 规则资料包

`cases/OD-*`、`cases/OT-*`、`cases/SA-*` 对应核心文档的例子，子目录是补充的放行或例外场景；`cases/LANG-*` 核对语言和文件级能力边界。每个资料包有 base、change.patch、ledger.yaml、expected.yaml 和 README.md。

两份 YAML 使用 YAML 1.2 兼容的 JSON 写法。`TestCasePacks` 用标准库严格读取字段，从 base 建立临时 Git 工作区，在独立目录核对并应用补丁，然后通过现有任务、需求、方案、编辑和检查入口运行。Go 小项目有独立 go.mod，不参与产品包的递归编译。

expected.yaml 核对检查完成情况、指定规则的问题位置和事实、处理状态、拦截前提、编辑拦截数及用户内容。交付拦截案例还核对第二次 Stop 不重复阻断、检查观察与问题身份保持。命令案例使用明确标出的构造事件和可复用范围证明；动态案例使用当前机器实际安装的 Go 工具，在固定快照副本中运行基线、覆盖及变异。

```sh
GOTOOLCHAIN=go1.26.0 go test ./eval/... -json -count=1 > /tmp/taskbelay-eval.jsonl
GOTOOLCHAIN=go1.26.0 go test ./eval -run '^TestCasePacks/OD-07$' -count=1
```

`complete`、`partial`、`expired` 是产品检查结果；测试 pass 表示结果符合预期。例如未知公开约定对应 partial，检查期间的用户修改对应 expired，不能把这两类案例的测试通过写成产品检查全部完成。OT-14 的准备门目前使用 POSIX，Windows 子例明确跳过。

Python、JavaScript、Java、Rust 原生动态流程沿用现有可选测试工具目录。未设置目录时会明确 skip；需要完整语言验收时必须提供实际工具并保留其版本与执行结果，不能把 skip 计入通过。

## 开发流程与本地结果

`coverage.json` 列出二十项规则、WF-01 至 WF-18 和十二项语言／文件范围的测试入口。它是证据索引，不是通过声明；已列测试未运行时显示 not_run，仍有明确缺口时显示 partial。十八个流程均已有实际事件计数入口；只有本次日志包含完整、通过的事件证据才给出额外审批、重复提示和无效阻断计数，报告中的 null 表示未测量。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s eval/scripts -p 'test_*.py'
python3 eval/scripts/collect_local_results.py /tmp/taskbelay-eval.jsonl \
  --output /tmp/taskbelay-local-result.json
```

结果收集器保留每次执行的 pass、fail、skip，不用重试通过覆盖先前失败；保存资料包文件摘要和日志来源。完整验收还包括其他平台原生运行、真实 Agent 行为、性能分位数和使用成本。

语言矩阵的分层依据见 `language-matrix.md`。原生框架结果通过 `--native-logs /tmp/taskbelay-native.jsonl` 单独提供；普通轮次里的可选测试跳过继续保留，只有各项语言事实、关联规则和所需原生证据全部通过，该语言行才通过。结果目录中的 Go 源码留证采用 `.go.txt` 后缀，避免递归测试把它们当作独立包。

## 完整入口的性能测量

`TestPerformanceProfiles` 默认跳过。启用后，每个样本都启动独立 hook 测试进程，计时包括进程启动；任务准备在计时前完成，静态 Stop 另存检查阶段耗时、完成状态与未完成原因。冷启动、已有账本、新文件和数据库竞争使用 SessionStart，两个大仓库样本均包含约五万行 Go 代码。

```sh
TASKBELAY_PERFORMANCE_SAMPLES=20 GOTOOLCHAIN=go1.26.0 \
  go test ./cmd/taskbelay -run '^TestPerformanceProfiles$' -count=1 -timeout=15m -json \
  > /tmp/taskbelay-performance.jsonl
python3 eval/scripts/collect_performance.py /tmp/taskbelay-performance.jsonl \
  --output /tmp/taskbelay-performance.json
```

可用 `TASKBELAY_PERFORMANCE_PROFILE` 单独选择 `cold`、`warm`、`new-files`、`large-static`、`large-static-unknown` 或 `database-busy`。`large-static` 使用常量赋值，核对完整检查；`large-static-unknown` 保留复合赋值导致的既有分析未知。两者分别报告，partial 不能计为完整通过。性能日志只展示前八个未完成项，同时记录总数；原始定位样本另存。

正式结果每类二十个样本，P95 采用排序后的第十九个值；偶数样本的中位数取中间两项均值。一个样本只用于探测，不作分位数验收。收集器重组 Go JSON 的输出片段，核对逐样本与汇总一致，再复算统计；不完整或矛盾的日志报错。

数据库竞争场景持有临时账本写事务。`runHook` 将内部错误记录后正常退出，让原工具继续；样本分别保存进程错误与本次诊断，`degraded` 统计有诊断的样本数。早期日志未采集诊断时明确显示 `not_recorded`，不能把零退出写成无故障或原工具被阻断。测试 pass 只说明测量流程结束，仍须检查进程错误、降级、超预算数和完成状态。这些本机进程计时不代替真实模型端到端计时或其他平台结果。

## 原生 Agent 场景脚本

`scripts/run_agents.py` 运行一个已准备好的评估工作区，按 turns 顺序发送脚本消息。Claude Code 使用 stream-json，Codex 使用 JSONL；后续调用传入本次记录的明确 session 标识，不使用 `--last`。选项依据本机 Claude Code 2.1.309、codex-cli 0.163.0 帮助，以及 [Codex 非交互模式文档](https://developers.openai.com/codex/noninteractive.md)。

运行前在该评估环境按产品文档安装或关闭 TaskBelay，并完成实际需要的信任确认。脚本不修改全局 Agent 配置，也不代替 `/hooks` 确认。`database` 必须指向该环境实际使用的 taskbelay.db，`settings_files` 列出参与本次运行的 Agent、hook、MCP 和项目设置；只保存这些文件的路径和摘要，不复制内容。每轮核对版本和这些设置没有变化。

清单示例；路径和固定模型须替换为实际评估配置：

```json
{
  "task_id": "dataset-task-id",
  "agent": "codex",
  "model": "fixed-model-id",
  "enabled": true,
  "workspace": "/absolute/path/to/evaluation-checkout",
  "database": "/absolute/path/to/taskbelay.db",
  "settings_files": ["/absolute/path/to/agent-settings"],
  "arguments": [],
  "timeout_seconds": 600,
  "turns": [
    {"prompt": "按该任务的用户请求实现行为。"},
    {"prompt": "按该场景的固定步骤继续。"}
  ]
}
```

```sh
python3 eval/scripts/run_agents.py /tmp/scenario.json /tmp/new-evaluation-output
```

输出目录必须新建且位于 Agent 工作区之外，避免日志改变被评估的 diff。保留每轮输入、原始 JSON、stderr、耗时、token 原始统计、工具事件及账本中的 hook 登记。开启样本要求同一工作区和 session 有成功登记的 SessionStart、UserPrompt、Stop 和工具活动，且 CLI 本身报告实际工具调用。身份不能对应、hook 缺失、hook 失败、session 续用错误或 CLI 失败，都产生无效样本；关闭样本出现 TaskBelay hook 也无效。

可选 `command_trace` 指向本轮转发程序将写入的日志：必须位于工作区外、父目录已存在，且启动时文件尚不存在，避免把旧运行当作新执行。评估启动器按“实际工具执行留证”配置本轮 PATH；运行器记录 PATH、GOTOOLCHAIN、调用来源，以及本轮 Claude shell 和前缀环境值，结束后将原始日志固定为输出目录中的 `command-trace.jsonl`，同时保存摘要与解析结果。原始记录缺失、损坏、为空或未结束时，状态为 unknown、可用启动计数为 null；接入结果另行保留。`recorded` 只表示已观察调用都有结束或启动失败证据，非零退出仍原样保留，不能解释为验证通过或全部进程已覆盖。

脚本退出码 0 仅表示所列轮次的接入核对通过，1 表示运行或接入失败，2 表示准备或证据读取失败。它不等于实现正确，也不等于产品效果达标。未知的 Codex 原生 session 对应关系保留为失败，不按最近活动猜测。Windows 进程树清理和两个 Agent 的原生脚本运行还需目标环境验证，见 notice.md。

三十任务资料位于 `dataset/`，准备与验证方法见 `dataset/README.md`。隐藏测试留在 Agent 工作区外，在 Agent 完成后才加入；参考改动、验收通过率、补跑、半自动、双向接续、三百六十次对比和人工误判审计仍按评估文档逐项实施。任务资料原生验证不计入 Agent 对照次数。

## 候选交付验收

`scripts/assess_candidate.py` 读取已结束的 Agent 运行回执，保存工作区当前全部受管及未忽略文件、完整候选 tree 和补丁，再在独立副本中核对已有与隐藏测试。Agent 工作区、真实暂存区和其新增测试保持原样。当前接受任务集中 Go、pytest、Vitest、Maven／Surefire 和 Cargo 的固定命令；`scripts/assess_go_candidate.py` 保留原来的 Go 专用入口和参数。

```sh
python3 eval/scripts/assess_go_candidate.py eval/dataset/pflag-330 \
  /absolute/path/to/source/pflag /absolute/path/to/agent-workspace \
  /absolute/path/to/agent-output /absolute/path/to/new-assessment \
  --runtime /absolute/path/to/runtime.json
```

Go 的验证副本恢复任务起点的 `_test.go` 和 `testdata`，不采用 Agent 对这些原断言的修改，也不混入其新增测试；完整候选仍保留这些改动供之后统计。先运行已有测试，再加入隐藏补丁运行隐藏验收，隐藏补丁不得修改非测试文件。命令增加 `-count=1 -json`，避免缓存命中被计作实际执行，跳过记录继续保留。测试改写输入、超时、无法解析结果、没有测试执行或补丁冲突均不能算通过。

Python 使用相同的位置参数，例如将入口换成 `assess_candidate.py`、任务换成 `eval/dataset/itsdangerous-267`。验收副本恢复起点的 `test_*.py`、`*_test.py`、`conftest.py` 及 `tests/`、`test/` 目录内文件，隐藏补丁也只能改这些测试位置。固定的 `python -m pytest` 命令追加独立 JUnit XML 输出，记录逐例失败、错误与跳过，并核对 suite 汇总和退出码。XML 摘要随最终报告保存；原命令回执保持原样，不能只用终端中的“通过”字样补出结果。

Python 验收设置 `PYTHONDONTWRITEBYTECODE=1`。候选改变了 pytest.ini、pytest.toml（含点前缀版本）、pyproject.toml、setup.cfg 或 tox.ini 时，先保留 unknown 和具体配置路径；当前尚不能证明配置变化后原有测试仍完整收集，不把这种变化判为实现错误。全部跳过、收集中断、缺失或矛盾的 XML 同样不能算通过；部分跳过单列保留，不计作实际执行。

TypeScript 保留固定 Vitest 命令的子目录、前缀和类型检查参数，追加 JSON 报告，恢复起点测试和 runtime 中明确列出的 `test_support_files`。Java 恢复整个 `src/test/`，包括测试支撑类和资源，保留上一次 Surefire 报告，再核对本次各份 XML、控制台汇总和构建结果。两种语言的收集或构建配置变化均保留 unknown；旧报告、部分输出或退出码不能单独证明测试通过。

Rust 需要先在候选工作区之外构建语法辅助程序，并在 runtime 的 `rust_test_inputs` 字段填写其绝对路径：

```sh
GOTOOLCHAIN=go1.26.0 go build -o /absolute/path/to/rust-test-inputs ./eval/cmd/rust-test-inputs
```

Rust 验收保留固定工具链、feature 和目录，在独立副本恢复起点的测试文件、源码内测试及可定位的外部支撑输入。原有文档用例必须保留；新增文档示例的执行另列。共享文档只有模式和字节未变时才能继续，测试生成宏或输入来源无法核对时返回 unknown。Cargo.toml、Cargo.lock、工具链配置、build.rs 或 `.cargo/` 被修改时也不能认定原有测试仍完整。

Cargo 使用稳定的构建 JSON 与 libtest 普通文本，核对实际启动目标、用例、汇总和退出码，不启用 nightly 输出。编译期隐藏失败还要求已有测试通过、错误位于隐藏材料新增行且符合固定任务记录的诊断；环境或其他编译失败仍是 unknown。报告保存辅助程序摘要、语法输入、原生输出、隐藏输入及恢复记录，原生证据不能用手填状态替代。

结果分别记录接入是否有效、已有／隐藏验收状态和命令输出。`valid_correctness_sample` 表示这次正确性观察可用于比较；失败行为也是有效观察，不表示测试通过。退出码 0 对应有效观察，1 对应无效或未完成。数量、token、审批和 TaskBelay 成本尚未由此脚本统计，`metrics_evaluated` 保持 false。其他语言、内联测试、自定义验证命令及尚未核验的 Python 测试收集配置继续保留未完成。

## 正确性对比汇总

`scripts/collect_comparison.py` 只读已有证据，分别汇总已有测试与隐藏验收，不启动 Agent，也不执行候选代码。它复核原生 JSON 与接入回执、候选报告对 Agent 回执的摘要、任务资料、候选补丁和测试日志；不符或缺失的证据不能填入有效运行位置。当前按上述五种原生格式重新解析结果；Rust 还复核测试恢复和隐藏输入证据，保留不同目标、源码及编译错误的失败身份。其他格式不能仅凭手填的 pass 字段纳入。

```sh
python3 eval/scripts/collect_comparison.py /absolute/path/to/comparison.json \
  /absolute/path/to/new-comparison-result.json
```

清单使用 `schema_version: 1`，路径相对于清单所在目录。`dataset` 指向任务集 `index.json`；同目录下的各任务资料必须完整且摘要一致。`series` 中每组固定 `id`、`agent`、`version`、`model`、`platform` 和 `dynamic_checks`，`configuration.off` 与 `configuration.on` 分别保存对应原始 metadata.json 中的 `arguments`、`settings`。两个 Agent、不同模型、平台或动态开关独立报告，不合并收益。

`attempts` 保存全部尝试，每项包含唯一 `id`、所属 `series`、`task_id`、`phase`、`replicate`、`agent_directory` 与可选 `assessment_directory`。`phase` 取 `pilot`、`main` 或 `followup`，`replicate` 为 1 至 3；开启状态从 Agent 原始 metadata.json 读取。试跑不能占正式三次，补跑也单独保存。相同回执重复登记无效；同一正式位置存在多次有效观察时标为冲突，不挑选成功结果。接入无效或验收未完成后的重试仍保留全部记录。

每个任务、每种模式的三次结果齐全后才给出该项通过率；整组齐全后才给出总通过率。3/3 降到 2/3 会列为退步；通过次数或失败测试集合改变时，列出双方各三次补跑的缺口。相同测试名可能包含不同失败原因，因此失败行为仍须查阅原始输出，不能只靠测试名认定相同或已经修复。报告保留这个复核要求。

动态开关在此清单中只是分组声明；两个模式的配置是否仅有预定差异、动态设置是否实际生效，仍需运行环境证据。汇总会保存可核对的调用墙钟、原生 token 字段和转发日志成本；缓存 token 字段单列，嵌套迭代不重复累加。损坏的成本证据不抹掉已核验的正确性，采集期间原始输出变化则成本证据无效。当前不推算完整验证次数、阶段成本或人工误判率，采用结论固定为 `not_evaluated`。零退出只表示完成汇总，不表示样本齐全或产品通过。实例见 `results/2026-10-25-macos/comparison/`。

## 固定快照的候选数量

`eval/cmd/quantity` 是评估专用程序，不安装到产品 CLI。它只读取指定仓库中的起点、候选与参考 tree，输出新增非测试源码行数、新增测试用例和相对参考的比值。调用前应核对候选 tree 来自对应的验收报告，参考 tree 与任务资料中的参考补丁一致；程序不会从当前分支或工作区内容猜测这三个输入。

```sh
GOTOOLCHAIN=go1.26.0 go run ./eval/cmd/quantity \
  -repository /absolute/path/to/candidate \
  -base-tree BASE_TREE_ID -tree CANDIDATE_TREE_ID \
  -reference-repository /absolute/path/to/reference-repository \
  -reference-tree REFERENCE_TREE_ID > /absolute/path/to/quantities.json
```

入口覆盖任务集的 Go、Python、TypeScript、Java、Rust，也复用已有 JavaScript 规则。源码按 Git 新增物理行计，注释和空行对候选、参考采用同一口径；生成与依赖目录中的实际新增非测试源码也计入。纯移动不产生新增行，从非源码或测试文件转为生产源码时计入相应行。文档、配置、锁文件和测试源码不计入这个分子；构建或文档目录中的实际脚本源码仍按源码计数，与任务资料筛选时的二十至三百行门槛分别记录。读取新文件直接使用已有 blob，不创建空 blob，也不改索引或工作区。

Rust 内联测试复用 `eval/internal/rustinputs` 的语法范围，测试专用模块、函数和属性不计入生产代码。一个物理行仍含生产内容时只计一行；移除测试限定后转为生产范围的未改文字也计入。外部测试模块或包含文件的生产共享范围无法核对时，源码总数为 null；符号链接不跟随，不能按链接目标猜测数量。

用例复用各语言已有分析规则：可核对的参数组、子测试各算一条，修改已有用例及唯一对应的移动、改名不算新增。变化的用例无法完整枚举、名字重复或 Rust 宏可能生成测试时，测试总数和比值为 null；生成或依赖测试未分析、出现尚未接通的源码语言时也保留对应未知。列出的 `added_tests` 是已识别条目，不能在总数为 null 时把其长度充当完整数量。静态名字仍需与实际框架结果对应，不能用源码枚举替代尚未取得的原生数量证据。

改动量比为候选新增非测试源码行除以参考对应行数，参考为零时保留 null；测试量比的参考新增用例为零时按一计算。结果不包含实际验证次数、动态运行成本或采用判断。Go 试跑的只读采集见 `results/2026-10-25-macos/quantity/`，跨语言资料及 Python 候选采集见 `results/2026-10-25-macos/quantity-languages/`；隐藏验收失败的候选仍是失败，较少的代码不能替代正确性。

## 实际工具执行留证

`scripts/trace_command.py` 只供隔离评估环境转发已选的原生工具。配置指定原工具绝对路径 `command` 和工作区外的 JSONL 文件 `trace`；评估启动器可以在本轮 PATH 前加入对应转发命令，不改全局 PATH 或工具安装。调用保留入口路径，不把 Python 虚拟环境的解释器符号链接替换为基础解释器路径。标准输入、输出与错误直接连接原工具，正常退出码原样返回。

```json
{
  "command": "/absolute/path/to/native/go",
  "trace": "/absolute/path/outside/workspace/commands.jsonl"
}
```

```sh
python3 eval/scripts/trace_command.py /tmp/go-trace.json -- test -count=1 ./...
```

每次调用依次记录 attempt、实际 start、exit，无法启动时记录 launch_failed。来源由评估启动器的 `TASKBELAY_EVAL_TRACE_ORIGIN` 区分 agent、taskbelay、assessment；这些只用于评估，不是产品设置。嵌套工具通过父记录编号关联，不能把父、子重复计作两次 Agent 验证。未结束、损坏或矛盾的日志不能算成功；记录失败时仍等待已经启动的原生工具结束，不遗留未等待的直接子进程。

配置的 `role` 默认是 `native`；评估用 shell 前缀可显式标为 `shell_invocation`。目前仅在 macOS、Claude Code 2.1.309、Zsh 中完成真实调用核对：本轮同时设置 `CLAUDE_CODE_SHELL=/bin/zsh` 和 `CLAUDE_CODE_SHELL_PREFIX`，前缀通过转发程序执行 `/bin/zsh -c "$1"`。前缀收到的是包含环境准备的完整脚本。按 [Claude 官方说明](https://code.claude.com/docs/en/env-vars)，hook、状态命令和 MCP 启动也可能经过前缀，因此角色本身不能证明一次 Agent Bash 验证；来源及原生工具请求还须分别核对。配置只用于隔离评估进程，不改用户全局环境。

`eval/cmd/commands` 复用产品的 shell 参数解析和验证分类，只解析已有记录，不执行记录中的命令。构建后，可以将它传给对比收集器：

```sh
GOTOOLCHAIN=go1.26.0 go build -o /tmp/taskbelay-eval-commands ./eval/cmd/commands
python3 eval/scripts/collect_comparison.py /absolute/path/to/comparison.json \
  /absolute/path/to/new-comparison-result.json \
  --command-classifier /tmp/taskbelay-eval-commands
```

`verification_invocations` 按字面命令摘要核对 Claude 原生 Bash 请求和 shell 父记录，把同一次调用内实际启动的多个验证进程合计为一次。同名命令重复执行时保留整组身份，不凭顺序配对；未启动、权限拒绝、缺少父记录或来源冲突均不能补成执行。已开始但失败的验证仍属于实际执行，缺少终态另外标为不完整。`observed_verification_invocations` 仅表示已关联的转发范围内次数；`association_complete` 也只描述该范围。完整的 `total_verification_invocations` 和 `agent_verification_invocations` 继续为 null，未转发工具、绝对路径绕过、Codex 及其他平台还没有完整证据。旧日志没有角色和关联信息时保持 unknown，不追补身份。原生探测与脚本回归见 `results/2026-10-25-macos/invocation-trace/`。

转发进程收到 SIGTERM 或 SIGINT 时，将取消交给实际子进程并等待其结果，exit 同时保存原生退出码和 `termination_signal`。即使原生工具清理后返回零，转发入口仍以 `128 + 信号编号` 返回取消；日志完整与执行成功分别判断。无法处理的强制终止仍可能缺少 exit，继续报告未知。macOS Claude 的独立关闭探测见 `results/2026-10-25-macos/traced-agent-pilot/pflag-330/shutdown-probe/`，不能据此推定其他 Agent 或 Windows 的关闭方式。

耗时、退出码和可取得的 CPU／最大驻留内存原始值随调用保存，平台不提供时为 null。资源范围可能在父子调用间重叠，不直接求和。当前只证明经过转发的命令确实执行；绕过转发、工具内部未观察到的进程以及完整验证分类仍为未知。不能仅凭命令文本、权限拒绝、`&&` 后未执行的命令或空日志补填执行次数。Windows 原生启动与取消仍待核验。

TaskBelay 的 MCP 进程会等待 Agent 调用，存活时长可能覆盖整段会话；这段时间不能再加到 Agent 总耗时上当作额外成本。转发程序自身启动与留证也有开销，完整成本对比须按固定采集方式分别说明，不能直接把这些原始记录的耗时或资源求和。

### 已保存动态阶段的成本

`run_agents.py` 还会在评估前记下账本的检查序号，在本次 Agent 运行后以只读方式保存该工作区新增的动态检查，原文和序号范围写入 `phase-checks.json`，摘要写入运行回执的 `phase_checks`。账本缺失、读取失败或运行中被替换时只报告成本未知，不改变已有的 Agent 接入及正确性结论。

`collect_usage.py` 通过 `collect_phase_costs.py` 校验原文摘要，再按阶段汇总到 `taskbelay_phase_costs`。每个阶段分别报告命令尝试数、已观察的直接启动数、启动信息缺失数、命令耗时之和及可取得的原生 CPU／最大驻留内存。历史记录没有启动信息时，完整直接启动数为 null；任何已启动命令缺少资源时，该阶段资源总量为 null。最大驻留内存按原生单位换算后只取最大报告值，不能视为所有并发进程的内存峰值。阶段列表缺失的检查序号另列在 `checks_without_stages`，不能用其他检查的阶段代替。

此结果只覆盖序号范围内已保存的工作区阶段。同一检查被复用时没有新序号，不重复计费；同一工具环境指纹下的两个新检查则分别计入。报告中的基线和变异明细是阶段结果的另一种组织，不再次求和。多个 session 共用该工作区时，记录不能据此归给某个 Agent。未落账的检查、缓存身份核对、Git 和副本操作、内部未观察到的子进程仍在范围之外；阶段时间可能重叠，`complete_runtime_cost` 和 `full_cost_complete` 继续为 false。

### 已登记 hook 的提示量

`run_agents.py` 保存本次工作区、Agent 和原生 session 对应的 hook 决定原文。`recorded` 且没有错误表示产品已完成输出；失败、pending 和历史回执缺少原文时不补成零提示。离线测量复用产品适配器与摘要的 token 估算方式：

```sh
GOTOOLCHAIN=go1.26.0 go build -o /tmp/taskbelay-eval-feedback ./eval/cmd/feedback
python3 eval/scripts/collect_comparison.py /absolute/path/to/comparison.json \
  /absolute/path/to/new-comparison-result.json \
  --feedback-renderer /tmp/taskbelay-eval-feedback
```

`taskbelay_feedback` 分别报告已核对 hook 数、包含文本的 hook 数、文本字段数、UTF-8 字节、Unicode 字符与估算 token。只计适配器实际输出的文本字段，不计 JSON 封装；例如 Stop 的 `AdditionalContext` 不输出，也不计入提示量。中文、非 BMP 字符和 ASCII 按产品已有规则估算，每个输出字段分别计算后求和，不冒充模型原生 token。

收集器核对工作区、Agent、原生 session、唯一事件编号及决定结构，保存渲染器二进制和输入／输出摘要。准备好的内部决定不能代替输出成功的证据，已写标准输出也不能单独证明客户端消费或模型理解。MCP 回复、未登记的故障提示和用户确认不在此统计中，完整提示量继续为 null；权限拒绝或 hook 阻断不算用户确认。

macOS Claude Code 2.1.309 的独立只读会话已核对 5 条已登记 hook，其中 1 条包含文本，原生 SessionStart 输出与账本原文一致。该场景没有 MCP、修改或正式任务对比；原始输出、账本快照与两平台回归见同日 `feedback-costs/`。
