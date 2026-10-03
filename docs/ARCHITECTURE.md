# TaskBelay 架构

[中文](ARCHITECTURE.md) | [English](ARCHITECTURE_en.md)

> 本文说明当前工作区与分支实现、协议和持久化。判断是否适合使用，请先读
> [README](../README_zh-CN.md) 和[产品定义](PRODUCT.md)。完整命令见[命令参考](COMMANDS.md)。

## 核心原则

TaskBelay 只保存一份业务状态。Go Core 管理 Task、节点、合法流转、范围、验证、Recovery、Blocker、
claims 和 outcome；Codex、DeepSeek、Claude Code、ZCode 与 WebUI 是 Host Adapter。Core 只读观察 Git，Host 才能在用户
确认后执行 fetch、branch、worktree、relaunch、handoff 和 cleanup。

```mermaid
flowchart TB
    U[Developer] --> H[Codex / DeepSeek / Claude Code / ZCode Adapter]
    H --> A[只读改动量评估]
    A --> C{选择 TaskBelay?}
    C -->|否| D[直接开发 · 无 Core Task]
    C -->|是| P[选择工作位置 · 默认原目录新分支]
    P --> W[Host 启动记录与工作区准备]
    W --> M[Local STDIO MCP · 18 tools]
    M --> S[Application Service]
    S --> G[Read-only Git Observer]
    S --> F[Workflow / Recovery]
    S --> DB[(Current-only SQLite)]
```

## 创建 Task 之前

Host 根据当前用户指令和适用的 `AGENTS.md` 调查仓库范围。指令要求检查项目索引时，先在现有权限内
只读检查索引、候选项目说明及相关代码与配置，再将完整候选仓库列入评估。每个仓库都需完成后续
确认和 provisioning；Core 保存确认后的固定 Scope。代码索引偏好只在用户和 AGENTS 未指定检索方式时生效。

新请求、显式 selector 和并行批次先由 Host 做只读评估。允许读取请求、仓库说明、相关代码、调用方、
测试、manifest 与 Git 状态；不得调用 TaskBelay Core、运行测试、fetch 或创建 branch/worktree。输出包含：

```text
change_level: small | standard | large | uncertain
observed_repositories
candidate_components
candidate_paths
public_contract_flags
persistence_or_state_flags
host_or_platform_flags
verification_shape
unknowns
recommendation: direct | taskbelay | clarify
reasons
```

Codex 沿用当前请求和评估下仍有效的明确选择与授权；没有待决定事项或必需输入时直接继续，不为进度说明或技能规则解释增加“确认后继续”的暂停。需要输入时，在同一回复中提出具体问题；开发方式、工作树参数、理解确认、独立操作授权及阻塞处理仍按各自规则执行。

评估绑定 request、canonical root、HEAD 和 status digest。等待选择期间任一项变化都要重新评估。
明确 resume 或已核验启动记录的 bootstrap 接续不重复评估。bootstrap 复用已保存的评估和确认，
仍须核对记录的 surface、phase、目标工作树身份与权限，再决定初始化、创建或恢复 Task。

用户选择 TaskBelay 后，`workspace_mode` 默认采用 `new_branch`，从当前 HEAD 在原目录新建任务
分支；显式选项还有 `current_branch` 与 `dedicated_worktree`。Core `WorkspaceOrigin.mode` 保存同一
选择。本地模式使用本地来源、当前起始分支和 HEAD；新分支创建由 Host 负责，Core 只读验证。
`carry_changes` 明确接受初始改动，本地模式保留文件及 index，不应用复制快照。准备前和切分支前
由 Core 的 `host-check workspace-available` 只读检查占用；创建仍由 Core 一次取得全部 claims。
检查不预占目录，Host 保持单一执行者。新模式复用现有内容、历史及实例摘要，不修改 SQL 表结构。

Codex 本地模式使用 `current_session` surface：`prepare -> local-provision -> scope -> open_task`。
`handoff_file` 和 `handoff_digest` 为 null，当前会话已获全部目录权限时直接继续。DeepSeek 全本地
选择返回 `ready` 与 `open_task`，在原 Workspace Root 继续；含独立工作树时仍 relaunch/consume，
混合选择从共同父目录启动并检查全部目录权限。

明确选择 `dedicated_worktree` 时，逐仓确认 `source_type`、`base_branch`、`carry_changes` 和新的
`target_branch`。远端来源确认 `remote_name` 后执行精确 fetch；本地来源直接解析本地 ref。Host 固定
`base_commit`，按选择保存并应用快照，核对独立工作树和 HEAD。源目录保持原样；冲突停止 Task 创建
并保留现场。完整规则见[工作位置与分支](WORKTREE-SOURCES.md)。

Host 在第一次 Git 写入前保存窄 provisioning receipt：launch/host/request digest、`handoff_digest`、源仓库身份、repository
key、workspace mode、source/base/target/carry、frozen commit、worktree path、operation status 与时间。它不保存凭据、remote
URL、文件内容或流程节点。结果不确定时读取 receipt/Host 状态，禁止盲目再次 dispatch。

Codex 创建结果由 Host Adapter 解析，包括完整返回值中单个文本块的 JSON。`clientThreadId` 写入 `host_client_thread_id` 并进入 `queued`；相同启动记录补交有效结果可从 `uncertain` 恢复到 `queued`，再由就绪结果进入 `dispatched`。这两个转换保留原派发标识，`dispatch-start` 继续拒绝重复派发；Core Task 状态不受这些 Host 记录转换影响。

## Codex 需求交接

原 Codex 会话从本次需求的完整相关讨论整理交接材料，保留用户早期要求、后续更正、明确采纳的方案、
术语与例子、范围限制、代码调查和工作要求。确定要求通过消息 ID 关联用户原话；未采纳建议、假设和
待确定问题分别保存。历史原文按消息顺序保留，后续更正决定当前要求。用户开始开发或确认分支不会将
所有助手建议变成确定要求，也不新增一次交接摘要确认。

交接中的工作要求仅包含会话专属指示和授权。
全局及仓库 `AGENTS.md` 由目标 Codex 会话正常加载，交接材料不重复其正文或摘要，也不将自动注入的规则块保存为原始需求讨论。
用户在对话中提出的规则修改仍保留为真实需求。
目标无法自动加载的适用规则注明来源路径、适用范围和具体原因，优先引用可读取的源文件；源文件不可读取时仅补充本次需要的规则文本并排除凭据。
可用性尚未核实时记入 `open_questions`，不预防性复制整份文件。

内容筛选由发送会话负责；格式校验和渲染保留已提供的内容，不通过关键词判断规则来源或删除文本。

