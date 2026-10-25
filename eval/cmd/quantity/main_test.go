package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func git(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func put(t *testing.T, directory, name, text string) {
	t.Helper()
	path := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func saveTree(t *testing.T, directory string) string {
	t.Helper()
	git(t, directory, "add", "--all")
	return git(t, directory, "write-tree")
}

func TestFixedQuantitiesPreserveWorkspaceAndDistinguishExistingTests(t *testing.T) {
	directory := t.TempDir()
	git(t, directory, "init")
	put(t, directory, "value.go", "package fixture\nfunc Value() int { return 1 }\n")
	original := "package fixture\nimport \"testing\"\n" +
		"func TestKeep(t *testing.T) { if Value() < 0 { t.Fatal(\"negative\") } }\n" +
		"func TestRename(t *testing.T) { if Value() == 9 { t.Fatal(\"nine\") } }\n"
	put(t, directory, "value_test.go", original)
	base := saveTree(t, directory)
	put(t, directory, "value.go", "package fixture\nfunc Value() int { return 2 }\n")
	put(t, directory, "added.go", "package fixture\n\nfunc Added() {}")
	put(t, directory, "README.md", "not source\n")
	put(t, directory, "value_test.go", strings.ReplaceAll(original, "TestRename", "TestRenamed")+
		"func TestAdded(t *testing.T) { t.Run(\"one\", func(t *testing.T) { t.Fatal(\"one\") }); t.Run(\"two\", func(t *testing.T) { t.Fatal(\"two\") }) }\n")
	tree := saveTree(t, directory)
	put(t, directory, "value.go", "user edit after recorded tree\n")
	indexPath := filepath.Join(directory, ".git", "index")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	objects := git(t, directory, "count-objects", "-v")
	result, err := measure(context.Background(), directory, base, tree)
	if err != nil {
		t.Fatal(err)
	}
	if result.NonTestAddedLines == nil || *result.NonTestAddedLines != 4 || result.AddedTestCount == nil || *result.AddedTestCount != 2 {
		t.Fatalf("wrong counts: %+v", result)
	}
	if len(result.TestMigrations) != 1 || len(result.Unknown) != 0 || !reflect.DeepEqual(result.SourceLines, map[string]int{"value.go": 1, "added.go": 3}) {
		t.Fatalf("wrong change identities: %+v", result)
	}
	empty, err := measure(context.Background(), directory, base, base)
	if err != nil {
		t.Fatal(err)
	}
	comparison := ratios(result, empty)
	if comparison.CodeRatio != nil || comparison.TestRatio == nil || *comparison.TestRatio != 2 || comparison.Adoption != "not_evaluated" {
		t.Fatalf("zero reference or adoption was inferred: %+v", comparison)
	}
	if objects != git(t, directory, "count-objects", "-v") {
		t.Fatal("measurement changed Git objects")
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	afterStat, err := os.Stat(indexPath)
	if err != nil || !reflect.DeepEqual(index, after) || !stat.ModTime().Equal(afterStat.ModTime()) {
		t.Fatalf("measurement changed the real index: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "value.go"))
	if err != nil || string(data) != "user edit after recorded tree\n" {
		t.Fatalf("measurement changed the current file: %s %v", data, err)
	}
}

func TestUnknownEnumerationAndOtherLanguagesDoNotBecomeZero(t *testing.T) {
	directory := t.TempDir()
	git(t, directory, "init")
	put(t, directory, "value.go", "package fixture\n")
	base := saveTree(t, directory)
	put(t, directory, "dynamic_test.go", "package fixture\nimport \"testing\"\nfunc TestDynamic(t *testing.T) { for _, name := range names() { t.Run(name, func(t *testing.T) {}) } }\n")
	put(t, directory, "generated.go", "// Code generated; do not edit.\npackage fixture\nfunc Generated() {}\n")
	put(t, directory, "vendor/pkg/value.go", "package pkg\nfunc Vendored() {}\n")
	tree := saveTree(t, directory)
	result, err := measure(context.Background(), directory, base, tree)
	if err != nil {
		t.Fatal(err)
	}
	if result.NonTestAddedLines == nil || *result.NonTestAddedLines != 5 || result.AddedTestCount != nil || len(result.Unknown) == 0 {
		t.Fatalf("unknown enumeration or generated source was lost: %+v", result)
	}
	comparison := ratios(result, result)
	if comparison.CodeRatio == nil || *comparison.CodeRatio != 1 || comparison.TestRatio != nil {
		t.Fatalf("unknown count became a ratio: %+v", comparison)
	}
	put(t, directory, "foreign.cs", "class Unsupported {}\n")
	result, err = measure(context.Background(), directory, base, saveTree(t, directory))
	if err != nil || result.NonTestAddedLines != nil || result.AddedTestCount != nil {
		t.Fatalf("unsupported source was counted as zero: %+v %v", result, err)
	}
}
