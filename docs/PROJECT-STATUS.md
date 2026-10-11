# TaskBelay 项目状态

[中文](PROJECT-STATUS.md) | [English](PROJECT-STATUS_en.md)

当前源码实现开发前需求理解与完整方案确认、明确路径范围、验证必要性、跨仓库证据适用性、累计成本、已知失败例外和恢复。Core 保持唯一 Task；没有恢复旧节点流程或历史数据兼容。公开 npm、Tag、GitHub Release 和原生 Host 支持声明没有随源码变更更新。

## 验证入口与适用范围

| 检查 | 覆盖范围 |
| --- | --- |
| go test -count=1 -timeout=90s ./... | 当前领域、批准、存储、执行、CLI/MCP/HTTP 和保留的仓库观察契约 |
| tests/journeys/engineering.test.mjs | 隔离 Git/SQLite 中的真实进程、批准阻断、验收降级反例、跨仓库失效、重复投入、例外和真实中断恢复 |
| tests/host-engineering.test.mjs | 从独立目录运行四个 Host 包及当前 Core，检查工具与引用闭包 |
| 四 Host 包的 Node 测试 | 启动、目录、钩子、Skill、安装管理的直接消费方；平台或能力不可用项明确跳过 |
| scripts/build-webui.mjs | TypeScript 检查和生成资源；浏览器交互另行核对 |
| scripts/sync-skill-references.mjs --check、scripts/check-versions.mjs | 生成契约和版本责任一致 |

Windows x64 本机 Go 1.27.0、Node.js 24.18.0 是本次开发环境。检查使用临时仓库和数据目录，保留用户安装及历史数据。完整入口是 node scripts/validate-engineering.mjs；单项命令按改动范围选择，实际通过、失败和跳过以本次执行输出为准。旧 tests/journeys/evidence/ 记录只说明各自记录时的产物，不能替代当前源码验证。

## 尚未证明的结果

真实 Core 执行与实际用户、模型会话不同。旅程的用户答复和语义判断是 fixture 输入，不能证明 AI 会正确理解全部需求或杜绝过度设计。没有完成原生 Codex/Claude Code/ZCode/DSH 模型会话、macOS 原生执行、最终注册表安装产物或长期用户成本对照，不据此增加支持或收益声明。

Core 可以拒绝未确认方案、明确越权、过期证据和无依据重复；语义必要性、比较检查充分性和宿主消息真实性仍需真实 Host 与用户审查。宿主绕过接入边界的写入不能声称已被预先拦截。详细验收见[工程控制](ENGINEERING-CONTROL.md)，已交付源码边界见[支持矩阵](SUPPORT-MATRIX.md)。
