# 工作位置、分支与本地改动

[中文](WORKTREE-SOURCES.md) | [English](WORKTREE-SOURCES_en.md)

## 使用场景

开发者希望复用当前目录中的依赖、本地配置和构建输出，并让 TaskBelay 保存开发进度。默认在当前
目录从当前 HEAD 新建任务分支；也可以明确选择继续当前分支，或创建独立工作树。
Host 在创建 Task 前展示实际目录、分支和未提交修改，沿用已有明确选择。

## 工作位置与默认行为

`workspace_mode` 必填，Host 在用户未选择其他方式时使用 `new_branch`：

| 模式 | Host 准备 | Core 保存的 `workspace_origin.mode` |
| --- | --- | --- |
| `new_branch`（默认） | 在原目录从当前 HEAD 创建并切换到确认的新分支 | `new_branch` |
| `current_branch` | 使用原目录及当前具名分支 | `current_branch` |
| `dedicated_worktree` | 按下文来源规则准备独立目录 | `dedicated_worktree` |

两个本地模式要求 `source_type=local`、`remote_name=""`、`base_branch` 为启动时的当前分支，
`base_commit` 为启动时 HEAD。`current_branch` 的目标分支等于当前分支，`new_branch` 的目标分支必须
尚不存在。创建前已提交的内容属于起始代码；已确认的未提交内容和之后的改动进入 Task 修改范围。
`carry_changes` 在本地模式中表示接受原目录的初始修改，保留 index、文件及 ignored 内容，不应用复制快照。
有初始修改但未接受时停止准备；不自动 stash、清理或忽略其他需求的修改。

Host 在本地分支操作前调用 Core 的只读 `host-check workspace-available`，输入
`{"repository_path":"<canonical root>"}`，读取 `available`、`repository_path` 和可选 `task_id`。
活动 Task 占用时停止，失败或无法读取时也停止。检查不预占目录；Core 创建仍在 transaction 中取得
全部目录的唯一占用。用户和其他工具负责避免在准备期间启动另一个写入者。

当前会话已获全部目录权限时，本地模式在原会话继续。Codex 使用 `current_session` surface 和
`local-provision`；其 `prepare.handoff_file=null`，启动记录的 `handoff_digest=null`。只有会话交接才
保存需求交接材料。DeepSeek 全部使用本地模式时返回 `ready` 及完整 `open_task`，无需 relaunch/consume。
同一 Task 的仓库可以有不同选择，但执行会话必须实际覆盖每个目录；DeepSeek 混合模式从共同父目录
重启，原会话权限不扩大。任何仓库失败均不创建部分 Core Task；本地分支及修改保留供检查。

本地模式沿用原目录实例恢复，切分支、历史异常、内容变化、文件范围和验证规则保持有效。
本地模式不提供工作区迁移或辅助删除目录、分支；DONE/CANCELLED 只结束 Task 并释放占用。上一 Task
结束后仍未提交的内容会再次出现在下一 Task 的初始修改中。界面按模式展示可用的生命周期操作。

`standard-development` 的定义摘要、节点、全部出边、guard、Action allowed effects 和验证要求保持不变。
本次改变创建与工作区生命周期约定，不增加流程节点或第二个 Task 状态。

## 可用数据与职责

Host 读取本地分支、remote、源 checkout 的 HEAD、暂存、未暂存及未跟踪路径，显示待确认内容。
Host 负责 Git 创建和内容复制；Core 只读核对工作树身份、来源、分支、HEAD 和实际修改内容，并创建 Task。

`source_type` 必填，取 `local` 或 `remote`；`carry_changes` 必填，取布尔值。
本地来源的 `remote_name` 为空字符串，`base_branch` 是本地分支；远端来源要求有效的 remote 名称，
且 `carry_changes=false`。`target_branch` 是待创建的任务分支。省略来源或携带选择不会采用默认值。

## 独立工作树来源规则

