让 Parameter、Option 和 Argument 一致接受可选的 help，并在各自的 to_info_dict 中暴露整理后的 help。清理帮助文本的规则保持现有行为，Option 和 Argument 的命令行帮助输出不变；自定义 Parameter 子类 时也能获得同样的信息字典行为。
