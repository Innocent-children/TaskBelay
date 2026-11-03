// Package dynamic 在固定快照的副本中运行可信命令、撤回试验和变异；见 docs/languages.md 副本中的运行及 algorithm.md 四、五节。
package dynamic

import "time"

const (
	Budget             = 120 * time.Second      // 一次动态检查总预算；见 product-architecture.md 性能预算。
	CommandBudget      = 30 * time.Second       // 单次副本命令等待上限；见 languages.md 副本中的运行。
	RunLimit           = 128                    // 进程运行数量预算，确认运行也计入；见 algorithm.md 五节。
	MutationLimit      = 64                     // 逐个变异的抽样预算；见 over-testing.md 三节。
	Epsilon            = 0.05                   // Good–Turing 新发现集合概率停止精度；见 over-testing.md 三节。
	Significance       = 0.05                   // 识别接口清单和禁用高无效率算子的统计精度；见 over-design.md 三节及 over-testing.md 三节。
	InvalidProbability = 0.5                    // 算子无效率需要显著超过的水平；见 over-testing.md 三节。
	OutputBytes        = 1 << 20                // 单次输出保留及运行保护预算；见 product-architecture.md 性能预算。
	PollInterval       = 100 * time.Millisecond // 检查 Agent 验证和输出预算的间隔；见 languages.md 副本中的运行。
	ConfirmReserve     = 0.2                    // 为最终确认保留的时间比例；见 algorithm.md 五节。
	ProbeLimit         = 4                      // 接口清单探针的命令数量预算；见 over-design.md 三节。
	ProbeColumnGap     = 2                      // 清单名称与说明之间的空格间距，制表符另行识别；见 over-design.md 三节。
	ParallelMin        = 1                      // 副本并发下限；见 languages.md 副本中的运行。
	ParallelMax        = 4                      // 副本并发上限；见 languages.md 副本中的运行。
)
