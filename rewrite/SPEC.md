# TaskBelay 功能说明书

本文是重写 TaskBelay 的唯一依据。实现、文档、测试与本文冲突时，以本文为准；本文没写的功能不做。文中所有数值都是初始值，后面按第 18 节的对照实验调整。

## 1. TaskBelay 是什么

TaskBelay 是一个本地命令行程序，接在 AI 编程 Agent（Claude Code、Codex、ZCode、DeepSeek Harness）上，做三件事：

1. **防止过度设计**：按 Git 里的实际改动计算规模和结构指标，与预算比较。超出预算时，拦下改动或要求说明理由；超出很多时，必须问用户。
2. **防止过度测试**：按输入文件的哈希复用检查结果，代码没变就不重跑。测试写多少也有预算，和本次改动无关的测试会被找出来。
3. **让工作接得上**：开新会话或换一个 Agent，都能拿到同一个 Task 的进展，接着做。

代码由 Agent 写。TaskBelay 只负责计算、记录、拦截和出报告。

## 2. 解决的问题

| 问题 | 常见表现 | TaskBelay 怎么做 |
| --- | --- | --- |
| 过度设计 | 修一个 bug 新建三个文件；接口只有一个实现；新函数没人调用；生产代码只给测试用；顺手格式化无关文件；加新依赖 | 第 7 节的算法：算指标，对照预算分级，再要求删减、说明理由或问用户 |
| 过度测试 | 代码没变还反复跑测试；改一行就跑全量；给没改的代码写测试；写一堆结构一样的测试；失败后不改代码就重跑 | 第 8 节的算法：测试规模指标、按输入哈希复用、run 的决定流程、全量检查放到最后 |
| 接不上 | 新会话从头再问一遍；换了 Agent 后不知道做到哪、检查过什么 | 第 9 节：Task 摘要、交接、占用 |

## 3. 做什么，不做什么

**做：**

1. 记录 Task：目标、不做的事、用户限制、规模、检查计划、进展、需要问用户的问题。
2. 防过度设计算法（第 7 节）。
3. 防过度测试算法（第 8 节）。
4. 新会话接续和换 Agent 接续（第 9 节）。
5. 完成条件和交付报告（第 10 节）。
6. 装到各个 Agent、卸载、自检（第 13 节）。

**不做（直接删除，不留开关）：**

| 不做 | 原因 |
| --- | --- |
| 沙箱、权限系统 | Agent 自带的权限设置已经在管 |
| 通用工作流、方案审批、反复确认 | 和"少折腾"的目标矛盾 |
| Token、时间预算和计费 | Agent 不提供 token 数据，测不准的东西不做 |
| 建 branch、建 worktree、commit、push 等 Git 写操作 | Agent 自己用 git 做；TaskBelay 只读 Git |
| 用模型判断代码对不对、给代码打分 | 程序只算能确定的事实，判断交给 Agent 和用户 |
| WebUI、桌面宠物、交互式管理菜单 | 终端里的 `status` 和 `report` 已经够用 |
| 有副作用的命令中断后的恢复机制 | 检查必须是可以重复执行的验证命令 |
| 一个 Task 跨多个仓库；同一个 worktree 同时开多个 Task | 要并行就用不同的 worktree，每个开一个 Task |
| 中英文以外的语言 | 维护成本远大于收益 |

## 4. 用词规则

本项目的文档、Skill、工具输出和代码命名都要遵守以下规则：

1. 开发者平时直接说英文的词，不翻译：Agent、Task、MCP、Skill、hook、session、commit、diff、branch、worktree、glob、CLI。工具名、字段名、错误码一律用英文。
2. 其他内容用普通中文。一个常用词能说清楚的，就不用生造词、比喻或口号。
3. 一句话只说一件事，先说结论。
4. 下表左边的词不许出现：

| 不用 | 改用 |
| --- | --- |
| 代理、宿主、Host | Agent，或者直接写 Claude Code、Codex 等名字 |
| Core（面向用户时） | TaskBelay |
| 授权、授权边界 | 用户要求、用户限制 |
| 证据、收据、凭证 | 检查结果、运行记录 |
| 适用性、新鲜度 | 是否仍然有效 |
| 语义审查、语义判断 | Agent 自己判断 |
| 控制建议、advice | 下一步 |
| 指纹 | 输入哈希 |
| 基线 | 起点 |
| 租约 | 占用 |
| 原生 | Agent 自带的 |
| 工程控制、治理、编排、范式、闭环、赋能、抓手、链路、心智、沉淀、对齐、颗粒度、兜底、落地、口径 | 直接写具体做什么 |

5. 代码命名也不用以下英文词：authorization、evidence、applicability、freshness、testimony、receipt、concern、assessment、admission、provisioning、orchestration。
6. 文档、Skill 和输出模板由自动测试扫描，出现上面的禁用词，测试就失败。

## 5. 一次完整使用

用户对 Agent 说："给登录接口加失败限流：连续失败 5 次锁 15 分钟。别动账号管理，不要加新依赖。"

1. Agent 调用 `taskbelay_start`，传入：用户原话、两个目标、不做的事、`allow_new_deps: false`、规模 S，以及一个验收检查 `c1`（`go test ./internal/auth -run TestLogin`）。TaskBelay 创建 Task `tb-7f3k`，返回 Task 摘要。
2. Agent 改代码时想新建第二个生产文件，被 hook 拒绝：S 规模只允许新建 1 个，要么写进已有文件，要么先说明理由。
3. Agent 在 shell 里运行 `taskbelay run c1`，通过。代码没变时再运行一次，TaskBelay 直接返回上次的结果，不执行命令。
4. Agent 调用 `taskbelay_checkpoint`。结果显示：生产代码改了 2 个文件、新增 86 行，另外新加了 1 个只有一个实现的接口，而 S 规模的预算是 0。Agent 删掉这个接口，直接用具体类型，再次 checkpoint 后全部在预算内，下一步是 `finish`。
5. Agent 调用 `taskbelay_finish` 并填写自评，拿到交付报告，转述给用户。

**换会话：** 会话断了，用户开了新会话。SessionStart hook 提示"这个 worktree 有进行中的 Task tb-7f3k"。Agent 调用 `taskbelay_start`，拿到 Task 摘要（目标、进展、下一步、检查状态），接着做。

