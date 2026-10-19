# TaskBelay 命令参考

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

## 安装与本机入口

公开入口：npm install -g taskbelay@latest；taskbelay。统一安装、status、doctor、upgrade、uninstall 等生命周期选项仍由 packages/taskbelay/lib/cli.mjs 定义。源码新契约须在仓库使用 node scripts/taskbelay-local.mjs install --host codex --yes 构建安装，宿主可换 deepseek、claude、zcode；这不发布版本。

### 交互菜单与维护

不带参数的 TTY 使用方向键、Enter、空格多选、A 全选和 Esc 返回。升级页先显示查询进度；全部最新时提供返回首页和重新检查，不要求勾选。R 重新检查，V 先选一个组件再指定其版本，只有本地安装渠道显示 L 选择完整分发包；空路径清除上次分发包选择。DeepSeek Profile 页的 N 输入其他 Profile。文本输入支持左右键、Home/End、Ctrl+A/E、Ctrl+U/K/W、退格和 Delete，格式错误留在输入框。Ctrl+C 退出；确认页的取消显示“已取消，未执行”，不生成带确认参数的执行命令。非交互缺少确认仍返回 confirmation_required。NO_COLOR 关闭颜色，不关闭键盘选择；非 TTY 不弹出交互，空参数打印帮助。

菜单诊断先显示摘要，可选择查看详细信息；未安装的可选 Agent 不作为安装故障。安装信息保留路径详情；打开控制中心显示面向用户的文本，脚本仍可使用 --json。

| 命令 | 行为 |
| --- | --- |
| `status` | 显示插件、Core、命令入口的实际版本、状态和本地路径 |
| `doctor` | 只读检查安装与配置，提供对应下一步 |
| `install` | 安装到所选 Agent，保留已有版本 |
| `upgrade` | 只更新已安装插件，默认最新稳定版；`--self` 同时或单独更新命令入口 |
| `repair` / `reinstall` | 修复安装或替换当前版本的文件，保留用户数据 |
| `uninstall` | 卸载所选插件及注册，保留数据 |
| `delete-data` | 清空任务记录、配置和宠物偏好，保留插件、注册和安装记录，在原路径重建空目录 |
| `delete-all` | 移除全部受管插件及确认的数据，保留命令入口与桌面程序 |

`--host codex|deepseek|claude|zcode` 可以重复，多选 Agent；`--host all` 不与其他 Agent 混用。`--profile` 可重复，`--all-known-profiles` 选择全部受管 DeepSeek Profile。共享数据删除隐含全部 Agent 和 Profile，不接受按 Agent/单个 Profile 删除数据。

```sh
taskbelay install --host codex --host claude --yes
taskbelay upgrade --host all --all-known-profiles --self --yes
taskbelay upgrade --self --yes
taskbelay delete-data --json
taskbelay delete-all --json
```

`--version latest|<x.y.z>` 用于安装、升级、修复和重装；降级须额外 `--confirm-downgrade <token>`。命令入口自更新只适用于当前全局 npm 安装，必须保持原渠道。本地开发安装使用 `--self --self-package <完整本地分发包的绝对路径.tgz>`，先校验包内四个插件及摘要，再安装；不自动切换到 npm。临时源码管理器不更新另一份全局入口。普通维护的 `--yes` 不批准数据删除、永久删除或降级。

数据删除先返回具体目录和与本次计划绑定的 `--confirm-delete <token>`；永久删除还需 `--permanent --confirm-permanent <token>`。额外目录用 `--confirm-explicit-data <绝对路径>` 单独确认。ZCode 安装存在时，先关闭其全部窗口和会话，再提供 `--confirm-zcode-closed`；这个参数记录用户实际操作，程序仍检查可识别的 Core 进程。delete-all 在任何插件尚未真正卸载时保留数据，并返回剩余 UI 步骤。

