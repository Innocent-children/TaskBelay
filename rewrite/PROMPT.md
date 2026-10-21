# 任务：按功能说明书重写 TaskBelay

你要在这个仓库里从头重写 TaskBelay。唯一的产品依据是 `rewrite/SPEC.md`（下文简称"说明书"）。动手前先完整读一遍说明书，再读本文件剩下的部分。

## 一、依据的优先顺序

1. 我在对话里明确说的话。
2. 说明书。
3. 本文件。

现有的 AGENTS.md、CONTRIBUTING*、docs/、contracts/、tests/ 和全部代码都属于旧实现，只能当参考，不是约束。AGENTS.md 里关于九种语言同步、契约样例、发布流程、CORE_VERSION 的要求，这次都不适用；第 9 阶段会重写 AGENTS.md。

## 二、可以做的和不能做的

**可以做：**

- 从 `main` 新建 branch `rewrite/v0.4`，在这个 branch 上按阶段 commit。第一个 commit 只放 `rewrite/` 目录。commit 信息要写清楚做了什么、删了什么。
- 删除、移动、重写仓库里所有被 Git 跟踪的文件。

**不能做：**

- 不能 push、merge、打 tag，不能发布 npm，不能改 GitHub 设置。
- 不能读、写或删除 `~/.taskbelay` 下已有的任何文件。所有测试和手动验证都要把 `TASKBELAY_HOME` 指向临时目录。
- 不能修改我电脑上各个 Agent 的全局配置（比如 `~/.claude`、`~/.codex`）。验证 install 和 uninstall 时，把 `HOME`（Windows 上是 `USERPROFILE`）指向临时目录。
- 只删除被 Git 跟踪的文件。没被跟踪的文件和被忽略的文件都不要动，在最终报告里列出来。
- 文档只介绍产品本身，不写旧数据、迁移、兼容、"已经去掉的功能"这类内容。

## 三、这次重写本身也要守住说明书的要求

说明书禁止的做法，你写实现时同样不能用：

- 不为"以后可能用到"加抽象、接口、插件机制或配置项。只有一个实现时，不定义接口。
- 包按职责划分，函数之间直接调用。不用依赖注入框架，不硬套分层。
- 测试只覆盖说明书里写到的行为和第 17 节的验收场景，尽量写成表驱动测试。不给测试写测试，不生成大型契约 JSON 或样例文件。
- 体量以说明书第 16 节为准，超出时先删减。
- 平台差异（比如结束进程树）用 `_unix.go` 和 `_windows.go` 两个文件区分，不搞平台抽象层。
- 文档、Skill、工具输出和代码命名都按说明书第 4 节的用词规则来写。旧代码里的类型名、字段名和说法（authorization、evidence、freshness、receipt、assessment、concern、Host、Core 等）一律不要沿用。

## 四、目标目录结构

只要不增加层级，可以微调：

```text
cmd/taskbelay/      main 函数和子命令分发
internal/task/      Task 模型和规则：分级、完成条件、下一步、摘要、报告。纯函数，不读写文件和数据库
internal/diff/      取改动、文件分类、行数统计、只改格式的判断、依赖解析（说明书 7.2、7.3）
internal/symbols/   各语言的定义、抽象、实现、测试用例识别；引用计数；无关测试和结构相同测试的判断（7.4、8.1）
internal/check/     输入哈希、Go 输入推断、run 的决定流程、执行和超时（8.2 到 8.5）
internal/store/     SQLite、事务、数量上限裁剪
internal/mcp/       6 个 MCP 工具，只做转发，调用上面这些包
internal/hook/      三种 hook：解析各 Agent 传来的事件，按各 Agent 要求的格式输出
internal/install/   install、uninstall、doctor；各 Agent 的接入文件用 go:embed 打包进程序
assets/             Skill（只有一份）和各 Agent 的插件、MCP、hook 模板
npm/                启动脚本和平台包模板
scripts/            构建和打包，越少越好
docs/               见第 9 阶段
```

## 五、旧代码怎么处理

**可以参考或复制片段。** 复制过来的代码要按新结构和用词规则改写。

- `internal/repository/`：只读的 Git 调用、路径安全检查。
- `internal/execution/`：启动进程、超时、截断输出、按平台结束进程。
- `internal/store/`：SQLite 的打开方式和事务写法。
- `internal/mcp/`：MCP Go SDK 的用法。
- `packages/*/`：各 Agent 的插件目录结构、`hooks.json` 格式、MCP 配置格式、hook 事件 JSON 的字段。这些是各 Agent 实际格式的依据，写 `assets/` 和 `internal/hook/` 之前必须先读。

**在第 1 阶段末尾统一删除**（删除前先确认新结构不依赖它们）：

- `packages/` 整个目录，包括 desktop-pet、webui 和 host-* 等。
- `internal/` 下所有旧包，由新包取代。
- `contracts/`、`tests/`、`release/`、`skills/`（Skill 迁到 `assets/`）。
- `scripts/` 里的旧脚本。
- 根目录的 `package.json`、`pnpm-workspace.yaml`、`pnpm-lock.yaml`、`CORE_VERSION`（由 `VERSION` 取代）、`MANIFEST.md`、`MANIFEST_en.md`。
- 7 个 README 翻译版本，只保留 `README.md` 和 `README_zh-CN.md`。
- `docs/` 下全部旧文档（第 9 阶段写新的）。
- `.github/workflows/publish-npm.yml`。`ci.yml` 在第 8 阶段改写。