`packages/codex/lib/task-handoff.mjs` 负责材料格式、保存和 Prompt 渲染；`task-launch.mjs` 负责在现有
启动步骤中调用它。原会话在只读评估时整理内容，在已有工作树确认后将 JSON 草稿写到所有已评估仓库
之外，并通过 `prepare.handoff_file` 传入。`prepare` 在 fetch 前保存完整 JSON、Markdown 和材料哈希。
材料位于当前 Host 产品目录的 `provisioning/codex/<launch_id>/handoffs/<repository_key>.json` 与
`<repository_key>.md`；receipt 只增加 `handoff_digest`，材料不进入源 checkout 或 Task 工作树。

桌面 `dispatch-start` 与 `cli-provision` 从同一份保存内容生成 Prompt，调用方直接发送返回内容。
完整结构化 Prompt 不超过 24 KiB UTF-8 时内联正文；超过时提供完整文件路径及读取说明。原始讨论在
两种情况下都保留在文件中，内容不截断。发送前材料缺失或被修改时，Host 在记录桌面 dispatch 或创建
CLI 工作树之前拒绝操作。现有 Core 状态图和工作树初始化步骤继续承担原有职责。

启动 Prompt 要求接收会话先按 receipt 的 surface/phase 区分首次 managed 初始化与已准备工作树的接续。
首次初始化核对固定起点和干净目录；已 provisioned 的工作树保留携带内容及后续工作，不重复应用快照。
Host 单独检查当前工作树身份与权限，再根据 Core 实际状态选择创建或恢复；读取 receipt 不等于重新检查 Git。

格式检查能确认字段和消息引用；原文是否完整、用户是否确实采纳建议仍由发送会话根据实际讨论判断。
原会话只能使用可读取的原始消息；无法读取的历史在 `open_questions` 中说明，不把摘要重建成原文。

验收使用 `packages/codex/tests/task-handoff.test.mjs` 检查多轮更正、来源引用、Unicode、长文件和保存
内容完整性；`packages/codex/tests/task-launch.test.mjs` 用本机临时 Git 仓库验证两个启动入口一致、删除
草稿后仍能发送、材料修改后停止发送。这些检查不代表实际 Codex 新会话已完整理解需求。

## WorkspaceOrigin 和 RepositoryBinding

创建新 Task 的 `taskbelay_open_task` 保存请求、初始范围、已知验收和 method profile，不接收最终
verification budget；为 primary 接收 `workspace_origin`，每个 additional
repository 也带一个同形字段：

```json
{
  "mode": "dedicated_worktree",
  "source_type": "remote",
  "carry_changes": false,
  "remote_name": "origin",
  "base_branch": "main",
  "base_commit": "<frozen SHA>",
  "task_branch": "feature/example",
  "provisioning_receipt_id": "launch-example"
}
```

Core 不信任这段文本本身。Observer 核对本地 branch、HEAD、来源 ref、common dir、
worktree-specific Git dir 和已确认的携带选择，再补齐并保存 `WorkspaceOrigin`：

```text
mode
source_type
carry_changes
remote_name
base_branch
base_commit
task_branch
source_repository_group_digest
canonical_worktree_root
worktree_git_dir_digest
provisioning_receipt_id
```

当前 `RepositoryBinding` 是一次不可变观察：

```text
worktree_instance_digest
identity_digest
history_digest
content_digest
current_branch / detached
current_head
head_tree
history_relation
changed_entries
task_surface
observed_at
binding_digest
```

`changed_entries` 是按数量限制保存并排序的 path/change type/file mode/gitlink/content digest，不保存文件正文。
`task_surface` 是相对固定 base commit 的 committed diff，再叠加 index、worktree 和 untracked 状态；rename
按旧路径 deleted 与新路径 added 处理。`CurrentChangedPaths` 每次从当前 surface 推导，恢复到 base 的
路径不会继续阻塞交付。

`content_digest` 表示当前有效内容和 mode，而不是 commit ID 或 staged 标签。因此只把完全相同内容
提交成线性 commit 时摘要不变；内容变化才使 Test 与 Comprehension 失效。Action 绑定 issuance
identity/history/content digests，Recovery 也使用这些事实。

Repository 保留 index 与工作文件各自的原始摘要。路径未设置 `assume-unchanged` 或 `skip-worktree`，
Git 判断文件没有未暂存差异、mode 相同，且工作文件
仅将 CRLF 转成 LF 就与 index 一致时，有效内容沿用工作文件摘要；因此 Git 的换行转换不会让纯暂存或
线性提交使验证失效。实际工作文件字节、mode 或暂存内容的其他变化仍进入摘要，观察过程不写 index。

## 工作树观察和 Blocker

`taskbelay_open_task` 的显式 resume 与 `taskbelay_get_next_action` 在返回实际工作前观察所有 Task roots。
普通 Action、Recovery 和 cancel 也使用同一观察与分类路径。

| 观察 | 结果 |
| --- | --- |
| 目录内的其他修改 | 本地模式计入观察；独立工作树的源 checkout 修改与 Task 分开 |
| task branch 线性前进 | 重新计算 surface，继续 |
| 相同内容被 commit | 保留 Test/Comprehension |
| 内容变化 | 使对应下游记录失效 |
| 计划外路径 | `file_scope_decision` Blocker |
| branch switch、detach、rewind、rewrite | `workspace_history_conflict` Blocker |
| 原 worktree 或其 Git dir 丢失/被替换 | `WORKSPACE_UNAVAILABLE`，不能普通 resolve |
| 两次读取期间继续变化 | 不稳定观察，零 Task 写入 |

结构化工具写计划外路径前仍调用 `host-check pre-file-write`；`allow_once` 绑定 source Action、精确路径
和 intent，`expand_scope` 回到 TASKS，reject/restore 要求实际恢复。Bash 或外部进程可能先写，Core 在
下一次观察中用当前内容摘要建立同样的范围决定。两种目录形态都没有“外部改动可忽略”分支。

DeepSeek 写入新目录前，从目标最近的现存父目录请求 Core 定位仓库，并将完整绝对目标路径交给范围
检查；多仓 Workspace Root 下仍按目标所属目录定位。Core 定位失败时保留输入目录，以核对已有占用；
超时、输出超限等观察失败不能按“没有活动 Task”放行。正常无 Task 的仓库和非 Git 目录仍允许普通写入。

## Action 提交

八个普通节点提交工具只接收 Host 的语义结果、artifact、method result、合法 transition、summary 和
reason。所有 node result 都不再含 `changed_paths` 或 `no_file_changes`。Core 在提交前观察 Git，计算
相对 Action issuance 的 delta 与完整 Task surface，检查当前节点 allowed effects、process artifact 和
ExpectedPaths，再构造一次完整 `TaskMutation`。

一次普通 mutation 的顺序是：

1. 校验 Task/Action/revision/process，并检查 payload 只包含约定字段；
2. 观察并分类全部工作树；
3. 计算 Action delta、当前 surface、记录失效与目标节点；
4. 在内存中验证完整 Task、Action、Event 与 Claim 结果；
5. 暂存规范化 Action operation；
6. 在一个 SQLite transaction 中 CAS Task、追加 Event、更新完整 claims，并标记 operation applied。

