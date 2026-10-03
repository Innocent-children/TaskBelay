package mcp

import (
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"reflect"
	"strings"
	"testing"
	"time"
)

func historyProjectionTask() domain.ProcessTask {
	task := domain.ProcessTask{TaskID: "task", Revision: 7, CurrentNode: domain.NodeRequirements}
	for i := 1; i <= 600; i++ {
		task.BaselineHistory = append(task.BaselineHistory, domain.BaselineReference{Kind: domain.BaselineRequirements, Revision: uint32(i), Digest: domain.Digest(strings.Repeat("a", 64)), Summary: strings.Repeat("\"", domain.MaxEvidenceSummaryBytes), CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)})
	}
	return task
}

func TestBaselineHistoryProjectionMetadataAndCompletePages(t *testing.T) {
	task := historyProjectionTask()
	projected := projectTask(task).(map[string]any)
	baselines := projected["baselines"].(map[string]any)
	first := baselines["history"].([]domain.BaselineReference)
	if baselines["history_total"] != uint64(600) || baselines["history_revision"] != uint64(7) || len(first) >= 32 || !reflect.DeepEqual(first, task.BaselineHistory[:len(first)]) {
		t.Fatal("first page or explicit total/revision is incorrect")
	}
	after := *baselines["history_next_after"].(*uint64)
	if after != uint64(len(first)) {
		t.Fatal("first page cursor incorrect")
	}
	got := append([]domain.BaselineReference(nil), first...)
	for {
		page, err := task.ReadBaselineHistory(domain.BaselineHistoryQuery{Revision: 7, After: after, Limit: 32})
		if err != nil {
			t.Fatal(err)
		}
		encoded := EncodeSuccess("request", ToolGetTask, map[string]any{"task": projectTask(task), "recovery_assessment": nil, "baseline_history": &page})
		if encoded.IsError || !WithinResultEnvelopeLimit(encoded.JSON) {
			t.Fatalf("page response: %s", encoded.JSON)
		}
		validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(ToolGetTask)), encoded.JSON)
		for _, entry := range page.Entries {
			if entry.Sequence != uint64(len(got)+1) {
				t.Fatal("gap/duplicate")
			}
			got = append(got, entry.Reference)
		}
		if page.NextAfter == nil {
			break
		}
		after = *page.NextAfter
	}
	if !reflect.DeepEqual(got, task.BaselineHistory) {
		t.Fatal("paged response lost a saved reference")
	}
	empty := projectTask(domain.ProcessTask{TaskID: "empty", Revision: 1}).(map[string]any)["baselines"].(map[string]any)
	if empty["history_total"] != uint64(0) || empty["history_next_after"].(*uint64) != nil {
		t.Fatal("empty history incorrectly has more pages")
	}
}

func TestBaselineHistoryEnvelopeShrinksWithoutChangingStoredHistory(t *testing.T) {
	task := historyProjectionTask()
	for _, wrapped := range []bool{false, true} {
		projected := projectTask(task).(map[string]any)
		baselines := projected["baselines"].(map[string]any)
		original := len(baselines["history"].([]domain.BaselineReference))
		// A transport-size fixture isolates envelope behavior from domain field limits.
		// With no inline references the envelope fits by one byte; every reference must
		// move to the explicit pager without falsely marking this as the end.
		history := baselines["history"]
		baselines["history"] = []domain.BaselineReference{}
		cursor := uint64(0)
		baselines["history_next_after"] = &cursor
		projected["intent"] = map[string]any{"request": ""}
		var result any = projected
		tool := ToolSubmitRequirements
		if wrapped {
			result = map[string]any{"task": projected, "recovery_assessment": nil}
			tool = ToolGetTask
		}
		raw, err := encodeEnvelope(Envelope{OK: true, RequestID: "request", Tool: tool, Result: result})
		if err != nil {
			t.Fatal(err)
		}
		projected["intent"] = map[string]any{"request": strings.Repeat("x", domain.MaxResultEnvelopeBytes-len(raw)-1)}
		baselines["history"] = history
		cursor = uint64(original)
		encoded := EncodeSuccess("request", tool, result)
		if encoded.IsError || !WithinResultEnvelopeLimit(encoded.JSON) {
			t.Fatalf("shrink failed: %s", encoded.JSON)
		}
		validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(tool)), encoded.JSON)
		if len(baselines["history"].([]domain.BaselineReference)) != 0 || baselines["history_next_after"] != uint64(0) || baselines["history_total"] != uint64(600) || len(task.BaselineHistory) != 600 {
			t.Fatal("shrinking changed retention or cursor")
		}
		projected["intent"] = map[string]any{"request": strings.Repeat("x", domain.MaxResultEnvelopeBytes)}
		if output := EncodeSuccess("request", tool, result); !output.IsError {
			t.Fatal("oversized non-history response accepted")
		}
	}
	// Requested pages retain at least one entry and resume after the last sent one.
	page, err := task.ReadBaselineHistory(domain.BaselineHistoryQuery{Revision: 7, After: 32, Limit: 32})
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"baseline_history": &page}
	for shrinkResultHistoryPage(value) {
	}
	if len(page.Entries) != 1 || *page.NextAfter != 33 || page.Total != 600 {
		t.Fatal("explicit page shrink skipped references")
	}
	raw, _ := json.Marshal(page)
	validateSkillSchema(t, mustSchemaJSON(t, outputBaselineHistorySchema()), raw)
}