删除前用 `git ls-files` 统计要删的文件数，写进 commit 信息。

## 六、阶段和完成标准

每个阶段结束时，`go build ./...` 和 `go test ./...` 都要通过，然后 commit 一次。不要每改一个文件就把全部测试跑一遍。

1. **骨架和清理**：新建 branch 和新目录结构，写入 `VERSION`（0.4.0），删除旧实现。完成标准：`taskbelay version` 能用。
2. **Task 和存储**：`internal/task` 的模型、规模选择规则（7.9）和存储。完成标准：`taskbelay call` 能调用 `taskbelay_start`、`taskbelay_update`、`taskbelay_status`；A14、A26 通过。
3. **取改动和设计指标**：`internal/diff`，以及 7.5 中除符号以外的指标、7.6 分级、7.7 下一步。完成标准：checkpoint 返回指标、超标项和下一步；A3、A6、A11、A13、A19、A20 通过。
4. **符号分析和测试指标**：`internal/symbols`。说明书 7.4 和 8.1 表格里的每种语言，都要有表驱动的单元测试，覆盖定义、抽象、实现、测试用例的识别。完成标准：A16、A17、A18、A21、A22、A23 通过。
5. **检查**：`internal/check` 和 `taskbelay run`。完成标准：A4、A5、A24、A25 通过，A1 里"复用"这部分通过。
6. **接续和交付**：handoff、占用、Task 摘要、finish、cancel、report。完成标准：A1 完整通过（包括报告），A7、A8、A12 通过。
7. **hook 和 Agent 接入**：`internal/hook`、`internal/install`、`assets/`、MCP 服务。先做 Claude Code 和 Codex，再做 ZCode 和 DeepSeek Harness。每个 Agent 的事件格式、hook 事件名、shell 工具名，都要以旧 `packages/` 里的实际格式或该 Agent 的官方文档为准；查不到的能力，在 docs/AGENTS-SUPPORT 里标成"没验证"，不要猜。Skill 必须包含说明书第 13 节的 8 条规则，每个 Agent 的 Skill 不超过 6 KB。完成标准：A2、A9、A10、A15 通过。
8. **分发和 CI**：npm 启动脚本和平台包；darwin、linux、windows 交叉编译全部通过；把 `ci.yml` 改成三步：`go vet`、`go test`、交叉编译。
9. **文档**：
   - `README.md`（英文）和 `README_zh-CN.md`：讲清楚是什么、解决什么问题、怎么安装、一个完整例子（直接用说明书第 5 节）、命令速查、使用限制。每份不超过 120 行。
   - `docs/PRODUCT.md` 和 `docs/PRODUCT_en.md`：把说明书改写成给用户看的产品说明，去掉第 4、18、19 节。
   - `docs/AGENTS-SUPPORT.md` 和 `docs/AGENTS-SUPPORT_en.md`：每个 Agent 怎么安装、支持哪些 hook、哪些能力还没验证。
   - `AGENTS.md`：重写，不超过 60 行，写明：依据的顺序（产品说明优先于代码）、各个包负责什么、说明书第 13 节的开发规则、用词规则的前三条（禁用词表不要抄进来，只写"见禁用词扫描测试"）、怎么验证、版本和发布方面不能做的事。
   - `CONTRIBUTING.md` 和 `CONTRIBUTING_zh-CN.md`：每份不超过 60 行。
   - `SECURITY.md` 按新产品更新。
   - 加一个 Go 测试，扫描文档、Skill 和输出模板里有没有说明书第 4 节的禁用词（A27）。禁用词表只在这个测试里保存一份；英文词按整词匹配，不区分大小写，比如 `release` 不能因为包含 `lease` 而命中。
   - `rewrite/` 目录保持不动，删不删由我决定。
10. **最终验收**：跑完 A1–A27，执行 `go vet ./...` 和交叉编译；在临时 HOME 下对四个 Agent 各执行一遍 install、doctor、uninstall；按说明书第 16 节统计体量，并在一个约 1 万文件的仓库上测一次 checkpoint 和 hook 的耗时。

## 七、遇到问题怎么办

- 说明书有歧义，而且不同的理解会改变用户能看到的行为：停下来问我，同时给出你推荐的做法。
- 其他实现细节：自己选最简单的做法，在最终报告里列出来。
- 发现说明书里某个算法实现起来明显不合理，或者误报太多：先按最接近说明书本意的简单方式实现，在最终报告里说明原因，并给出修改说明书的建议。

## 八、最终报告（用中文，遵守用词规则）

1. 结论：哪些阶段做完了，哪些没做完。
2. 体量：删了多少文件、多少行；新代码按语言统计的行数，并和说明书第 16 节对比。
3. 验收：A1–A27 每一条的结果，以及实际运行过的命令。
4. 速度：checkpoint 和 hook 的实测耗时。
5. 没验证的部分：有没有在 Claude Code、Codex、ZCode、DeepSeek Harness 里真的跑过一次会话，没跑就直接写"没跑"；有没有在 Windows 和 Linux 上实际运行过。
6. 你自己做的实现决定，以及对说明书的修改建议（尤其是哪些算法规则误报多）。
7. 没动过的未跟踪文件和被忽略文件清单。
8. 下一步：按说明书第 18 节做对照实验的具体安排，包括选哪 6 个任务、怎么记录。
