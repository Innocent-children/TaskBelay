package eval_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestBuildDynamicLedgerFlowKeepsBoundaryAndOriginalWorkspace(t *testing.T) {
	tools := os.Getenv("TASKBELAY_TEST_BUILD_TOOLS")
	javaHome := os.Getenv("TASKBELAY_TEST_JAVA_HOME")
	if tools == "" || javaHome == "" {
		t.Skip("需要独立 Java、Rust 原生环境")
	}
	t.Setenv("JAVA_HOME", javaHome)
	t.Setenv("CARGO_HOME", filepath.Join(tools, "cargo"))
	t.Setenv("RUSTUP_HOME", filepath.Join(tools, "rustup"))
	t.Setenv("PATH", filepath.Join(tools, "cargo", "bin")+string(os.PathListSeparator)+filepath.Join(javaHome, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tool := range []string{"cargo", "mvn", "gradle"} {
		t.Run(tool, func(t *testing.T) {
			service, task, message, _ := deliveryFixture(t)
			files := map[string]string{}
			language, location, command := "rust", "src/lib.rs:allowed", "cargo test"
			if tool == "cargo" {
				files["Cargo.toml"] = "[package]\nname=\"taskbelay-build-flow\"\nversion=\"0.1.0\"\nedition=\"2021\"\n"
				files["src/lib.rs"] = `pub fn allowed(n:i32)->bool { n < 5 }
pub fn other(n:i32)->bool { n > 0 }
#[cfg(test)] mod tests {
 #[test] fn inside(){assert!(super::allowed(1));}
 #[test] fn same(){assert!(super::allowed(2));}
 #[test] fn edge(){assert!(!super::allowed(5));}
 #[test] fn unrelated(){assert!(super::other(1));}
}
`
			} else {
				language, location, command = "java", "src/main/java/example/Limit.java:Limit.allowed", "mvn test"
				files["src/main/java/example/Limit.java"] = "package example;\npublic class Limit {\n public static boolean allowed(int n){return n < 5;}\n public static boolean other(int n){return n > 0;}\n}\n"
				files["src/test/java/example/LimitTest.java"] = `package example;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;
class LimitTest {
 @Test void inside(){assertTrue(Limit.allowed(1));}
 @Test void same(){assertTrue(Limit.allowed(2));}
 @Test void edge(){assertFalse(Limit.allowed(5));}
 @Test void unrelated(){assertTrue(Limit.other(1));}
}`
				if tool == "mvn" {
					files["pom.xml"] = `<project><modelVersion>4.0.0</modelVersion><groupId>test.taskbelay</groupId><artifactId>build-flow</artifactId><version>1</version><properties><maven.compiler.release>21</maven.compiler.release></properties><repositories><repository><id>alimaven</id><url>https://maven.aliyun.com/repository/public</url></repository></repositories><pluginRepositories><pluginRepository><id>alimaven</id><url>https://maven.aliyun.com/repository/public</url></pluginRepository></pluginRepositories><dependencies><dependency><groupId>org.junit.jupiter</groupId><artifactId>junit-jupiter</artifactId><version>5.10.2</version><scope>test</scope></dependency></dependencies><build><plugins><plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-resources-plugin</artifactId><version>3.4.0</version></plugin><plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-compiler-plugin</artifactId><version>3.14.1</version></plugin><plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-surefire-plugin</artifactId><version>3.5.5</version></plugin></plugins></build></project>`
				} else {
					home, err := os.UserHomeDir()
					if err != nil {
						t.Fatal(err)
					}
					files["settings.gradle"] = "rootProject.name='build-flow'\n"
					files["build.gradle"] = "plugins { id 'java' }\nrepositories { maven { url = uri('" + filepath.ToSlash(filepath.Join(home, ".m2", "repository")) + "') } }\ndependencies { testImplementation 'org.junit.jupiter:junit-jupiter:5.10.2'; testRuntimeOnly 'org.junit.platform:junit-platform-launcher:1.10.2' }\ntest { useJUnitPlatform() }\n"
					path, err := os.ReadFile(filepath.Join(tools, "gradle-path"))
					if err != nil {
						t.Fatal(err)
					}
					command = "'" + strings.TrimSpace(string(path)) + "' test"
				}
			}
			for path, data := range files {
				verifiedTaskEdit(t, service, task.ID, "edit-"+path, path, "", data)
			}
			if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, "test."+language+".command", command); err != nil {
				t.Fatal(err)
			}
			positions := []ledger.Location{{At: location, Purpose: "范围边界"}}
			saveRequirement(t, service, taskActor("req"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "范围边界得到验证", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &positions})
			before, err := service.Workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Check(t.Context(), taskActor("dynamic"), check.RunInput{Depth: "tests"})
			if err != nil || result.Status != ledger.CheckComplete || result.Report.Dynamic == nil {
				t.Fatal(result.Status, result.Missing, err)
			}
			report := result.Report.Dynamic
			if !report.Complete || len(report.Observations) != 4 || len(report.Groups) != 1 || report.Reusable {
				t.Fatal(report)
			}
			group := report.Groups[0]
			if !group.Complete || len(group.Mutations) == 0 || len(group.Candidates) == 0 {
				t.Fatal(group)
			}
			kept := false
			for _, observation := range report.Observations {
				if strings.Contains(observation.Case.Name, "edge") {
					kept = slices.Contains(group.Keep, observation.Case.At) && !slices.Contains(group.Candidates, observation.Case.At)
				}
			}
			if !kept {
				t.Fatal(group)
			}
			t1, t2 := false, false
			for _, issue := range result.Issues {
				t1 = t1 || issue.Rule == "T1"
				t2 = t2 || issue.Rule == "T2"
				if issue.BlockOnDelivery {
					t.Fatal(issue)
				}
			}
			if !t1 || !t2 {
				t.Fatal(result.Issues)
			}
			after, err := service.Workspace.Capture(t.Context())
			if err != nil || after.Tree != before.Tree {
				t.Fatal("original workspace changed", err)
			}
			for _, name := range []string{"target", "build", ".gradle", "Cargo.lock"} {
				if _, err := os.Stat(filepath.Join(service.Workspace.Root, name)); !os.IsNotExist(err) {
					t.Fatal("analysis output in original", name, err)
				}
			}
		})
	}
}
