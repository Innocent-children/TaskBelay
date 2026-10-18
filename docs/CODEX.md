# codex 宿主使用

[中文](CODEX.md) | [English](CODEX_en.md)

使用统一入口 npm install -g taskbelay@latest；taskbelay install --host codex。新工程契约在当前源码构建中提供，已发布包与稳定记录未变；从仓库运行 node scripts/taskbelay-local.mjs install --host codex --yes。保留旧数据并明确选择独立 TASKBELAY_DATA_DIR。Node.js >=24，源码构建还需 Go >=1.26 与 pnpm >=11 <12。

重新加载 Codex 插件，在 /hooks 中审阅并信任 apply_patch 钩子。

调用 TaskBelay Skill，说明期望结果和必要限制。代理在已有授权内推进，调整实施和验证方法无需重复确认；改变用户目标或明确限制时才记录实际新决定。默认沿用选定工作区。工具返回当前摘要，完整记录按需读取。Core 实际执行与代理声明、人工观察分开；中断后读取原 run，不盲目重跑。可选工作区助手保留真实 authorization_ref，返回当前 repositories；跨工作区迁移不支持。

status、doctor、update、remove 使用统一入口 --host codex，普通移除保留任务数据。结束不授予 Git、发布或清理权限。详见[包说明](../packages/codex/README.md)、[命令](COMMANDS.md)、[验证边界](SUPPORT-MATRIX.md)。适配器测试不等于实际宿主会话验证。
