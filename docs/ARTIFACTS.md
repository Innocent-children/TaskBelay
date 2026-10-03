# 文件收集与提交

[中文](ARTIFACTS.md) | [English](ARTIFACTS_en.md)

## 职责

Core 观察 Git，计算当前 Action 的完整文件变化。Codex 逐项判断文件用途并填写说明，只读准备命令
根据这些实际记录生成 artifact 引用。准备成功后可以构造提交；提交时 Core 仍校验节点结果、转换、
工作树身份、Git 历史、允许的改动类型和文件范围。

收集和准备保持流程定义、转换、Task revision 和持久化 Schema 不变，不写入 Task、Event、Claim、
Action operation 或 Git 状态。临时 JSON 文件保存在 Task 工作树之外。被 Git 忽略的文件不属于
Core 的 Git 可见范围。

## 命令

构造输入前可查询已安装命令的帮助：

```bash
taskbelay-codex artifacts --help
taskbelay-codex artifacts collect --help
taskbelay-codex artifacts prepare --help
```

总帮助列出两个操作；操作帮助返回 JSON，包含 `operation`、`description`、`transport`、`input_example`、`input_fields`、`output_fields` 和 `next_step`。示例中的身份及观察字段必须替换为当前 Action 或完整 collection 的实际值。帮助查询不读取 stdin、安装路径或 Task 数据，也不启动 Core；实际命令输入由 Core 校验。

使用自定义数据目录时，在启动 Codex 前设置 `TASKBELAY_DATA_DIR`。Plugin 显式将其转发给 MCP，hook 和文件准备命令沿用同一 Host 环境。目录必须已存在并使用规范化绝对路径；省略变量时使用默认数据目录。启动环境的变化需要在新的 Codex 会话中生效。


`taskbelay-codex artifacts collect` 转发到包内的 `taskbelay artifacts collect`；
`taskbelay-codex artifacts prepare` 转发到包内的 `taskbelay artifacts prepare`。
两个命令均从 stdin 读取一个不超过 1 MiB 的 UTF-8 JSON 对象，拒绝未知字段、重复字段、尾随 JSON
和非法 UTF-8。命令读取已有 Task 数据库，通过 stdout 返回 JSON；成功退出码为 0，失败为 1，
不初始化存储。

collect 输入：

```json
{"host":"codex","task_id":"task-example","action_id":"action-example"}
```

成功响应为 `{ "ok": true, "result": <collection> }`。collection 包含 `task_id`、`action_id`、
`revision`、`observation_digest` 和 `files`。每个文件包含 `path`、`change_type`、`digest`、`slot`
和 `summary`。Core 填写文件信息，后两个字段为空字符串；保留完整 collection，只填写 `slot` 和
`summary`。

单仓库 `path` 使用仓库相对路径，已有多仓库 Task 使用 `<repository-key>::<relative-path>`。
变化包含隐藏文件、已暂存和未暂存修改、未跟踪文件及当前 Action 期间已 commit 的改动。
`change_type` 描述当前 Task 相对冻结 base 的文件状态；`restored` 表示之前保留的改动路径已恢复到
base 状态。摘要表示实际路径状态，包含删除状态，不由模型编写。在 Core 现有仓库数量和路径数量
限制内，文件枚举完整。

prepare 输入为 `{ "host": "codex", "collection": <classified collection> }`：

- `slot="current"`：当前节点的流程文档，仅用于节点开放此 artifact 槽位的情况。
- `slot="other_process"`：相关方法文档或辅助文件。
- `slot="product"`：产品代码变更，仅允许在 IMPLEMENT 和 REFACTOR 使用，不进入 artifact 数组，
  普通提交仍按 Task Plan 检查这些改动。
- 每项必须填写符合长度限制的非空 `summary`；用途不明时先明确用途再准备提交。

prepare 重新观察 Git，拒绝已变化的观察、缺失或重复路径、被修改的文件信息、未分类条目及当前
节点不允许的角色。成功的 `result` 就是生成的 `artifacts` 对象，按节点规则包含 `current` 和
`other_process`。普通提交直接使用该对象，可另附已核实的未修改文档引用到允许的槽位。
之后再次修改仓库文件时必须重新收集和准备。准备命令保留普通提交的实时 Schema 校验和授权检查。

