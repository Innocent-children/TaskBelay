# ZCode 使用指南

[中文](ZCODE.md) | [English](ZCODE_en.md)

本指南适用于智谱 ZCode 的 TaskBelay 原生插件。Adapter 已作为 `taskbelay-zcode@latest` 发布。目标平台为 Windows x64 和 macOS arm64；macOS 实机验证留待后续。实际记录见[支持矩阵](SUPPORT-MATRIX.md)。

## 安装与启用

需要 Node.js `>=24`、Git，以及提供原生插件、Skills、MCP 和 Hooks 的 ZCode。按 ZCode 自身提示完成登录与权限设置。

从 npm 安装：

```sh
npm install --global taskbelay@latest
taskbelay install --host zcode --yes
taskbelay status --host zcode
```

管理器会返回本地 marketplace 路径和需要在 ZCode 界面完成的步骤；已验证的环境见支持矩阵。

源码安装另外需要 Go `>=1.26` 和 pnpm `>=11 <12`。在仓库根目录执行：

```sh
pnpm taskbelay:local -- install --host zcode --yes
node packages/taskbelay/bin/taskbelay.mjs status --host zcode
node packages/taskbelay/bin/taskbelay.mjs doctor --host zcode
```

源码命令通过仓库管理器构建本地包。如有维护者提供的本地 tarball，可以不编译 Go，直接安装：

```sh
npm install --global "<taskbelay-zcode包路径.tgz>"
taskbelay-zcode setup --json
taskbelay-zcode status --json
```

`setup` 校验并准备本地插件来源，返回 `action_required` 和 `next_steps`。随后在 ZCode 中完成：

1. 打开 **Settings → Plugins → Create → Add marketplace**。
2. 使用命令返回的本地 marketplace 路径，安装并启用 `taskbelay-zcode`。
3. 开始新会话，使插件 Hook 生效；检查插件、MCP 和权限提示。
4. 在输入框输入 `/`，打开 **Skills**，选择 `taskbelay`。界面中的实际名称以 ZCode 为准。

