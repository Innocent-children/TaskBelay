# Windows 适配与当前验证

[中文](WINDOWS-ADAPTATION.md) | [English](WINDOWS-ADAPTATION_en.md)

当前工程控制源码在 Windows x64、Go 1.27.0、Node.js 24.18.0 上执行验证。平台实现负责路径、文件身份、进程权限与寿命，Core 语义不选择操作系统。执行使用隐藏进程及 Windows Job，恢复核对原执行者启动身份并读取真实收据；它不是沙箱。

完整 Go 检查、本机开发/修订/恢复、四宿主独立包及 Windows 桌面显示规则通过。ZCode 最终本地产物完成隔离 launcher、MCP、真实检查与路径钩子验证，未操作实际 ZCode UI。文件符号链接等本机不可用条件在测试输出明确跳过，不算通过。旧原生适配记录位于 Git 历史，当前结果以 [项目状态](PROJECT-STATUS.md) 为准。

macOS arm64 只交叉构建与检查元数据；未执行 Swift 原生测试、模型会话、签名、公证或发布。已有用户安装、任务数据及原 main 未改变。当前支持边界见 [支持矩阵](SUPPORT-MATRIX.md)，运行方式见 [命令](COMMANDS.md) 和 [桌面指南](DESKTOP-PETS.md)。