清理先停止宠物和相关 Core，通过兼容 Core 的只读接口确认无运行中或未确定的 Task 执行，再清理准确目录。存在数据库但无可用的兼容 Core 时停止，不猜测历史数据内容。保留本次删除结果及未解决的维护记录，清理普通已完成维护记录。默认可恢复删除使用 macOS 废纸篓或 Windows 的 TaskBelay 恢复目录；后者不是系统回收站。不会清空以前的恢复目录、项目代码或 Agent 会话历史。旧 factory-reset、--confirm-reset 和删除后自动重装选项已移除。

Core：taskbelay version；config validate（stdin 配置）；mcp --stdio；call <tool_name>（stdin 完整 JSON）；host-check pre-file-write / workspace-available；webui start/open/status/stop（--json 或 --plain，start 可 --no-open）。Core 运行必须设置绝对 TASKBELAY_DATA_DIR；保留旧数据，选择新目录，不自动迁移。统一 npm launcher 将 call 与 webui 转发到已配置 Core；Codex、Claude、ZCode 的 taskbelay-<host> call <tool_name> 转发到各自 bundled Core。DeepSeek 使用已发现的 MCP namespace。

host-check pre-file-write 输入 host、repository_path、tool_name、paths、intent_digest、path_parse_complete；返回 decision allow/deny 和明确 coverage，不判断未来修改语义。workspace-available 输入 repository_path 或 repositories [{key,repository_path}]，只读返回 available 和逐仓 repositories [{key,repository_path,active_task_id}]，未占用为 null。检查失败不按无 Task 放行。普通 artifact 阶段命令已移除。

`webui start/open/status/stop --json` 在顶层返回 `readiness`、`core_identity`、`data_root_digest` 和 `url`。`core_identity` 是 Core 版本号，不带产品名前缀；未记录服务进程时省略 `pid`，存在时它是非负整数。响应不包含 `operation`，调用方已知所执行的命令。桌面启动器核对这些字段后，仍须读取 `/api/system/status`，确认版本身份、数据目录摘要和本机地址与实际服务一致。

## MCP 工具

| 工具 | 当前输入 |
| --- | --- |
| taskbelay_server_info | 可选 contracts 数组；实际能力及所选方法契约 |
| taskbelay_list_tasks | 可选 page、page_size（1..100）、include_archived、status（active/done/cancelled）；返回 page 与 has_next |
| taskbelay_open_task | operation_id, host, repositories, authorization, plan |
| taskbelay_get_task | task_id, refresh（缺省 true；false 读取保存状态）, detail（缺省 false） |
| taskbelay_checkpoint | identity；可选 assessment/progress/issue、host_usage {source_id,observed_through,totals,coverage} |
| taskbelay_revise_task | identity、reason；按需提供 authorization、plan |
| taskbelay_resolve_issue | identity, issue_digest, user_ref, decision；答复不自动改变授权 |
| taskbelay_record_evidence | identity, check_id, source claim/human, status, summary, content_digest；human 的 user_ref |
| taskbelay_get_run | run_id；只读真实结果 |
| taskbelay_recover_run | identity, run_id, resolution_basis, 必需时 user_ref |
| taskbelay_finish_task | identity, summary, acceptance [{goal_id,evidence_ids,basis}] |
| taskbelay_cancel_task | identity, reason |
| taskbelay_accept_known_failure | identity, check_id, failed_evidence_id, comparison_evidence_id, failures, user_ref, reason |
| taskbelay_update_budget | identity, authorization_digest, budget {runs,execution_ms,work_ms,tokens}, user_ref, response；实际批准的完整投入决定 |
| taskbelay_archive_task | identity, archived；仅终态 |

identity 是 task_id、当前 revision、稳定 operation_id。命令输入拒绝未知和重复成员。完整字段和数量约束以服务器发布 Schema、internal/application DTO 和 domain 验证为准。authorization 包含 user_ref/request/goals，每个 goal 有 id/statement；plan.checks 通过必填 goal_ids 单向关联目标；plan.checks 至少有 id/goal_ids/kind/rationale/conditions，command 还需 argv，缺省 cwd/inputs 为 .，timeout_ms 为 60000，retry 为 requires_user。limits 的字段缺省为零，表示未设置上限；正数表示实际设置的累计上限。

