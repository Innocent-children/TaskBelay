# TaskBelay 命令参考

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

## 安装与本机入口

公开入口：npm install -g taskbelay@latest；taskbelay。统一安装、status、doctor、upgrade、uninstall 等生命周期选项仍由 packages/taskbelay/lib/cli.mjs 定义。源码新契约须在仓库使用 node scripts/taskbelay-local.mjs install --host codex --yes 构建安装，宿主可换 deepseek、claude、zcode；这不发布版本。

Core：taskbelay version；config validate（stdin 配置）；mcp --stdio；call <tool_name>（stdin 完整 JSON）；host-check pre-file-write / workspace-available；webui start/open/status/stop（--json 或 --plain，start 可 --no-open）。Core 运行必须设置绝对 TASKBELAY_DATA_DIR；保留旧数据，选择新目录，不自动迁移。统一 npm launcher 将 call 与 webui 转发到已配置 Core；Codex、Claude、ZCode 的 taskbelay-<host> call <tool_name> 转发到各自 bundled Core。DeepSeek 使用已发现的 MCP namespace。

host-check pre-file-write 输入 host、repository_path、tool_name、paths、intent_digest、path_parse_complete；返回 decision allow/deny 和明确 coverage，不判断未来修改语义。workspace-available 输入 repository_path 或 repositories [{key,repository_path}]，只读返回 available 与已存在的 task_id。检查失败不按无 Task 放行。普通 artifact 阶段命令已移除。

`webui start/open/status/stop --json` 在顶层返回 `readiness`、`core_identity`、`data_root_digest` 和 `url`。`core_identity` 是 Core 版本号，不带产品名前缀；未记录服务进程时省略 `pid`，存在时它是非负整数。响应不包含 `operation`，调用方已知所执行的命令。桌面启动器核对这些字段后，仍须读取 `/api/system/status`，确认版本身份、数据目录摘要和本机地址与实际服务一致。

## MCP 工具

| 工具 | 当前输入 |
| --- | --- |
| taskbelay_server_info | 空对象；实际能力 |
| taskbelay_list_tasks | 可选 page、page_size（1..100）、include_archived、status（active/done/cancelled）；返回 page 与 has_next |
| taskbelay_open_task | operation_id, host, repositories, authorization, plan, checks |
| taskbelay_get_task | task_id, refresh（缺省 true；false 读取保存状态） |
| taskbelay_checkpoint | identity；可选 assessment/progress/issue、host_ms/wait_ms/tokens |
| taskbelay_revise_task | identity, authorization, plan, checks, reason |
| taskbelay_run_check | identity, check_id, rationale；重试的 previous_run_id/new_information；完整套件的 full_suite_basis |
| taskbelay_record_evidence | identity, check_id, source claim/human, status, summary, content_digest；human 的 user_ref |
| taskbelay_get_run | run_id；只读真实结果 |
| taskbelay_recover_run | identity, run_id, resolution_basis, 必需时 user_ref |
| taskbelay_finish_task | identity, summary, acceptance [{goal_id,evidence_ids,basis}], risks |
| taskbelay_cancel_task | identity, reason |
| taskbelay_confirm_plan | identity, plan_digest, user_ref, response；真实用户确认 |
| taskbelay_accept_known_failure | identity, check_id, failed_evidence_id, comparison_evidence_id, failures, user_ref, reason |
| taskbelay_update_budget | identity, plan_digest, budget {runs,execution_ms,work_ms,tokens}, user_ref, response；实际批准的完整投入决定 |
| taskbelay_archive_task | identity, archived；仅终态 |

identity 是 task_id、当前 revision、稳定 operation_id。命令输入拒绝未知和重复成员。完整字段和数量约束以服务器发布 Schema、internal/application DTO 和 domain 验证为准。authorization 包含 user_ref/request/goals，每个 goal 有 id/statement/checks；checks 至少有 id/kind/rationale/conditions，command 还需 argv，缺省 cwd/inputs 为 .，timeout_ms 为 60000，retry 为 requires_user。累计 limits 缺省 runs=20、execution_ms/core_ms=600000、host_ms=3600000、tokens=1000000。

预算 view.budget 显示每个有效上限的 actual/reserved/uncertain/remaining 和每项检查所需 timeout。PLAN_BUDGET_INFEASIBLE 表示某个必需检查确定无法预留或次数不够；整套最坏耗时与一次重试只作风险提示。taskbelay_update_budget 将 work_ms 同时应用于 core_ms 与 host_ms，保留原始批准和累计消耗；调用必须引用当前方案以及具体投入决定，不能用普通继续消息扩张。

