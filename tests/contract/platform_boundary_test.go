package contract_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCoreSemanticPackagesContainNoOperatingSystemDecision(t *testing.T) {
	root := contractRepositoryRoot(t)
	for _, directory := range []string{"internal/domain", "internal/application"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if regexp.MustCompile(`runtime\.GOOS|//go:build|golang\.org/x/sys/windows|syscall\.`).Match(raw) {
				t.Errorf("Core semantic source contains an operating-system decision: %s", filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestNodeConsumersUseClosedPlatformImplementations(t *testing.T) {
	root := contractRepositoryRoot(t)
	for _, relative := range []string{
		"packages/codex/lib/platform.mjs",
		"packages/deepseek/lib/platform.mjs",
		"packages/claude/lib/platform.mjs",
		"packages/zcode/lib/platform.mjs",
		"packages/taskbelay/lib/platform.mjs",
	} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, runtimeKey := range []string{"darwin-arm64", "win32-x64"} {
			if !strings.Contains(string(raw), `"`+runtimeKey+`"`) {
				t.Errorf("%s is missing %s", relative, runtimeKey)
			}
		}
	}

	// Command selection is an OS boundary, independent of Core runtime architecture.
	for _, relative := range []string{
		"packages/host-command/command.mjs",
		"packages/codex/lib/command.mjs",
		"packages/claude/lib/command.mjs",
		"packages/zcode/lib/command.mjs",
	} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, platform := range []string{"macos", "windows"} {
			if !strings.Contains(string(raw), `"./platform/`+platform+`/command.mjs"`) {
				t.Errorf("%s is missing its %s command implementation", relative, platform)
			}
		}
	}

	for _, relative := range []string{
		"packages/codex/lib/paths.mjs",
		"packages/codex/lib/install-experience.mjs",
		"packages/codex/lib/lifecycle.mjs",
		"packages/codex/bin/taskbelay-codex.mjs",
		"packages/codex/plugin/hooks/pre-tool-use.mjs",
		"packages/deepseek/lib/index.mjs",
		"packages/deepseek/lib/paths.mjs",
		"packages/deepseek/lib/runtime.mjs",
		"packages/deepseek/lib/provisioning-receipt.mjs",
		"packages/claude/lib/runtime.mjs",
		"packages/claude/lib/lifecycle.mjs",
		"packages/claude/lib/workspace.mjs",
		"packages/claude/bin/taskbelay-claude.mjs",
		"packages/claude/plugin/hooks/pre-tool-use.mjs",
		"packages/zcode/lib/runtime.mjs",
		"packages/zcode/lib/lifecycle.mjs",
		"packages/zcode/lib/workspace.mjs",
		"packages/zcode/bin/taskbelay-zcode.mjs",
		"packages/zcode/hooks/pre-tool-use.mjs",
		"packages/taskbelay/lib/configuration.mjs",
		"packages/taskbelay/lib/lifecycle.mjs",
		"packages/taskbelay/lib/ownership.mjs",
		"packages/taskbelay/lib/plan.mjs",
		"packages/taskbelay/lib/runtime.mjs",
	} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`platform\s*(?:===|!==)\s*["'](?:win32|darwin)["']`).Match(raw) {
			t.Errorf("%s branches on a concrete platform outside a platform implementation", relative)
		}
	}
}

func TestHostPlatformImplementationsRemainSeparate(t *testing.T) {
	root := contractRepositoryRoot(t)
	for _, surface := range []struct {
		directory    string
		requiredFile string
	}{
		{"packages/codex/lib/platform", "policies.mjs"},
		{"packages/deepseek/lib/platform", "policies.mjs"},
		{"packages/claude/lib/platform", "policies.mjs"},
		{"packages/zcode/lib/platform", "policies.mjs"},
		{"packages/taskbelay/lib/platform", "policies.mjs"},
		{"packages/host-command/platform", "command.mjs"},
	} {
		for _, platform := range []string{"windows", "macos"} {
			directory := filepath.Join(root, filepath.FromSlash(surface.directory), platform)
			if _, err := os.Stat(filepath.Join(directory, surface.requiredFile)); err != nil {
				t.Fatal(err)
			}
			err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() || filepath.Ext(path) != ".mjs" {
					return err
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				other := "macos"
				if platform == "macos" {
					other = "windows"
				}
				if strings.Contains(string(raw), "../"+other+"/") || strings.Contains(string(raw), "process.platform") {
					t.Errorf("platform implementation must not select or import the other platform: %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRuntimeBuildHasOneClosedTargetCatalog(t *testing.T) {
	root := contractRepositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "build-core-runtimes.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "export const CORE_RUNTIME_TARGETS") {
		t.Error("runtime builder does not expose its closed target catalog")
	}
	for _, value := range []string{`runtimeKey: "darwin-arm64"`, `runtimeKey: "win32-x64"`} {
		if strings.Count(source, value) != 1 {
			t.Errorf("runtime builder must contain exactly one %q", value)
		}
	}
	if !strings.Contains(source, `CGO_ENABLED: "0"`) {
		t.Error("runtime builder does not disable CGo")
	}
	for _, relative := range []string{"scripts/taskbelay-local.mjs"} {
		consumer, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(consumer), "buildCoreRuntimes") {
			t.Errorf("%s does not consume the shared runtime builder", relative)
		}
	}
}