预算 view.budget 显示 limited、actual、reserved、uncertain、remaining，以及每项检查所需 timeout。未设上限时 limited=false，remaining 不表示可用额度。明确额度不足时拒绝执行，整套超时合计仅作提示。taskbelay_update_budget 引用当前 authorization_digest 和实际预算决定，work_ms 同时更新 core_ms 与 host_ms，保留累计消耗。

## 宿主工作区与 HTTP

Codex/Claude/ZCode 使用 taskbelay-<host> host-launch freshness 检查所有仓库；DeepSeek 使用 workspace_coordinator 的 freshness 操作与 freshness_repositories。已绑定仓库成员为 {key,repository_path,source_type,remote_name,base_branch}，后三项复制原 workspace_origin。fetch 缺省 true，integrate 缺省 false；fetch 需 authorization_ref，整合另需 integration_authorization_ref 与稳定 operation_id。

结果中的 remote_freshness 复制到 open_task.repositories；后续通过 checkpoint.repository_freshness [{key,observation}] 保存。整合在 Git 修改前保存意图，并返回 operation.receipt_path、状态及逐仓 integration 结果。相同身份读取原收据，不重跑；recover:true 使用原请求，只在原所有者停止后核对实际 Git，不重复 fetch/merge 或回退已有整合。partial/uncertain/reconciled 不代表整批成功；结果未知仍保留未知。恢复锁记录 PID、随机 token 与文件身份；只有确认原所有者停止、独占回收声明并核对身份未变时才回收。活跃、PID 重用、权限拒绝或身份不明时保守拒绝，不按超时删锁；回收声明自身结果不明时保留检查。剩余工作先检查原副作用，再按新事实与真实授权建立新操作。无上游、跳过或失败不声称最新。

可选 host-launch prepare 使用已有真实 authorization_ref、request、精确仓库来源/分支/携带选择；不要求再填评估表或重新审批已授权方案。scope 返回当前 repositories 数组，Core 补齐观察身份。恢复读取 receipt/status；不重派不确定操作。清理另需真实删除授权和当前终态 Task；跨工作区迁移不支持。 工作区准备不增加任务授权；已有请求足够时无需另行确认计划。

HTTP GET /api/tasks?page=1&include_archived=false&status=active（status 可省略）、/api/tasks/{id}、/api/dashboard、/api/system/status；详情 ?refresh=1 核对实际条件。POST /api/core/{tool} 输入 csrf 和 arguments，同一 Core 契约；POST /api/tasks/{id}/purge 输入 csrf、revision、confirm_task_id，仅清除匹配终态本地记录。变更须来自当前 Origin 与 CSRF 会话。

已退休的 submit_*、get_next_action、resolve_blocker、relocation、阶段 artifact 协议没有兼容入口。错误与恢复见[响应规范](CORE-RESPONSES.md)。

control_cost 在 server_info、list_tasks 返回跨 Task 的 calls、wall_ms、unknown_ms、pending。普通变更返回 identity、digests、decision.gaps、progress、checks、concerns、limitations 和 Task 自身 usage/budget。open/get 额外返回 context.authorization/plan/repositories。get_task detail:true 增加明确的 details 字段：facts、evidence_applicability、仓库绑定、assessment、evidence、attempts、changes 和交付记录。check run/recover_run 返回 view 和运行摘要；get_run 读取 output.stdout/output.stderr 与实际执行条件。

checkpoint.assessment 可仅提交新增或受影响的 paths、constraints、facts；全局摘要须当前，Core 生成每项 scope_digest 并合并未受影响判断。revise_task 不自动清除 issue；实际用户决定后使用 resolve_issue、当前 issue_digest 和 user_ref 保存答复。

