package domain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBaselineHistoryRetainsEveryExactReference(t *testing.T) {
	var task ProcessTask
	var want []BaselineReference
	for revision := uint32(1); revision <= 50; revision++ {
		for _, kind := range []BaselineKind{BaselineRequirements, BaselineDesign, BaselineTaskPlan} {
			ref := BaselineReference{Kind: kind, Revision: revision, Digest: Digest(strings.Repeat("a", 64)), Summary: fmt.Sprintf("%s revision %d", kind, revision), CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}
			want = append(want, ref)
			if err := task.AppendBaselineHistory(ref); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := task.BaselineHistory; !reflect.DeepEqual(got, want) {
		t.Fatal("lost or rewritten reference")
	}
	if err := task.AppendBaselineHistory(want[0]); err != nil || !reflect.DeepEqual(task.BaselineHistory, want) {
		t.Fatal("exact duplicate was not idempotent")
	}

}

func TestBaselineHistoryRejectsGapsAndChangedReferences(t *testing.T) {
	ref := BaselineReference{Kind: BaselineRequirements, Revision: 1, Digest: Digest(strings.Repeat("a", 64)), Summary: "Original", CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}
	var task ProcessTask
	if err := task.AppendBaselineHistory(ref); err != nil {
		t.Fatal(err)
	}
	ref.Summary = "Rewritten"
	if err := task.AppendBaselineHistory(ref); err == nil {
		t.Fatal("rewritten revision accepted")
	}
	ref.Revision = 2
	empty := ProcessTask{}
	if err := empty.AppendBaselineHistory(ref); err == nil {
		t.Fatal("missing first revision accepted")
	}
}

func TestBaselineHistoryPaginationUsesActualBytesAndExactCursors(t *testing.T) {
	task := ProcessTask{TaskID: "task", Revision: 9}
	for i := 1; i <= 80; i++ {
		task.BaselineHistory = append(task.BaselineHistory, BaselineReference{Kind: BaselineRequirements, Revision: uint32(i), Digest: Digest(strings.Repeat("a", 64)), Summary: strings.Repeat("\"", MaxEvidenceSummaryBytes), CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)})
	}
	var got []BaselineReference
	var after uint64
	for {
		page, err := task.ReadBaselineHistory(BaselineHistoryQuery{Revision: 9, After: after, Limit: 32})
		if err != nil {
			t.Fatal(err)
		}
		size, err := compactJSONSize(page)
		if err != nil || size > MaxBaselineHistoryPageBytes || len(page.Entries) >= 32 {
			t.Fatalf("page bytes=%d entries=%d err=%v", size, len(page.Entries), err)
		}
		if page.Total != 80 || page.Revision != 9 {
			t.Fatal("wrong page metadata")
		}
		for _, entry := range page.Entries {
			if entry.Sequence != uint64(len(got)+1) {
				t.Fatal("cursor skipped/repeated a reference")
			}
			got = append(got, entry.Reference)
		}
		if page.NextAfter == nil {
			break
		}
		after = *page.NextAfter
	}
	if !reflect.DeepEqual(got, task.BaselineHistory) {
		t.Fatal("pagination changed saved references")
	}
	for _, q := range []BaselineHistoryQuery{{Revision: 8, Limit: 1}, {After: 1, Limit: 1}, {Revision: 9, After: 81, Limit: 1}, {Limit: 33}, {Limit: 0}} {
		if _, err := task.ReadBaselineHistory(q); err == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
	end, err := task.ReadBaselineHistory(BaselineHistoryQuery{Revision: 9, After: 80, Limit: 32})
	if err != nil || end.NextAfter != nil || len(end.Entries) != 0 {
		t.Fatal("end cursor is not final")
	}
}