**换 Agent：** 用户让 Claude Code 执行 `handoff`，然后到 Codex 里说"继续"。Codex 调用 `taskbelay_start`，拿到同一个 Task 的摘要和交接说明，Task 改由 Codex 占用。

## 6. Task

**对应关系。** 一个 Task 对应一个 Git worktree 的根目录，也就是 `git rev-parse --show-toplevel` 的结果。每个 worktree 同时最多有一个进行中的 Task。

**状态。** 只有三种：`active`、`done`、`cancelled`。`active` 只能变成另外两种，结束的 Task 不能重新打开。要继续做，就开新 Task，新 Task 的摘要会附上同一个 worktree 上一个 Task 的结论。

**起点。** 开 Task 时记录两样东西：当时 HEAD 指向的 commit（`start_commit`），以及当时已有的未提交改动（文件路径和内容）。之后所有计算都从起点算起，开工前就有的改动不算在这个 Task 上。如果未提交改动超过 8 MiB，拒绝开 Task（`START_TOO_LARGE`），请用户先 commit 或 stash。如果后来 `start_commit` 不再是 HEAD 的祖先（比如做过 rebase），会得到 `start_moved`；这时 Agent 用 `update.move_start{commit, reason}` 指定新起点，这一操作会写进报告。

**用户要求和限制。** 这些内容来自用户，必须附上 `user_quote`，也就是 Agent 照抄的用户消息原文。之后修改其中任何一项，都要附上新的用户原话。TaskBelay 不能判断原话的真假，只负责记录，并在报告里原样展示。

- `goals`：1–5 条，每条是一句能验证的结果，编号依次为 `g1..gN`。不需要检查的目标（比如只改文档）要填写 `no_check_reason`。
- `non_goals`：明确不做的事。
- `forbidden_paths`：glob 列表，写这些文件会被拦下。
- `allow_new_deps`：是否允许加新依赖，默认 `false`。
- 规模：用户指定了规模时，`size_set_by_user = true`。

**Agent 的计划。** 这部分不需要用户原话，但每次修改都会记进历史，并出现在报告里。

- `size`、`size_reason`、`design_note`：规则见 7.9。
- `expected_paths`：预计会改的位置，只用于提示。
- `checks`：见 8.2。

**进展。** 包括 `note`、`next`，以及每个目标的状态（`open`、`done`、`dropped`）。把目标改成 `dropped` 需要用户原话。

**问题。** Agent 需要用户做决定时，先登记问题。还有没回答的问题，就不能交付。记录回答时要附上用户原话。

**历史。** 每个 Task 保留最近 200 条记录，每条写明谁、什么时候、做了什么。

## 7. 防过度设计算法

### 7.1 总流程

```text
measure(task):
  changes = get_changes(task.start)        # 7.2 取改动
  files   = classify(changes)              # 7.3 文件分类
  symbols = analyze_symbols(files)         # 7.4 符号分析
  metrics = compute_metrics(files, symbols) # 7.5 指标；测试指标见 8.1
  items   = check_budgets(metrics, task)    # 7.6 对照预算，用 grade 分级
  return next_steps(items, task)           # 7.7 下一步
```

`taskbelay_checkpoint`、`taskbelay_finish` 和 `taskbelay report` 都会跑完整流程。写文件的 hook 只做 7.8 里的快速判断。

### 7.2 取改动

1. **文件列表：**
   - 已跟踪的文件：用 `git diff --name-status -M50% <start_commit>` 比较当前 worktree 和起点 commit。开工后的 commit、已暂存和未暂存的改动都会包括在内。
   - 新文件：`git ls-files --others --exclude-standard` 的结果。
   - 起点时已经有未提交改动的文件：用起点时保存的内容和当前内容比较，不和 `start_commit` 比较；内容没变的直接排除。
2. **重命名：** 相似度 ≥ 50% 的重命名算作修改，不算新文件，只计算改动的行。
3. **行数：** 对每个文件，统计新增的非空行数和删除的行数。二进制文件只计入文件数，不计行数。
4. **只改了格式：** 一个被修改的文件，忽略所有空白差异之后（相当于 `git diff -w --ignore-blank-lines`）没有任何改动，就算"只改了格式"。
5. **新目录：** 起点时不存在、现在存在、而且里面有新文件的目录，每个都算一个新目录。

### 7.3 文件分类

文件按以下规则分成四类。仓库配置（第 14 节）可以追加规则。

| 类别 | 默认规则 | 是否计入预算 |
| --- | --- | --- |
| 生成文件 | 锁文件（`go.sum`、`*-lock.*`、`*.lock`）、`vendor/`、`node_modules/`、`dist/`、`*.min.*`、`*.pb.go`、`*_gen.*`、`*.gen.*`，以及带 `linguist-generated` 属性的文件 | 不计入 |
| 测试 | `*_test.*`、`*.test.*`、`*.spec.*`、`test_*.py`，以及路径里有 `test/`、`tests/`、`__tests__/`、`testdata/`、`fixtures/` 的文件 | 计入测试指标 |
| 文档 | `*.md`、`*.mdx`、`*.rst`、`docs/**` | 只报告，不计入 |
| 生产代码 | 其余所有文件 | 计入设计指标 |

### 7.4 符号分析

这一步的目的是找出没必要的新代码：从来没被调用的、只被测试调用的，以及只有一个实现的抽象。

**支持的语言**（其他语言只计算 7.5 里的规模指标，报告会列出没有做符号分析的文件类型）：

| 语言 | 算作"定义"的写法 | 算作"抽象"的写法（只算声明了方法的） | 怎么数"实现" | 算作"测试用例"的写法 |
| --- | --- | --- | --- | --- |
| Go | 顶层 `func Name(`（不含带接收者的方法）、`type Name` | `type Name interface` | 用 go/parser 解析同一个 module，方法名覆盖接口全部方法名的类型 | `func TestXxx(`、`func BenchmarkXxx(`、`func FuzzXxx(` |
| TypeScript / JavaScript | 顶层 `function Name`、`class Name`、`const Name = (...) =>`、`const Name = function`、`interface Name`、`type Name =` | 带方法签名的 `interface Name`、`abstract class Name` | 出现 `implements Name` 或 `extends Name` 的 class | `it(`、`test(`，包括 `.only`、`.each` 的写法 |
| Python | 顶层 `def name`、`class Name` | 继承 `ABC` 或 `Protocol` 的 class | `class X(... Name ...)` | `def test_xxx`（顶层或 class 内） |
| Java / Kotlin | `class`、`interface`、`enum`、`record Name`；Kotlin 顶层 `fun name` | `interface Name`、`abstract class Name` | `implements Name`、`extends Name`；Kotlin 的 `: Name` | 带 `@Test` 注解的方法 |
| Rust | 不在 `impl` 块里的 `fn name`、`struct`、`enum`、`trait Name` | `trait Name` | `impl ... Name for` | 带 `#[test]` 的 fn |

