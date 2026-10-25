package codescan

import (
	"context"
	"testing"
)

func TestBehaviorLinesPreserveUnchangedRiskBetweenChanges(t *testing.T) {
	old := scanSources(t, map[string]string{"a.go": "package p\nfunc f(x int) int {\n x += 1\n keep()\n x += 2\n return x\n}\n"}).Declarations()["a.go:f"]
	current := scanSources(t, map[string]string{"a.go": "package p\nfunc f(x int) int {\n x += 3\n keep()\n x += 4\n return x\n}\n"}).Declarations()["a.go:f"]
	a, b, ok := BehaviorLines(t.Context(), old, current)
	if !ok || len(a) != 2 || len(b) != 2 || !a[3] || !a[5] || !b[3] || !b[5] {
		t.Fatal(a, b, ok)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, ok := BehaviorLines(ctx, old, current); ok {
		t.Fatal("canceled comparison marked complete")
	}
}