## 宿主工作区与 HTTP

Codex/Claude/ZCode 使用 taskbelay-<host> host-launch freshness 检查所有仓库；DeepSeek 使用 workspace_coordinator 的 freshness 操作与 freshness_repositories。已绑定仓库成员为 {key,repository_path,source_type,remote_name,base_branch}，后三项复制原 workspace_origin。fetch 缺省 true，integrate 缺省 false；fetch 需 authorization_ref，整合另需 integration_authorization_ref 与稳定 operation_id。

结果中的 remote_freshness 复制到 open_task.repositories；后续通过 checkpoint.repository_freshness [{key,observation}] 保存。整合在 Git 修改前保存意图，并返回 operation.receipt_path、状态及逐仓 integration 结果。相同身份读取原收据，不重跑；recover:true 使用原请求，只在原所有者停止后核对实际 Git，不重复 fetch/merge 或回退已有整合。partial/uncertain/reconciled 不代表整批成功；结果未知仍保留未知。恢复锁记录 PID、随机 token 与文件身份；只有确认原所有者停止、独占回收声明并核对身份未变时才回收。活跃、PID 重用、权限拒绝或身份不明时保守拒绝，不按超时删锁；回收声明自身结果不明时保留检查。剩余工作先检查原副作用，再按新事实与真实授权建立新操作。无上游、跳过或失败不声称最新。

可选 host-launch prepare 使用已有真实 authorization_ref、request、精确仓库来源/分支/携带选择；不要求再填评估表或重新审批已授权方案。scope 返回当前 repositories 数组，Core 补齐观察身份。恢复读取 receipt/status；不重派不确定操作。清理另需真实删除授权和当前终态 Task；跨工作区迁移不支持。 工作区准备不能代替 Core 的需求理解与完整方案确认。

HTTP GET /api/tasks?page=1&include_archived=false&status=active（status 可省略）、/api/tasks/{id}、/api/dashboard、/api/system/status；详情 ?refresh=1 核对实际条件。POST /api/core/{tool} 输入 csrf 和 arguments，同一 Core 契约；POST /api/tasks/{id}/purge 输入 csrf、revision、confirm_task_id，仅清除匹配终态本地记录。变更须来自当前 Origin 与 CSRF 会话。

已退休的 submit_*、get_next_action、resolve_blocker、relocation、阶段 artifact 协议没有兼容入口。错误与恢复见[响应规范](CORE-RESPONSES.md)。

control_cost 是本地数据目录跨任务累计成本，server_info、list_tasks 及 Task view 均返回 calls、wall_ms、unknown_ms、pending；unknown_ms 单独表示崩溃遗留调用的保守估算。类别可能重叠，不直接相加。

checkpoint.assessment 可仅提交新增或受影响的 paths、constraints、facts；全局摘要须当前，Core 生成每项 scope_digest 并合并未受影响判断。revise_task 不自动清除 issue；实际用户决定后使用 checkpoint.clear_issue 与 resolution_ref。

plan 包含 understanding/root_cause/approach/simpler_alternative/verification_basis/completion 以及 work（id、goal_ids、change、owner、necessity）。view 返回 plan_digest；确认绑定该摘要和提案时内容。authorization.allowed_paths 是硬允许范围。checks.purpose 为 acceptance/regression/diagnostic（缺省 acceptance），诊断项需 goal_ids；full_suite 标记完整套件，compares 关联被比较的 regression 项。默认 inputs:["."] 覆盖全部已绑定仓库；缩小范围说明 input_basis，跨仓路径用 key::path。checkpoint 不接受 checks；assessment.paths 需 work_id 和 necessity，最终 assessment.review 需 root_cause/responsibilities/proportionality/verification/would_choose_again/uncertainty，摘要由 Core 生成。

HTTP 列表另支持 lifecycle=active/blocked/done/cancelled，按 Core 保存摘要过滤后分页；status=active 则包含所有活动任务（包括受阻），供不同界面选择。

运行记录的 process 保存命令进程的 pid 和启动 identity；激活前为 null，身份持久化后才允许命令执行。恢复时 Core 和受管命令均停止才可处理缺失收据；macOS 核对整个进程组。

首次核验在冻结来源和创建分支之前进行。绑定后始终使用原 source_type/remote_name/base_branch：本地来源核对原分支配置上游，远端来源直接核对指定远端追踪分支，不要求本地同名分支。任务分支获得自己的上游不会改变来源；不得自行改用 main。
