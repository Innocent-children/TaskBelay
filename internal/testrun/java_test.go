package testrun

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const javaNativePOM = `<project xmlns="http://maven.apache.org/POM/4.0.0"><modelVersion>4.0.0</modelVersion><groupId>test.taskbelay</groupId><artifactId>native-fixture</artifactId><version>1</version><properties><maven.compiler.release>21</maven.compiler.release><project.build.sourceEncoding>UTF-8</project.build.sourceEncoding></properties><dependencies><dependency><groupId>org.junit.jupiter</groupId><artifactId>junit-jupiter</artifactId><version>5.10.2</version><scope>test</scope></dependency></dependencies><build><plugins><plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-compiler-plugin</artifactId><version>3.14.1</version></plugin><plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-surefire-plugin</artifactId><version>3.5.5</version></plugin></plugins></build></project>`

const javaNativeTests = `package example;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import static org.junit.jupiter.api.Assertions.*;
class LimitTest {
 @Test void inside(){assertTrue(Limit.allowed(1));}
 @Test void edge(){assertFalse(Limit.allowed(5));}
 @ParameterizedTest @ValueSource(ints={1,2}) void rows(int n){assertTrue(Limit.allowed(n));}
}`

func nativeJavaRequest(t *testing.T, tool string) (*Executor, Request) {
	t.Helper()
	tools := os.Getenv("TASKBELAY_TEST_BUILD_TOOLS")
	if tools == "" {
		t.Skip("需要独立 Java 构建工具环境")
	}
	files := map[string]string{"src/main/java/example/Limit.java": "package example;\npublic class Limit { public static boolean allowed(int n){return n < 5;} }\n", "src/test/java/example/LimitTest.java": javaNativeTests}
	if tool == "maven" {
		files["pom.xml"] = strings.Replace(javaNativePOM, "<build>", "<repositories><repository><id>alimaven</id><url>https://maven.aliyun.com/repository/public</url></repository></repositories><pluginRepositories><pluginRepository><id>alimaven</id><url>https://maven.aliyun.com/repository/public</url></pluginRepository></pluginRepositories><build>", 1)
		files["pom.xml"] = strings.Replace(files["pom.xml"], "<plugins>", "<plugins><plugin><groupId>org.apache.maven.plugins</groupId><artifactId>maven-resources-plugin</artifactId><version>3.4.0</version></plugin>", 1)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		files["settings.gradle"] = "rootProject.name='native-fixture'\n"
		files["build.gradle"] = "plugins { id 'java' }\nrepositories { maven { url = uri('" + filepath.ToSlash(filepath.Join(home, ".m2", "repository")) + "') } }\ndependencies { testImplementation 'org.junit.jupiter:junit-jupiter:5.10.2'; testRuntimeOnly 'org.junit.platform:junit-platform-launcher:1.10.2' }\ntest { useJUnitPlatform() }\n"
	}
	executor, request := nativeToolRequest(t, files, tools)
	executor.Output = filepath.Join(executor.Output, "build output")
	if err := os.Mkdir(executor.Output, 0700); err != nil {
		t.Fatal(err)
	}
	javaHome := os.Getenv("TASKBELAY_TEST_JAVA_HOME")
	if javaHome == "" {
		t.Skip("需要 TASKBELAY_TEST_JAVA_HOME 明确原生验收 JDK")
	}
	executor.Env = mergeEnvironment(executor.Env, []string{"JAVA_HOME=" + javaHome, "PATH=" + filepath.Join(javaHome, "bin") + string(os.PathListSeparator) + os.Getenv("PATH")})
	if tool == "gradle" {
		path, err := os.ReadFile(filepath.Join(tools, "gradle-path"))
		if err != nil {
			t.Fatal(err)
		}
		request.Config["java"] = Config{Command: scopeQuote(strings.TrimSpace(string(path))) + " test"}
	}
	return executor, request
}

func TestJavaNativeJUnitNamesCoverageAndMutations(t *testing.T) {
	for _, tool := range []string{"maven", "gradle"} {
		t.Run(tool, func(t *testing.T) {
			executor, request := nativeJavaRequest(t, tool)
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			runner := NewJavaRunner(executor, request)
			defer runner.Terminate()
			if result := runner.Prepare(ctx); !result.Passed() {
				t.Fatal("prepare", result)
			}
			if result := runner.Compile(ctx); !result.Passed() {
				t.Fatal("compile", result)
			}
			cases, result := runner.Enumerate(ctx, request.Cases)
			if !result.Passed() || len(cases) != 3 {
				t.Fatal("enumerate", result, cases)
			}
			var edge Case
			for _, test := range cases {
				run := runner.Run(ctx, test, "baseline")
				if !run.Passed() || !run.Coverage.Valid || !run.Coverage.CopyExecuted || !slices.Contains(run.Coverage.Lines, Line{File: "src/main/java/example/Limit.java", Number: 2}) {
					t.Fatal(test, run)
				}
				if strings.HasSuffix(test.Selector, "#edge") {
					edge = test
				}
				if strings.HasSuffix(test.Selector, "#rows") && (!test.Whole || len(test.Expected) != 2) {
					t.Fatal("parameter identity", test)
				}
			}
			if edge.Name == "" {
				t.Fatal(cases)
			}
			path := filepath.Join(executor.Root, "src/main/java/example/Limit.java")
			if err := os.WriteFile(path, []byte("package example;\npublic class Limit { public static boolean allowed(int n){return n <= 5;} }\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if result := runner.Compile(ctx); !result.Passed() {
				t.Fatal(result)
			}
			if run := runner.Run(ctx, edge, "mutation"); run.Failure != AssertionFailure {
				t.Fatal(run)
			}
			if err := os.WriteFile(path, []byte("package example;\npublic class Limit { public static boolean allowed(int n){throw new RuntimeException(\"infra\");} }\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if result := runner.Compile(ctx); !result.Passed() {
				t.Fatal(result)
			}
			if run := runner.Run(ctx, edge, "mutation"); run.Failure != EnvironmentFailure {
				t.Fatal(run)
			}
			if err := os.WriteFile(path, []byte("public class Limit {"), 0600); err != nil {
				t.Fatal(err)
			}
			if result := runner.Compile(ctx); result.Failure != CompileFailure {
				t.Fatal(result)
			}
		})
	}
}
