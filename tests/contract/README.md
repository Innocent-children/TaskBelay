# 仓库接口规范测试

本目录检查当前源码是否遵守公开接口规范，包括允许的工具、输入字段、返回格式和失败处理。
测试读取当前 checkout 中的定义文件，并使用临时目录或数据库。它们不启动实际 Codex，也不发布
安装包，因此测试通过不能代替实际 Codex 运行或公开安装包的验证。

## 覆盖范围

- `mcp_contract_test.go`：固定的十八个 MCP 工具、Action 提交的步骤结果字段、工作树/迁移/放弃字段、
  Recovery、ServerInfo 和存储标识；
- `result_envelope_test.go`：返回结构、固定公开错误、允许的返回字段和敏感信息隐藏；
- `graph_contract_test.go`：MCP 工具数量、流程图 JSON 样例可解析、需求提交的 `method_results` 字段，以及拒绝 `get_task` 的额外输入字段；
- `current_storage_contract_test.go`：新存储初始化、快照校验、只接受当前运行格式、不支持时停止、
  启停时保留数据，以及隐藏私有路径；
- `platform_boundary_test.go`：Core 的任务规则不判断操作系统；Codex、DeepSeek、Claude、ZCode 和统一管理器的系统差异各自实现；
  两个平台的可执行文件都来自统一构建目录；
- `fixture_contract_test.go`：当前 Core 流程图样例，以及不同 Host 返回同一 Core 标识；
- `repository_root_test.go`：仓库目录、安装包清单、脚本和允许执行的文件；
- `final_local_payload_test.go`：本地返回数据及各 Host 接收的数据；
- `webui_package_closure_test.go`：嵌入安装包的 WebUI 文件是否齐全。

## 测试方式

- **静态检查**：检查仓库中保存的清单、Schema 和 JSON 样例。
- **可重复的接口测试**：执行 Go 测试，检查字段限制、顺序、允许项、拒绝请求时不写入数据，以及敏感信息隐藏。

Host 对照样例只检查 Core 标识一致，不能代替 DeepSeek 产品测试。包结构检查通过，也不表示已经
构建本地安装包、在实际 Codex 中运行，或完成 npm 公开安装包的验收。
