// Package units 将固定内容转换成词流和版面单元，见 docs/languages.md 的统一词法与版面树。
package units

const (
	PollBytes       = 4096 // 每隔这些字节响应取消，见 docs/product-architecture.md 性能预算。
	ConfidencePrior = 8.0  // 结构样本的收缩强度，见 docs/languages.md 结构置信度。
	GramSize        = 5    // 内容签名的词窗口，见 docs/algorithm.md 变更簇及 over-design.md 重复实现。
	WindowSize      = 4    // winnowing 的最小哈希窗口，见 docs/algorithm.md 变更簇。
	SignatureSize   = 64   // MinHash 签名长度，见 docs/over-design.md 重复实现。
)