响应丢失时 Host 只保留 Task ID 与 Action ID，读取 Core retained operation 后按 `next_advice` 继续；不
重新拼装 payload，也不从文件状态猜测提交是否成功。

blocker 的普通提交和保存后恢复共用 `recovery.ValidateBlockerResolution`，检查文件范围决定、已确认历史
及保存的观察是否仍匹配。Application 负责构造载荷与 mutation，Recovery 负责纯判断。单仓和多仓遵守
同一规则；stage 已成功而 commit 未完成时重放原决定。再次漂移时保留已有 blocker 和待恢复操作，不创建
以 BLOCKED 为恢复节点的新 blocker。Relocation 继续使用独立的目的地观察和迁移检查。

未执行的写前请求（file-scope record 的 `observed=false`）只核对保存的当前 Repository 到新观察的
增量，不要求该请求覆盖未变的既存 Task surface。这个判定比较路径并集以及完整 base/index/worktree
模式和摘要，因而也包含消失路径及仅暂存层变化；计划和已保留流程文件以外的新变化必须属于该
pending 的精确路径。仓库实例、分支、HEAD 必须保持，不能借文件范围决定接受历史变化。已观察
请求（`observed=true`）继续核对完整未解释范围。这些规则由普通提交与恢复共同执行。

未变内容没有因此获得授权。使用 `expand_scope` 后仍须在 TASKS 逐项安排保留和开发责任，并确认
修订计划；`expected_paths` 的精确路径没有只读标志，保留承诺依靠 Host 执行约束和内容核验。
TASKS 的现有写前检查不提供产品文件隔离，Host 在该阶段只做规划，节点提交仍检查允许的 effects。


## 完成记录关联

`internal/domain/completion.go` 校验计划、实现、验收和当前检查的记录关系；Application 在写入前返回具体的字段或转换错误，并在构造 mutation 时再次使用同一约束。持久化快照也校验这些关系。

IMPLEMENT→TEST、REFACTOR→TEST 要求 `completed_work_item_ids` 覆盖当前计划全部工作项。返工转换仍可保存部分完成的实现记录。DELIVERY 的 `acceptance` 是调用方必填的语义结果，每项包含 `criterion`、`status`、`work_item_ids`、`evidence_ids`。验收条件按当前 Requirements 顺序完整出现；关联工作项必须已完成并包含对应 `acceptance_indexes`；检查 ID 必须属于当前 Test、状态为 passed 且属于当前 Task Plan revision。Core 只补齐 Test/Comprehension record IDs 和汇总验证记录 ID，不自动生成通过的验收结果。

## HTTP 与 MCP 的共同提交路径

普通 Action 使用 `Service.SubmitAction`，blocker 使用 `Service.ResolveBlockerAction`；两者都由 Core 组装内部载荷，再经过 `StageActionOperation` 和 `CommitActionOperation`。HTTP 保留当前页面的 revision 检查。`workflow.ActionSubmissionSchema` 定义共享语义字段，MCP 适配 Host Schema 限制，WebUI 使用同一份语义结构生成表单。

`recovery.ActionCorrectionPaths` 统一普通 Action 零写入错误的纠正资格与允许字段。MCP 和 HTTP 仅投影
字段路径与恢复响应；HTTP 保留 `payload.` 前缀并返回 `allowed_paths`，不维护另一份可纠正规则白名单。

任务详情返回 `pending_action_id`，表示 Core 已保存但尚未应用的操作。HTTP 恢复只接收 Action ID，由 `GetTask` 和 `RecoverAction` 读取保存载荷并决定下一步。浏览器网络异常先查询 Core，待恢复期间隐藏普通提交表单；刷新页面后仍从 Core 读取待恢复标识。浏览器不再组装内部 OperationProbe。

### Action HTTP 字段

| 路径 | 请求字段（另含 csrf） |
| --- | --- |
| `POST /api/tasks/{task_id}/actions/submit` | request_id、task_revision、action_id、payload；payload 使用当前表单返回的语义字段 |
| `POST /api/tasks/{task_id}/recovery/assess` | action_id |
| `POST /api/tasks/{task_id}/recovery/apply` | action_id |

详情的 `pending_action_id` 为空时表示当前读取没有待应用操作；存在时页面显示恢复入口。Core 的再次读取和 Action 校验决定实际执行结果。

## 验证计划、预算增加和复核范围

`standard-development` 保留现有 11 个节点。REQUIREMENTS 和 DESIGN 由 Host 展示并讨论，既有前进与返回边保持。TASKS 的完整出边为：`tasks_plan_saved` → TASKS（完整 baseline、空 findings、null confirmation），`tasks_ready` → IMPLEMENT（baseline 为 null，当前计划已明确确认），`tasks_require_design` → DESIGN 和 `tasks_require_requirements` → REQUIREMENTS（null baseline/confirmation、非空 findings 和具体 reason）。等待确认不建立 blocker。

`task_plan` 保存 `confirmation` 和 `confirmed_at`。确认包含 source=user、status=passed、summary、requirements_digest、design_digest、task_plan_digest 和 task_plan_revision，必须全部匹配当前保存内容。Core 自行记录确认时间。保存草案包含 expected_paths、工作项、验收映射和 verification_plan，并递增计划轮次；确认只引用该草案，不同时替换内容。重新保存、上游修订或 expand_scope 后旧确认不能用于开发。每次进入执行节点都要求有效确认；恢复读取同一草案无需重复保存。当前 SQLite Schema 与快照同步更新，仅完整合法的已部署 0.7.0 布局可按下述窄升级读取。

Codex `prepare` 接收完整 `assessment`（含 anchor）与 `user_choice`，先校验评估、根集合、未知项和明确选择，再执行 receipt/Git 准备。receipt.admission 保存这两个对象，重复 prepare 须与记录匹配；status/bootstrap/scope 接续已有回执。真实会话展示和用户回答由 Host 负责，校验不能证明自然语言请求必然触发 Skill。

最终验证预算不属于创建时的 `TaskIntent`。TASKS 已经完成 Requirements、Design、工作拆分、影响面和
现有测试结构分析，因此 `TaskPlanBaseline.verification_plan` 在这里保存：

```text
checks[]: name + rationale
initial_budget: level + max_automatic_commands + allow_full_suite + allow_manual_handoff
full_suite_expected
test_code_changes_expected
```

TASKS 还包含必需 method step `tasks.plan_verification`。没有完整计划及当前用户确认不能进入 IMPLEMENT。

Evidence 绑定 `task_plan_revision`。自动命令消耗只统计当前 Task Plan revision；计划被正式重建后使用
新计划的初始预算，旧 Evidence 和调整仍保留为历史。当前容量不足时，Host 在运行额外命令前提交
`verification_budget_increased`。这是 TEST→TEST 自循环，要求：

