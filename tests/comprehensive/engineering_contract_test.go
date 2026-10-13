package comprehensive_test

import (
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/mcp"
	"testing"
)

func TestCurrentContractHasOneLifecycleAndNoClaimExecution(t *testing.T) {
	if len(mcp.Catalog) != 15 {
		t.Fatal("current tool surface differs")
	}
	for _, tool := range mcp.Catalog {
		if tool.Name == "taskbelay_submit_test" || tool.Name == "taskbelay_get_next_action" {
			t.Fatal("retired cursor retained")
		}
	}
	task := domain.Task{Status: "active", Authorization: domain.Authorization{Goals: []domain.Goal{{ID: "goal", Checks: []string{"check"}}}, Limits: domain.Limits{}}, Evidence: []domain.Evidence{{ID: "claim", CheckID: "check", Source: "claim", Status: "passed"}}}
	d := domain.Decide(task, []domain.Freshness{{EvidenceID: "claim", CheckID: "check", Valid: true}}, nil)
	if len(d.MissingChecks) != 1 || d.Ready {
		t.Fatal("claim became acceptance")
	}
}
func TestDirectGitMutationIsNotVerificationAuthority(t *testing.T) {
	for _, argv := range [][]string{{"git", "commit", "-m", "no"}, {"git", "-C", "repo", "reset", "--hard"}, {"git", "push"}} {
		if execution.CheckCommand(argv) == nil {
			t.Fatal("Git mutation granted", argv)
		}
	}
	if e := execution.CheckCommand([]string{"git", "--no-pager", "diff"}); e != nil {
		t.Fatal(e)
	}
}