1. 用户明确上述选择后才开始准备。远端执行精确 fetch；本地直接解析 `refs/heads/<base>`，无需联网。
2. Host 保存 `base_commit`，随后从该 commit 创建独立工作树；Codex managed dispatch 也使用这个固定 commit。
3. 携带本地改动时，Host 创建 Git 快照对象，记录 `snapshot_commit`。快照保留暂存与未暂存层，
   包含未被 Git 忽略的未跟踪文件。源 checkout、源 index、HEAD 和 `refs/stash` 保持原样。
4. Host 在新分支中应用快照，并恢复暂存状态。选择不同本地分支时通过 Git 应用改动；冲突或文件覆盖
   导致应用失败时，不创建 Core Task，保留目标工作树与失败记录供检查。
5. Core 核对工作树、具名分支、固定 commit 与来源 ref。本地 `carry_changes=true` 可以带着初始修改创建
   Task；其余选择要求干净工作树。携带内容属于该 Task 的实际修改范围，后续仍按范围和验证规则处理。
6. 每个仓库独立保存选择，全部准备成功后才能创建一个多仓库 Task。恢复时读取原启动记录；结果不确定
   不重复创建或应用快照。源工作区后续修改不会改变已经捕获的内容。

独立工作树不要求源 checkout 没有修改或活动 Task。选择不携带时，从固定 commit 创建干净目标，
源目录及其占用保持原样；原目录模式的修改接受和占用检查不适用于这种只读来源。

快照准备拒绝尚未解决的冲突和子模块修改。Git 忽略的文件不复制。DeepSeek 自动辅助清理继续保留
本地来源分支，供用户单独检查处理；不为本地创建过程增加远端访问。

四个 Host 使用同一份共享快照实现。返回快照前连续读取两次 HEAD、暂存内容树、工作文件内容树和
未跟踪内容树；任一项不同就停止准备，不返回快照、不自动重试。已修改文件继续变化时，即使 Git
status 相同也会按内容树判断。失败保留源文件、暂存状态和启动记录，检查源目录后再决定如何继续。
准备期间仍应只有一个写入者；该检查不提供任意外部并发下的原子快照，也不保证发现内容改变后又恢复的时序。

评估 anchor 的职责不同：它保留请求、目录、HEAD 和 status 等观察，不比较工作文件内容。
相同 status 摘要不能证明已修改文件没有再次变化；等待选择期间已知需求或代码发生变化时，Host 应重新评估。

## 启动记录与 Task 状态

Host 启动记录保存 `workspace_mode`、`source_type`、`carry_changes`、`snapshot_commit`，使用 `base_commit` 保存起点。
准备阶段为 `confirmed -> resolving -> prepared`，随后进入现有派发与工作树准备阶段。
失败或结果不确定继续使用 `failed`、`uncertain`。这属于 Host 操作记录，不增加 Core 流程节点。

`WorkspaceOrigin` 及 CLI/MCP/WebUI 投影包含 `source_type` 和 `carry_changes`，其余身份、恢复及流程
规则继续由 Core 管理。当前 `standard-development` 的定义摘要、节点、出边、guard、Action 和
验证要求不变；`tasks.snapshot` 保存工作区模式及来源字段，SQL 表结构保持不变。

## 预期结果与影响

本地分支可以离线复用现有开发环境；独立工作树的未提交内容由用户决定是否复制。应用失败不会改动源 checkout，也不会产生
误报成功的 Task。复制的内容计入 Task 修改面，不能作为已经完成的工作或已通过的验证。

## 验收方式

- Codex `task-launch.test.mjs`：临时 Git 仓库验证远端创建、本地离线两种携带选择、暂存分离、未跟踪文件、
  忽略文件排除、源内容保留、快照冻结、不同起始分支、managed bootstrap 及冲突保留。
