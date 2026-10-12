package execution

import (
	"context"
	"errors"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestActualGoSubprocessAndDescendantsComplete(t *testing.T) {
	goPath, e := exec.LookPath("go")
	if e != nil {
		t.Skip("Go executable is not on test PATH")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module executionexample\ngo 1.26\n"), 0600)
	os.WriteFile(filepath.Join(root, "example_test.go"), []byte("package executionexample\nimport (\"testing\";\"os/exec\")\nfunc TestChild(t *testing.T){c:=exec.Command(\"git\",\"--version\");if e:=c.Run();e!=nil{t.Fatal(e)}}\n"), 0600)
	task := domain.Task{Repositories: []domain.RepositoryScopeEntry{{Key: "primary", Origin: domain.WorkspaceOrigin{CanonicalWorktreeRoot: root}}}}
	run := Run{Check: domain.Check{Kind: "command", Argv: []string{goPath, "test", "-v", "-timeout=20s"}, Cwd: ".", TimeoutMS: 30000}, Before: Fingerprint{Executable: goPath}, StartedAt: time.Now().UTC()}
	result := Perform(context.Background(), task, run, func(context.Context, Run) error { return nil })
	if result.ExitCode == nil || *result.ExitCode != 0 || result.Status == "timeout" {
		t.Fatalf("actual Go descendants did not complete: %+v", result)
	}
}

func TestCommandCannotExecuteBeforeItsProcessIdentityIsSaved(t *testing.T) {
	goPath, e := exec.LookPath("go")
	if e != nil {
		t.Skip("Go executable is not on test PATH")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "must-not-exist")
	script := filepath.Join(root, "main.go")
	if e = os.WriteFile(script, []byte("package main\nimport \"os\"\nfunc main(){os.WriteFile(os.Args[1], []byte(\"ran\"), 0600)}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	task := domain.Task{Repositories: []domain.RepositoryScopeEntry{{Key: "primary", Origin: domain.WorkspaceOrigin{CanonicalWorktreeRoot: root}}}}
	run := Run{Check: domain.Check{Kind: "command", Argv: []string{goPath, "run", script, marker}, Cwd: ".", TimeoutMS: 30000}, Before: Fingerprint{Executable: goPath}}
	called := false
	result := Perform(context.Background(), task, run, func(ctx context.Context, observed Run) error {
		called = true
		if observed.Process == nil || observed.Process.PID <= 0 || observed.Process.Identity == "" {
			t.Fatal("missing launch identity")
		}
		if _, e := os.Stat(marker); !os.IsNotExist(e) {
			t.Fatal("command ran before reservation")
		}
		return errors.New("injected persistence failure")
	})
	if !called || result.Status != "failed" {
		t.Fatal("launch failure not recorded", result)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("command ran after reservation failure")
	}
}
