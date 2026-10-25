package main

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestNativeArgvUsesDocumentedClassificationWithoutExecuting(t *testing.T) {
	for _, args := range [][]string{
		{"/tools/go", "test", "./..."}, {"/venv/bin/python", "-m", "pytest", "-k", "literal $HOME `text` 中文"},
		{"/tools/npm", "run", "typecheck"}, {"/tools/mvn", "-q", "test"}, {"/tools/cargo", "nextest", "run"},
	} {
		got := classify(input{ID: "native", Role: "native", Args: args, Directory: "/repo"})
		if got.Verification == nil || !*got.Verification || len(got.Unknown) != 0 {
			t.Fatalf("%q: %+v", args, got)
		}
	}
	for _, args := range [][]string{{"/tools/go", "version"}, {"/bin/printf", "%s", "go test ./..."}} {
		got := classify(input{Role: "native", Args: args, Directory: "/repo"})
		if got.Verification == nil || *got.Verification {
			t.Fatalf("literal argument became a verification: %+v", got)
		}
	}
}

func TestInvocationMatchesOnlyLiteralEvalInput(t *testing.T) {
	command := "go test './space path' && printf '%s' '$HOME'"
	digest := sha256.Sum256([]byte(command))
	script := `source /missing/snapshot 2>/dev/null || true && eval 'go test '\''./space path'\'' && printf '\''%s'\'' '\''$HOME'\''' < /dev/null && pwd -P >| /tmp/unused-cwd`
	got := classify(input{ID: "prefix", Role: "shell_invocation", Args: []string{"/bin/zsh", "-c", script}, Directory: "/repo"})
	if !reflect.DeepEqual(got.LiteralCommands, []string{hex.EncodeToString(digest[:])}) || got.Verification != nil {
		t.Fatalf("literal command identity changed: %+v", got)
	}
	unknown := classify(input{Role: "shell_invocation", Args: []string{"/bin/sh", "-c", `eval "$DYNAMIC"`}, Directory: "/repo"})
	if len(unknown.LiteralCommands) != 0 {
		t.Fatalf("dynamic eval input was treated as a fixed command: %+v", unknown)
	}
}

func TestNodeHostedNpxRetainsLiteralArguments(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         []string
		verification bool
	}{
		{"vitest", []string{"/native/node", "/tools/npx", "vitest", "run"}, true},
		{"quoted filter", []string{"/native/node", "/space path/npx", "--no-install", "vitest", "run", "literal $HOME `text` 中文"}, true},
		{"no script", []string{"/native/node"}, false},
		{"no npx command", []string{"/native/node", "/tools/npx"}, false},
		{"other npx command", []string{"/native/node", "/tools/npx", "cowsay", "vitest"}, false},
		{"other script", []string{"/native/node", "/tools/script.js", "npx", "vitest"}, false},
		{"eval literal", []string{"/native/node", "-e", "npx vitest run"}, false},
		{"other runtime", []string{"/native/other", "/tools/npx", "vitest", "run"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]string(nil), tc.args...)
			got := classify(input{Role: "native", Args: tc.args, Directory: "/missing/repo"})
			if got.Verification == nil || *got.Verification != tc.verification || len(got.Unknown) != 0 {
				t.Fatalf("%q: %+v", tc.args, got)
			}
			if !reflect.DeepEqual(before, tc.args) {
				t.Fatalf("classification changed recorded argv: %q -> %q", before, tc.args)
			}
		})
	}
}
