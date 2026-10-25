// Command feedback measures recorded hook text using the product adapters.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/summary"
)

type input struct {
	ID       int64           `json:"id"`
	Agent    string          `json:"agent"`
	Kind     event.Kind      `json:"kind"`
	Decision *event.Decision `json:"decision"`
}

type field struct {
	Name       string `json:"name"`
	Bytes      int    `json:"utf8_bytes"`
	Characters int    `json:"characters"`
	Tokens     int    `json:"estimated_tokens"`
}

type result struct {
	ID      int64    `json:"id"`
	Fields  []field  `json:"fields"`
	Unknown []string `json:"unknown"`
}

func measure(item input) result {
	out := result{ID: item.ID, Fields: []field{}, Unknown: []string{}}
	registry := agent.Registry{"claude": claude.Adapter{}, "codex": codex.Adapter{}}
	adapter, err := registry.Find(item.Agent)
	if err != nil || item.Decision == nil {
		out.Unknown = append(out.Unknown, "unsupported Agent or missing recorded decision")
		return out
	}
	hooks := map[event.Kind]string{event.SessionStart: "session-start", event.UserPrompt: "user-prompt-submit",
		event.CommandUse: "pre-tool-use", event.EditUse: "pre-tool-use", event.MCPUse: "pre-tool-use",
		event.ToolEnd: "post-tool-use", event.Stop: "stop"}
	encoded, err := adapter.RenderDecision(hooks[item.Kind], *item.Decision)
	if err != nil {
		out.Unknown = append(out.Unknown, err.Error())
		return out
	}
	if len(encoded) == 0 {
		return out
	}
	var rendered struct {
		SystemMessage      string `json:"systemMessage"`
		Reason             string `json:"reason"`
		HookSpecificOutput struct {
			AdditionalContext        string `json:"additionalContext"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(encoded, &rendered); err != nil {
		out.Unknown = append(out.Unknown, err.Error())
		return out
	}
	for _, text := range []struct{ name, content string }{
		{"systemMessage", rendered.SystemMessage}, {"reason", rendered.Reason},
		{"additionalContext", rendered.HookSpecificOutput.AdditionalContext},
		{"permissionDecisionReason", rendered.HookSpecificOutput.PermissionDecisionReason},
	} {
		if text.content != "" {
			out.Fields = append(out.Fields, field{text.name, len(text.content), utf8.RuneCountInString(text.content), summary.Tokens(text.content)})
		}
	}
	return out
}

func main() {
	var inputs []input
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inputs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	outputs := make([]result, 0, len(inputs))
	for _, item := range inputs {
		outputs = append(outputs, measure(item))
	}
	if err := json.NewEncoder(os.Stdout).Encode(outputs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
