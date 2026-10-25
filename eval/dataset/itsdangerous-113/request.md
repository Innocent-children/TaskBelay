普通 Serializer 和 TimedSerializer 支持 fallback_signers：验签先用当前 signer，失败后依次尝试指定的后备 signer，签名仍使用当前 signer。后备项接受参数字典、signer 类或类与参数字典的二元组；保留 salt、时间戳返回和失效时间检查。默认 HMAC 摘要恢复为 SHA-1，以便旧签名仍能按用户配置迁移。
