命令的位置参数中只要有一项提供了 help，帮助页的 Positional arguments 部分就列出全部位置参数，包括没有 help 的参数，并保持声明顺序。全部参数都没有 help 时继续省略该部分。默认值、环境变量、类型和多个参数的现有帮助格式应保持一致。

另外，Argument.get_help_record 无论是否提供 help，都返回由参数的 metavar 和帮助文本组成的二元组；没有帮助文本时使用空字符串，不再返回 None。
