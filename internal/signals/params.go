// Package signals 提取可以复核的设计与测试候选，见 docs/over-design.md 与 docs/over-testing.md。
package signals

const (
	LSHRows           = 8    // 每个 LSH 桶使用的签名行数，见 docs/over-design.md 重复实现。
	SimilaritySamples = 64   // 无关单元相似度分布的抽样预算，见 docs/over-design.md 重复实现。
	CallPrecision     = 0.01 // 调用写法统计低于此精度时参数分析未知，见 docs/languages.md 结构置信度。
)
