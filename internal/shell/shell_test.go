package shell_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
)

func parse(text string, style event.Shell) shell.Script {
	return shell.Parse(text, "/repo", style, map[event.Shell]shell.Parser{event.POSIX: posix.Parser{}, event.PowerShell: pwsh.Parser{}})
}

func TestDocumentedVerificationCommandsAndPrefixes(t *testing.T) {
	for _, text := range []string{
		"go test ./...", "go build ./cmd/app", "go vet ./...", "golangci-lint run", "staticcheck ./...",
		"npm test", "npm run typecheck", "pnpm build", "yarn lint", "bun test", "tsc --noEmit", "jest", "vitest run", "eslint src", "deno test",
		"pytest tests", "python3 -m unittest", "tox", "nox", "ruff check .", "mypy src", "pyright",
		"cargo test", "cargo build", "cargo check", "cargo clippy", "cargo nextest run",
		"cargo +1.97.0 test", "cargo +nightly build", "cargo +stable check", "cargo +nightly clippy", "cargo +nightly nextest run",
		"mvn -q test", "./mvnw verify", "mvn package", "mvn install", "gradle compile", "./gradlew build", "gradle check", "gradle assemble",
		"dotnet test", "dotnet build", "rspec", "rake test", "rails test", "vendor/bin/phpunit", "swift test", "swift build", "ctest", "cmake --build build", "make ci",
		"npx --no-install jest", "pnpm exec vitest run", "bunx eslint src", "uv run pytest", "poetry run python -m pytest", "pipenv run pytest", "bundle exec rspec",
		"TOKEN='secret value' env OTHER='value' go test ./...", "bash -lc 'go test ./...'", "pwsh -NoProfile -Command 'go test ./...'",
	} {
		t.Run(text, func(t *testing.T) {
			if got := parse(text, event.POSIX); !got.HasVerification() {
				t.Fatalf("not classified: %+v", got)
			}
		})
	}
}

func TestArgumentIdentityDirectoryAndEnvironmentPrivacy(t *testing.T) {
	first := parse("cd 'sub dir' && TOKEN='top secret' env MORE='very private' go test -run 'a  b' ./...", event.POSIX)
	second := parse("cd 'sub dir' && TOKEN='top secret' env MORE='very private' go   test  -run \"a  b\" ./...", event.POSIX)
	if first.CommandKey() != second.CommandKey() || first.Mixed() {
		t.Fatal(first, second)
	}
	segment := first.Segments[len(first.Segments)-1]
	if segment.Dir != "/repo/sub dir" || segment.Args[3] != "a  b" || !segment.Known {
		t.Fatal(segment)
	}
	if parse("go test -run 'a b'", event.POSIX).CommandKey() == parse("go test -run 'a  b'", event.POSIX).CommandKey() {
		t.Fatal("quoted whitespace was collapsed")
	}
	if parse("cd one && go test", event.POSIX).CommandKey() == parse("cd two && go test", event.POSIX).CommandKey() {
		t.Fatal("directories were merged")
	}
	one := first.HashEnvironment([]string{"PATH=/bin", "INHERITED=do-not-save"})
	two := second.HashEnvironment([]string{"PATH=/bin", "INHERITED=changed"})
	if one == two {
		t.Fatal("environment changes ignored")
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"top secret", "very private", "do-not-save"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("environment leaked", string(encoded))
		}
	}
	wrapped := parse("bash -lc 'SECRET=hidden go test ./...'", event.POSIX)
	encoded, _ = json.Marshal(wrapped)
	if strings.Contains(string(encoded), "hidden") {
		t.Fatal("wrapper leaked environment", string(encoded))
	}
}

func TestUnknownWritesAndMixedCommandsNeverReusable(t *testing.T) {
	for _, text := range []string{"go test > result.txt", "go test | tee result.txt", "go test || go test", "go test &", "echo hello; go test", "go test $PACKAGES", "go test $(select-packages)", "go test --fix", "pytest --update", "go test -coverprofile=coverage.out", "go build ./...", "cargo +nightly build", "X=value > file; go test", "cd missing; go test", "tsc", "npx jest", "uv run pytest"} {
		got := parse(text, event.POSIX)
		reusable := got.HasVerification() && !got.Mixed()
		for _, segment := range got.Segments {
			if segment.Kind == shell.Verification {
				reusable = reusable && shell.ReusableCommand(segment)
			}
		}
		if reusable {
			t.Fatalf("unsafe reuse for %s: %+v", text, got)
		}
	}
	for _, text := range []string{"sed -i 's/a/b/' f", "find . -delete", "cat f > g", "git diff --output=diff.txt", "rg --pre=script x"} {
		got := parse(text, event.POSIX)
		for _, segment := range got.Segments {
			if segment.Kind == shell.ReadOnly {
				t.Fatalf("write classified read-only: %s", text)
			}
		}
	}
	for _, text := range []string{"cat 'some file'", "git status --short", "git diff --stat", "rg needle .", "find . -name '*.go'"} {
		got := parse(text, event.POSIX)
		if len(got.Segments) != 1 || got.Segments[0].Kind != shell.ReadOnly {
			t.Fatal(text, got)
		}
	}
}

