package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestDeliveryFeedbackRecords(t *testing.T) {
	hidden := ledger.Issue{ID: "i-ef54f0d5206be549dff7-1", At: "internal/check/timing.go", Status: "open", Advice: "先核对变更簇"}
	kept := ledger.Issue{ID: "kept", Status: "open", Selected: true, Advice: "保留当前内容"}
	report := ledger.CheckRecord{ID: 1, Depth: "static", Issues: []ledger.Issue{hidden, kept}}
	// 同一条 open 建议只改变是否选中，对照数量摘要和实际展示的正文。
	for _, wantShown := range []int{0, 1} {
		selected := wantShown == 1
		report.Issues[0].Selected = selected
		d := DeliveryFeedback(ledger.Delivery{Check: report}, nil)
		var details []string
		var shown int
		for _, r := range d.Records {
			details = append(details, r.Detail)
			if !strings.Contains(d.UserMessage, r.Detail) || !slices.Contains(r.Sources, "check:1") {
				t.Fatalf("记录与本次输出不符：%+v\n%s", r, d.UserMessage)
			}
			if slices.Contains(r.Sources, "issue:kept") {
				t.Fatalf("数量摘要被记录为具体建议：%+v", r)
			}
			if slices.Contains(r.Sources, "issue:"+hidden.ID) {
				shown++
				if r.Object != hidden.At || !strings.Contains(r.Detail, hidden.Advice) {
					t.Fatalf("记录无法定位建议正文：%+v", r)
				}
			}
		}
		logged := strings.Join(details, "\n")
		if !strings.Contains(logged, "2 条提示") || !strings.Contains(logged, "另有 1 条") ||
			strings.Contains(d.UserMessage, hidden.ID) != selected || shown != wantShown {
			t.Fatalf("selected=%t，具体记录 %d 条\n%s", selected, shown, logged)
		}
	}
}
