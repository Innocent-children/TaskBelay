转换 JSON Schema 时保留连续 value、values、notValue 和 notValues 的全部限制。允许值集合取交集并去重，不相交时表示不可能匹配；排除值合并且保留既有排除。重复相同常量继续成功，不同常量冲突按配置的错误模式报告。兼容已有 const、enum、not 和 OpenAPI 3.0 的表示，不能通过覆盖旧限制放宽校验。
