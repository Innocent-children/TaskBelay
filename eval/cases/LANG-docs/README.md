# LANG-docs

docs 只按文件核对需求归属，不生成函数级结论。

输入按 ledger.yaml 依次建立任务、方案和编辑，再执行检查。expected.yaml 只约束本例负责的规则，不以其他提示数量代替结论。两份 YAML 使用 YAML 1.2 兼容的 JSON 写法。

运行：`go test ./eval -run '^TestCasePacks/LANG-docs$' -count=1`。这是隔离工作区中的内部流程测试，不代表真实 Agent 或其他平台接入通过。
