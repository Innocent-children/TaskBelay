//go:build unix

package platform

import (
	"runtime"
	"strings"
)

// ExeName 是 TaskBelay 程序的文件名。
const ExeName = "taskbelay"

// CodexPowerShell 表示 Codex 在这个平台上是否用 PowerShell 运行命令。
// macOS 和 Linux 上 Codex 用 bash 或 zsh。
const CodexPowerShell = false

// macOS 默认不区分大小写，Linux 区分。
var caseInsensitive = runtime.GOOS == "darwin"

// CodexCommand 写出 Codex 的 hook 命令：程序路径加单引号，后面跟参数。
// Codex 用 bash 或 zsh 的 -c 运行它。路径里的单引号这样写：先结束引号，
// 写一个反斜杠转义的单引号，再重新开始引号。
// 参数都是 TaskBelay 自己的子命令名，不需要加引号。
func CodexCommand(exe string, args ...string) string {
	quoted := "'" + strings.ReplaceAll(exe, "'", `'\''`) + "'"
	return strings.Join(append([]string{quoted}, args...), " ")
}
