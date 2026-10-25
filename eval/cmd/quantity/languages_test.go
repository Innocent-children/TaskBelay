package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestLanguageQuantitiesSeparateProductionAndParameterizedTests(t *testing.T) {
	for _, sample := range []struct {
		language, source, original, replacement, extra, extraText, tests, originalTests, addedTests string
	}{
		{"python", "value.py", "def value():\n    return 1\n", "def value():\n    return 2\n", "extra.py", "def extra():\n    pass\n", "tests/test_value.py",
			"import pytest\ndef test_keep():\n    assert 1 == 1\n",
			"@pytest.mark.parametrize('value', [1, 2])\ndef test_rows(value):\n    assert value > 0\n"},
		{"typescript", "value.ts", "export const value = 1;\n", "export const value = 2;\n", "extra.ts", "export function extra() {\n}\n", "value.test.ts",
			"import {test, expect} from 'vitest';\ntest('keep', () => { expect(1).toBe(1); });\n",
			"test.each([[1], [2]])('row %s', value => { expect(value).toBeGreaterThan(0); });\n"},
		{"javascript", "value.js", "export const value = 1;\n", "export const value = 2;\n", "extra.js", "export function extra() {\n}\n", "value.test.js",
			"test('keep', () => { expect(1).toBe(1); });\n",
			"test.each([[1], [2]])('row %s', value => { expect(value).toBeGreaterThan(0); });\n"},
		{"java", "src/main/java/Value.java", "class Value { int value() { return 1; } }\n", "class Value { int value() { return 2; } }\n", "src/main/java/Extra.java", "class Extra {\n}\n", "src/test/java/ValueTest.java",
			"import org.junit.jupiter.api.Test;\nimport org.junit.jupiter.params.ParameterizedTest;\nimport org.junit.jupiter.params.provider.ValueSource;\nclass ValueTest {\n@Test void keep() { assert 1 == 1; }\n",
			"@ParameterizedTest @ValueSource(ints={1,2}) void rows(int value) { assert value > 0; }\n"},
		{"rust", "src/lib.rs", "pub fn value() -> i32 { 1 }\n", "pub fn value() -> i32 { 2 }\n", "src/extra.rs", "pub fn extra() {\n}\n", "src/lib.rs",
			"#[cfg(test)]\nmod tests {\nuse rstest::rstest;\n#[test] fn keep() { assert_eq!(1, 1); }\n",
			"#[rstest]\n#[case(1)]\n#[case(2)]\nfn rows(#[case] value: i32) { assert!(value > 0); }\n"},
	} {
		t.Run(sample.language, func(t *testing.T) {
			directory := t.TempDir()
			git(t, directory, "init")
			suffix := ""
			if sample.language == "java" || sample.language == "rust" {
				suffix = "}\n"
			}
			put(t, directory, sample.source, sample.original)
			prefix := ""
			if sample.tests == sample.source {
				prefix = sample.original
			}
			put(t, directory, sample.tests, prefix+sample.originalTests+suffix)
			base := saveTree(t, directory)
			put(t, directory, sample.source, sample.replacement)
			put(t, directory, sample.extra, sample.extraText)
			put(t, directory, "README.rst", "Project\n=======\nDocumentation only.\n")
			if sample.tests == sample.source {
				prefix = sample.replacement
			}
			put(t, directory, sample.tests, prefix+sample.originalTests+sample.addedTests+suffix)
			result, err := measure(context.Background(), directory, base, saveTree(t, directory))
			if err != nil || result.NonTestAddedLines == nil || *result.NonTestAddedLines != 3 || result.AddedTestCount == nil || *result.AddedTestCount != 2 || len(result.Unknown) != 0 {
				t.Fatalf("language quantities: %+v, %v", result, err)
			}
		})
	}
}

func TestRustInlineTestsDoNotInflateSourceLines(t *testing.T) {
	for _, sample := range []struct {
		name, source string
		lines, tests int
	}{
		{"module", "#[cfg(test)]\nmod checks {\nuse super::*;\nfn helper() -> i32 { 1 }\n#[test] fn one() { assert_eq!(helper(), value()); }\n}\n", 0, 1},
		{"mixed-line", "pub fn another() {} #[test] fn one() { assert_eq!(value(), 1); }\n", 1, 1},
		{"conditional-production", "#[cfg(any(test, feature = \"production\"))]\npub fn another() {}\n", 2, 0},
		{"test-helper", "#[cfg(all(test, feature = \"checks\"))]\nfn helper() {}\n", 0, 0},
	} {
		t.Run(sample.name, func(t *testing.T) {
			directory := t.TempDir()
			git(t, directory, "init")
			put(t, directory, "src/lib.rs", "pub fn value() -> i32 { 1 }\n")
			base := saveTree(t, directory)
			put(t, directory, "src/lib.rs", "pub fn value() -> i32 { 1 }\n"+sample.source)
			result, err := measure(context.Background(), directory, base, saveTree(t, directory))
			if err != nil || result.NonTestAddedLines == nil || *result.NonTestAddedLines != sample.lines || result.AddedTestCount == nil || *result.AddedTestCount != sample.tests {
				t.Fatalf("inline quantities: %+v, %v", result, err)
			}
		})
	}
}