- `basis` 只能是 `new_impact`、`new_risk`、`verification_failure` 或 `verification_gap`；
- transition reason 具体说明新事实，`additional_checks` 记录需要补充额度的检查与理由；
- 自动命令数量和 full-suite/manual-handoff 权限只能单调增加，并且只增加当前所需部分；
- checks、失败、未验证、handoff 和 findings 列表为空；本次调整不生成 Evidence、TestRecord 或验证尝试。

Core 保存调整前后预算和原因，并签发新的 TEST Action。无具体原因、无检查说明或没有实际增加的提交
零写入拒绝。普通 TEST 结果必须发送 `budget_adjustment=null`。

完整套件结果的每个 check 还带非空 `full_suite_reason`；非完整套件该字段必须为空。这个字段保存本次
判断，但 Core 不解析 shell，也不能在执行前拦截全部命令。Codex/DeepSeek Skill 负责在每条命令前选择
与当前改动最接近的检查，在每次完整套件前重新判断影响、定向检查是否足够、待补风险和仓库检查点，
并在修改测试文件前判断长期价值。

修改后的代码复核同样由 Host 根据当前改动判断：只读当前 diff、直接或间接影响的调用路径和验收所需内容。
修复复核发现后只重新确认原问题与相关回归。显式 code review 保持只读并在完整交付发现后停止；
无因果关系的历史问题不进入当前复核、Task 工作或交付报告。

## Relocation、取消和终态

工作区迁移只接受全部采用 `dedicated_worktree` 的 Task，本地模式在进入 BLOCKED 前拒绝迁移。
`taskbelay_prepare_task_relocation` 把当前 Task 放入 `BLOCKED`，保存 relocation ID、源 bindings、base、
content、surface 和 resume node，源 claims 继续有效。Host 执行一次同机 handoff。随后
`taskbelay_resolve_blocker` 提交 relocation ID 与目标 repository paths；Core 验证同一 repository group、
base、等价 surface 和 claim 可用性，在一个 transaction 中替换全部 bindings 与 claims，再恢复节点。

普通 `taskbelay_cancel_task` 仍先观察工作树。原实例确实丢失时，只有
`taskbelay_abandon_task(host, task_id, revision, reason)` 可以保存最后已知 binding、进入 CANCELLED 并释放
claims；它不访问或删除 Git 对象。

DONE/CANCELLED 只结束 Task 和释放 claims。本地目录与分支保留，cleanup 不适用。终态投影 source/base/base commit、task branch/current
HEAD、worktree path、clean/dirty、当前 paths 和验证记录。keep/review/handoff/worktree cleanup/branch
cleanup 是 Host 后续操作，其中两个 cleanup 分别授权。

## 基线历史与已准备的分支改名

`BaselineHistory` 按原保存顺序完整保留真实引用，无 archive 层、保留窗口或配置性条数上限。
每条引用仍校验类型、revision、digest、摘要和时间，每种类型从 1 连续、唯一，当前基线接续最高历史
revision。Store 在同一 CAS 事务中比较前后数组，拒绝删除、改写或重排已有引用；新增引用随 Task、
Action commit 与 Event 原子提交，失败保留原快照及可恢复的 staged operation。历史只保存引用，不补造全文。
历史不计入原 1 MiB 快照聚合字节预算；其余 Task 字段及所有输入/证据限制保持。编码、解码、读取、
Stage 和 Commit 保存完整数组，不另设历史总字节上限。资源与 uint32 revision 表示范围仍构成实际边界。
分页只约束传输：默认 Task 投影显示完整数组的有界首页、总数、下一游标及 Task revision；显式读取
按最多 32 条、64 KiB 实际 JSON 字节缩页，整个 MCP 响应仍受 1 MiB 保护。游标始终指向最后返回项。

Schema 0.8.0 仅增加 `branch_rename_operations` 审计表及其 pending 索引，不含历史归档表。仅完整合法的已部署 0.7.0 布局可读取并
在一个事务内升级，保留 Task 和 pending operation 原字节。DDL 或校验失败保留原 0.7.0；其他布局拒绝，
不引入迁移注册表或回退。升级后的数据库不能由旧可执行文件继续写入。

`PrepareTaskBranchRename` 把一个普通节点暂停到现有 BLOCKED，保存单个 repository key、源/目标名、
原因、全部仓库 binding、完整逻辑 index 条目及标志、resume 节点。准备时源 ref/当前分支/HEAD 一致且目标
ref 不存在。Core 只读 Git；Host 执行一次非 force `git branch -m`。complete 要求源 ref 消失、目标为当前
分支且 HEAD 不变，所有仓库实例、身份、历史、内容和 index 不变。cancel 要求原事实及目标不存在仍成立。
两种决定复用 ResolveBlockerAction、StageActionOperation/CommitActionOperation 和 recover_action；pending
普通提交阻止准备，旧 Action/revision CAS 不能在准备后提交。漂移保持 BLOCKED，Task 取消沿用 CANCELLED。
流程摘要与普通节点完整出边不变。审计准备/结果与 Task/Event 同事务保存，后续改名不删除旧审计。

WorkspaceOrigin.task_branch 保留创建来源，RepositoryBinding.current_branch 表示 Core 已批准的有效分支。
普通 Git 观察不能批准切分支；历史复核、relocation 和 Host cleanup 使用保存的有效 binding。Host receipt
保留原 target 名。cleanup 必须使用终态 Core 事实并核对回执工作区和 HEAD，不能猜测或退回旧名。

## MCP、Store 和 WebUI

当前 MCP 工具列表固定包含十八个工具：

```text
taskbelay_server_info
taskbelay_open_task
taskbelay_get_task
taskbelay_get_next_action
taskbelay_submit_requirements
taskbelay_submit_design
taskbelay_submit_tasks
taskbelay_submit_implementation
taskbelay_submit_test
taskbelay_submit_comprehension
taskbelay_submit_refactor
taskbelay_submit_delivery
taskbelay_resolve_blocker
taskbelay_recover_action
taskbelay_cancel_task
taskbelay_prepare_task_branch_rename
taskbelay_prepare_task_relocation
taskbelay_abandon_task
```

Store 只实现当前 SQLite Schema、严格 snapshot codec、Action operation、append-only TaskEvent、claims 和
revision CAS。claim key 使用可直接
观察的 worktree instance identity，使写前 hook 即使遇到非法 branch switch 仍能找到 Task。

启动校验在同一个读事务中读取 Schema、Task、Action operation、relocation、claim 和 event。
活跃数据库预检与普通只读连接都使用 SQLite `mode=ro`，不把仍会变化的数据库声明为 immutable。
并发提交只会体现为一个完整版本，不会把先读到的旧 Task 与新 operation 或 claim 混在一起。
不支持的 Schema 和不一致的记录仍直接报错，不增加回退或重试。
只读预检不写 Task 或持久化数据库页；读取活跃 WAL 时，SQLite 的锁协调可能更新已有 `-shm` 的读者标记。
对应检查分别核对数据库与 `-wal` 内容不变、sidecar 集合不变，不把共享内存读锁误报为 Task 写入。

