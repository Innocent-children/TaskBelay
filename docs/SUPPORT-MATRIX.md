# TaskBelay 支持矩阵

[中文](SUPPORT-MATRIX.md) | [English](SUPPORT-MATRIX_en.md)

本次记录针对当前工程控制源码，未改变公开版本的支持承诺。安装公开包请参见对应宿主指南；它们的已发布身份由 release/ 下机器记录维护。

## 当前源码

| 组件 | 实现范围 | 此次实际验证 |
| --- | --- | --- |
| Go Core、CLI、MCP、HTTP | 已有用户授权复用、路径和验证控制、Git 观察、真实 argv 执行、跨仓库证据与恢复 | 此前 Windows x64 记录；本次 macOS arm64 原生回归，Windows 启动修复仅交叉编译 |
| Codex | 当前工具、Skill、工作位置、apply_patch 路径预检 | 独立包的实际 Core 调用与适配器测试；未运行模型聊天 |
| Claude Code | 当前工具、Skill、工作位置、Write/Edit/NotebookEdit 预检 | 独立包的实际 Core 调用与适配器测试；未运行模型聊天 |
| ZCode | 当前工具、Skill、工作位置、已注册结构化写入钩子 | 独立包的实际 Core 调用与适配器测试；未执行 UI 安装与模型聊天 |
| DeepSeek/DSH | 当前目录、Skill、工作位置、write/edit/str_replace_editor 预检 | 独立包的实际 Core 调用、目录与适配器测试；未运行 DSH 模型聊天 |
| WebUI | 本地结果、证据、缺口、累计成本和用户观察 | 前端构建、实际 HTTP 检查及本机浏览器展示核对 |
| 桌面宠物 | 显示 Core 的活动、阻碍、完成或取消结果 | Windows 显示规则单元检查；macOS Swift 检查此次不可用 |

适配器包要求 Node.js >=24。工具与权限能力必须以实际宿主会话为准；安装成功或独立 Core 握手不代表模型已使用插件。ZCode 的 action_required 仍须用户完成 UI 操作。

## 平台

| 精确 runtime pair | 本次范围 |
| --- | --- |
| win32-x64 | Windows 10/11 桌面 x64 实现；保留此前验证记录，新的挂起启动需原生复核 |
| darwin-arm64 | macOS arm64 实现；Core 定向测试、真实进程恢复及独立 Host 包检查，未运行模型会话 |

Runtime 选择拒绝 win32-ia32、win32-arm64 和 darwin-x64。Linux、Windows Server、Rosetta 和远程 MCP 没有当前支持承诺；可编译或模拟平台分支不扩大支持范围。桌面宠物正式分发签名与公证此次未验证。

## 控制边界

钩子覆盖的结构化写入可预先核对禁止路径。Shell、外部编辑以及钩子覆盖外操作只在检查点观察，不能宣称实时拦截。语义范围需要带依据的宿主判断；Go 导出声明与依赖变化是程序观察，不是通用语言语义证明。声明输入和环境之外的服务、文件与条件不视为已验证。

当前内部协议与旧数据不兼容，Core 保留并拒绝旧数据。使用源码应选择独立数据目录。完整记录见 [项目状态](PROJECT-STATUS.md)；安装、权限和命令见 [命令参考](COMMANDS.md)。
