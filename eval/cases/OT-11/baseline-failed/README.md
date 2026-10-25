# OT-11/baseline-failed

同一外层测试 TestGrouped 的两个子例共享初始化状态：共同枚举时先运行 setup，dependent 可以通过；逐例基线使用独立进程，单独选择 dependent 时因前置状态不存在而断言失败。该失败不能当成发现变异；保留真实 Go 基线观察且不提供 T2 删除候选，也不为这一对用例运行变异。
