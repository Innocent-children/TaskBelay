# TaskBelay 项目状态

[中文](PROJECT-STATUS.md) | [English](PROJECT-STATUS_en.md)

当前源码实现已有用户授权复用、按需修订实施与检查、明确范围限制、选择性结果失效、可选成本上限及持续计量、已知失败例外和恢复。默认工具结果为摘要，完整详情按需读取。Core 保持唯一 Task；没有恢复旧节点流程或历史数据兼容。公开 npm、Tag、GitHub Release 和原生 Host 支持声明没有随源码变更更新。

## 验证入口与适用范围

| 检查 | 覆盖范围 |
| --- | --- |
| go test -count=1 -timeout=90s ./... | 当前领域、授权、存储、执行、CLI/MCP/HTTP 和保留的仓库观察契约 |
| tests/journeys/engineering.test.mjs | 隔离 Git/SQLite 中的真实进程、已有授权直接开工、新检查保留独立结果、验收缺口、跨仓库失效、重复投入、例外和真实中断恢复 |
| tests/host-engineering.test.mjs | 从独立目录运行四个 Host 包及当前 Core，检查工具与引用闭包 |
| 四 Host 包的 Node 测试 | 启动、目录、钩子、Skill、安装管理的直接消费方；平台或能力不可用项明确跳过 |
| scripts/build-webui.mjs | TypeScript 检查和生成资源；浏览器交互另行核对 |
| scripts/sync-skill-references.mjs --check、scripts/check-versions.mjs | 生成契约和版本责任一致 |

原工程升级记录来自 Windows x64、Go 1.27.0、Node.js 24.18.0。2026-10-12 的修复在 macOS arm64、Go 1.27.0、Node.js 24.21.0 验证检查结果复用、分支与历史写入预检、Claude/ZCode 清理以及 Core 强制退出后的进程组恢复；Windows 新的挂起启动实现仅完成交叉编译，尚未原生运行。检查使用临时仓库和数据目录，保留用户安装及历史数据。完整入口是 node scripts/validate-engineering.mjs；单项命令按改动范围选择，实际通过、失败和跳过以本次执行输出为准。旧 tests/journeys/evidence/ 记录只说明各自记录时的产物，不能替代当前源码验证。

2026-10-13 的本次改造在 macOS arm64、Go 1.27.0、Node.js 24.21.0 完成 node scripts/validate-engineering.mjs：Go vet 和全部 Go package 测试通过，四宿主独立安装包检查通过，Node 套件 499 项中 487 项通过、12 项因 Windows 原生或指定 DSH 环境条件跳过。补充需求变更场景后，tests/journeys/engineering.test.mjs 的 10 项全部通过；相关 Go 检查再次通过。Windows x64 Core 交叉编译通过，未原生运行。实际本机浏览器已核对结果刷新、按需展开、未设额度显示及预算保存回读，无控制台错误。所有操作使用临时仓库与独立数据目录，没有安装或更改用户插件。

MCP 同一初始任务样例的摘要为 2737 字节，完整应用视图为 4712 字节。这只说明该样例的返回体缩减，不能换算为真实模型任务的时间或 token 收益。

## 尚未证明的结果

真实 Core 执行与实际用户、模型会话不同。旅程的用户答复和语义判断是 fixture 输入，不能证明 AI 会正确理解全部需求或杜绝过度设计。没有完成原生 Codex/Claude Code/ZCode/DSH 模型会话、最终注册表安装产物或长期用户成本对照，不据此增加支持或收益声明。

Core 可以拒绝无新用户引用的边界变更、明确越权、过期结果和无依据重复；语义必要性、比较检查充分性和宿主消息真实性仍需真实 Host 与用户审查。宿主绕过接入边界的写入不能声称已被预先拦截。详细验收见[工程控制](ENGINEERING-CONTROL.md)，已交付源码边界见[支持矩阵](SUPPORT-MATRIX.md)。
