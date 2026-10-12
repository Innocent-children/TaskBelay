# 工作位置与分支

[中文](WORKTREE-SOURCES.md) | [English](WORKTREE-SOURCES_en.md)

每次新建 Task，都先展示实际目录、当前分支和已有修改，再询问用户选择当前分支、原目录新分支或独立工作树。不预设工作位置，也不沿用其他 Task 的选择。恢复已有 Task 时使用原绑定目录，不重复询问。按用户本次选择，由宿主准备工作位置。Host 保存 authorization_ref、具体来源、起始分支、冻结 commit、目标分支、携带选择和操作结果；Core 只读核对工作区身份并保存目录占用。

可选 local new_branch/current_branch 与 dedicated_worktree 准备助手保留来源核验、初始内容及不确定操作回执。沿用已经明确的选择，只询问缺少的关键项。携带已暂存、未暂存及非 ignored 内容时保留源目录；ignored 内容不自动复制。scope 返回当前 repositories 数组；它不是 Core 状态机。

恢复回到原绑定实例。切分支、回退/重写或实例替换不能由语义判断放行；恢复原状态后继续。跨工作区迁移未支持。完成/取消保留目录、分支和代码；删除需要独立真实授权、当前终态 Core 记录及清洁和推送状态核对。未知、dirty 和未推送资源保留。详见各宿主指南。
