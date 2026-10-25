为 Map<String, Value> 增加消耗 map 的 into_values，返回拥有各个 Value 的迭代器。顺序与同一 map 的 values 一致，包括默认排序和 preserve_order；保持精确长度、双端遍历、耗尽后的行为，空 map 返回空迭代器，不要求调用方复制值。
