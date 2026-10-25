package codescan

import "testing"

func TestGoBindingScopesKeepAvailabilityShadowingAndGlobalAmbiguity(t *testing.T) {
	global := &goBinding{name: "n"}
	g := &goFile{globals: map[string][]*goBinding{"n": {global}, "ambiguous": {{}, {}}}}
	outer := &goBinding{name: "n", scope: Span{Start: 10, End: 100}, available: 10}
	inner := &goBinding{name: "n", scope: Span{Start: 20, End: 40}, available: 25}
	later := &goBinding{name: "n", scope: outer.scope, available: 60}
	sibling := &goBinding{name: "n", scope: Span{Start: 110, End: 150}, available: 110}
	// 先加入内层和后面的函数，再加入外层，覆盖非单调的遍历插入顺序。
	for _, binding := range []*goBinding{inner, sibling, outer, later} {
		g.addBinding(binding)
	}
	for _, test := range []struct {
		at   uint32
		want *goBinding
	}{{0, global}, {10, outer}, {24, outer}, {25, inner}, {39, inner}, {40, outer},
		{59, outer}, {60, later}, {99, later}, {100, global}, {110, sibling}, {149, sibling}, {150, global}} {
		if got, exists := g.lookup("n", test.at); !exists || got != test.want {
			t.Errorf("at %d: got %+v, exists %v; want %+v", test.at, got, exists, test.want)
		}
	}
	if got, exists := g.lookup("ambiguous", 50); got != nil || !exists {
		t.Fatal("ambiguous global became a unique or missing binding", got, exists)
	}
	if got, exists := g.lookup("missing", 50); got != nil || exists {
		t.Fatal("missing name became a binding", got, exists)
	}
}

func TestGoBindingIndexDoesNotLeakAcrossDisjointFunctions(t *testing.T) {
	g := &goFile{}
	var expected []*goBinding
	for i := uint32(0); i < 2500; i++ {
		binding := &goBinding{name: "n", scope: Span{Start: i * 100, End: i*100 + 80}, available: i * 100}
		g.addBinding(binding)
		expected = append(expected, binding)
	}
	for _, binding := range expected {
		if got, exists := g.lookup("n", binding.scope.Start+40); !exists || got != binding {
			t.Fatal("another function's parameter selected", binding.scope, got)
		}
		if got, exists := g.lookup("n", binding.scope.End); got != nil || exists {
			t.Fatal("parameter escaped its function", binding.scope, got)
		}
	}
}