**第 1 步：找出新符号。**

```text
new_symbols = 生产文件新增行里的定义 − 删除行里同名的定义    # 只是挪了位置的代码不算新符号
跳过：名字短于 3 个字符；main、init、New、setup、teardown、String、Error；
      Go 中带接收者的方法；写在测试文件里的定义
最多分析 500 个，超出的部分不分析，报告里注明
```

**第 2 步：数引用。** 对所有新符号只运行一次 `git grep -o -w -I --untracked -e 名字1 -e 名字2 ...`，搜索范围是生产文件和测试文件（不含文档和生成文件），按文件类别分别计数：

```text
prod_refs(name) = 在生产文件中出现的次数 − 它的定义次数
test_refs(name) = 在测试文件中出现的次数
```

**第 3 步：归类。**

```text
unused     : prod_refs == 0 且 test_refs == 0      → 计入 unused_symbols
test_only  : prod_refs == 0 且 test_refs > 0       → 计入 test_only_symbols
abstraction: 新定义属于"抽象"，且实现数 ≤ 1      → 计入 new_abstractions
single_use : 不是抽象的新类型，且 prod_refs == 1  → 只作为提示
```

**已知局限。** 这种按名字计数的方法比较宽松：别处有同名的标识符时，引用数会偏高，可能漏掉一些没用的符号。只有通过反射、字符串或框架按名字注册来调用的代码，才会被误判成没被使用；遇到这种情况，Agent 用 `justify` 写一句理由即可。

### 7.5 设计指标和预算

| 指标 | 怎么算 | S | M | L |
| --- | --- | --- | --- | --- |
| `prod_files` | 有新增行的生产文件数（只删代码的文件不算） | 3 | 8 | 25 |
| `prod_lines` | 生产文件新增的非空行数（删除的行只报告，不计入） | 120 | 500 | 1500 |
| `new_prod_files` | 新建的生产文件数 | 1 | 3 | 10 |
| `new_dirs` | 新建的目录数 | 0 | 1 | 3 |
| `new_abstractions` | 实现数 ≤ 1 的新抽象 | 0 | 1 | 3 |
| `unused_symbols` | 没被用到的新符号 | 0 | 0 | 0 |
| `test_only_symbols` | 只被测试用到的新生产符号 | 0 | 0 | 0 |
| `format_only_files` | 只改了格式的文件 | 0 | 0 | 0 |
| `new_deps` | 新增的依赖（`allow_new_deps = true` 时不计） | 0 | 0 | 0 |

**新增依赖的识别：** 能解析的清单文件（`go.mod`、`package.json`、`Cargo.toml`、`requirements*.txt`、`pyproject.toml`）会列出新增的包名；其他清单文件有改动时，记为 `manifest:<路径>`。

仓库配置可以覆盖表里的数值。

### 7.6 分级

每个指标超出预算时会生成一项超标，ID 就是指标名。另外有两类按个记录的超标：`dep:<包名>` 和 `forbidden:<路径>`。

```text
grade(v, b):                      # v 是实际值，b 是预算
  if v <= b:               ok
  if v <= max(2*b, b+1):   justify   # Agent 写一句理由即可
  else:                    user      # 需要用户原话
```

特殊规则：

- 改了 `forbidden_paths` 里的文件：每个文件生成一项 `forbidden:<路径>`，等级为 `revert`，必须撤回。只有用户用原话修改限制后才放行。
- `allow_new_deps = false` 时，每个新依赖生成一项 `dep:<包名>`，等级直接是 `user`。

怎样算解决：

- `justify` 等级：在 checkpoint 中用 `justify[{item, reason}]` 写了理由，就算解决。
- `user` 等级：写了用户原话，即 `justify[{item, reason, user_quote}]`，才算解决。
- 已解决的超标项数值继续变大时，只要等级不变，仍然算解决；等级从 `justify` 升到 `user` 时，需要补上用户原话。
- 数值回到预算以内时，这一项自动消失，之前写的理由保留在历史里。

### 7.7 下一步

checkpoint 返回一份按优先级排好的下一步列表。每一项都包含 `code`、`item`，以及一句说明要做什么的话，比如："新函数 parseLimitConfig 没有被调用：删掉，或者说明理由。"

| 顺序 | `code` | 什么时候出现 |
| --- | --- | --- |
| 1 | `revert` | 有 `forbidden` 项 |
| 2 | `ask_user` | 有没解决的 `user` 项，或者有没回答的问题 |
| 3 | `reduce_or_justify` | 有没解决的 `justify` 项 |
| 4 | `rethink` | 某个检查连续 3 次没通过（8.4） |
| 5 | `run_checks` | 有检查还没跑过，或者结果已经失效 |
| 6 | `continue` | 还有没完成的目标 |
| 7 | `finish` | 以上情况都没有 |

### 7.8 写文件时的快速判断（hook）

Agent 用自带的写文件工具写文件之前，hook 会运行以下判断：

```text
pre_write(path):
  task = path 所在 worktree 里进行中的 Task；没有就放行
  if path 匹配 forbidden_paths: 拒绝
  if path 原来不存在:
    kind = classify(path)
    if kind 是 生产代码 或 测试:
      n = 已新建的同类文件数 + 1
      按 7.6 给 new_prod_files 或 new_test_files 分级，并查看已有的理由和原话：
        需要理由但还没有  → 拒绝："写进已有文件，或者先在 checkpoint 里说明理由"
        需要原话但还没有  → 拒绝："先问用户，再用 justify 记下原话"
    对每个将要新建的目录，用同样的方法检查 new_dirs
  放行
```

这一步只用 `git status` 和 Task 里的数据，不做符号分析，以保证速度。通过 shell 命令新建的文件不经过 hook，会在 checkpoint 和 finish 时按同样的规则算出来。

### 7.9 规模怎么选

