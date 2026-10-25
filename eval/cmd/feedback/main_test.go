package main

import (
	"reflect"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
)

func TestOnlyRenderedFeedbackContributesToCounts(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		stop := measure(input{ID: 4, Agent: agent, Kind: event.Stop, Decision: &event.Decision{
			AdditionalContext: "internal context that Stop does not send", UserMessage: "中文😀abcd", BlockStop: "stop"}})
		want := []field{{"systemMessage", 14, 7, 4}, {"reason", 4, 4, 1}}
		if !reflect.DeepEqual(stop.Fields, want) || len(stop.Unknown) != 0 {
			t.Fatal(agent, stop)
		}
		pre := measure(input{Agent: agent, Kind: event.EditUse, Decision: &event.Decision{AdditionalContext: "read", DenyTool: "deny"}})
		if !reflect.DeepEqual(pre.Fields, []field{{"additionalContext", 4, 4, 1}, {"permissionDecisionReason", 4, 4, 1}}) {
			t.Fatal(pre)
		}
		for _, kind := range []event.Kind{event.SessionStart, event.UserPrompt, event.MCPUse, event.ToolEnd} {
			got := measure(input{Agent: agent, Kind: kind, Decision: &event.Decision{AdditionalContext: "中文"}})
			if !reflect.DeepEqual(got.Fields, []field{{"additionalContext", 6, 2, 2}}) || len(got.Unknown) != 0 {
				t.Fatal(kind, got)
			}
		}
	}
}

func TestEmptyOutputAndUnsupportedDecisionsAreDistinct(t *testing.T) {
	empty := measure(input{Agent: "claude", Kind: event.Stop, Decision: &event.Decision{AdditionalContext: "not sent"}})
	if len(empty.Fields) != 0 || len(empty.Unknown) != 0 {
		t.Fatal(empty)
	}
	for _, item := range []input{
		{Agent: "other", Kind: event.Stop, Decision: &event.Decision{}},
		{Agent: "codex", Kind: event.Stop},
		{Agent: "claude", Kind: "unknown", Decision: &event.Decision{}},
		{Agent: "claude", Kind: event.Stop, Decision: &event.Decision{DenyTool: "not valid at Stop"}},
	} {
		got := measure(item)
		if len(got.Unknown) == 0 || len(got.Fields) != 0 {
			t.Fatal(item, got)
		}
	}
}
