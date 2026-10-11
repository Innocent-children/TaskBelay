# TaskBelay 命令参考

[中文](COMMANDS.md) | [English](COMMANDS_en.md)

## 安装与本机入口

公开入口：npm install -g taskbelay@latest；taskbelay。统一安装、status、doctor、upgrade、uninstall 等生命周期选项仍由 packages/taskbelay/lib/cli.mjs 定义。源码新契约须在仓库使用 node scripts/taskbelay-local.mjs install --host codex --yes 构建安装，宿主可换 deepseek、claude、zcode；这不发布版本。

Core：taskbelay version；config validate（stdin 配置）；mcp --stdio；call <tool_name>（stdin 完整 JSON）；host-check pre-file-write / workspace-available；webui start/open/status/stop（--json 或 --plain，start 可 --no-open）。Core 运行必须设置绝对 TASKBELAY_DATA_DIR；保留旧数据，选择新目录，不自动迁移。统一 npm launcher 将 call 与 webui 转发到已配置 Core；Codex、Claude、ZCode 的 taskbelay-<host> call <tool_name> 转发到各自 bundled Core。DeepSeek 使用已发现的 MCP namespace。

host-check pre-file-write 输入 host、repository_path、tool_name、paths、intent_digest、path_parse_complete；返回 decision allow/deny 和明确 coverage，不判断未来修改语义。workspace-available 输入 repository_path 或 repositories [{key,repository_path}]，只读返回 available 与已存在的 task_id。检查失败不按无 Task 放行。普通 artifact 阶段命令已移除。

## MCP 工具

| 工具 | 当前输入 |
| --- | --- |
| taskbelay_server_info / taskbelay_list_tasks | 空对象；实际能力或最近 200 个摘要 |
| taskbelay_open_task | operation_id, host, repositories, authorization, checks |
| taskbelay_get_task | task_id, refresh（缺省 true；false 读取保存状态） |
| taskbelay_checkpoint | identity；可选 assessment/progress/checks/issue、host_ms/wait_ms/tokens |
| taskbelay_revise_task | identity, authorization, checks, reason |
| taskbelay_run_check | identity, check_id；真实本机 argv 执行 |
| taskbelay_record_evidence | identity, check_id, source claim/human, status, summary, content_digest；human 的 user_ref |
| taskbelay_get_run | run_id；只读真实结果 |
| taskbelay_recover_run | identity, run_id, resolution_basis, 必需时 user_ref |
| taskbelay_finish_task | identity, summary, acceptance [{goal_id,evidence_ids,basis}], risks |
| taskbelay_cancel_task | identity, reason |

identity 是 task_id、当前 revision、稳定 operation_id。命令输入拒绝未知和重复成员。完整字段和数量约束以服务器发布 Schema、internal/application DTO 和 domain 验证为准。authorization 包含 user_ref/request/goals，每个 goal 有 id/statement/checks；checks 至少有 id/kind/rationale/conditions，command 还需 argv，缺省 cwd/inputs 为 .，timeout_ms 为 60000，retry 为 requires_user。累计 limits 缺省 runs=20、execution_ms/core_ms=600000、host_ms=3600000、tokens=1000000。

## 宿主工作区与 HTTP

可选 host-launch prepare 使用已有真实 authorization_ref、request、精确仓库来源/分支/携带选择；不要求再填评估表或重新审批已授权方案。scope 返回当前 repositories 数组，Core 补齐观察身份。恢复读取 receipt/status；不重派不确定操作。清理另需真实删除授权和当前终态 Task；跨工作区迁移不支持。

HTTP GET /api/tasks、/api/tasks/{id}、/api/dashboard、/api/system/status；详情 ?refresh=1 核对实际条件。POST /api/core/{tool} 输入 csrf 和 arguments，同一 Core 契约；POST /api/tasks/{id}/purge 输入 csrf、revision、confirm_task_id，仅清除匹配终态本地记录。变更须来自当前 Origin 与 CSRF 会话。

已退休的 submit_*、get_next_action、resolve_blocker、relocation、阶段 artifact 协议没有兼容入口。错误与恢复见[响应规范](CORE-RESPONSES.md)。

control_cost 是本地数据目录跨任务累计成本，server_info、list_tasks 及 Task view 均返回 calls、wall_ms、unknown_ms、pending；unknown_ms 单独表示崩溃遗留调用的保守估算。类别可能重叠，不直接相加。

checkpoint.assessment 可仅提交新增或受影响的 paths、constraints、facts；全局摘要须当前，Core 生成每项 scope_digest 并合并未受影响判断。revise_task 不自动清除 issue；实际用户决定后使用 checkpoint.clear_issue 与 resolution_ref。
