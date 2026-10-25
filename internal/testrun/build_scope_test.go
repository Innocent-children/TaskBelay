package testrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

func TestNativeBuildScopesUseReverseModuleDependencies(t *testing.T) {
	tools := os.Getenv("TASKBELAY_TEST_BUILD_TOOLS")
	if tools == "" {
		t.Skip("需要独立 Java/Rust 构建工具环境")
	}
	for _, tool := range []string{"cargo", "mvn"} {
		t.Run(tool, func(t *testing.T) {
			dir := t.TempDir()
			put := func(path, data string) {
				t.Helper()
				path = filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := exec.CommandContext(t.Context(), "git", "init", "-q", dir).CombinedOutput(); err != nil {
				t.Fatal(err, string(out))
			}
			env := map[string]string{}
			changed := "core/src/lib.rs"
			if tool == "cargo" {
				put("Cargo.toml", "[workspace]\nmembers=[\"core\",\"bridge\",\"other\"]\nresolver=\"2\"\n")
				for _, name := range []string{"core", "bridge", "other"} {
					manifest := "[package]\nname=\"" + name + "\"\nversion=\"0.1.0\"\nedition=\"2021\"\n"
					if name == "bridge" {
						manifest += "[dependencies]\ncore={path=\"../core\"}\n"
					}
					put(name+"/Cargo.toml", manifest)
					put(name+"/src/lib.rs", "pub fn value()->i32{1}\n")
				}
				env["CARGO_HOME"] = filepath.Join(tools, "cargo")
				env["RUSTUP_HOME"] = filepath.Join(tools, "rustup")
				env["PATH"] = filepath.Join(tools, "cargo", "bin") + string(os.PathListSeparator) + os.Getenv("PATH")
			} else {
				changed = "core/src/main/java/Value.java"
				put("pom.xml", `<project><modelVersion>4.0.0</modelVersion><groupId>test.taskbelay</groupId><artifactId>parent</artifactId><version>1</version><packaging>pom</packaging><modules><module>core</module><module>bridge</module><module>other</module></modules><pluginRepositories><pluginRepository><id>alimaven</id><url>https://maven.aliyun.com/repository/public</url></pluginRepository></pluginRepositories></project>`)
				for _, name := range []string{"core", "bridge", "other"} {
					pom := `<project><modelVersion>4.0.0</modelVersion><parent><groupId>test.taskbelay</groupId><artifactId>parent</artifactId><version>1</version></parent><artifactId>` + name + `</artifactId>`
					if name == "bridge" {
						pom += `<dependencies><dependency><groupId>test.taskbelay</groupId><artifactId>core</artifactId><version>1</version></dependency></dependencies>`
					}
					put(name+"/pom.xml", pom+"</project>")
					put(name+"/src/main/java/Value.java", "class Value { int value(){return 1;} }\n")
				}
				javaHome := os.Getenv("TASKBELAY_TEST_JAVA_HOME")
				if javaHome == "" {
					t.Skip("需要明确原生验收 JDK")
				}
				env["JAVA_HOME"] = javaHome
			}
			workspace, err := gitstate.Discover(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			before, err := workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, changed))
			if err != nil {
				t.Fatal(err)
			}
			put(changed, strings.Replace(string(data), "1", "2", 1))
			after, err := workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			script := shell.Script{Segments: []shell.Segment{{Args: []string{tool, "test"}, Dir: workspace.Root, Kind: shell.Verification, Known: true, Environment: env}}}
			result, err := (Scope{OriginTree: before.Tree}).VerificationScope(t.Context(), workspace, script, after.Tree)
			if err != nil || result.Suggestion == "" || result.InputID == "" || result.EnvironmentKnown || !strings.Contains(result.Suggestion, "core") || !strings.Contains(result.Suggestion, "bridge") || strings.Contains(result.Suggestion, "other") {
				t.Fatal(result, err)
			}
			current, err := workspace.Capture(t.Context())
			if err != nil || current.Tree != after.Tree {
				t.Fatal("scope altered original", err)
			}
			if tool == "cargo" {
				put("Cargo.toml", "[workspace]\nmembers=[\"core\",\"bridge\",\"other\"]\nresolver=\"2\"\n# changed\n")
			} else {
				data, _ := os.ReadFile(filepath.Join(dir, "pom.xml"))
				put("pom.xml", string(data)+"\n")
			}
			current, err = workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			result, err = (Scope{OriginTree: before.Tree}).VerificationScope(t.Context(), workspace, script, current.Tree)
			if err != nil || result.Suggestion != "" || result.Complete {
				t.Fatal(result, err)
			}
		})
	}
}
