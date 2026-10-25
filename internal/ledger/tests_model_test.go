package ledger

import (
	"fmt"
	"testing"
)

func TestTestRulesAlwaysRemainHintsAtDelivery(t *testing.T) {
	record := CheckRecord{Status: CheckComplete}
	for i := 1; i <= 6; i++ {
		record.Issues = append(record.Issues, Issue{Rule: fmt.Sprintf("T%d", i), Level: "block", BlockOnDelivery: true, Status: "open"})
	}
	result := WithDelivery(record, true)
	for _, issue := range result.Issues {
		if issue.Level != "hint" || issue.BlockOnDelivery {
			t.Fatal(issue)
		}
	}
	if record.Issues[0].Level != "block" {
		t.Fatal("caller record mutated")
	}
}