| 规模 | 适用场景 | 选择条件 |
| --- | --- | --- |
| S | 修 bug、小改动 | 默认值 |
| M | 一个完整的小功能 | Agent 写明 `size_reason` |
| L | 跨模块的改动 | 需要用户原话，并且要写 `design_note`，说明准备新建哪些模块、各自为什么需要 |

- 用户指定了规模时直接使用，并设置 `size_set_by_user = true`，之后改规模需要用户原话。
- 中途调大：从 S 调到 M 要写理由；调到 L 需要用户原话。调小不需要任何条件。每次调整都会写进报告。
- 报告会给出"实际规模"：让 `prod_files`、`prod_lines`、`new_prod_files`、`new_dirs` 四项全部落在预算内的最小规模；四项超过 L 时写"超过 L"。声明的规模和实际规模不一样时，报告里会写出来，以后据此调整预算。

### 7.10 提示（不拦截）

以下情况只作为提示，每次最多显示 5 条，由 Agent 自己判断要不要处理：

- 新文件名或新目录名里出现：factory、registry、manager、provider、adapter、strategy、abstract、base、helper、util、wrapper、compat、legacy、fallback、v2（不区分大小写）。
- 新增的代码行里出现：fallback、backward compat、legacy、deprecated、for future、in the future、extensible、pluggable。
- 新目录里只有一个文件。
- 只在一个地方用到的新类型（7.4 里的 `single_use`）。
- 结构相同的新测试（8.1）。
- 填写了 `expected_paths` 时，改动不在这些位置里。

仓库配置可以替换前两条的词表。

## 8. 防过度测试算法

### 8.1 测试规模指标

测试指标使用 7.2–7.4 取到的同一份改动来计算。

**第 1 步：找出新测试用例。** 按 7.4 表格最后一列的写法，取测试文件新增行里的测试用例定义，减去删除行里同名的定义。

**第 2 步：列出本次改动涉及的名字，记为 C。**

```text
C = 新的生产符号
  ∪ 每个被改动的生产代码块所在的定义（在当前文件里，从改动位置往上找到的最近一个定义的名字）
  ∪ 被改动的生产文件名（不含扩展名）；Go 再加上包所在目录的名字
```

**第 3 步：找出和本次改动无关的测试。** 一个测试用例的内容，是从它的定义行开始，到下一个测试用例定义（或文件末尾）为止的所有行。如果这些内容里没有出现 C 中的任何一个名字（按整词匹配），这个测试就和本次改动无关，计入 `unrelated_tests`。

**第 4 步：找出结构相同的测试。** 先把每个新测试用例的内容规整一遍：去掉第一行，把字符串字面量都换成 `"S"`，数字都换成 `0`，连续空白合成一个，然后计算哈希。有 3 个或以上哈希相同的用例时，给出提示："这 N 个测试结构相同，考虑合并成表驱动测试。"

| 指标 | 怎么算 | S | M | L |
| --- | --- | --- | --- | --- |
| `test_lines` | 测试文件新增的非空行数 | max(60, prod_lines) | max(150, prod_lines) | max(400, prod_lines) |
| `new_test_files` | 新建的测试文件数 | 1 | 2 | 5 |
| `new_test_cases` | 新的测试用例数 | 4 | 10 | 25 |
| `unrelated_tests` | 和本次改动无关的新测试用例数 | 0 | 0 | 0 |
| `checks` | 计划里的检查数 | 2 | 4 | 8 |
| `full_runs` | 全量检查实际执行的次数（复用不算） | 1 | 2 | 3 |

`test_lines` 预算里的 `prod_lines` 指本次实际的生产新增行数。超标的分级和处理方式与 7.6 相同。

### 8.2 检查

| 字段 | 说明 |
| --- | --- |
| `id` | Agent 起的短 ID，比如 `c1` |
| `goals` | 关联的目标 ID。为空表示这是整个 Task 的回归检查 |
| `kind` | `acceptance`（验收检查：证明某个目标做到了）或 `regression`（回归检查：证明没有改坏别的东西） |
| `run` | shell 命令。Unix 用 `/bin/sh -c` 执行，Windows 用 `cmd.exe /d /s /c` 执行 |
| `cwd` | 相对仓库根目录的路径，默认是 `.` |
| `inputs` | 会影响检查结果的文件 glob，默认值见 8.3 |
| `env` | 会影响结果的环境变量名 |
| `full` | 是否是全量检查，全量检查计入 `full_runs` |
| `timeout_s` | 超时秒数，默认 900，最大 3600 |

验收检查至少要关联一个目标。每个没有 `no_check_reason` 的目标，至少要有一个验收检查。

### 8.3 输入哈希

```text
input_hash(check):
  files = input_files(check)
  return sha256(
    check.run, check.cwd, sorted(check.inputs), sorted(check.env),
    操作系统, CPU 架构,
    每个 env 变量当前值的 sha256,
    files 按路径排序后，每个文件的 (路径, 文件权限, Git blob ID))
```

文件的 Git blob ID：没改过的已跟踪文件，直接取 index 里的值；其他文件按 Git 的算法自己计算。这样内容相同的文件，ID 一定相同。

`input_files(check)` 按以下顺序取第一条符合的规则：

1. 写了 `inputs`：所有 Git 能看到的文件中，匹配这些 glob 的文件。
2. 没写 `inputs`，并且 `run` 以 `go test` 开头：在检查的 `cwd` 中运行 `go list -deps -test -f '{{.Dir}}' <包参数>`，取仓库内的那些目录里的文件（不进入子目录，但包括这些目录下的 `testdata/**`），再加上 `go.mod` 和 `go.sum`。如果 `go list` 失败，就用第 3 条，并在运行输出里注明。
3. 其他情况：所有 Git 能看到的文件，去掉文档和生成文件。

"Git 能看到的文件"指 `git ls-files --cached --others --exclude-standard` 列出的文件。被 Git 忽略的文件、数据库、网络服务和工具链版本都不在输入哈希里。如果需要把它们算进去，就把对应的版本文件或配置文件写进 `inputs`。

### 8.4 run 的决定流程

Agent 用自带的 shell 工具运行 `taskbelay run <check-id> [--reason "<新情况>"]`。这样命令会沿用 Agent 自己的权限、环境和长时间运行的处理方式。MCP 工具不启动命令。

