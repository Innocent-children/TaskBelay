修正 prefault 的 undefined 输出与对象缺失键行为。prefault 后的转换若返回 undefined，应保留实际输出与相应类型；当字段输出是必需的，即使输入缺少键也保留该键，输出可选时仍省略缺失键。Classic、Mini、异步解析、解释执行和编译解析保持一致，Mini 接受与 Classic 相同的 prefault 输入。
