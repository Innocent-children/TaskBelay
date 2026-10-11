# TaskBelay 贡献指南

[中文](CONTRIBUTING_zh-CN.md) | [English](CONTRIBUTING.md)

TaskBelay 接受可复现的缺陷、文档修正、经过最终安装包验证的平台支持，以及围绕真实开发问题提出的
范围明确的产品改进。

## 变更分类

| 变更 | 要求 |
| --- | --- |
| 拼写、链接、翻译或现有行为说明修正 | 可以直接提交范围明确的 Pull Request；按照 [I18n 策略](docs/I18N.md) 同步受影响文档族的维护语言 |
| 模板或文档维护规则变化 | 说明影响范围；不修改产品版本，不执行发布 |
| 不改变公开接口行为的实现缺陷 | 说明接口规范与实际行为的差异，并只修复该问题 |
| 业务功能、用户操作流程、Core/MCP 约定、持久化、状态图或 Host Adapter 约定变化 | 在 Pull Request 中说明用户问题、范围、验收条件和方案，并同步实现、测试、文档与 i18n；修改 shipped Core 时同步更新 `CORE_VERSION` |
| npm package 版本、npm、Tag 或 GitHub Release | 不作为普通 Pull Request 的交付步骤；由维护者在功能合并后使用独立发布流程执行 |

分类不明确时，先提交 Issue，描述用户问题、当前行为和预期结果。不要先实现较大方案，再要求规格
接受已经完成的代码。

## 提交 Issue

缺陷报告请尽量包含：

- 使用的产品与版本，例如 Core、`taskbelay-codex`、`taskbelay-deepseek`、`taskbelay-claude` 或 `taskbelay-zcode`；
- 操作系统、CPU、Node.js 与 Host 版本；
- 最小复现步骤；
- 预期结果与实际结果；
- 已去除密钥、私有路径和个人数据的日志或错误输出；
- 问题是否涉及安装、显式触发、Task 流转、Recovery、数据目录或卸载。

产品提案应先说明具体用户问题、当前流程为什么无法解决，以及如何判断改进有效。实现方案可以讨论，
但不能直接替代需求定义。

### 产品功能提案模板

```markdown
## 使用场景

真实发生了什么？

## 当前做法

没有 TaskBelay 时，用户如何处理？

## 可用数据

从 Task、授权、仓库和已保存的结果中能确定什么？

## 处理规则

继续、复核、重试、阻塞还是请求用户决定？

## 预期效果

用户最终会看到什么变化？

## 风险与影响

误放行和误阻塞分别有什么后果？

## 验收方式

在哪个环境执行哪些步骤或测试，预期看到什么结果？

## 非目标

本次不扩展哪些能力？
```

### 实现前评估

进入实现前，提案需要清楚回答：

1. 是否帮助长时间运行的任务从正确状态继续？
2. 是否基于 Task、授权、仓库观察或已有记录，而不是只依赖 Agent 自述？
3. 是否减少用户判断当前状态和下一步的成本？
4. 能否在实际 Codex、DeepSeek、Claude Code 或 ZCode 会话中重复验证完整操作流程？
5. 是否保持一个 Core Task 状态？
6. 是否增加了不必要的流程步骤？
7. 是否解决了任务中的实际问题，还是仅增加一个平台、Host 或界面？

不能回答清楚用户问题、用户可见结果和验收方式的提案，不应直接进入实现。

## 文档表述规范

按读者和用途选择表述。根 README 是项目介绍和用户说明书，使用日常开发语言讲清用途、安装、
操作和必要限制；技术文档说明组件职责、接口、实现和验证方法。标题简洁，术语在需要时解释。
文档与源码入口见 [文档目录](MANIFEST.md)，仓库 AI 的工作规则统一维护在 [AGENTS.md](AGENTS.md)。

- 验收部分写“验收方式”，列出操作步骤或测试、运行环境和预期结果。
- 描述实际结果时使用“测试结果”“运行记录”“已保存的结果”，并说明哪些检查没有执行。
- 描述限制时直接写允许的字段、命令和操作；描述职责时写清哪个组件负责哪件事。
- 正文使用自然中文；代码标识符、字段名、命令和路径保留原样，首次使用时解释含义。
- 保留“代码基线”“接口规范”“状态机”“幂等性”等含义明确的技术术语。是否改写由上下文和准确性决定，不按词表机械替换。
- 按上下文改写整句；改写后保持功能、权限要求、失败处理和支持范围的含义一致。

