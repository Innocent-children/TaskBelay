解析 JSON 字符串遇到无效 UTF-8 时，InvalidUnicodeCodePoint 的行列位置应指向第一个无效字节，而不是字符串结束引号之后。字节切片和流式读取、跨行输入、截断的多字节序列与已有 raw_value 路径均保持一致，合法 UTF-8 和其他语法错误位置不变。
