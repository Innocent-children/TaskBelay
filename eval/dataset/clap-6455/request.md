公开 possible_value_candidates 和 complete_possible_values，使自定义 eager 或 lazy 补全器可以复用 PossibleValue 列表。结果保留值名称、帮助和隐藏标记，按输入前缀筛选并保持顺序；空前缀返回所有候选，无匹配返回空。内置可能值、自定义值和外部子命令的补全行为保持兼容。

两个函数从 clap_complete::engine 公开：possible_value_candidates(values) 和 complete_possible_values(current, values)。values 接受元素为 PossibleValue 的 IntoIterator，current 为 &OsStr，均返回 Vec<CompletionCandidate>；complete_possible_values 的当前输入在前、候选值在后。
