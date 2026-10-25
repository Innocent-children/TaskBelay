改进无效数据的异常来源展示。base64 解码、普通和 URL-safe 反序列化、JWS 头与载荷解析、签名过期时间转换失败时，保留现有异常类别和 original_error，同时通过明确的异常链保留底层原因。请求不支持的 JWS 算法时只展示 NotImplementedError，不把内部字典查找的 KeyError 作为可见原因。成功路径保持不变。
