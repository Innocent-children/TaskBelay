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

func nativeRustRequest(t *testing.T, source string) (*Executor, Request) {
	t.Helper()
	tools := os.Getenv("TASKBELAY_TEST_BUILD_TOOLS")
	if tools == "" {
		t.Skip("需要 TASKBELAY_TEST_BUILD_TOOLS 指向独立 Rust/Java 工具环境")
	}
	manifest, err := os.ReadFile(filepath.Join(tools, "rust-fixture", "Cargo.toml"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(filepath.Join(tools, "rust-fixture", "Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	executor, request := nativeToolRequest(t, map[string]string{"Cargo.toml": string(manifest), "Cargo.lock": string(lock), "src/lib.rs": source}, tools)
	executor.Env = mergeEnvironment(executor.Env, []string{"CARGO_HOME=" + filepath.Join(tools, "cargo"), "RUSTUP_HOME=" + filepath.Join(tools, "rustup"), "PATH=" + filepath.Join(tools, "cargo", "bin") + string(os.PathListSeparator) + os.Getenv("PATH")})
	return executor, request
}

func TestRustNativeOrdinaryTokioRstestAndMutation(t *testing.T) {
	source := `pub fn allowed(n:i32)->bool { n < 5 }
#[cfg(test)]
mod tests {
 use rstest::rstest;
 #[test]
 fn ordinary(){assert!(super::allowed(1));}
 #[tokio::test]
 async fn async_case(){assert!(super::allowed(1));}
 #[rstest]
 #[case::inside(1,true)]
 #[case::edge(5,false)]
 fn rows(#[case] n:i32,#[case] expected:bool){assert_eq!(super::allowed(n),expected);}
 #[rstest]
 fn dynamic(#[values(1,2)] n:i32){assert!(super::allowed(n));}
}
`
	executor, request := nativeRustRequest(t, source)
	request.Requirements["R1"] = []Line{{File: "src/lib.rs", Number: 1}}
	runner := NewRustRunner(executor, request)
	defer runner.Terminate()
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	if result := runner.Prepare(ctx); !result.Passed() {
		t.Fatal(result)
	}
	if result := runner.Compile(ctx); !result.Passed() {
		t.Fatal(result)
	}
	cases, result := runner.Enumerate(ctx, request.Cases)
	if !result.Passed() || len(cases) != 5 {
		t.Fatal(result, cases)
	}
	var edge Case
	for _, test := range cases {
		run := runner.Run(ctx, test, "baseline")
		if !run.Passed() || !run.Coverage.Valid || !run.Coverage.CopyExecuted || !slices.Contains(run.Coverage.Lines, Line{File: "src/lib.rs", Number: 1}) {
			t.Fatal(test, run)
		}
		if strings.HasSuffix(test.Name, "case_2_edge") {
			edge = test
		}
		if strings.HasSuffix(test.Name, "dynamic") && (!test.Whole || len(test.Expected) != 2) {
			t.Fatal(test)
		}
	}
	if edge.Name == "" {
		t.Fatal(cases)
	}
	path := filepath.Join(executor.Root, "src", "lib.rs")
	if err := os.WriteFile(path, []byte(strings.Replace(source, "n < 5", "n <= 5", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if result := runner.Compile(ctx); !result.Passed() {
		t.Fatal(result)
	}
	if run := runner.Run(ctx, edge, "mutation"); run.Failure != AssertionFailure {
		t.Fatal(run)
	}
	if err := os.WriteFile(path, []byte("pub fn bad( {"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := runner.Compile(ctx); result.Failure != CompileFailure {
		t.Fatal(result)
	}
}
