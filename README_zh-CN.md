# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**为真实 AI 编程任务提供工程控制。**

TaskBelay 配合 Codex、DeepSeek Harness、Claude Code 和 ZCode 使用，将授权目标、实际代码变化与验证证据保存在一起，让代理跨会话继续工作时少做重复决定。编码和技术判断仍由代理负责。

## 你能做什么

- 让目标、明确非目标，以及接口、数据、依赖和设计约束在开发中持续有效。复核文件内的变化，而不仅是路径。范围偏离优先在已有授权内纠正；只有需要改变授权或作出重要取舍时才询问用户。
- 执行已授权的本地检查，保存真实退出码、输入、环境条件和结果。代理声明、进程执行和验收结论分别记录。相关变化使证据过期；有明确依赖依据的独立检查可以保留。重新规划不重置累计投入。
- 合理交替实施与验证，修订需求，并从当前成果、失败尝试和未确定执行恢复。近期历史有界保存，有效结果与累计成本继续保留。

## 开始使用

需要 Node.js >=24 及已安装的编码宿主。源码开发还需要 Go >=1.26 和 pnpm >=11 <12。当前源码包含破坏性工程升级；已发布包版本和稳定支持记录没有改变。使用新契约须构建当前源码。保留旧数据并明确选择独立数据目录；不提供迁移或自动重置。

公开安装入口：

```sh
npm install -g taskbelay@latest
taskbelay
```

在本机构建并安装当前源码：

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

按相应[宿主指南](packages/codex/README.md)启用适配器，再向代理描述范围明确的结果：

```text
使用 TaskBelay 增加登录失败限流。保留公开认证接口和已有用户数据，不重做账号管理。运行相关已有测试，报告真实结果。
```

说明期望结果与限制。实施前，代理展示需求理解、根因、完整方案、责任、范围、检查和投入上限，由你明确确认。实质修订需要重新确认，普通进展沿用有效批准。检查必须对应当前缺口，已有适用通过结果应复用。WebUI 展示方案、证据与投入，支持确认和终态任务归档，并能通过详情链接定位任务。恢复时提供 Task ID，返回原工作区。

```sh
taskbelay webui start
```

## 使用边界

结构化钩子覆盖受支持的写入路径；shell 和外部编辑在检查点观察。只有 Core 实际启动的检查具有 Core 执行来源。声明输入与环境条件不会自动涵盖 ignored 依赖、数据库或远端服务。语义推理明确保留，但不构成正确性证明。TaskBelay 不是沙箱，也不是另一个编码代理。

Core 开销实际计量；宿主分析、等待和 token 来自报告，可能不完整。小型机械任务通常直接使用原生代理更简单。源码测试证明具体能力，不证明缺陷率或人工监督成本已经降低。完成保留文件，不授予 Git、发布或删除权限。

## 进一步了解

[产品定义](docs/PRODUCT.md) · [命令](docs/COMMANDS.md) · [支持与验证](docs/SUPPORT-MATRIX.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE.md) · [ZCode](docs/ZCODE.md)