WebUI 是 loopback HTTP Adapter，只投影 WorkspaceOrigin、当前 observation/surface、blocker、relocation、
verification plan、当前预算/消耗、调整原因和 cleanup choices。它展示三种工作区模式及对应的迁移、清理操作；创建仍从 Host 启动入口进行，
也不执行 Git 或 Host handoff。

## Host 差异

- 独立工作树模式中，Codex App 使用原生 managed worktree、snapshot、task creation 和 handoff；Skill 保存一次 launch 状态，
  child 初始化目标分支后才 open Core。Codex CLI 使用
  `codex -C <worktree> [--add-dir <additional-worktree>] -- <prompt>` relaunch。
- DeepSeek 的 Workspace Root 在进程启动时固定。本地模式沿用原会话；独立模式由 WorkspaceCoordinator 创建安全 sibling worktree，输出
  `{command,arguments,cwd}` relaunch descriptor；新会话消费 receipt 后才 open Core。它不扩大旧会话权限，
  也不在源仓库内嵌套 worktree。
- 多仓库 Task 要求所有 roots 全部 provision、授权和验证；任一失败时不创建部分 Task 或 claims。

### Claude Code

`packages/claude/` 独立拥有 Claude 插件注册、MCP/Hook 传输、会话和启动记录。插件根是完整 package 根，缓存内包含 lib、bin、runtime 与 Skill；不得引用缓存外兄弟包。`packages/host-workspace/` 是 Git 观察、准备、快照的维护源，构建向各 Host 复制，不包含 Core 节点或 Claude/Codex 会话决策。

Claude 的 Write/Edit/NotebookEdit 解析完整目标和原始输入摘要，再由 Core file_scope 判定。放行不覆盖 Host 权限。启动记录保存请求、来源、操作状态及 Claude 会话身份，不保存第二流程游标。独立工作树迁移保留部分操作结果，Core 最后核对全部新绑定。统一管理器由 `hosts/claude.mjs` 管理包和注册，并读取 Claude 私有安装记录，向公共 runtime 提供 WebUI/宠物可用的 Core 候选。

### ZCode

`packages/zcode/` 拥有 ZCode 原生插件、MCP/Hook 传输、本地准备记录和工作区接续说明。独立 `zcode` 身份参与 Core、MCP Schema、配置偏好和 Task 所有权校验；它不复用其他 Host 身份，也不改变 SQLite 布局、流程定义、节点或边。

包根内的 `.zcode-plugin/plugin.json`、`marketplace.json`、`.mcp.json`、`skills/taskbelay/`、`hooks/hooks.json`、CLI 和两个 Core runtime 构成自包含产物。ZCode process executor 使用独立 command/args 和 `ZCODE_PLUGIN_ROOT`，Hook 由标准目录发现。Write/Edit 提取完整 `tool_input.file_path` 与原始输入摘要，交给 Core 判定。Core 明确拒绝时，Hook 输出 `permissionDecision=deny` 并退出 0；事件解析或检查调用异常时退出 2，拒绝受保护操作。退出 0 本身不代表写入获准。Shell 与外部写入仍依赖后续观察。

Adapter 校验本地文件和 Core 后保存准备记录，统一管理器的 `hosts/zcode.mjs` 负责包发现、生命周期与 Core 候选。记录只证明本地来源，无法观察 UI 安装、缓存或启用，因此返回 `action_required`；真实 Host 加载与模型会话验证另行记录。普通卸载保留 Adapter 和 `removal_required` 记录，用户在 UI 移除插件并关闭会话后通过 `remove --confirm-host-removed` 清除记录，再卸载包。存在包或记录时拒绝共享数据 reset，避免将源包进程检查误当作全部缓存进程已停止。

工作区 Git 操作复用 `packages/host-workspace/`，命令调用复用 `packages/host-command/`；生成副本只依赖当前包。`open`/`resume` 返回目录与接续提示，由实际 ZCode UI 完成打开和授权；不生成虚构会话身份或维护第二流程游标。平台目录负责路径和进程差异，Core Task 语义不分支判断操作系统。

## 版本、构建和源码导航

Core、Codex、DeepSeek、Claude、ZCode 和统一 lifecycle package 独立版本。Core 的机器可读版本文件是 `CORE_VERSION`；npm
版本由各自 `package.json` 管理，普通产品改造不执行发布。Host package 按精确 runtime pair 携带
`darwin-arm64/taskbelay` 与 `win32-x64/taskbelay.exe`。

| 路径 | 职责 |
| --- | --- |
| `internal/domain/` | Task、WorkspaceOrigin/Binding、verification plan/adjustment、records、blocker、outcome |
| `internal/repository/` | 固定、只读 Git observation 与摘要 |
| `internal/application/` | open/resume/read/submit/recover/relocate/cancel/abandon 编排 |
| `internal/workflow/` | 11 个节点、普通边、payload、guard、invalidation |
| `internal/store/` | current-only SQLite、codec、operations、events、claims |
| `internal/mcp/` | 十八个工具、字段限制、工具属性和统一返回结构 |
| `internal/webui/`, `packages/webui/` | loopback Adapter 与内嵌界面 |
| `packages/codex/`, `packages/deepseek/`, `packages/claude/`, `packages/zcode/` | 新请求评估、工作树创建、会话接续/交接和安装包 |
| `packages/host-workspace/` | Git 观察、准备和快照助手的维护源；构建时复制到使用它的 Host 包 |
| `packages/host-command/` | Codex、Claude、ZCode 共用的命令执行维护源；生成到各自包内并在构建时直接装配 |
| `protocol/fixtures/`, `tests/` | 公开接口规范、故障注入和宿主完整流程测试 |

源码、机器可读 Schema、package manifest、CLI parser 和可执行测试是判断当前行为的依据。

## 桌面宠物职责

`packages/taskbelay/lib/pet.mjs` 复用已安装 Adapter 的 Core 选择与 WebUI 入口；macOS 调用位于
`lib/platform/macos/pet.mjs`。`packages/desktop-pet/macos` 负责 AppKit 窗口、只读 HTTP、展示、进程身份、
单实例和偏好。每轮观察检查同一 Core 与服务身份；取消任务使过期响应失效。每轮分页读取受阻和进行中任务，按任务 ID 去重；从列表消失的已观察任务及固定任务通过详情确认当前状态。列表缺失本身不能表示完成，读取失败保留最后成功集合；过期响应不能覆盖新选择。任务面板按需分页。
`PetTaskCollection` 和 Windows `task-collection.cjs` 负责桌面关注顺序、明确固定与本会话完成提示，Core 负责生命周期。自动关注受阻优先，同级保持稳定；完成提示结束后切换未完成任务。`PetBubbleStackView` 和 Windows renderer 负责最多三层叠加、展开滚动及逐任务点击；导航前重新核对同一 Core 与数据目录。
`productRoot/pet/settings.json` 保存位置、动画开关、按数据目录分组的 `pinned_tasks`、`selected_appearance`、默认开启的 `idle_activities_enabled` 与角色缩放比例 `scale`。自动关注和未读完成提示仅在内存中维护；`runtime.json` 记录进程身份。