func TestPowerShellEnvironmentLiteralWhitespaceIsSignificant(t *testing.T) {
	for _, style := range []string{"$env:X=' a  b '; go test", "$env:X = ' a  b '; go test"} {
		value := parse(style, event.PowerShell)
		actual := value.Segments[len(value.Segments)-1].Environment["X"]
		if actual != " a  b " {
			t.Fatalf("quoted environment changed: %q", actual)
		}
		before := value.HashEnvironment(nil)
		other := parse("$env:X='a  b'; go test", event.PowerShell)
		if before == other.HashEnvironment(nil) {
			t.Fatal("environment whitespace was ignored")
		}
	}
	unknown := parse("$env:X=$Dynamic; go test", event.PowerShell)
	if unknown.Segments[len(unknown.Segments)-1].Known {
		t.Fatal("unknown assignment claimed a known environment", unknown)
	}
}

func TestPowerShellLiteralsAliasesAndUnknownStructures(t *testing.T) {
	got := parse("$env:TOKEN='private token'; Set-Location 'C:\\项目\\sub dir'; & 'go.exe' test -run 'a; b''s'", event.PowerShell)
	if !got.HasVerification() {
		t.Fatal(got)
	}
	segment := got.Segments[len(got.Segments)-1]
	if segment.Dir != "C:/项目/sub dir" || segment.Args[3] != "a; b's" {
		t.Fatal(segment)
	}
	got.HashEnvironment(nil)
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "private token") {
		t.Fatal("PowerShell assignment leaked", string(encoded))
	}
	for _, text := range []string{"gc 'a b'", "dir .", "pwd"} {
		result := parse(text, event.PowerShell)
		if len(result.Segments) != 1 || result.Segments[0].Kind != shell.ReadOnly {
			t.Fatal(text, result)
		}
	}
	for _, text := range []string{"go test > out", "go test $Packages", "go test $env:Packages", "go test | Out-File out", "if ($true) { go test }"} {
		result := parse(text, event.PowerShell)
		for _, segment := range result.Segments {
			if !result.Mixed() && shell.ReusableCommand(segment) {
				t.Fatal("unknown PowerShell considered reusable", result)
			}
		}
	}
	unknownAssignment := parse("$env:SECRET = echo 'do-not-save'; go test", event.PowerShell)
	encoded, _ = json.Marshal(unknownAssignment)
	if strings.Contains(string(encoded), "do-not-save") {
		t.Fatal("unknown environment assignment leaked", string(encoded))
	}
}

func TestCargoToolchainIdentityAndNegativeCommands(t *testing.T) {
	for _, style := range []event.Shell{event.POSIX, event.PowerShell} {
		t.Run(string(style), func(t *testing.T) {
			versioned := parse("cargo +1.97.0 test --locked", style)
			if !versioned.HasVerification() || len(versioned.Segments) != 1 {
				t.Fatal("versioned Cargo test not recognized", versioned)
			}
			segment := versioned.Segments[0]
			if segment.Language != "rust" || segment.Args[1] != "+1.97.0" {
				t.Fatal("toolchain selection or language lost", segment)
			}
			for _, other := range []string{"cargo +1.99.1 test --locked", "cargo test --locked"} {
				if versioned.CommandKey() == parse(other, style).CommandKey() {
					t.Fatal("different toolchain commands share execution identity", other)
				}
			}
			for _, command := range []string{"cargo + test", "cargo +stable", "cargo +stable fmt", "cargo +stable install tool", "cargo +stable nextest list"} {
				if got := parse(command, style); got.HasVerification() {
					t.Fatal("unsupported or incomplete command classified as verification", command, got)
				}
			}
			build := parse("cargo +stable build", style)
			if len(build.Segments) != 1 || !build.HasVerification() || shell.ReusableCommand(build.Segments[0]) {
				t.Fatal("build must be recorded but never automatically reused", build)
			}
		})
	}
}
