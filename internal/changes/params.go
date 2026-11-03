// Package changes 计算累计改动、变更簇和需求相关区，见 docs/algorithm.md 一至三节。
package changes

const (
	Epsilon         = 0.0001  // 局部推送的残量精度及边权精度，见 docs/algorithm.md 需求相关区。
	Alpha           = 0.15    // 局部推送停留概率，见 docs/algorithm.md 需求相关区。
	FileWeight      = 0.05    // 文件包含关系的相对权重，见 docs/algorithm.md 图。
	ClusterWeight   = 1.0     // 改动与单元及同一调用的连接权重，见 docs/algorithm.md 图。
	QuoteWeight     = 1.0     // 原话及有效方案的种子初始权重，见 docs/algorithm.md 种子和校准。
	ContentWeight   = 0.6     // 需求及验收的种子初始权重，见 docs/algorithm.md 种子和校准。
	LocationWeight  = 0.2     // 实现位置的种子初始权重，见 docs/algorithm.md 种子和校准。
	DiffTraceBudget = 8 << 20 // Myers 回溯最多保存的整数数量，到期返回未完成，见 docs/product-architecture.md 性能预算。
)
