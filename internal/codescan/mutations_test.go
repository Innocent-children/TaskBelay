package codescan

import (
	"testing"
)

func TestMutationsOnlyUseChangedCoveredBehaviorAndFiveKinds(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"range.go": `package sample
func Allowed(n int) bool { if n > 0 { return n + 1 > 2 }; return !false }
func Untouched(n int) bool { return n < 9 }
`})
	mutations, err := Mutations(t.Context(), snapshot.Files["range.go"], map[int]bool{2: true})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	seen := map[string]bool{}
	for _, mutation := range mutations {
		if mutation.Line != 2 || seen[mutation.ID] || mutation.Before == mutation.After {
			t.Fatal(mutation)
		}
		seen[mutation.ID] = true
		kinds[mutation.Kind] = true
	}
	for _, kind := range []string{"comparison", "negate_condition", "integer", "arithmetic", "remove_not"} {
		if !kinds[kind] {
			t.Fatal(kind, mutations)
		}
	}
}