```text
run(c, reason):
  if c 不在计划里:
    refuse("先用 taskbelay_update 把它加进计划")
  h = input_hash(c)
  last = c 在输入哈希 h 下最近一次有结果的执行（passed、failed 或 timeout）
  if last == passed:
    return reused(last)                                    # 不执行命令
  if last 是 failed 或 timeout，并且 reason 为空:
    refuse("代码没变，重跑结果也一样。先改代码，或者用 --reason 说明有什么新情况")
  if c 最近连续 3 次实际执行都没通过，并且 reason 为空:
    refuse("已经连续失败 3 次。换个思路，再用 --reason 说明这次哪里不一样")
  if c.full:
    if 有验收检查在当前代码下还没通过，并且 reason 为空:
      refuse("先让验收检查通过，再跑全量检查")
    g = grade(已执行的全量检查次数 + 1, budget.full_runs)    # 7.6
    if g == justify 并且 full_runs 这一项还没有理由:
      if reason 为空: refuse("全量检查次数超出预算，用 --reason 说明理由")
      把 reason 记为 full_runs 这一项的理由
    if g == user 并且 full_runs 这一项还没有用户原话:
      refuse("全量检查次数需要用户同意。先问用户，再用 justify 记下原话")
  执行 c，记录结果，更新计数
```

- 连续失败次数在检查通过时清零。状态为 `interrupted` 的运行没有结果，不算失败，可以重跑。
- **输出：** 第一行是结果（`passed`、`failed`、`timeout`、`reused`、`refused`）和耗时，后面是命令输出的最后 200 行。
- **退出码：** 0 表示通过或复用了通过的结果；1 表示失败、超时或中断；2 表示拒绝执行；3 表示用法错误。
- **超时：** 杀掉整个进程树。每次运行保存最后 64 KB 的输出。
- **TaskBelay 自己被中途杀掉：** 运行记录会停在 `running`。下次读到这条记录时，如果对应进程已经不在了，就把它标成 `interrupted`。

### 8.5 检查结果什么时候失效

一个检查结果只有在它的输入哈希和当前的输入哈希相同时才有效。所以：

- 改了 `inputs` 以外的文件，结果仍然有效。
- 使用默认 `inputs` 时，只改文档或生成文件，结果仍然有效。
- Go 检查只依赖它实际导入的包。改了没被导入的包，结果仍然有效。
- 改了检查本身的定义（`run`、`cwd`、`inputs`、`env`），结果失效。

### 8.6 拦截 shell 里的测试命令

如果 Agent 支持在执行 shell 命令前调用 hook，并且命令所在的 worktree 有进行中的 Task：

- 命令（空白规整后）和计划里某个检查的 `run` 完全一样：拒绝，提示改用 `taskbelay run <id>`，代码没变时会直接复用结果。
- 命令匹配常见的测试命令（`go test`、`pytest`、`npm test`、`pnpm test`、`yarn test`、`vitest`、`jest`、`cargo test`、`mvn test`、`gradle test`、`make test`，以及仓库配置里追加的命令）：放行，同时记一次"shell 里临时跑测试"，次数会写进报告。
- 其他命令：放行，不做记录。

## 9. 接续

### 9.1 Task 摘要

`taskbelay_start`、`taskbelay_status` 和 SessionStart hook 返回的是同一份摘要，最多 60 行，使用用户设置的语言。示例：

```text
TaskBelay Task tb-7f3k · 进行中 · 规模 S · claude 占用中（2 分钟前活动）
目标
  [x] g1 登录连续失败 5 次后锁定 15 分钟
  [ ] g2 锁定期间返回 429 和剩余秒数
不做：账号管理 | 不加新依赖 | 不改 api/public/**
进展：限流逻辑写在 internal/auth/limit.go
下一步：在 handler 里返回 429，补 TestLockedResponse
设计：生产 2/3 文件 · +86/120 行 · 新文件 0/1 · 新抽象 0/0 · 没用到的符号 0
测试：+40/86 行 · 新用例 2/4 · 无关测试 0
超标：无    提示：无
检查：c1 通过（当前代码）· c2 还没跑
待用户回答：无
上次交接：codex 10-20 18:02「g1 做完了；redis mock 在 testdata/」
```

### 9.2 新会话

- Agent 支持 SessionStart hook 时：会话开始时运行 `taskbelay hook session-start`。如果当前 worktree 有进行中的 Task，就输出不超过 3 行的提示（Task ID、目标、下一步），并要求 Agent 开始开发前先调用 `taskbelay_start`；没有 Task 时什么都不输出。
- Agent 不支持 SessionStart hook 时：由 Skill 要求 Agent 在仓库里开始开发前，先调用 `taskbelay_start`。
- 如果 worktree 里已经有进行中的 Task，`taskbelay_start` 会返回这个 Task 的摘要，不会再新建一个。如果 Agent 其实要做另一件事，必须先请用户决定：结束（finish）还是取消（cancel）当前的 Task。

### 9.3 交接

`taskbelay_handoff` 记录三项内容：`done`（做了什么）、`next`（接下来做什么）、`pitfalls`（遇到的坑和注意事项），然后解除占用。每个 Task 保留最近 5 次交接，摘要里显示最近一次。

以下情况 Agent 应该交接：自己要停下、用户要换 Agent、会话快结束了。

### 9.4 占用

- 每个 MCP 服务进程启动时生成一个占用者 ID，格式为 `<Agent 名>:<8 位随机字符>`。
- `taskbelay_start` 会占用 Task，之后每次修改 Task 都会刷新最后活动时间。占用默认在 30 分钟没有活动后过期，时长可以配置。
- 占用没有过期时，别的会话修改这个 Task 会得到 `TASK_IN_USE`，同时返回占用者、最后活动时间和只读的摘要。这时 Agent 有两个选择：问用户之后，用 `taskbelay_start` 带上 `takeover_quote`（用户原话）接手；或者等待。
- 占用已经过期，或者已经交接过时，下一次 `taskbelay_start` 直接接手，并写进历史。占用者自己再次调用 `taskbelay_start`，只返回摘要。
- 只读操作（`taskbelay_status`、`taskbelay report`）和 `taskbelay run` 都不受占用限制。

### 9.5 换 Agent

所有 Agent 共用同一个数据目录和同一个程序，Task 里也不存任何某个 Agent 特有的内容，所以任何支持的 Agent 都能接着做另一个 Agent 开始的 Task。摘要和报告会写明每一段工作是哪个 Agent 做的。

