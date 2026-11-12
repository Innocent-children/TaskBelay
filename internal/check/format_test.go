package check

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestDeliveryFeedbackRecords(t *testing.T) {
	hidden := ledger.Issue{ID: "i-ef54f0d5206be549dff7-1", At: "internal/check/timing.go", Status: "open", Advice: "先核对变更簇"}
	report := ledger.CheckRecord{ID: 1, Depth: "static", Issues: []ledger.Issue{
		hidden, {ID: "unselected", Status: "open", Advice: "核对需求位置"},
	}}
	for i := 0; i < 6; i++ {
		report.Issues = append(report.Issues, ledger.Issue{
			ID: fmt.Sprintf("kept-%d", i), Status: "open", Selected: true, Advice: "保留当前内容",
		})
	}
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
			if slices.Contains(r.Sources, "issue:"+hidden.ID) {
				shown++
				if r.Object != hidden.At || !strings.Contains(r.Detail, hidden.Advice) {
					t.Fatalf("记录无法定位建议正文：%+v", r)
				}
			} else if len(r.Sources) != 1 {
				t.Fatalf("摘要应只关联本次检查：%+v", r)
			}
		}
		logged := strings.Join(details, "\n")
		if !strings.Contains(logged, "共发现 8 条提示。") || !strings.Contains(logged, "另有 6 条") ||
			!strings.Contains(logged, fmt.Sprintf("本次列出 %d 条具体建议。", wantShown)) ||
			strings.Count(d.UserMessage, "[建议]") != wantShown ||
			strings.Contains(d.UserMessage, hidden.ID) != selected || shown != wantShown {
			t.Fatalf("selected=%t，具体记录 %d 条\n%s", selected, shown, logged)
		}
	}
}