func TestUnknownLanguageEnumerationCannotBecomeZero(t *testing.T) {
	for _, sample := range []struct{ path, source string }{
		{"test_dynamic.py", "import pytest\n@pytest.mark.parametrize('x', rows())\ndef test_dynamic(x):\n    assert x\n"},
		{"dynamic.test.ts", "test.each(rows())('row %s', x => expect(x).toBe(1));\n"},
		{"src/test/java/Dynamic.java", "import org.junit.jupiter.params.ParameterizedTest;\nimport org.junit.jupiter.params.provider.MethodSource;\nclass Dynamic { @ParameterizedTest @MethodSource(\"rows\") void rows(int n) {} }\n"},
		{"src/lib.rs", "use rstest::rstest;\n#[rstest] fn rows(fixture: i32) { assert!(fixture > 0); }\n"},
		{"src/macro.rs", "macro_rules! generate { () => { #[test] fn case() {} }; }\ngenerate!();\n"},
	} {
		t.Run(sample.path, func(t *testing.T) {
			directory := t.TempDir()
			git(t, directory, "init")
			put(t, directory, "README.md", "fixture\n")
			base := saveTree(t, directory)
			put(t, directory, sample.path, sample.source)
			result, err := measure(context.Background(), directory, base, saveTree(t, directory))
			if err != nil || result.AddedTestCount != nil || len(result.Unknown) == 0 || ratios(result, result).TestRatio != nil {
				t.Fatalf("unknown enumeration: %+v, %v", result, err)
			}
		})
	}
}

func TestLanguageTestMovesKeepExistingCaseIdentity(t *testing.T) {
	for index, sample := range []struct{ path, renamed, source string }{
		{"test_a.py", "test_b.py", "def test_existing():\n    assert 1 == 1\n"},
		{"a.test.ts", "b.test.ts", "test('existing', () => expect(1).toBe(1));\n"},
		{"src/test/java/Cases.java", "src/test/java/Moved.java", "import org.junit.jupiter.api.Test;\nclass Cases { @Test void existing() {} }\n"},
		{"tests/a.rs", "tests/b.rs", "#[test] fn existing() { assert_eq!(1,1); }\n"},
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			directory := t.TempDir()
			git(t, directory, "init")
			put(t, directory, sample.path, sample.source)
			base := saveTree(t, directory)
			git(t, directory, "mv", sample.path, sample.renamed)
			result, err := measure(context.Background(), directory, base, saveTree(t, directory))
			if err != nil || result.AddedTestCount == nil || *result.AddedTestCount != 0 || len(result.TestMigrations) != 1 || result.NonTestAddedLines == nil || *result.NonTestAddedLines != 0 {
				t.Fatalf("moved test: %+v, %v", result, err)
			}
			for from, to := range result.TestMigrations {
				if !strings.Contains(from, sample.path) || !strings.Contains(to, sample.renamed) {
					t.Fatalf("wrong migration: %s -> %s", from, to)
				}
			}
		})
	}
}

func TestRustTestScopeChangesPreservePhysicalLineCounts(t *testing.T) {
	directory := t.TempDir()
	git(t, directory, "init")
	body := "mod support {\n    pub fn value() -> i32 { 1 }\n}\n"
	put(t, directory, "src/lib.rs", "#[cfg(test)]\n"+body)
	base := saveTree(t, directory)
	put(t, directory, "src/lib.rs", body)
	tree := saveTree(t, directory)
	result, err := measure(context.Background(), directory, base, tree)
	if err != nil || result.NonTestAddedLines == nil || *result.NonTestAddedLines != 3 || result.AddedTestCount == nil || *result.AddedTestCount != 0 {
		t.Fatalf("test scope moved to production: %+v, %v", result, err)
	}
	reverse, err := measure(context.Background(), directory, tree, base)
	if err != nil || reverse.NonTestAddedLines == nil || *reverse.NonTestAddedLines != 0 {
		t.Fatalf("production moved to test scope: %+v, %v", reverse, err)
	}
}

func TestDuplicateCaseAndExternalRustScopeStayUnknown(t *testing.T) {
	directory := t.TempDir()
	git(t, directory, "init")
	original := "test('same', () => expect(1).toBe(1));\n"
	put(t, directory, "same.test.ts", original)
	base := saveTree(t, directory)
	put(t, directory, "same.test.ts", original+original)
	result, err := measure(context.Background(), directory, base, saveTree(t, directory))
	if err != nil || result.AddedTestCount != nil || len(result.Unknown) == 0 {
		t.Fatalf("duplicate test disappeared in map: %+v, %v", result, err)
	}
	put(t, directory, "src/lib.rs", "#[cfg(test)] #[path=\"support.rs\"] mod support;\n")
	put(t, directory, "src/support.rs", "fn helper() {}\n")
	result, err = measure(context.Background(), directory, base, saveTree(t, directory))
	if err != nil || result.NonTestAddedLines != nil || !strings.Contains(fmt.Sprint(result.Unknown), "外部 Rust 测试模块") {
		t.Fatalf("external test support became production code: %+v, %v", result, err)
	}
}
