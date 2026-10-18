# 仓库契约检查

本目录检查当前仓库、包资源和平台职责边界。运行 go test ./tests/contract。repository_root_test.go 核对当前源目录、安装包清单及允许的执行入口；platform_boundary_test.go 核对 Core 语义与平台实现分离；webui_package_closure_test.go 核对嵌入资源完整性。

执行响应、输入校验、持久化、执行证据和生命周期测试由 internal/mcp、internal/store、internal/execution、internal/application 及 tests/journeys 拥有。它们使用实际生产接口；旧阶段协议及静态成功结果样例已删除。原生宿主模型会话与适配器/API 检查分别记录，测试通过不代表公开版本已发布。
