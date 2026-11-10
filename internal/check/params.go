// Package check 组织内容分析，依据已核对的事实决定是否拦截，并生成交付报告；职责见 docs/algorithm.md 六至九节。
package check

import "time"

const (
	Algorithm               = "content-14"           // 计算格式版本；见 algorithm.md 一、八节。
	FeedbackVersion         = 5                      // 反馈格式版本；见 requirements-ledger.md MCP 工具。
	PruneBatch              = 200                    // 每批删除的无引用索引页数；每批都在普通写入的等待时限内提交。
	GapExamples             = 2                      // 反馈里列出的无法判断项示例数；完整清单在检查记录中。
	OutputHead              = 600                    // 验证反馈中上次输出保留的开头字符数；与结尾合计不超过反馈预算。
	OutputTail              = 3000                   // 验证反馈中上次输出保留的结尾字符数；测试汇总通常在结尾。
	FeedbackBudget          = 6000                   // 交付反馈字符预算；见 product-architecture.md 性能预算。
	FeedbackUnit            = 20                     // 背包容量的字符换算单位；见 algorithm.md 六节。
	CategoryLimit           = 3                      // 每类建议的反馈上限；见 algorithm.md 六节。
	FitIterations           = 128                    // 逻辑回归迭代预算；见 algorithm.md 七节。
	FitRate                 = 0.1                    // 确定性梯度下降步长；见 algorithm.md 七节。
	Regularization          = 4.0                    // 向初始权重收缩的强度；见 algorithm.md 六节。
	FitPrecision            = 0.000001               // 拟合梯度停止精度；见 algorithm.md 六节。
	PublishReserve          = 300 * time.Millisecond // 为结果写入账本预留时间；见 product-architecture.md 性能预算。
	BackgroundPoll          = time.Second            // MCP 后台观察已登记输入变化的间隔；见 algorithm.md 九节。
	ReuseHints              = 3                      // 用户消息中已有单元提示上限；见 algorithm.md 九节。
	CalibrationSignificance = 0.05                   // 恢复内容的多重比较显著性；见 algorithm.md 七节。
	IndexBatch              = 1024                   // 索引读写的单个短事务行数；见 algorithm.md 九节。
	IndexBatchBytes         = 512 * 1024             // 索引短事务的正文批量目标；大条目单独写入，见 algorithm.md 九节。
	MatcherPageSize         = 128                    // 自动机索引页的状态数，查询只读触及的页；见 algorithm.md 九节。
)

var initialWeights = map[string]float64{
	"bias":          -2,   // 无本机标签时采用保守先验；见 algorithm.md 六节。
	"unrelated":     0.5,  // 相关区外仅提供有限支持；见 algorithm.md 三、六节。
	"unanchored":    0.5,  // 缺少词汇对应的排序权重；见 over-design.md 二节。
	"reliable":      0.5,  // 可靠识别的排序权重；见 over-design.md 二节。
	"agent":         0.25, // 可核对来源的排序权重；见 algorithm.md 二节。
	"structure":     0.25, // 结构置信度的排序权重；见 languages.md 结构置信度。
	"confirmed":     2,    // 确认运行的排序权重；见 algorithm.md 五、六节。
	"seed_quote":    0,    // 原话种子校准的中性起点；见 algorithm.md 三、七节。
	"seed_content":  0,    // 需求内容种子校准的中性起点；见 algorithm.md 三、七节。
	"seed_location": 0,    // 实现位置种子校准的中性起点；见 algorithm.md 三、七节。
}
