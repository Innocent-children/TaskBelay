package hook

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func DebugInput(ctx context.Context, e event.Event, raw []byte) {
	if !ledger.DebugEnabled(ctx) {
		return
	}
	details := map[string]any{"input_bytes": len(raw), "input_sha256": fmt.Sprintf("%x", sha256.Sum256(raw))}
	if e.Command != nil {
		script := CommandScript(e)
		command := *e.Command
		command.Text = script.Display
		e.Command = &command
		details["command"] = script
		if e.Result != nil {
			result := *e.Result
			details["command_output_bytes"] = len(result.Output)
			result.Output = ""
			e.Result = &result
		}
		details["omitted"] = []string{"raw_input", "command_output", "environment_values"}
	} else {
		details["omitted"] = []string{"raw_input"}
	}
	details["event"] = e
	ledger.DebugRecord(ctx, "hook.input", details)
}
