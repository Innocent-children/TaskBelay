//go:build unix

package platform

import (
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
)

func ExecutableName() string { return "taskbelay" }

func CodexShell() event.Shell { return event.POSIX }

func CodexHookCommand(executable, hookEvent string) string {
	return quote(executable) + " hook codex " + quote(hookEvent)
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
