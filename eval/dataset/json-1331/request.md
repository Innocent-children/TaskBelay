在 raw_value 功能下增加 unsafe RawValue::from_string_unchecked，将已知合法的 JSON String 转成 Box<RawValue>，发布构建不再重新解析。契约要求单个完整 JSON 值且无首尾空白；调试构建检查契约。保留原始 JSON 文本与嵌入序列化效果，已有安全 from_string 的验证行为不变。

调试构建违反契约时触发 panic，诊断消息包含 from_string_unchecked，以明确指出失败的公开入口。
