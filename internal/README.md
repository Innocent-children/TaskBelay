# Core 源码

Go Core 使用 domain 的当前授权和证据规则，application 组合 repository、execution 与 store，mcp、CLI 和 webui 只投影同一结果。固定阶段流程已移除。平台差异位于 repository/execution/webui 的平台文件。

按[架构](../docs/ARCHITECTURE.md)选择职责，定向运行对应包测试。完整系统检查使用 node scripts/validate-engineering.mjs；真实执行场景在 tests/journeys/engineering.test.mjs。
