# OT-14

动态检查到达固定副本准备阶段后用户无声修改边界，旧结果标记过期，用户当前行为保留。此准备门使用 POSIX，Windows 原生并发编辑验证仍待完成。

输入按 ledger.yaml 依次建立任务、方案和编辑，再执行检查。expected.yaml 只约束本例负责的规则，不以其他提示数量代替结论。两份 YAML 使用 YAML 1.2 兼容的 JSON 写法。

运行：`go test ./eval -run '^TestCasePacks/OT-14$' -count=1`。这是隔离工作区中的内部流程测试，不代表真实 Agent 或其他平台接入通过。
