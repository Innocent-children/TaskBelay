//go:build windows

package platform

import "strings"

// ExeName 是 TaskBelay 程序的文件名。
const ExeName = "taskbelay.exe"

// CodexPowerShell 表示 Codex 在这个平台上是否用 PowerShell 运行命令。
const CodexPowerShell = true

// Windows 不区分大小写。
var caseInsensitive = true

// pwshQuotes 是 PowerShell 当作单引号的字符：除了 '，还有几个弯引号。
const pwshQuotes = "'‘’‚‛"

// CodexCommand 写出 Codex 的 hook 命令：& 加上带单引号的程序路径，后面跟参数。
// Codex 用 PowerShell 的 -Command 运行它；路径里的单引号写两遍。
// 参数都是 TaskBelay 自己的子命令名，不需要加引号。
func CodexCommand(exe string, args ...string) string {
	var b strings.Builder
	b.WriteString("& '")
	for _, r := range exe {
		if strings.ContainsRune(pwshQuotes, r) {
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteString("'")
	for _, a := range args {
		b.WriteString(" " + a)
	}
	return b.String()
}