Adapter 没有可靠接口读取 ZCode 的插件加载与缓存状态，因此准备成功后 `status` 仍为 `action_required`，`registration.host` 为 `unverified`。这不是登录或模型会话已验证的声明。统一 `doctor` 对该状态返回退出码 1，表示 Host 状态尚待核验；`partial` 则表示本地准备或校验不完整，应先处理诊断结果。官方界面说明见 [ZCode 插件文档](https://zcode.z.ai/en/docs/plugin)，Hook 生效规则见 [Hooks 文档](https://zcode.z.ai/en/docs/hooks)。

## 开始与恢复任务

打开要开发的仓库，选择 TaskBelay Skill 后发送：

```text
使用 TaskBelay 为登录接口增加失败限流，只修改认证相关文件。
```

ZCode 先评估请求，让你选择直接开发或 TaskBelay。TaskBelay 默认在当前目录新建分支，也支持当前分支或独立工作树。明确目标分支及已有未提交内容是否属于任务；独立工作树还需明确来源、起始分支与目标目录。一项任务最多包含八个明确选择的仓库，全部准备好后才创建 Core Task。

实现前查看完整需求、方案、工作项、预计文件和验证安排，确认后开始开发。需要新工作区时，Adapter 返回准备好的目录和接续提示；按 ZCode 实际界面打开目录并授予所需访问权限。返回描述不代表界面已经打开，也不代表新会话已经建立。

恢复时回到原工作目录，选择同一 Skill，并明确请求继续保存的任务。先读取已有状态和启动记录；启动结果不明时，不重复创建任务或工作树。原目录丢失或被替换时先恢复原实例，或明确放弃任务。

可以请求取消任务并保留文件。完成和取消会释放任务占用，保留文件及分支；独立工作树与分支清理分别授权。仅全部仓库采用独立工作树时支持工作区迁移，本地模式在原目录恢复。迁移必须先取得并保留 Core 的完整准备结果，核对同一任务和全部源目录后再移动；结果不确定时先读取原记录，不能重新移动。完成 Core 核验后可再发起下一次迁移。

## 文件范围与权限

原生 PreToolUse Hook 在 ZCode 的 **Write** 和 **Edit** 写入前检查目标。计划外写入或无法完成检查时会拒绝操作；按 Core 返回的信息处理单次允许、修订计划或恢复文件。放行不覆盖 ZCode 自身权限。Hook 更新需开始新会话。

Shell 和外部程序写入仍可能先发生，再由 Core 观察发现。不要改用其他工具绕过拒绝。OpenSpec、Spec Kit 和代码索引均按实际可用能力使用，缺失工具不代表对应工作已经完成。

## 维护与查看进度

源码安装用户继续在仓库根目录使用：

```sh
pnpm taskbelay:local -- repair --host zcode --yes
node packages/taskbelay/bin/taskbelay.mjs webui start
node packages/taskbelay/bin/taskbelay.mjs webui stop
```

维护只更新本地来源。按 `next_steps` 在 ZCode 中刷新或重新安装插件，并开始新会话；同版本源码更新也需确认缓存刷新。WebUI 可以筛选 ZCode Task 并查看保存进度。桌面宠物另外需要桌面应用，单独安装 Adapter 不提供该应用，见[桌面宠物指南](DESKTOP-PETS.md)。

## 移除与数据保留

结束相关 ZCode 会话，停止通过该 Adapter 启动的 WebUI，再从源码管理器发起卸载：

```sh
node packages/taskbelay/bin/taskbelay.mjs uninstall --host zcode --yes
```

只有 Adapter 包时，先运行 `taskbelay-zcode remove --json`。普通移除返回 `action_required`；统一管理器此时也保留 Adapter 包和待移除记录，以便后续确认。它不能替你确认 ZCode 缓存已经删除。必须在 ZCode UI 中卸载插件、移除对应 marketplace，并关闭相关会话。

上述界面操作确实完成后，在全局包仍存在时执行以下命令，确认 Host 移除并卸载 Adapter 包：

```sh
taskbelay-zcode remove --confirm-host-removed --json
npm uninstall --global taskbelay-zcode
```

该确认返回 `absent` 和 `registration.host=user_confirmed_removed`，表示人工确认，不是自动检测。确认后的包卸载也可以重新运行 `uninstall --host zcode --yes`，使用上方同一管理器入口。普通维护和卸载保留 Task 数据及无关配置。存在 ZCode 包或安装记录时，管理器拒绝 `factory-reset`，因为无法可靠枚举 ZCode 缓存中的 Core 进程。确需清空数据时，先完成 UI 移除、关闭会话、确认移除并卸载全局包，再运行管理器的独立 reset 流程。

`TASKBELAY_DATA_DIR` 可指定已有的规范绝对数据目录，ZCode、MCP 与管理器须保持一致。默认数据位于 macOS 的 `~/.taskbelay/data` 或 Windows 的 `%LOCALAPPDATA%\taskbelay\data`。

完整命令见[命令参考](COMMANDS.md)，实际检查和后续验收见[项目状态](PROJECT-STATUS.md)。

## 持续修订与任务分支改名

同一 Task 可以继续多轮需求、设计和计划修订，旧基线引用超过 32 条后仍可分页核对；系统保留
真实引用，不保证恢复未保存的旧正文。活动 Task 需要改分支名称时，先由 Core 准备一个仓库的改名，
再让获授权的 Host 执行一次非 force 的 `git branch -m`，最后由 Core 核对并确认。工作树、HEAD、
index、内容和其他仓库必须不变；改名前可以取消准备，已经改名但未确认时先回读，不重复执行。

后续搬迁和终态清理使用 Core 已确认的有效分支，创建时分支仍作为来源保留。清理命令须传完整终态 `core_task`；缺少或不匹配的事实会拒绝清理。
精确条件和恢复步骤见[工作树来源与恢复](WORKTREE-SOURCES.md)。
