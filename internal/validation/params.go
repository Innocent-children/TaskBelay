// Package validation 按内容与运行事实判断验证是否提供新信息，见 docs/over-testing.md 验证输入记录。
package validation

const (
	Epsilon         = 0.05 // Beta 后验允许的不一致概率精度，见 docs/over-testing.md 范围包含与通过规律。
	TailProbability = 0.05 // Beta 后验超过精度的尾部概率，见 docs/over-testing.md 验证输入记录。
)
