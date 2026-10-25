package codescan

import (
	"slices"
	"testing"
)

func TestRustTestIdentitiesAndAssertions(t *testing.T) {
	file := scanSources(t, map[string]string{"src/lib.rs": `#[cfg(test)]
mod tests {
 use rstest::rstest;
 use mockall::automock;
 #[automock]
 trait Save {fn save(&self);}
 #[test]
 fn ordinary(){assert_eq!(parse(1),1);}
 #[tokio::test]
 async fn async_case(){assert!(parse(1)>0);}
 #[rstest]
 #[case::low(1)]
 #[case::high(9)]
 fn rows(#[case] value:i32){assert_ne!(parse(value),0);}
 #[test]
 fn only_mock(){let mut mock = MockSave::new();mock.expect_save().times(1);}
}
`}).Files["src/lib.rs"]
	want := []string{"tests::ordinary", "tests::async_case", "tests::rows::case_1_low", "tests::rows::case_2_high", "tests::only_mock"}
	var names []string
	for _, test := range file.Tests {
		names = append(names, test.Name)
		if len(test.Assertions) == 0 {
			t.Fatal("missing assertions", test, file.Missing)
		}
		for _, assertion := range test.Assertions {
			kind := "result"
			if test.Name == "tests::only_mock" {
				kind = "mock"
			}
			if assertion.Kind != kind {
				t.Fatal(test, assertion)
			}
		}
	}
	if !slices.Equal(names, want) || !file.Operations["tests"].Complete {
		t.Fatal(names, file.Missing)
	}
}

func TestRustUnknownParameterAndNonMockMethods(t *testing.T) {
	file := scanSources(t, map[string]string{"tests/integration.rs": `use rstest::rstest;
#[rstest]
fn dynamic(#[values(1,2)] value:i32){assert!(value>0);}
struct Fake;
impl Fake {fn expect_call(&self){}}
#[test]
fn custom(){let fake=Fake;fake.expect_call();}
`}).Files["tests/integration.rs"]
	if len(file.Tests) != 2 || file.Tests[0].Name != "dynamic" || len(file.Tests[0].Unknown) == 0 || len(file.Tests[1].Assertions) != 0 {
		t.Fatal(file.Tests)
	}
}

func TestRustMockMacroExpectations(t *testing.T) {
	file := scanSources(t, map[string]string{"tests/save.rs": `use mockall::mock;
mock! { Save {fn save(&self);}}
#[test]
fn save_once(){let mut m=MockSave::new();m.expect_save().with().times(1);}
`}).Files["tests/save.rs"]
	if len(file.Tests) != 1 || len(file.Tests[0].Assertions) != 3 || len(file.Tests[0].Unknown) != 0 {
		t.Fatal(file.Tests, file.Missing)
	}
}