macOS 宠物的 `process_start_identity` 由内核进程信息中的启动秒和微秒组成，保存为十进制 `seconds:microseconds` 字符串，与语言环境和时区无关。恢复与停止同时核对 PID、启动标识、所属用户及程序路径；原生信息读取失败时拒绝识别该进程。应用替换前先停止旧实例，新实例写入当前格式的运行记录。
Core 数据、流程图和 MCP 工具保持现有职责。

`PetMenuBarIcon` 负责以 AppKit 路径绘制 18 pt TaskBelay 流线标识，并提供模板图像；`PetMenu` 将图像安装到菜单栏按钮，macOS 负责外观着色。

`scripts/build-desktop-pet.mjs` 负责 macOS 编译、资源和 ad-hoc 签名；`scripts/build-desktop-pet-windows.mjs` 负责 Windows 应用装配，本地开发包复用这些函数。`release/taskbelay/prepare.mjs` 按源码清单暂存文件，构建 macOS 应用并装配锁定的 Windows x64 Electron 分发文件。它将两个 runtime 目录装入同一包，核对应用版本与默认素材、保留执行权限，并在写入发布记录前逐文件比对解包结果。正式包不含本地 Adapter 归档；Core 由已配置的 Adapter 提供。

包内有应用时，`plan.mjs` 为已确认的维护增加明确的宠物安装操作，即使无需更新 Adapter 也执行。`lifecycle.mjs` 在维护前停止宠物并执行该操作；平台安装器暂存并替换应用目录，保留设置与形象。启动优先使用用户目录副本，其次使用包内应用，详见[桌面宠物指南](DESKTOP-PETS.md#更新程序与素材)。

用户形象保存到 `productRoot/pet/appearances/<id>`。`PetAppearanceStore` 负责受限文件读取、导入校验和
替换；转换后的完整包在临时目录通过与加载时相同的校验后才安装。`AppearanceImages` 在像素解码前
按图片尺寸和位深检查内存预估。`CodexPetImporter` 在导入时拆分 Codex 标准格式 1/2 图集与 TaskBelay
自有高分辨率扩展图集，完整保留九类动作、57 帧与原始单格分辨率。`AnimationCatalog` 定义五类必需
任务动作和四类可选附加动作，并校验所有已提供动作；`PetAppearanceSelection` 负责资源加载成功与选择保存
的一致性；`PetCharacterView` 负责统一播放，并将素材锚点换算为 AppKit 坐标。`SVGArtwork` 负责静态矢量内容和尺寸校验，AppKit 保留 SVG 表示并按显示尺寸绘制。偏好增加 `selected_appearance`，与按数据目录保存的
`pinned_tasks` 独立；偏好更新在同一锁内完成，写入失败保留原值。切换释放旧帧，直接显示当前状态，
不重新播放旧提示。

`PresentationRules` 从 Core 快照决定任务展示和审核节点动作。`PetActivityController` 负责本地休闲动作、
冷却与下一次截止时间；普通轮询保持已有动作和截止时间。`PetController` 连接窗口事件、一次性唤醒计时与播放请求，
`PetAnimationTimeline` 按单调时间与素材时长计算当前帧；`PetCharacterView` 通过视图的 `CADisplayLink` 驱动播放，
只在帧变化时替换图片，有限循环结束通知一次，并在停止播放时释放显示刷新回调。`PetWindow` 以原生动画更新临时散步位置。
`PetMotion` 集中选择系统动画：macOS 15 及以上使用 `NSAnimationContext.animate` 与 SwiftUI 动画，macOS 14
使用 `NSAnimationContext` 动画分组。`PetBubbleStackView` 保持卡片和滚动容器身份，由目标布局计算高度，
在同一动画事务中调整卡片与窗口；序号隔离过期完成回调。macOS 26 的 `NSGlassEffectContainerView` 聚合卡片玻璃，
`PetBubbleView` 将文字与交互区域放入 `NSGlassEffectView.contentView`，macOS 27 启用 `effectIsInteractive`。
`PetController` 传入动画开关与系统减少动态效果选项，负责菜单、隐藏、睡眠和恢复的协调。
角色悬停与气泡悬停分别处理，任务提示可中断休闲。只有手动拖动更新保存位置；动作顺序、冷却和临时位移均留在内存中。
`PetMenu` 提供大小选择；`PetController` 保存缩放比例与位置后应用变更；`PetContentView` 调整角色约束并保持气泡文字大小，`PetActivityController` 按缩放比例计算行走速度。
完整使用、形象格式与触发规则见[桌面宠物指南](DESKTOP-PETS.md)。

`scripts/build-desktop-pet.mjs` 调用 `scripts/desktop-pet-artwork.mjs`，从 `packages/desktop-pet/default-appearance/` 复制默认蓝色海洋精灵的九类动作、57 个 PNG 帧，并逐文件核对内容。自定义形象由用户从外部包导入；`PetAppearanceStore` 负责用户目录中的形象列表与加载，`PetAppearanceSelection` 在默认形象与已导入形象之间切换。生成的应用包保存在仓库外，不纳入 Git 跟踪。

## 平台职责

四个 Host Adapter 和统一管理器在 `lib/platform.mjs` 选择各自的平台策略，具体系统实现位于 `lib/platform/windows/` 和 `lib/platform/macos/`。各 Adapter 的平台策略分别维护在对应系统目录的 `policies.mjs`；Claude 与 ZCode 的 `platformPolicy()` 保留各自的小接口，Host 注册、生命周期和会话规则继续由各 Adapter 负责。Go Core 的平台机制使用所在职责包内的平台专用文件，任务语义保持平台中立。

`packages/host-command/` 是 Codex、Claude、ZCode 共用的命令执行维护源。`command.mjs` 提供共同入口并选择平台命令实现，`platform/windows/command.mjs` 和 `platform/macos/command.mjs` 各自处理命令发现、启动器身份和调用参数。`scripts/sync-host-commands.mjs` 在三个 Host 的 `lib/` 下生成同路径副本并标注来源；构建也直接从共享源装配，安装后只依赖包内文件。统一管理器与 DeepSeek 的命令接口仍由各自模块维护。生成与一致性检查见[脚本维护说明](../scripts/README.md#共享-host-命令)。

Windows Git 进程隐藏控制台窗口；Codex 版本与状态预检使用所选平台的可执行文件策略，PowerShell 启动器使用 UTF-8。原生验证单独记录在[Windows 报告](WINDOWS-ADAPTATION.md)。

Windows Codex 注册回读核对 marketplace 的 `name`、`root` 与 Plugin 身份，并在 Windows 实现中规范化 `\\?\` 路径前缀；macOS 使用自己的回读规则。

## Windows 桌面职责

`packages/desktop-pet/windows/` 负责 Electron 窗口、托盘、渲染器、本地观察与素材处理，macOS 保留 Swift/AppKit。两者只读取 Core 状态。`scripts/build-desktop-pet-windows.mjs` 装配包含 Codex、DeepSeek、Claude 和 ZCode Adapter 包的 Windows 桌面分发包。统一入口校验内置包摘要并管理程序替换，保留 `%LOCALAPPDATA%\taskbelay\pet` 中的设置和形象。

Windows 路径实现将已有 AppData 目录解析为实际路径，包括打包桌面 Host 提供的目录别名，同时拒绝符号链接。GUI 使用 `Start-Process` 和每次启动的确认记录，避免常驻桌面进程保留调用终端的输出句柄。平台维护通过完整可执行路径、命令和创建时间识别 Core 实例，再停止需要替换的实例。

## 当前 DSH 接口

当前源码的 DeepSeek Adapter 要求 DSH `>=0.1.2-rc.1`。Adapter 通过 Session 的 `snapshotEvents()` 读取当前轮次和用户直接输入，核对 `/taskbelay`、工作树确认及结构化文件写入；Core 继续负责 Task 状态。

工作区命令执行器 `runClosedCommand()` 在子进程退出且 stdout/stderr 关闭后返回完整结果，避免使用尚未收齐的 Git 输出判断仓库身份或内容。超时、取消或输出超限会停止收集并拒绝返回成功，即使父进程已退出而后代仍持有输出管道也保持有界；已经启动的修改命令在这些情况下标记结果不确定，供 Host 保留现场并恢复。

## 生命周期 CLI 职责

`packages/taskbelay/lib/cli.mjs` 解析参数并组织交互菜单，`terminal.mjs` 保留同一次交互中的输入，`presentation.mjs` 展示计划、进度和结果。`plan.mjs` 生成维护动作与确认要求，`lifecycle.mjs` 先观察、解析目标版本、展示计划和取得确认，再执行并记录操作结果。它按明确选定的 Host/Profile 调用驱动，安装目录规范化后重新创建驱动，使后续操作使用同一组路径。`diagnostics.mjs` 汇总安装与用户配置检查。重试重新观察实际安装；安装记录不决定 Core Task 的状态。

DeepSeek 驱动从 `npm pack --json` 的列表或包名映射中读取唯一制品报告，核对包名、版本和文件名后才修改 Profile。Claude 与 ZCode 的 CLI 入口按真实文件路径判断直接执行，支持 npm 生成的命令软链接。两者的管理器驱动校验 `setup` 返回的 JSON 和预期状态后才记录注册步骤完成，再通过 `status` 回读安装；空输出或无效 JSON 的错误会指出具体命令。

| 模块 | 职责 |
| --- | --- |
| `packages/taskbelay/lib/hosts/` | Codex、DeepSeek、Claude、ZCode 驱动各自拥有包定位和私有安装记录，DeepSeek 同时负责 Profile 规则；各驱动执行已确认的 Adapter 操作。`runtimeCandidates()` 提供启动候选；`maintenanceTargets()` 提供已注册或安装中断后仍存在的包及 Core 位置。 |
| `packages/taskbelay/lib/core-runtime.mjs` | 核对公共包内 runtime 布局、package 身份、规范路径、可执行文件和 Core 版本。 |
| `packages/taskbelay/lib/core-maintenance.mjs` | 为 reset 协调已知 Core 服务位置，复用 WebUI status/stop 协议；进程检查和停止由平台实现负责。 |
| `packages/taskbelay/lib/runtime.mjs` | 汇总各驱动的候选并调用公共校验，按 Core 版本及来源排序选择运行时，准备数据目录并转发启动参数和信号。 |
| `packages/taskbelay/lib/platform/` | 接收已确定的包及 Core 可执行文件位置，处理系统路径、权限、进程停止、清理和命令参数引用。Windows 维护按实际可执行文件、命令和创建时间识别需要停止的进程。 |

reset 的共享服务停止与可执行文件替换分别处理。确认后，管理器保留已管理 Adapter 的实际位置，停止
宠物、WebUI 和可识别的 STDIO Core，卸载 Adapter，再在清理数据前复查。退出失败、进程身份变化或
匹配 Core 重连时停止清理。该检查覆盖能确认归属的受管安装，不枚举其他位置手工启动的任意 Core。
普通维护继续保留 Task 数据。
已确认的 Core 路径保存在私有 reset 维护记录中，直到数据清理成功才移除。卸载后失败的独立重试仍检查
这些路径；包已不存在的记录只用于阻止不安全清理，不能授权向未经当前核对的进程发送停止信号。

计划按规范路径合并相同清理目标，并保留显式数据目录确认；展示、确认和执行使用同一组目标。
DeepSeek Profile 的位置、格式和读写移除由 `hosts/deepseek-receipts.mjs` 负责；公共 ownership 模块
只处理受管文件与权限，不解释这些 Host 字段。

## 共享用户配置

`internal/userconfig.Decode` 统一解释 Codex、DeepSeek、Claude 和 ZCode 的配置字段及缺省值。Core 启动读取配置时直接复用它；只读 `config validate` 命令接受标准输入中的原始 JSON，供安装和诊断调用。命令限制为 16 KiB，不读取配置文件、Task 存储或 Git，结果与退出码见[命令参考](COMMANDS.md#配置校验)。

Codex setup 先检查配置路径、文件类型和权限，再把已有文件的原始内容交给包内 Core 校验；合法文件保持原样。缺少配置时只写入 `{}`，默认偏好由 Core 解释。统一管理器的 factory-reset 同样以 `{}` 初始化配置；doctor 对已有文件复用选定 Core，找不到可用 Core 时明确报告无法完成配置语义校验。Node 安装和诊断代码不维护全产品字段白名单或 Host 默认值副本。

统一管理器的 `resolveManagerPaths()` 从平台实现取得权限策略，并通过 `paths.enforcePrivateModes` 传入 `configuration.mjs`。配置校验要求该字段为显式布尔值，缺失或类型错误时在读取文件前拒绝；它只按该策略检查文件权限，不再次识别操作系统或推导缺省策略。配置内容的语义校验继续交给 Core。

## 文件提交准备

Codex 在普通提交前执行 `taskbelay-codex artifacts collect` 和 `taskbelay-codex artifacts prepare`，复用 Core 对当前 Action 的完整 Git 观察。Codex 只补充文件用途和说明，准备命令检查清单与当前观察一致后生成 artifact 数组。流程文件漏报返回具体路径和仅修改 artifact 字段的一次纠正指示；实际仓库异常继续按原有恢复规则处理。详见[文件收集与提交](ARTIFACTS.md)。

## Host 接口描述与恢复入口

Codex 的命令说明由 Host 包的 `lib/host-launch-contract.mjs` 提供；`host-launch <operation> --help` 在读取 stdin、解析安装路径或执行操作前返回输入 Schema、字段来源、输出字段和下一步。`host-launch scope` 只读取已确认的仓库记录，复用协调器的完整性检查后生成 Core 仓库参数。Git 操作继续由 Host 协调器负责。

Core MCP 的 `inputSchema` 描述提交参数，`outputSchema` 描述公开结果外层和继续执行所需的字段路径，保存记录的内容由 Core 领域校验负责。`structuredContent` 与文本内容携带同一 JSON 结果。Host 先处理 `recovery_assessment.next_advice`，再读取当前 Action；新会话从 `recovery_assessment.operation.action_id` 获取保存的提交身份。生命周期操作回读 Task、创建来源或迁移记录，不借用普通 Action 恢复入口。

Codex 启动先由 `dispatch-start` 将完整 `host_request` 保存到 `receipt.operation_status.host_request`，进入 `dispatch_prepared`；重复调用和 `status` 均可回读。`dispatch-call` 使用当前 `dispatch_attempt_id` 将阶段改为 `dispatching`，仅首次返回 `should_dispatch=true` 时允许调用一次创建工具。调用方将命令完整 stdout 写入私有文件，检查退出码并从文件解析 JSON，再原样转发请求，避免显示长度限制截断内容。

确认原调用方已停止且创建工具尚未调用时，`dispatch-recover` 接收当前派发 ID、`host_call_not_made=true`、`previous_caller_stopped=true` 和具体 `reason`，保留原请求并换发调用许可 ID；随后执行 `dispatch-call`。空任务 ID 本身不能证明未调用。已经调用但结果未知时，Host 按保存的启动标题、启动 ID 和仓库标识查找任务及归档任务，读取候选任务完整初始消息，将 `candidates`（`thread_id`、`initial_prompt`）交给 `dispatch-reconcile`。唯一完整消息匹配才保存任务 ID；零匹配、多个匹配或查询不可用均不允许重新创建。Core Task 状态保持由 Core 管理。

## Codex 携带路径核对

Codex 携带本地改动时，会在 REQUIREMENTS 中记录保留要求，在 TASKS 中将完整的 `current_changed_paths` 与 `expected_paths` 及已保留的流程文件逐项核对。新开发工作和已有内容保留分别安排工作项与检查；当前 Action 的空文件清单不能代替完整 Task 路径核对。保留检查比较启动快照，不代表已有业务功能已经验证。已发生的文件范围阻塞仍通过现有 Core 选择与转移处理。 该核对由 Codex Host 的 skill 负责，继续使用现有 Requirements、Task Plan 和文件范围规则；Core 不保存第二份路径豁免列表。

## 已有检查的验证额度

增加验证额度时，`additional_checks` 可以引用原计划或此前增加记录中的检查名称，使用 `rationale` 说明本次补做或重跑。单次提交内名称仍需唯一，具体原因、实际增加量和上限继续校验；追加额度本身不生成通过结果。

## 既有失败验收

开发者需要在相关检查通过、完整套件无新增失败时接受已有问题。Core 保存真实的失败结果、比较检查和明确验收。Core 负责唯一的 Task 状态；Host 负责实际执行比较并取得用户决定。比较记录提供检查名称、来源、结果和说明，Core 核对引用、当前内容和计划；Core 不自行解释测试日志。

`standard-development` 保留 11 个节点，定义摘要由更新后的完整定义计算。TEST 完整出边为：

| transition | 目标 | guard | reason |
| --- | --- | --- | --- |
| `tests_passed` | COMPREHENSION_REVIEW | current_tests_pass | 可空 |
| `tests_accepted_with_known_failures` | COMPREHENSION_REVIEW | current_known_failures_accepted | 必填 |
| `tests_failed_implementation` | IMPLEMENT | implementation_failure_identified | 必填 |
| `tests_expose_design_issue` | DESIGN | test_design_failure_identified | 必填 |
| `tests_expose_requirement_issue` | REQUIREMENTS | test_requirement_gap_identified | 必填 |
| `verification_budget_increased` | TEST | verification_budget_adjustment_justified | 必填 |

TEST 进入条件仍为当前实现、仓库绑定和验证计划有效；允许读取仓库、运行检查、编辑流程产物和请求用户决定。完成时全部检查分类、额度有效、无未执行或待办检查。既有 `test.run_budgeted_checks`、`test.record_evidence`、`test.classify_failure` 分别执行必要检查、保存真实结果和区分新增/既有失败；后者同时取得确切失败集合的用户验收。

TEST 结果增加可选 `known_failure_acceptance`，只用于新增转换：`source=user`、`summary`、`failed_checks`、`comparison_check`、`task_plan_revision`、`content_digest`。失败集合必须恰好覆盖本次所有 failed 检查；失败检查须来自实际自动检查，其余检查全部 passed。比较引用必须指向本次单独的 automated/passed 检查；Host 的比较说明应明确失败项与错误内容没有新增或变化。用户确认绑定所见的计划轮次和内容摘要，Core 核对当前值。未比较、新增失败、缺少明确验收、过期确认、skipped/not_run/observed 均不能由此路径放行。

Core 将验收与原始检查一起原子保存在当前 TestRecord，时间字段为 `completed_at`，验收引用在记录内由检查名称关联 evidence IDs。失败仍为 failed，不生成伪造的用户 passed 检查。通过检查负责证明本次需求已满足；接受失败的记录负责保存已知问题，不能充当通过检查。

COMPREHENSION_REVIEW 和 DELIVERY 的进入条件改为当前测试已完成并满足普通通过或既有失败验收规则。它们的完整出边和各 guard/reason 保持上表之前的流程定义：理解确认可到 DELIVERY、IMPLEMENT、REFACTOR、DESIGN、TEST、REQUIREMENTS；交付可到 DONE、IMPLEMENT、TEST、COMPREHENSION_REVIEW、DESIGN、REQUIREMENTS。交付继续逐项关联真实通过的检查；失败集合和验收随 TestRecord 展示，自动/人工结果清单只列 passed 检查。内容或计划变化沿用当前失效处理，同时使附属验收失效。

保存布局使用当前 Schema；仅完整合法的已部署 0.7.0 布局按上述事务升级读取，其他历史布局拒绝。MCP、CLI 和 WebUI 返回相同记录和转换，错误遵守 [Core 响应规范](CORE-RESPONSES.md)。

验收使用 Core 单元和保存边界集成测试：全通过、已有失败且明确验收、缺少比较/确认、新失败遗漏、确认过期、重启后交付、真实数量超限、已完成用户检查、权限限制及检查说明为空后的零写入纠正。共享 Skill 示例验证四个 Host。该方案增加一个明确转换和附属记录，收益是保持失败事实并正常交付；不增加节点、第二套状态、自动测试日志解析、通用豁免或发布流程。实现范围是 workflow/domain/application/store、MCP/WebUI 直接消费者及维护文档和 Skills。
