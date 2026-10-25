package lang

type CallRule struct {
	Target string
	Kind   string
	Arg    int
}

/**
 * 表中的限定名来自导入或已知接收者类型；单凭末段方法名不确认外部入口。
 * 空项表示没有语言专用规则，由通用代码分析决定相应能力。
 */
type Rules struct {
	AbstractBases, StructuralBases, AbstractDecorators             []string
	ErrorBases, ExternalFieldBases, ConfigBases, IgnoredDecorators []string
	SimpleCalls                                                    []string
	BranchNodes                                                    []string
	BranchOperators                                                []string
	ImportNames                                                    map[string]string
	Entries                                                        []CallRule
	Risks                                                          []CallRule
	Constructors                                                   map[string]string
	ReceiverMethods                                                map[string]string
	FlagTypes                                                      []string
	CommandTypes                                                   []string
	ConfigTags                                                     []string
	ConcurrentNodes                                                []string
	ConcurrentImports                                              []string
	ImplicitNames, ImplicitMethods, ImplicitDirectives             []string
	DeprecatedMarkers, ErrorConstructors, ErrorChecks              []string
}