## 10. 交付

### 10.1 完成条件

`taskbelay_finish` 要求下表的条件全部满足。有任何一条不满足，就返回 `FINISH_BLOCKED` 和未满足条件的列表。

| `code` | 条件 |
| --- | --- |
| `goal_open` | 每个目标都是 `done`，或者已经凭用户原话改成了 `dropped` |
| `check_missing` | 每个需要检查的目标，至少有一个验收检查 |
| `check_stale` / `check_failed` | 每个验收检查和回归检查，在当前输入哈希下都有通过的结果（只关联了 `dropped` 目标的检查除外） |
| `over_budget` | 每个 `justify` 等级的超标项都已经删减或者写了理由 |
| `user_decision_needed` | 每个 `user` 等级的超标项都有用户原话 |
| `forbidden_changed` | 禁止的路径上没有遗留改动 |
| `question_open` | 没有没回答的问题 |
| `run_in_progress` | 没有正在运行的检查 |
| `start_moved` | 起点仍然有效 |

没通过的回归检查，可以凭用户原话列入 `known_failures`，这样视为满足条件，并在报告里单独列出。验收检查不能这样处理。

### 10.2 自评

finish 时必须填写三项自评，每项一句话：

- `root_cause`：解决的根本问题是什么。
- `not_done`：删掉了什么，或者刻意没做什么；没有就写"无"。
- `would_do_again`：从现在的实际需求出发，还会不会选择现在这个结构，为什么。

### 10.3 交付报告

交付成功后返回报告，最多 40 行。Agent 必须用用户的语言把报告转述给用户，超标项和已知问题不能省略。任何时候都可以用 `taskbelay report` 打印当前的报告或最终的报告。示例：

```text
TaskBelay 交付报告 · tb-7f3k · 已完成
结论：2/2 个目标完成
规模：声明 S，实际 S
设计：生产 2 文件 +86/−12 行（预算 3 / 120）· 新文件 1/1 · 新目录 0/0 · 新抽象 0/0
      没用到的符号 0 · 只给测试用的符号 0 · 只改格式的文件 0 · 新依赖 0
测试：+40 行（预算 86）· 新测试文件 1/1 · 新用例 3/4 · 无关测试 0
超标和理由：无
规模调整：无
检查：实际执行 3 次 · 复用 2 次 · 拒绝 1 次 · shell 里临时跑测试 0 次 · 共 48 秒
  c1 验收 g1,g2  go test ./internal/auth -run TestLogin   通过（当前代码）
  c2 回归        go test ./internal/...                   通过（当前代码）
已知失败：无
提示：无
自评：根因是登录失败没有计数 | 没做通用限流中间件 | 还会这样做：改动都在 auth 包里
参与：claude（开 Task、g1）→ codex（g2、交付）
```

### 10.4 取消

取消 Task 需要写明理由，并附上用户原话。取消后也会生成一份标着"已取消"的报告。

## 11. 接口

### 11.1 MCP 工具（6 个）

所有工具的输出都包含 `ok`、`task_id`，以及一段默认不超过 4 KB 的文字摘要，同时附上结构化数据。

| 工具 | 输入 | 输出 |
| --- | --- | --- |
| `taskbelay_start` | `cwd`（绝对路径，必填）。worktree 里没有进行中的 Task 时，必须提供 `task`，包括：`goals[{text, no_check_reason?}]`、`non_goals`、`forbidden_paths`、`allow_new_deps`、`user_quote`、`size`、`size_set_by_user`、`size_reason`、`design_note`、`expected_paths`、`checks`。可选 `takeover_quote` | `created`、Task 摘要、下一步 |
| `taskbelay_update` | `task_id`，以及下面任意几项：`goals`（完整列表，已有的目标保留原 ID）、`non_goals`、`forbidden_paths`、`allow_new_deps`、`size`、`size_reason`、`design_note`、`expected_paths`、`checks`（完整列表）、`move_start{commit, reason}`、`user_quote` | 改了什么、Task 摘要 |
| `taskbelay_checkpoint` | `task_id`。可选：`note`、`next`、`goals_done[]`、`justify[{item, reason, user_quote?}]`、`questions[]`、`answers[{question_id, user_quote}]` | 设计指标、测试指标、超标项、检查状态、下一步 |
| `taskbelay_handoff` | `task_id`、`done`、`next`、`pitfalls` | Task 摘要 |
| `taskbelay_finish` | `task_id`，再加以下两组之一：① `summary`、`self_review{root_cause, not_done, would_do_again}`、`known_failures[{check_id, user_quote}]`；② `cancel{reason, user_quote}` | 交付报告；或者 `FINISH_BLOCKED` 加上未满足的条件 |
| `taskbelay_status` | 可选：`task_id` 或 `cwd`、`detail` | Task 摘要。带 `detail` 时，再附上最近 30 条历史和每个检查最近一次的运行记录。两个参数都不给时，返回最近 20 个 Task |

调用时不需要传版本号。每次修改都在一个数据库事务里完成，多个 Agent 之间的冲突由"占用"来避免。

### 11.2 命令行

```text
taskbelay run <check-id> [--task <id>] [--reason <text>]
taskbelay status [--task <id>] [--json]
taskbelay report [--task <id>]
taskbelay list [--all]
taskbelay call <tool> [--agent <name>] [--session <id>]   # 从 stdin 读 JSON，效果和调用 MCP 工具一样，用于测试和调试
taskbelay mcp --agent <name>
taskbelay hook <session-start|pre-write|pre-shell> --agent <name>   # 从 stdin 读 Agent 传来的事件 JSON
taskbelay install <agent...> | --all
taskbelay uninstall <agent...> | --all
taskbelay doctor
taskbelay version
```

- 省略 `--task` 时，按当前目录所在的 worktree 找进行中的 Task。
- `taskbelay call` 的占用者 ID 是 `<agent>:<session>`，默认为 `cli:local`。

### 11.3 错误码

