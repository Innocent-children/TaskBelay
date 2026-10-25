通过 SetNormalizeFunc 设置别名后，长参数的弃用警告应显示用户实际输入的名称。例如 dir 被归一化到 src，输入 --dir=test 应提示 --dir 已弃用。直接调用 Set 传入别名也如此；短参数仍显示对应的标准长名称。保持 Set 和 ParseAll 的公开调用方式以及解析结果不变。
