为 Parser 和 Subcommand 派生的枚举支持 #[command(defer = true)]，延后未选中子命令的参数构建，同时保持正常解析、帮助、嵌套与 flatten 行为。v4 默认仍立即构建，unstable-v5 默认延后，defer = false 可以明确关闭。错误的属性应用要被拒绝，避免静默改变非枚举的行为。