| 错误码 | 什么时候出现 |
| --- | --- |
| `NOT_A_REPO` | `cwd` 不在 Git worktree 里 |
| `NO_ACTIVE_TASK` | worktree 里没有进行中的 Task，却调用了需要 Task 的操作 |
| `ACTIVE_TASK_EXISTS` | 要新建 Task，但 worktree 里已经有进行中的 Task（会同时返回这个 Task 的摘要） |
| `TASK_IN_USE` | Task 正被别的会话占用 |
| `TASK_NOT_FOUND` | 找不到这个 Task |
| `TASK_CLOSED` | 修改已经结束的 Task |
| `USER_QUOTE_REQUIRED` | 这次修改需要用户原话，但没有提供 |
| `INVALID_INPUT` | 字段缺失或不合法（会附上字段路径） |
| `START_TOO_LARGE` | 开 Task 时，未提交的改动超过 8 MiB |
| `FINISH_BLOCKED` | 完成条件没有全部满足（会附上未满足的条件） |
| `RUN_REFUSED` | `taskbelay run` 被拒绝（会附上原因和解决办法） |

每个错误都会附带一句话，说明下一步该做什么。

## 12. 数据

- **位置：** `~/.taskbelay/tasks.db`（Windows 上是 `%USERPROFILE%\.taskbelay\tasks.db`），可以用 `TASKBELAY_HOME` 换目录。使用 SQLite 的 WAL 模式和纯 Go 驱动。
- **表：** `tasks`（Task 的当前状态）、`start_files`（起点时未提交文件的内容）、`runs`（运行记录，包括输出的末尾部分）、`events`（历史）。数据库只有一个当前 Schema。
- **上限：** 每个 Task 保留最近 200 条历史、最近 100 次运行，每次运行保存 64 KB 输出。
- **安装记录：** `~/.taskbelay/installs.json`，记录给每个 Agent 写入了哪些文件。卸载时只删除这些文件。
- **Git：** TaskBelay 只读 Git，只会调用 `rev-parse`、`status`、`diff`、`show`、`ls-files`、`grep`、`merge-base`、`check-attr`、`cat-file`。
- **网络：** 不联网，不收集任何使用数据。

## 13. Agent 接入

| Agent | MCP | Skill | 写文件前 hook | shell 命令前 hook | SessionStart hook |
| --- | --- | --- | --- | --- | --- |
| Claude Code | 有 | 有 | Write / Edit / NotebookEdit | Bash | 有 |
| Codex | 有 | 有 | apply_patch | 看 Codex 是否支持 | 看 Codex 是否支持 |
| ZCode | 有 | 有 | Write / Edit | 看 ZCode 是否支持 | 看 ZCode 是否支持 |
| DeepSeek Harness | 有 | 有 | 看 DeepSeek Harness 是否支持 | 看 DeepSeek Harness 是否支持 | 看 DeepSeek Harness 是否支持 |

- `taskbelay install <agent>` 把程序里打包好的文件，按这个 Agent 自己的方式写进去：插件、MCP 配置、Skill 和 hook。hook 和 MCP 配置里使用 `taskbelay` 程序的绝对路径。重复执行 install 结果不变；升级 TaskBelay 后再执行一次就能更新这些文件。
- `taskbelay doctor` 检查以下内容：程序版本、每个 Agent 的文件是否齐全、是否和当前版本一致、数据库能不能打开、git 能不能用。每发现一个问题，都给出一条修复命令。
- 所有 Agent 用同一份 Skill 内容，只有开头的元数据和工具名写法不同。Skill 用英文写，每个 Agent 不超过 6 KB。

**Skill 必须包含以下规则：**

1. 在仓库里开始开发前，先调用 `taskbelay_start`。普通问答和一两行的小改动不需要。
2. 只做用户要求的事。先找到原因，以及现有代码里应该改的位置，优先修改已有的函数和文件。
3. 不添加用户没要求的接口、抽象类、工厂、插件机制、配置项、兼容分支、fallback，也不添加"以后可能用到"的参数；不改无关代码的格式、命名和结构。
4. 测试只覆盖本次改变的行为。优先修改已有测试；结构相同的用例写成表驱动测试；不给没改的代码补测试。
5. 检查都通过 `taskbelay run` 运行。先跑验收检查，全量检查最后跑一次。不是 Go 的检查要写 `inputs`。
6. checkpoint 结果里有 `reduce_or_justify` 时，先删减，删不掉才写理由；有 `ask_user` 时去问用户，并照抄用户原话。
7. 做完一个阶段时 checkpoint；停下或换 Agent 前 handoff；交付时 finish，并把报告转述给用户。
8. `user_quote` 必须照抄用户原话，不能改写，不能编造。

## 14. 配置

**仓库配置** `.taskbelay.json`（可选，可以 commit 到仓库里）：

```json
{
  "budgets": { "S": { "prod_lines": 150 } },
  "paths": { "test": ["e2e/**"], "docs": [], "generated": ["api/gen/**"] },
  "hint_words": { "names": ["..."], "code": ["..."] },
  "test_commands": ["make check"]
}
```

- `budgets`：只覆盖写出来的那些数值。
- `paths`、`test_commands`：在默认规则的基础上追加。
- `hint_words`：写了就整体替换 7.10 里默认的词表。

**用户配置** `~/.taskbelay/config.json`：

- `lang`：`zh-CN` 或 `en`，默认跟随系统语言。
- `release_after_minutes`：占用多少分钟没活动就过期，默认 30。

## 15. 分发与版本

- **平台：** macOS arm64 和 x64、Linux x64 和 arm64、Windows x64。
- **npm 包 `taskbelay`：** 由一个几十行的启动脚本，加上按平台拆分的可选依赖包组成，每个平台包里只有一个 `taskbelay` 可执行文件。也可以直接用 `go install` 安装。
- **版本号：** 所有组件共用仓库根目录 `VERSION` 文件里的同一个版本号。

## 16. 性能与体量

- **速度**（1 万个文件的仓库，改动的文件在 500 个以内）：
  - `taskbelay_start` ≤ 1 秒；
  - `taskbelay_checkpoint` 和 `taskbelay_finish` ≤ 2 秒（包括符号分析）；
  - hook 的 p95 ≤ 300 毫秒；
  - `taskbelay run` 自身的开销 ≤ 300 毫秒（不含被运行命令本身的时间）。
- **输出：** 工具默认输出 ≤ 4 KB，Task 摘要 ≤ 60 行，交付报告 ≤ 40 行。
- **体量：**
  - Go 非测试代码 ≤ 7000 行；
  - 测试代码不超过非测试代码；
  - 非 Go 代码（npm 启动脚本和各 Agent 的接入文件）≤ 800 行；
  - 全部文档中英文合计 ≤ 1500 行。
