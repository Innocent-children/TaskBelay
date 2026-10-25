package check

import (
	"encoding/json"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

func TestV1RequiresEveryReuseCondition(t *testing.T) {
	base := ledger.VerificationRecord{Command: shell.Script{Display: "go test ./...", Segments: []shell.Segment{{Args: []string{"go", "test", "./..."}, Dir: "/repo", Kind: shell.Verification, Known: true}}}, Tree: "tree", EnvironmentID: "process-env", Scope: testrun.VerificationScope{InputID: "input", ToolID: "tool", EnvironmentID: "runner-env", Complete: true, EnvironmentKnown: true}, State: "passed", Executed: true}
	if got := Verification(base, &base); got.Level != "block" {
		t.Fatal(got)
	}
	cases := map[string]func(*ledger.VerificationRecord, *ledger.VerificationRecord){
		"failed":       func(c, p *ledger.VerificationRecord) { p.State = "failed" },
		"interrupted":  func(c, p *ledger.VerificationRecord) { p.State = "interrupted" },
		"unknown":      func(c, p *ledger.VerificationRecord) { p.State = "unknown" },
		"running":      func(c, p *ledger.VerificationRecord) { p.State = "running" },
		"not-executed": func(c, p *ledger.VerificationRecord) { p.Executed = false },
		"mixed":        func(c, p *ledger.VerificationRecord) { c.Command.Complex = true },
		"fix": func(c, p *ledger.VerificationRecord) {
			c.Command.Segments[0].Args = append(c.Command.Segments[0].Args, "--fix")
		},
		"background":             func(c, p *ledger.VerificationRecord) { c.Command.Segments[0].Background = true },
		"write":                  func(c, p *ledger.VerificationRecord) { c.Scope.SideEffects = true },
		"tree":                   func(c, p *ledger.VerificationRecord) { c.Tree = "changed" },
		"input":                  func(c, p *ledger.VerificationRecord) { c.Scope.InputID = "changed" },
		"tool":                   func(c, p *ledger.VerificationRecord) { c.Scope.ToolID = "changed" },
		"environment":            func(c, p *ledger.VerificationRecord) { c.EnvironmentID = "changed" },
		"external-environment":   func(c, p *ledger.VerificationRecord) { c.Scope.EnvironmentID = "changed" },
		"unverified-environment": func(c, p *ledger.VerificationRecord) { c.Scope.EnvironmentKnown = false },
		"previous-incomplete":    func(c, p *ledger.VerificationRecord) { p.Scope.Complete = false },
		"unstable":               func(c, p *ledger.VerificationRecord) { c.Scope.Unstable = true },
	}
	data, _ := json.Marshal(base)
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var current, previous ledger.VerificationRecord
			if err := json.Unmarshal(data, &current); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &previous); err != nil {
				t.Fatal(err)
			}
			change(&current, &previous)
			if got := Verification(current, &previous); got.Level != "hint" {
				t.Fatal("incomplete reuse condition blocked", got)
			}
		})
	}
}
