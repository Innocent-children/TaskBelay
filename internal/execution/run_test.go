package execution

import (
	"context"
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
	result := Perform(context.Background(), task, run)
	if result.ExitCode == nil || *result.ExitCode != 0 || result.Status == "timeout" {
		t.Fatalf("actual Go descendants did not complete: %+v", result)
	}
}
