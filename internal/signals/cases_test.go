package signals

import (
	"slices"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func TestAssertionExpressionsMockOnly(t *testing.T) {
	// 调用名按原词匹配，实参按忽略大小写后的最高分匹配。
	repo := repostats.Repository{TestWords: map[string]float64{
		"Join": 8.216611620893918, "Details": 1.041720939258754, "n": 2.8491173643576744,
		"Record": 1.3162575742630815, "Expected": 0.7586256153435174,
		"Check": 0.6167931515037973, "T": 15.969731949116369,
	}}
	for _, tc := range []struct {
		name, body, wantCheck string
		wantMock              bool
	}{
		{"TestDeliveryFeedbackRecords", `d := DeliveryFeedback(ledger.Delivery{Check: report}, nil); logged := strings.Join(details, "\n")`, `Join(details , "\n")`, false},
		{"TestRecordedCall", `calls.Record("expected"); calls.Check(t)`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := units.Analyze(t.Context(), units.Source{Data: []byte(tc.body)}, units.Syntax{Quotes: []string{`"`}})
			if err != nil {
				t.Fatal(err)
			}
			checks, mock, _ := assertionExpressions(repo, tc.name, file.Tokens)
			if tc.wantCheck != "" && !slices.Contains(checks, tc.wantCheck) {
				t.Errorf("checks = %q, want to include %q", checks, tc.wantCheck)
			}
			if (mock != "") != tc.wantMock {
				t.Errorf("mock = %q, want example present: %t", mock, tc.wantMock)
			}
		})
	}
}
