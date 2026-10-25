package platform

import (
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
)

func ExecutableName() string { return "taskbelay.exe" }

func CodexShell() event.Shell { return event.PowerShell }

func CodexHookCommand(executable, hookEvent string) string {
	return "& " + quote(executable) + " hook codex " + quote(hookEvent)
}

func quote(value string) string {
	// PowerShell 也把这些弯引号当作单引号定界符。
	replacer := strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛")
	return "'" + replacer.Replace(value) + "'"
}
