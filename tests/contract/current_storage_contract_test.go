package contract_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestCurrentStorageProductionHasOneCurrentTaskRuntime(t *testing.T) {
	root := currentStorageRepositoryRoot(t)
	roots := []string{
		"internal",
		"cmd",
		"packages/codex/lib",
		"packages/codex/bin",
		"packages/codex/plugin",
		"scripts",
	}
	files := currentStorageProductionFiles(t, root, roots)

	forbiddenText := []*regexp.Regexp{
		regexp.MustCompile(`(?i)legacy[-_]linear`),
		regexp.MustCompile(`\b(?:LegacyTask|LegacyProcess|continueLegacyTask|resumeLegacyTask)\b`),
		regexp.MustCompile(`(?i)\b(?:codec_v1|decodeTaskV1|encodeTaskV1)\b`),
		regexp.MustCompile(`(?i)\bALTER\s+TABLE\b`),
		regexp.MustCompile(`(?i)\b(?:import|export)(?:Legacy|Historical|Old)?Task\b`),
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range forbiddenText {
			if pattern.Match(raw) {
				t.Errorf("production source %s contains forbidden historical runtime pattern %s", filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), pattern)
			}
		}
	}

	forbiddenGoSymbols := map[string]bool{
		"Task": true, "Contract": true, "Outcome": true, "Action": true, "Blocker": true,
		"Phase": true, "ActionResult": true, "NewContract": true,
		"decodeTaskV1": true, "encodeTaskV1": true, "persistedTaskV1": true,
	}
	for _, path := range files {
		if filepath.Ext(path) != ".go" {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.TypeSpec:
				if forbiddenGoSymbols[value.Name.Name] {
					t.Errorf("production source %s declares forbidden historical type %s", filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), value.Name.Name)
				}
			case *ast.FuncDecl:
				if forbiddenGoSymbols[value.Name.Name] {
					t.Errorf("production source %s declares forbidden historical function %s", filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), value.Name.Name)
				}
			}
			return true
		})
	}

	t.Logf("scanned %d production files under %s", len(files), strings.Join(roots, ", "))
}

func TestCurrentStorageHasOneSchemaCodecProcessAndProjection(t *testing.T) {
	root := currentStorageRepositoryRoot(t)
	read := func(path string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	schema := read("internal/store/schema.go")
	for _, required := range []string{
		`const DatabaseSchemaVersion = "0.8.0"`,
		"currentSchemaStatements",
		"func bootstrapCurrentSchema",
		"func verifyCurrentSchema",
		"CREATE TABLE action_operations",
		"CREATE TABLE relocation_operations",
		"CREATE TABLE branch_rename_operations",
		"issuance_identity_digest TEXT NOT NULL",
		"repository_delta_paths BLOB NOT NULL",
		"worktree_instance_digest TEXT PRIMARY KEY",
		"CREATE INDEX repository_claims_task_idx ON repository_claims (task_id)",
		"CREATE UNIQUE INDEX relocation_operations_unresolved_task_idx ON relocation_operations (task_id) WHERE resolved_revision IS NULL",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("current storage bootstrap missing %q", required)
		}
	}
	if strings.Contains(schema, "task_id TEXT NOT NULL UNIQUE") {
		t.Fatal("repository claims still restrict one claim per task")
	}
	if regexp.MustCompile(`(?i)ALTER\s+TABLE|schema_migrations|process_version|snapshot_version`).MatchString(schema) {
		t.Fatal("current storage bootstrap contains version metadata or a migration path")
	}

	codec := read("internal/store/codec.go")
	for _, required := range []string{
		"type persistedTask domain.ProcessTask",
		"func encodeTask(task domain.ProcessTask)",
		"func decodeTask(raw []byte) (domain.ProcessTask, error)",
	} {
		if strings.Count(codec, required) != 1 {
			t.Errorf("strict codec must contain exactly one %q", required)
		}
	}
	if strings.Count(codec, "workflow.ValidateProcessTask(task)") != 2 {
		t.Error("strict codec must validate the ProcessTask on both encode and decode")
	}
	if regexp.MustCompile(`(?i)(snapshot.*switch|switch.*snapshot|persistedTaskV[0-9]|decodeTaskV|encodeTaskV)`).MatchString(codec) {
		t.Fatal("task codec contains a version-selected branch")
	}

	process := read("internal/workflow/standard_process.go")
	if strings.Count(process, "func StandardProcess() domain.ProcessDefinition") != 1 ||
		!strings.Contains(process, "domain.ProcessStandardDevelopment") {
		t.Fatal("standard-development is not the single code-owned process entrypoint")
	}
	projection := read("internal/mcp/results.go")
	if strings.Count(projection, "func projectTask(t domain.ProcessTask) any") != 1 || strings.Contains(projection, "domain.Task") {
		t.Fatal("MCP must expose one ProcessTask projection and no historical Task projection")
	}
	applicationTypes := read("internal/application/types.go")
	if strings.Contains(applicationTypes, "domain.Task") || !strings.Contains(applicationTypes, "domain.ProcessTask") {
		t.Fatal("Application must use only the ProcessTask projection")
	}
	domainTask := read("internal/domain/task.go")
	if strings.Contains(domainTask, "ActionCommit") {
		t.Fatal("ProcessTask must not embed the recoverable Action payload")
	}
	storeContract := read("internal/store/store.go")
	if !strings.Contains(storeContract, "type ActionOperationStore interface") ||
		!strings.Contains(storeContract, "CommitActionOperation(context.Context, domain.ID, TaskMutation) error") {
		t.Fatal("Store must expose the independent Action operation commit boundary")
	}
}

