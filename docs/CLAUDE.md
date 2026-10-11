# claude 宿主使用

[中文](CLAUDE.md) | [English](CLAUDE_en.md)

使用统一入口 npm install -g taskbelay@latest；taskbelay install --host claude。新工程契约在当前源码构建中提供，已发布包与稳定记录未变；从仓库运行 node scripts/taskbelay-local.mjs install --host claude --yes。保留旧数据并明确选择独立 TASKBELAY_DATA_DIR。Node.js >=24，源码构建还需 Go >=1.26 与 pnpm >=11 <12。

重载插件或新建会话并审阅原生权限。

调用 TaskBelay Skill，说明期望结果、非目标与必要限制。已有授权继续有效，不重复确认内部阶段。代理交替实施、检查与范围复核，Core 真实执行与声明/人工结果分开；未覆盖 shell 和远端操作必须明示。中断恢复原目录，读原 run，不盲目重跑。可选工作区助手保留真实 authorization_ref，scope 返回当前 repositories；跨工作区迁移不支持。

status、doctor、update、remove 使用统一入口 --host claude，普通移除保留任务数据。结束不授予 Git、发布或清理权限。详见[包说明](../packages/claude/README.md)、[命令](COMMANDS.md)、[验证边界](SUPPORT-MATRIX.md)。适配器测试不等于实际宿主会话验证。
