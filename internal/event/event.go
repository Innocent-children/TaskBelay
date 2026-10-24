// Package event 定义 TaskBelay 关心的事件，以及处理之后给出的决定。
// Agent 适配器把各自的 hook 输入转成这里的事件，再把决定转成各自的输出；
// 其余代码只和这里的事件、决定打交道。
package event

// Kind 是事件的种类。
type Kind int

const (
	SessionStart   Kind = iota + 1 // session 开始
	UserPrompt                     // 用户消息
	CommandRequest                 // 命令请求：shell 工具运行命令之前
	EditRequest                    // 编辑请求：编辑工具改文件之前
	CommandDone                    // 命令结束
	Stop                           // 准备结束
)

// Syntax 是命令的写法。
type Syntax int

const (
	POSIX      Syntax = iota + 1 // bash、zsh、Git Bash
	PowerShell                   // PowerShell
)

// Result 是命令的结果，取值和账本里验证记录的结果一致。
type Result string

const (
	Pass    Result = "pass"    // 通过
	Fail    Result = "fail"    // 失败
	Unknown Result = "unknown" // 拿不到结果
)

// Event 是一次 hook 调用对应的事件。前五个字段每个事件都有，其余字段只属于
// 注释里写明的那种事件。
type Event struct {
	Kind    Kind
	Agent   string // 适配器的名字：claude、codex
	Session string // session 编号
	Turn    string // 轮次；session 开始时还没有轮次，为空
	Dir     string // 工作目录

	Source    string   // session 开始：startup、resume、clear、compact、fork
	Prompt    string   // 用户消息：原文
	Command   string   // 命令请求：命令文本
	Syntax    Syntax   // 命令请求：命令写法
	ToolUseID string   // 命令请求、命令结束：工具调用编号
	Files     []string // 编辑请求：要改的文件，写法和 Agent 给出的一致
	Result    Result   // 命令结束：结果
	ExitCode  *int     // 命令结束：退出码，拿不到时为 nil
}

// Decision 是处理一个事件之后的决定。几种决定可以同时给出，全部为空表示不做什么。
type Decision struct {
	Context string // 给 Agent 补充的上下文
	Deny    string // 拒绝这次工具调用，内容是给 Agent 的说明
	Block   string // 阻止结束，内容是给 Agent 的说明
	Notice  string // 给用户看的提示
}

// Empty 表示这个决定是“不做什么”。
func (d Decision) Empty() bool {
	return d == Decision{}
}