Action 过期、历史冲突或仓库漂移时，读取下一个 Core Action 并按其工作树处理指示继续。
观察变化时重新收集，条目遗漏或未分类时先补齐再准备；工作树不可用继续采用已有恢复或放弃规则。

## OpenSpec 文件

按用途将 proposal 和规格声明为当前节点文档，同时检查和分类初始化配置、change 元数据和生成的
README。目录位置本身不代表改动已获授权；系统没有 `openspec/**` 白名单，也不会将剩余文件全部
自动归为流程文件。OpenSpec validate 负责内容和结构检查，collect 和 prepare 负责文件覆盖核对。

## 拒绝与纠正

普通流程文档提交漏报已改动文件时返回 `INVALID_ARGUMENT` 和固定规则 `artifact_manifest_incomplete`。
`error.repository_paths` 返回经过校验的遗漏文件路径，与 JSON 字段路径分开表达。
确认本次未写入后，MCP 返回 `correct_current_action`、`retry_safe=true`，并在 `allowed_paths` 中
仅列出当前允许修改的 artifact 字段。Host 可以重新收集、分类和准备，通过同一当前提交工具纠正
这些字段一次；再次失败即停止。节点语义结果、转换和方法结论保持原意。
HTTP 同时提供遗漏路径和 `field_paths`，WebUI 展示两者。实际工作树或历史异常、结果不确定的操作
继续使用原有恢复方式；纠正提交始终接受当前 Core 检查。

## 验证方式

Application 定向回归覆盖五个改动文件只申报两个、返回准确遗漏路径、拒绝时零写入、回读保持同一
Action，以及补齐后成功进入 DESIGN。原生 Git/CLI 测试覆盖隐藏和中文路径、已 commit/已暂存/未跟踪
变化、准备清单、数据库和 Git 内容保持不变、收集后的内容变化及切换分支。Application 和 MCP 测试
覆盖当前阶段禁止产品代码分类、过期 Action、工作树身份异常、受限纠正及不安全公开路径的拒绝。
这些是本地自动化测试，不代表实际 Codex 会话执行 OpenSpec 命令的端到端结果，也不扩大 Windows
验证声明。

## 最终验证后的流程文件

`collectArtifacts` 在 TEST、COMPREHENSION_REVIEW 和 DELIVERY 比较当前内容与 Implementation/Test 保存的内容摘要，流程文件同样计入。Codex 因此在最终验证前完成流程文件更新，之后只读核对；若仍需更新，则使用当前合法返回路径并重新建立验证结果。把文件分类为 `other_process` 不能绕过内容检查。实现见 `internal/application/artifacts.go` 与 `internal/application/workspace.go`。

DeepSeek Skill 随包提供 `scripts/artifacts.mjs`，以 `node <实际 Skill 目录>/scripts/artifacts.mjs collect` 或 `prepare` 调用同一套 Core 只读文件准备命令。输入与返回结构与本文相同，`host` 使用 `deepseek`；脚本复用 Adapter 的运行时和数据目录解析，不创建存储。通过实际 DSH Skill 的 `resourceBase` 取得脚本路径。`--help` 不读取 stdin 或解析运行时。该脚本不是独立的 `taskbelay-deepseek` CLI，也不增加 `workspace_coordinator` 操作。

## 完整基线引用与操作身份

需求、设计和任务计划的全部旧基线引用按保存顺序留在 Task 内，不归档、不设配置性条数上限，
仍受资源与修订号表示范围约束。响应首页与完整历史分页保留原 revision、digest、摘要和时间，
明确给出总数及下一游标；页大小不决定保留量。不补造未存的完整正文，也不改变当前 artifact
集合及其内容校验。文件提交仍须覆盖当前 Core 要求的路径。

分支改名准备换发 BLOCKED Action；确认后按保存的有效分支重读当前 Action，再收集或准备该 Action
的 artifact。不得复用改名前的 Action 身份提交，也不得把创建来源的旧分支名当成当前绑定。
改名的实例、HEAD、index、内容验证属于 Core，不由 artifact 清单代替。