func TestCurrentStorageLifecycleHasNoTaskDataResetCapability(t *testing.T) {
	root := currentStorageRepositoryRoot(t)
	for _, relative := range []string{
		"internal/store/reset.go",
		"internal/store/reset_remove_darwin.go",
		"internal/store/reset_remove_windows.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("Core reset implementation remains at %s", relative)
		}
	}
	for _, relative := range []string{"cmd/taskbelay/main.go", "internal/webui/types.go"} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`webui reset|runWebUIReset|reset_required|ReadinessResetRequired`).Match(raw) {
			t.Fatalf("%s retains the removed Core reset surface", relative)
		}
	}
	lifecyclePaths := []string{
		"packages/codex/lib/lifecycle.mjs",
		"packages/codex/lib/paths.mjs",
		"packages/codex/bin/taskbelay-codex.mjs",
	}
	for _, relative := range lifecyclePaths {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []*regexp.Regexp{
			regexp.MustCompile(`\b(?:rm|rmdir|truncate)\s*\(`),
			regexp.MustCompile(`(?i)\b(?:DELETE|DROP)\s+(?:FROM\s+)?(?:tasks|task_events|action_operations|repository_claims|schema_migrations)\b`),
			regexp.MustCompile(`(?i)\b(?:migrate|convert|reset)(?:Task|Database|DataDirectory)\b`),
		} {
			if forbidden.MatchString(source) {
				t.Errorf("%s contains task-data lifecycle capability %s", relative, forbidden)
			}
		}
	}
	lifecycle, err := os.ReadFile(filepath.Join(root, "packages/codex/lib/lifecycle.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(lifecycle), "unlink(paths.receiptPath)") != 1 {
		t.Fatal("remove may unlink only the exact package-managed registration receipt")
	}
	if strings.Count(string(lifecycle), "rename(temporaryPath, receiptPath)") != 1 {
		t.Fatal("lifecycle may rename only the atomic package-managed receipt temporary file")
	}
}

func currentStorageProductionFiles(t *testing.T, root string, roots []string) []string {
	t.Helper()
	extensions := map[string]bool{".go": true, ".mjs": true, ".js": true, ".json": true, ".md": true, ".sh": true}
	var files []string
	for _, relativeRoot := range roots {
		base := filepath.Join(root, filepath.FromSlash(relativeRoot))
		count := 0
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" || entry.Name() == "testdata" || entry.Name() == "fixtures" {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.mjs") || !extensions[filepath.Ext(name)] {
				return nil
			}
			files = append(files, path)
			count++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("production scan root %s is empty", relativeRoot)
		}
	}
	sort.Strings(files)
	return files
}

func currentStorageRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
