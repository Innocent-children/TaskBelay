// Package repostats 从起点内容和运行结果学习项目约定，见 docs/languages.md 从起点学习的约定。
package repostats

const (
	Prior           = 0.5  // Dirichlet 与 Beta 的对称先验，见 docs/languages.md 测试内容。
	Significance    = 0.05 // 约定入口的二项检验显著水平，见 docs/languages.md 约定入口。
	IterationBudget = 32   // 测试内容重分类的计算预算，未收敛保留未知，见 docs/product-architecture.md 性能预算。
	OutputRadius    = 4    // 学习用例结果标记时采用邻近词窗口，见 docs/languages.md 运行结果。
	HashCharacters  = 16   // 清单与锁文件统计中哈希片段的最小字符数，见 docs/languages.md 从起点学习的约定。
)