## 本地环境

仓库开发需要：

- Go `>=1.26`；
- Node.js `>=24`；
- pnpm `>=11 <12`。

先在 GitHub Fork 本仓库，再从自己的 Fork 创建分支：

```bash
git clone https://github.com/<your-account>/taskbelay.git
cd taskbelay
git remote add upstream https://github.com/Innocent-children/taskbelay.git
git fetch upstream
git checkout -b <type>/<short-description> upstream/main
pnpm install --frozen-lockfile
```

开始修改前，请阅读 [I18n 策略](docs/I18N.md)、[命令参考](docs/COMMANDS.md) 和与变更直接
相关的文档。

## 实施原则

开发前展示需求理解、完整方案和验证安排，并取得用户对具体方案的明确确认；实质变更需重新确认。验收同时检查根因、责任归属和复杂度，不能仅以测试通过判断完成。

- 只解决 Pull Request 明确描述的问题，不加入未来能力、通用框架或无关重构；
- Task 授权、实际变化、证据适用性、累计成本、恢复和结束条件由 Go Core 决定；
- Core 只读观察 Git；已授权本地验证通过执行边界记录真实进程结果，仍遵守 Host 权限，不授予 Git 或发布权限；
- 只运行与改动表面、验收条件或已知风险直接相关的验证；
- 按文档职责更新受影响的说明：README 负责项目介绍和日常使用，`docs/PRODUCT*` 负责产品范围与行为规则，技术参考负责接口和实现细节；
- 业务功能或用户操作流程的变化影响项目用途、能力、安装步骤、常用操作或必要使用限制时，更新根 README 并同步 9 种语言；实现与接口细节在负责该内容的技术参考中更新；
- 文档修正同步受影响文档族的维护语言，不向其他文档复制变更说明；
- 新增或修改命令时，对照 package manifest、CLI parser、DSH lifecycle、Core parser 或 MCP catalog，并同步 `docs/COMMANDS*`；
- 面向用户的 npm 安装示例使用 `@latest`，人类阅读文档不记录精确产品版本；
- 修改实际交付的 Core 行为或接口规范时，同步更新机器可读 `CORE_VERSION`；不修改 npm
  package 发布版本，也不执行 Tag、npm 或 GitHub Release。

## 验证

文档改动按实际修改范围检查：

- Markdown、表格、代码块和 Mermaid 在 GitHub 上正常渲染；
- 语言导航中的所有文件存在且互相可达；
- 简中/英文文档族的章节结构、命令、平台和支持声明一致；
- 修改 README 时，9 种语言的项目介绍、安装、操作、必要限制和文档入口一致；
- 所有普通安装示例使用 `@latest`，精确产品版本只存在于机器可读文件和发布记录；
- 修改的命令说明与实际可执行 command/tool catalog 一致；
- 非英文文件没有占位翻译或整段英文 fallback；
- 没有扩大当前 [支持矩阵](docs/SUPPORT-MATRIX.md) 的声明。

代码改动优先运行受影响 package、节点、接口或用户操作流程的定向检查。完整仓库验证只在当前改动要求
的最终检查点运行：

```bash
pnpm run validate
```

每次完整测试都应有与改动相关的理由。模拟测试、静态检查和用户手工验证的结果，必须与实际
Codex / DeepSeek / Claude Code / ZCode 中对最终安装包的自动化测试结果分别说明。

## Pull Request 要求

从最新 `main` 创建分支，并在 Pull Request 中说明：

1. 当前问题；
2. 本次实际修改；
3. 明确未修改的范围；
4. 执行过的验证及结果；
5. 验收条件及对应的测试或接口规范；
6. 修改的文档族和已同步的 locale；
7. 命令或安装文档所对应的实现来源。

建议使用简洁的 Conventional Commit 风格，例如：

```text
docs: synchronize README locales
fix(store): reject invalid snapshot before writable open
```

Pull Request 应保持可独立审查。文档重写、产品行为变化、无关重构和版本发布应拆分为不同变更。

## 发布边界

合并产品工作不等于立即发布。Core、Codex、DeepSeek、Claude Code 和 ZCode 独立版本化；维护者在变更合并后填写产品、
channel 和精确版本，再执行固定检查、版本对齐、构建、回读、Tag、npm 和 GitHub Release。

提交 Pull Request 即表示你同意你的贡献按照本仓库的
[Apache License 2.0](LICENSE) 提供。
