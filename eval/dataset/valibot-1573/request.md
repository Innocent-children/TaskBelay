统一 literal、value、notValue、values、notValues 和 intersect 对特殊数值的相等判断：NaN 与自身匹配，正零和负零互换仍匹配，反向限制拒绝相应值。合并结果中的 NaN 和无效 Date 不应因为同值比较而错误冲突；不同实例但时间相同的有效 Date 保持现有行为。JSON Schema 转换不得把这些不可表示的特殊字面值当作普通有限值。