plan 包含 approach、verification_basis、checks、expected_paths 和 review_after_lines。authorization 只保存用户请求、目标和明确限制。revise_task 按对象替换提供的 authorization 或 plan，省略则保留。plan.checks 的 goal_ids 是唯一关联；purpose 为 acceptance/regression/diagnostic（缺省 acceptance）。每个目标至少一个 acceptance；acceptance 和 regression 均需解决，diagnostic 不作为交付要求。只有 regression 可以接受既有失败，finish.acceptance 只引用通过的 acceptance 记录。

HTTP 列表另支持 lifecycle=active/blocked/done/cancelled，按 Core 保存摘要过滤后分页；status=active 则包含所有活动任务（包括受阻），供不同界面选择。

运行记录的 process 保存命令进程的 pid 和启动 identity；激活前为 null，身份持久化后才允许命令执行。恢复时 Core 和受管命令均停止才可处理缺失收据；macOS 核对整个进程组。

只有任务需要上游核验时才执行；准备新工作区时，在冻结来源和创建分支之前核验。绑定后始终使用原 source_type/remote_name/base_branch：本地来源核对原分支配置上游，远端来源直接核对指定远端追踪分支，不要求本地同名分支。任务分支获得自己的上游不会改变来源；不得自行改用 main。

## 原生命令入口与保存状态读取

宿主原生进程工具执行 taskbelay check run，stdin 为完整 RunRequest JSON：task_id、revision、operation_id、check_id、rationale；重试还需 previous_run_id/new_information，完整套件需 full_suite_basis。响应 tool 为 check.run。Codex/Claude/ZCode 的宿主 CLI 同样支持 check run；DeepSeek 激活时将 bundled Core 路径和 TASKBELAY_DATA_DIR 写入 Skill 上下文。统一入口 taskbelay check run 转发给选定 Core。MCP/HTTP 没有启动进程的工具；server_info.native_check 提供原生命令及契约名称，按需请求 contracts:["check.run"] 获取完整输入输出说明。长命令沿用原生进程会话，读取 get_task 的 pending_runs 和 get_run，不因响应超时重新启动。

get_task refresh:false、list_tasks、get_run、server_info 读取已保存内容，不写计量、不查询进程身份。变更调用仍记录累计投入，计量失败拒绝执行。HTTP 列表额外接受 search（需求或任务 ID）和 host，先过滤后分页。checkpoint.progress.next_step 保存下一步；details.changes 保存最多 64 条近期决定和调整原因。plan.checks 的 kind、purpose、retry 枚举由同一 DTO 提供给公开 Schema。

## 无需源码的方法帮助

使用 `taskbelay call <tool> --help --json`、`taskbelay check run --help --json` 或当前配置的原生 Core。Codex/Claude/ZCode 支持 `host-launch <operation> --help --json`、`hook pre-tool-use --help --json` 和 `status|setup|remove --help --json`。统一入口支持 `<安装操作> --help --json`、`pet start|stop --help --json`。原生 Core 还提供 `config validate`、`webui <operation>`、`help`、`version`、`mcp` 的方法帮助。查询在打开数据目录之前读取随包说明，不执行所查询的操作。

`server_info.auxiliary_contracts` 列出可通过 contracts 查询的原生命令/HTTP 契约名称，不增加 MCP 执行工具。每个包携带 `contracts/interfaces.json`；DeepSeek 的六个 workspace_coordinator 分支也有随包说明。输入输出、空值和缺省值、实际成功/失败样例、授权要求和恢复行为均可独立读取，不需要源码或数据库。见[接口说明](INTERFACES.md)。

创建 Task 分支统一使用 `--no-track`，返回实际 upstream 与 push_configuration。原始来源选择与 Task 分支追踪关系分开；IDE 推送目标明确为 unknown。workspace-available 使用统一 Core 响应，逐项返回所有选定根目录的 active_task_id，未占用为 null，不只返回第一个冲突。