- **依赖：** 只用 Go 标准库、MCP Go SDK 和纯 Go 的 SQLite 驱动。加任何其他依赖，都要有明确的理由。
- **可靠性：** 多个进程同时读写同一个数据库时，不丢数据，也不会出现两个进行中的 Task。TaskBelay 进程被杀掉之后，下次启动能正常读取数据，没跑完的运行记录标为 `interrupted`。

## 17. 验收场景

每个场景都在临时 Git 仓库和临时 `TASKBELAY_HOME` 里，通过 `taskbelay call`、`taskbelay run`、`taskbelay hook` 自动执行。另外，7.4 和 8.1 中各语言的识别规则，要有表驱动的单元测试。

| 编号 | 场景 | 期望 |
| --- | --- | --- |
| A1 | S 规模 Task 改 1 个文件，`run c1` 两次，然后 finish | 第二次 run 返回 `reused`；报告里写实际执行 1 次、复用 1 次 |
| A2 | S 规模 Task 新建第 2 个生产文件 | pre-write 拒绝；写了理由之后放行；新建第 3 个时（超过 2×1），没有用户原话就拒绝 |
| A3 | `allow_new_deps = false` 时在 `go.mod` 里加依赖 | checkpoint 返回 `ask_user`；没有用户原话时 finish 被拦下 |
| A4 | 检查失败后不改代码再 run | 被拒绝；带 `--reason` 才执行；连续失败 3 次后，下一次必须带 `--reason`，并给出 `rethink` |
| A5 | 检查通过后，分别改 `inputs` 以内的文件、以外的文件，以及只改文档 | 改 `inputs` 以内：`check_stale`；改 `inputs` 以外、只改文档：结果仍然有效 |
| A6 | 开 Task 前就有未提交的改动 | 这些改动不计入任何指标 |
| A7 | 会话 1 开 Task 后没有交接就退出，会话 2 马上调用 start | 返回 `TASK_IN_USE`；占用过期后，或者带了 `takeover_quote`，摘要里能看到进展和下一步 |
| A8 | Claude Code 交接后，Codex 调用 start | Codex 直接接手；摘要里有交接内容；报告列出两个参与的 Agent |
| A9 | shell hook 收到和计划检查完全相同的命令 | 拒绝，并提示改用 `taskbelay run` |
| A10 | 修改 `forbidden_paths` 里的文件 | pre-write 拒绝；通过 shell 修改的，finish 返回 `forbidden_changed` |
| A11 | S 规模 Task 的测试新增行数超出预算 | 出现 `test_lines` 超标项，需要写理由 |
| A12 | 不带用户原话 cancel | 返回 `USER_QUOTE_REQUIRED` |
| A13 | S 规模 Task 新建 `utils/` 目录，里面只放一个 `UserFactory.go` | 出现 `new_dirs` 超标项；提示里包括目录名、文件名，以及"目录里只有一个文件"，共三条 |
| A14 | worktree 已有进行中的 Task 时再新建 Task | 返回 `ACTIVE_TASK_EXISTS`，同时返回现有 Task 的摘要 |
| A15 | 对每个 Agent 执行 install、doctor、uninstall | 只写入、只删除安装记录里的文件；手动删掉一个 hook 文件后，doctor 能发现 |
| A16 | 新增一个没人调用的 Go 函数；然后再新增一个 | 第一个：`unused_symbols = 1`，等级 `justify`；两个：等级 `user` |
| A17 | 新增一个只被测试调用的生产函数 | `test_only_symbols = 1` |
| A18 | 新增一个只有一个实现的 Go 接口；然后加上第二个实现 | 第一种情况：`new_abstractions = 1`；第二种情况：0 |
| A19 | 只改了某个文件的缩进和空行 | `format_only_files = 1` |
| A20 | 把一个文件重命名，同时改了 3 行 | 不算新文件，只计入 3 行改动 |
| A21 | 把一个函数从 a.go 挪到 b.go | 不算新符号 |
| A22 | 新增一个测试，只调用了没被改过的函数 | `unrelated_tests = 1` |
| A23 | 新增 3 个结构相同、只是数据不同的测试 | 出现"合并成表驱动测试"的提示 |
| A24 | 验收检查还没通过时，run 全量检查 | 被拒绝；验收检查通过后可以执行 |
| A25 | 改一个没被 `./internal/auth` 导入的 Go 包，然后 run `go test ./internal/auth` 的检查 | 返回 `reused` |
| A26 | 不带用户原话选 L 规模；选 M 规模但没写理由 | L：`USER_QUOTE_REQUIRED`；M：`INVALID_INPUT` |
| A27 | 扫描文档、Skill 和输出模板 | 没有第 4 节列出的禁用词 |

## 18. 怎么判断 TaskBelay 有用

源码测试只能证明功能能用，证明不了 TaskBelay 有用。发布 1.0 之前，至少做一次对照实验：

- **任务：** 6 个真实任务，S、M、L 各 2 个。每个任务做两遍：一遍只用 Agent，一遍用 Agent 加 TaskBelay。两遍使用同一个模型、同一个起点 commit。
- **记录：** 生产新增行数、新文件数、新抽象数、没用到的符号数、测试新增行数、测试实际执行次数、用户插手的次数、返工次数、总耗时，以及交付结果是否满足需求（由用户判断）。
- **判断标准：** 交付质量不下降，同时生产新增行数或测试执行次数至少减少 30%，用户插手的次数不增加，就算有用。达不到就回来改这份说明书，而不是继续加功能。
- **调预算：** 用报告里的"声明规模 / 实际规模"，以及每个指标的实际数值，调整第 7.5 和 8.1 节的预算。

## 19. 可以改的默认决定

以下是这份说明书替你做的决定，重写开始前可以修改：

- 4 个 Agent 都支持，但先做 Claude Code 和 Codex，再做 ZCode 和 DeepSeek Harness。
- 删掉 WebUI、桌面宠物和交互式菜单。
- 文档只保留中文和英文。
- 符号分析先支持 Go、TypeScript/JavaScript、Python、Java/Kotlin、Rust。
- 预算表的数值、占用的过期时间。
- 检查命令用 shell 字符串，不用参数数组。
- 配置文件用 JSON 格式。
- 所有组件共用一个版本号，从 0.4.0 开始。
