<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>长时 AI 编程，始终有人保护。</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## TaskBelay 能帮你做什么

技术判断，交给 Agent。<br />
任务边界，交给 TaskBelay。

配合 Codex、DeepSeek、Claude Code 或 ZCode 使用。

Belay 是攀岩中的绳索保护：攀登者自己选择路线，保护者控制绳索并在失误时阻止坠落。TaskBelay 沿用这个分工：Agent 做技术判断，TaskBelay 核对已批准范围和验证额度，并依据持久记录处理失败或结果不确定的操作。它不是沙箱，也不是另一个 Coding Agent。

- **范围明确：** 按已确认的文件核对改动。计划外的工作，另作决定。
- **验证有度：** 检查有计划、有上限。增加验证，先说明理由。
- **状态持久：** 任务以本机保存的状态为准，不随会话消失。
- **安全恢复：** 失败或结果不明，先查记录，再决定如何继续。

适合跨会话、需要明确文件范围和测试投入的仓库任务。一次性问答、代码解释和不需要保存进度的
小型修改，直接使用 Codex、DeepSeek、Claude Code 或 ZCode 通常更简单。

## 快速开始

> 使用 Node.js `>=24`，并先安装所使用的 Host。版本要求和已验证平台见[支持矩阵](docs/SUPPORT-MATRIX.md)。

### 1. 安装 TaskBelay

安装 TaskBelay 后，按 [Codex](packages/codex/README.md)、[DeepSeek](packages/deepseek/README.md)、[Claude Code](docs/CLAUDE.md) 或 [ZCode](docs/ZCODE.md) 指南启用对应 Host。安装包面向 Windows x64 和 macOS arm64；macOS ZCode 实机验证仍待完成。

```sh
npm install -g taskbelay@latest
taskbelay
```

在所用安装入口中选择对应 Host。Codex 安装后在 `/hooks` 中检查并信任 TaskBelay hook；DeepSeek 重启所选 Profile；Claude Code 重载插件或开始新会话，并按提示审阅权限。

ZCode 需在 Settings → Plugins 中完成插件安装和启用，再开始新会话使 Hook 生效。本地包准备完成不代表 ZCode 已加载插件。

### 2. 启动任务

完成对应安装后，在所用 Host 的对话中发送以下消息之一：

**Codex**

```text
$taskbelay-codex:taskbelay 增加登录失败限流。只修改认证相关文件，最多运行 4 项定向检查。
```

**DeepSeek Harness**

```text
/taskbelay 增加登录失败限流。只修改认证相关文件，最多运行 4 项定向检查。
```

**Claude Code**

```text
/taskbelay-claude:taskbelay 增加登录失败限流。只修改认证相关文件，最多运行 4 项定向检查。
```

**ZCode**

在输入框的 `/` → Skills 中选择 `taskbelay`，再描述任务。

```text
使用 TaskBelay 增加登录失败限流。只修改认证相关文件，最多运行 4 项定向检查。
```

这些消息发送到对话中，不在终端执行。尽量写清目标、验收条件、文件范围和测试上限。

首次回复会评估请求，并询问直接开发还是使用 TaskBelay。选择 TaskBelay 后，默认从当前 HEAD
在当前目录新建任务分支。确认新分支及是否将已有未提交修改纳入任务；现有依赖、本地配置、文件和
暂存状态保留。当前会话能访问全部参与目录时，直接在原会话继续。

你也可以明确选择使用当前分支，或创建独立 Git 工作树。独立工作树还需选择本地或远端来源及起始
分支；Codex 在宿主支持时打开新目录，DeepSeek 和 Claude Code 提供相应的重新启动命令。

同一目录只能有一个活动 Task。手工或其他工具的本地编辑也会被观察，任务中切分支会暂停流程。
本地目录和分支在任务结束后保留；开始下一任务时仍需明确未提交修改的归属。

实现前，先查看并讨论需求、方案、任务、预计文件和验证安排。完整计划得到你的明确认可后才开始开发；方案修订或文件范围扩大后需要再次确认。选择 TaskBelay 或工作树参数不代替方案确认。

需要修正活动 Task 的分支名时，请 Host 先准备 TaskBelay 分支改名，再只改一个仓库的分支名，保持
目录、HEAD、暂存区和文件不变。TaskBelay 核对后继续同一 Task；取消改名只允许原事实尚未改变时。
Codex 尚未执行的启动可以明确替代；失败启动必须有未调用目标操作的证据，不确定或部分完成的操作先检查。

多轮计划修订在 Task 内完整保留已保存的基线引用，不归档、无配置性条数上限，仍受资源及修订号表示范围约束。
响应显示有界首页和总数；复核旧决定时可让 Host 分页读取完整历史。
具体步骤和限制见[工作区操作](docs/WORKTREE-SOURCES.md)与[命令参考](docs/COMMANDS.md)。

### 3. 恢复和查看进度

会话重启后，回到任务的原工作目录，明确请求继续该任务。TaskBelay 会从已保存的进度继续。
原工作目录丢失或被替换时，任务会暂停，直到你恢复它或明确放弃任务。

在 DeepSeek Harness 中，继续任务的消息也要带上 `/taskbelay`。

Claude 用户应回到原工作目录和会话，使用 `/taskbelay-claude:taskbelay` 明确继续已保存的任务。

ZCode 用户应重新打开原工作目录，选择 TaskBelay Skill 并请求继续已保存的任务；准备新目录时按返回的工作区打开说明接续。

下列命令使用已安装的全局管理器；源码体验请使用指南中的对应入口。

```bash
# 查看已安装的集成
taskbelay status --host all

# 打开本机任务界面
taskbelay webui start
```

非交互安装、自定义 DSH Profile、升级、修复和移除方式见[命令参考](docs/COMMANDS.md)。

## 桌面宠物

宠物需要已配置的 Adapter 和已安装的桌面应用。仅安装 Adapter 不会安装桌面应用。

桌面宠物通过叠加气泡展示多个任务，并可分别打开对应 WebUI。它优先展示受阻任务，当前任务完成后自动关注其他未完成任务，也支持固定关注。你可以自定义形象、控制动画、调整大小，以及独立启动或停止宠物。

```bash
taskbelay pet start
taskbelay pet stop
```

桌面应用面向 macOS arm64 和 Windows 10/11 x64。安装与操作见[桌面宠物指南](docs/DESKTOP-PETS.md)，
已验证的可用范围见[支持矩阵](docs/SUPPORT-MATRIX.md)。

## 使用限制

TaskBelay 管理任务流程，不接管系统权限，也不拦截每次文件操作或命令执行。

专属工作树用于分开代码改动。进程、网络、凭据和外部服务仍与当前环境共享。

完成任务不会自动提交代码、推送或删除工作树；这些操作需要你另外授权。

## 文档

- **使用说明：** [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE.md) · [ZCode](docs/ZCODE.md) · [命令参考](docs/COMMANDS.md) · [Control Center](docs/WEBUI.md)
- **项目资料：** [产品定义](docs/PRODUCT.md) · [支持矩阵](docs/SUPPORT-MATRIX.md) · [安全策略](SECURITY.md)
- **开发与贡献：** [文档目录](MANIFEST.md) · [贡献指南](CONTRIBUTING_zh-CN.md)

## 社区

[LINUX DO](https://linux.do/)

## 许可证

[Apache License 2.0](LICENSE)
