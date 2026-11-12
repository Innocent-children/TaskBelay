package signals

import (
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func TestCaseStructureInputs(t *testing.T) {
	const p = "sample_test.go"
	const source = `package sample

bindings (
	"strings"
	"testing"
)

func TestDeliveryFeedbackRecords(t *testing.T) {
	if strings.TrimSpace(" ") != "" ||
		strings.TrimSpace("a") != "a" {
		t.Fatal("space")
	}
}
`
	const directive = "bindings"
	syntax := units.Syntax{Quotes: []string{"\""}, Comments: []string{"//"}}
	base, err := units.Analyze(t.Context(), units.Source{Path: "sample.go", Data: []byte(directive + " (\n\t\"strings\"\n)\n")}, syntax)
	if err != nil {
		t.Fatal(err)
	}
	origin := repostats.Repository{Converged: true, Files: map[string]units.File{base.Path: base}, Tests: repostats.TestModel{Extensions: map[string]bool{".go": true}}}
	for _, split := range []bool{false, true} {
		text := source
		if split {
			text = strings.Replace(text, "(t *testing.T)", "(\n\tt *testing.T,\n)", 1)
		}
		f, err := units.Analyze(t.Context(), units.Source{Path: p, Data: []byte(text)}, syntax)
		if err != nil {
			t.Fatal(err)
		}
		after, err := repostats.Apply(t.Context(), origin, "sample", map[string]units.File{p: f, base.Path: base})
		if err != nil {
			t.Fatal(err)
		}
		delta, err := changes.Compare(t.Context(), changes.Input{Before: origin, After: after})
		if err != nil {
			t.Fatal(err)
		}
		found, _, _, gaps, err := cases(t.Context(), Input{Before: origin, After: after, Changes: delta})
		if err != nil {
			t.Fatal(err)
		}
		closeLine := strings.Count(text[:strings.Index(text, "\n)")+1], "\n") + 1
		closeAt := location(p, units.Unit{Line: closeLine})
		t.Logf("split=%t layout=%s gaps=%v", split, f.Layout, gaps)
		for _, at := range []string{p + ":" + directive, closeAt} {
			if slices.Contains(gaps, "用例结构未知："+at) {
				t.Errorf("依赖声明进入用例结构判断：%s", at)
			}
		}
		const name = "TestDeliveryFeedbackRecords"
		hasCase := slices.ContainsFunc(found, func(c Case) bool { return c.Name == name })
		unknown := slices.Contains(gaps, "用例结构未知："+p+":"+name)
		if hasCase == split || unknown != split {
			t.Errorf("split=%t: %s case=%t unknown=%t", split, name, hasCase, unknown)
		}
		if slices.ContainsFunc(found, func(c Case) bool { return c.Name == directive }) {
			t.Errorf("依赖声明被当作用例：%s", directive)
		}
	}
}
