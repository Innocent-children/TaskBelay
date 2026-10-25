package rustinputs

import (
	"slices"
	"strings"
	"testing"
)

func TestRustTestRangesKeepProductionAndConditionalSupportDistinct(t *testing.T) {
	source := Input{Path: "src/lib.rs", Side: "base", Data: []byte(`
/// Production documentation.
pub fn value() -> i32 { 1 }
#[allow(clippy::test_attr_in_doctest)] fn ordinary() {}
#[cfg(all(test, feature = "checks"))]
mod checks { fn threshold() -> i32 { 1 } #[test] fn old() { assert_eq!(super::value(),threshold()); } }
impl Value { #[cfg(test)] fn helper() -> i32 { 2 } }
#[cfg(any(test, feature = "production"))] fn shared() {}
`)}
	result := Describe(source)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	var tests []string
	for _, Item := range result.Items {
		if Item.Docs == nil {
			t.Fatal("documentation inventory must serialize as an array")
		}
		if Item.Test {
			tests = append(tests, Item.ID)
			if !strings.Contains(string(source.Data[Item.Start:Item.End]), "#[") {
				t.Fatal("attribute omitted from test range")
			}
		}
	}
	if len(tests) != 3 {
		t.Fatalf("test declarations: %v", tests)
	}
	for _, name := range tests {
		if strings.Contains(name, "ordinary") || strings.Contains(name, "shared") {
			t.Fatal(name)
		}
	}
}

func TestRustReferencesAndDocumentationOwners(t *testing.T) {
	result := Describe(Input{Path: "src/lib.rs", Side: "base", Data: []byte(`
//! Crate documentation.
/// Reexport documentation.
pub use inner::Value;
trait Value { /// Associated type documentation.
type Output; }
#[cfg(test)] #[path="checks.rs"] mod checks;
#[cfg(doctest)] #[doc = include_str!("../README.md")] struct Readme;
#[test] fn fixture() { let _ = include_str!("fixture.txt"); }
`)})
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	var refs []string
	for _, ref := range result.References {
		if ref.Test {
			refs = append(refs, ref.Choices...)
		}
	}
	for _, name := range []string{"src/checks.rs", "README.md", "src/fixture.txt"} {
		if !slices.Contains(refs, name) {
			t.Fatalf("missing %s: %v", name, refs)
		}
	}
	if len(result.Items[0].Docs) != 1 {
		t.Fatalf("root docs: %+v", result.Items[0])
	}
}

func TestRustInputDescriptionDoesNotPretendMalformedSyntaxIsComplete(t *testing.T) {
	result := Describe(Input{Path: "src/lib.rs", Data: []byte("#[test] fn broken( {")})
	if result.Error == "" {
		t.Fatal("malformed syntax accepted")
	}
}

func TestInnerTestScopeAndDocIncludesKeepTheirInputs(t *testing.T) {
	result := Describe(Input{Path: "src/checks.rs", Data: []byte(`#![cfg(test)]
#![doc = include_str!("../README.md")]
fn fixture() { let _ = include_str!(concat!("part", ".txt")); }
`)})
	if !result.Items[0].Test || result.Error != "dynamic included test input" {
		t.Fatalf("test scope or dynamic Input was lost: %+v", result)
	}
	if len(result.References) != 2 || !result.References[0].Test || !result.References[0].Doc {
		t.Fatalf("documentation Input was lost: %+v", result.References)
	}
}