- DeepSeek `workspace-coordinator.test.mjs`：验证明确选择、本地离线创建、二进制新增文件及重启后 consume。
- 四个 Host 的定向快照回归在首次采集 tracked 内容后修改已有 dirty tracked/untracked 文件，确认
  status 不变时仍停止准备，保留源文件、暂存树、HEAD 和 stash；生成副本检查另行逐字核对包内文件与共享源。
- ZCode `workspace.test.mjs` 使用临时 Git 仓库检查三种工作区模式、内容携带、部分准备失败、实例替换、迁移和分别授权的清理；`open`/`resume` 只返回 UI 接续说明。
- Core `workspace_observer_test.go`：验证本地来源无需 remote、初始修改须明确携带、远端拒绝携带、实际修改面保存。
- Core `workspace_check_test.go`：真实 Git 和 SQLite 验证本地创建、接受初始修改、唯一占用、相同内容提交、恢复、切分支阻塞、拒绝迁移及取消后保留文件；只读占用检查不创建缺失的数据库。
- 本地 Host 测试验证两种分支模式、暂存及 ignored 文件保留、原会话 ready 结果、活动 Task 拒绝及 provisioned 重读不修改 Git。
- 包清单、CLI/MCP 输入及存储检查确认新字段和快照模块完整进入当前约定。

以上测试使用本机临时仓库和 Host helper。managed dispatch 使用模拟 Host 创建结果；不代表已完成实际
Codex、DSH、Claude Code 或 ZCode 新会话的端到端操作，也不扩大其他平台的支持声明。

## 非目标

本次不增加 Core Git 写入、自动提交或发布、历史数据兼容、跨机器复制、忽略文件复制或子模块内容复制。

## Codex 规划与保留检查

Codex 携带本地改动时，会在 REQUIREMENTS 中记录保留要求，在 TASKS 中将完整的 `current_changed_paths` 与 `expected_paths` 及已保留的流程文件逐项核对。新开发工作和已有内容保留分别安排工作项与检查；当前 Action 的空文件清单不能代替完整 Task 路径核对。保留检查比较启动快照，不代表已有业务功能已经验证。已发生的文件范围阻塞仍通过现有 Core 选择与转移处理。

## 修正未执行的 Codex 启动或活动分支名

修正尚未执行的 Codex 启动时，先读 host-launch status 并保留 receipt_digest。显式 supersede 提交该摘要、
原因和重新确认的完整 prepare 输入；新 launch_id 必须不同，repository key/源仓库保持一致。被替代回执
永久禁止执行，保存唯一完整后继 seed。supersede-resume 只接受前驱身份及 seed_digest，按该 seed 创建
缺失后继，不覆盖别的回执，也不生成另一个 launch。准备、替代、dispatch claim 和目标 Git 调用使用同一
receipt 锁。已授予 claim 或可能调用目标时禁止替代；failed/resolving 必须有正向 not_invoked 证据。
目标目录不存在、Host ID 为空或失败状态本身都不是证据；部分执行或结果不明先 reconcile。

活动 Task 先通过 Core 准备单个仓库改名，再由 Host 执行一次 `git branch -m`。Core 核对精确 refs 及未变
的工作区、index、内容后才能 complete；原事实未变时才可 cancel。中断后读取同一 Task 和实际 refs，有
保存的 resolution 时恢复它。相同 HEAD 的另分支切换、源 ref 重建、已存在目标、index 变化或其他仓库
变化都不能通过。准备不授权 commit、push 或 force。

receipt 与 WorkspaceOrigin 保留原分支；恢复读取同一 Core Task，以 repository.current_branch 作为后续
历史、relocation、cleanup 的有效分支。Codex/Claude/ZCode 清理命令要求真实终态 core_task；Claude/ZCode
迁移要求实际 core_preparation 结果；DeepSeek 直接读取终态 Task。缺失或不一致即停止，不退回旧名。
Codex 回执缺少已保存工作树实例证明时须人工检查再清理；这些操作不会引入自动清理。
