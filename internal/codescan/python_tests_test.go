package codescan

import (
	"slices"
	"testing"
)

func TestPythonTestIdentitiesAndAssertions(t *testing.T) {
	file := scanSources(t, map[string]string{"tests/test_range.py": `import pytest as pt
from unittest import TestCase
from unittest.mock import Mock

def test_plain():
    assert parse(1) == 1

@pt.mark.parametrize("value, expected", [(1, 2), (2, 3)], ids=["one", "two"])
def test_rows(value, expected):
    assert parse(value) == expected

@pt.mark.parametrize("value", [1, 2])
def test_ids(value):
    with pt.raises(ValueError):
        parse(value)

class TestRange:
    def test_mock(self):
        m = Mock()
        m.assert_called_once_with(1)
        assert m.call_count == 1

class Legacy(TestCase):
    def test_result(self):
        self.assertEqual(parse(1), 1)

def helper():
    def test_not_collected():
        assert False
`}).Files["tests/test_range.py"]
	want := []string{"test_plain", "test_rows[one]", "test_rows[two]", "test_ids[1]", "test_ids[2]", "TestRange::test_mock", "Legacy::test_result"}
	var names []string
	for _, test := range file.Tests {
		names = append(names, test.Name)
		if len(test.Assertions) == 0 {
			t.Fatal("missing assertions", test)
		}
		for _, assertion := range test.Assertions {
			kind := "result"
			if test.Name == "TestRange::test_mock" {
				kind = "mock"
			}
			if assertion.Kind != kind {
				t.Fatal(test.Name, assertion)
			}
		}
	}
	if !slices.Equal(names, want) || !file.Operations["tests"].Complete {
		t.Fatal(names, file.Operations["tests"], file.Missing)
	}
}

func TestPythonUnknownParamsAndShadowedMocks(t *testing.T) {
	file := scanSources(t, map[string]string{"test_dynamic.py": `import pytest
from unittest.mock import Mock
@pytest.mark.parametrize("x", source())
def test_dynamic(x):
    assert x
@pytest.mark.parametrize("x", [1, 1])
def test_duplicate(x):
    assert x
def test_shadow(Mock):
    m = Mock()
    assert m.call_count == 1
`}).Files["test_dynamic.py"]
	if len(file.Tests) != 3 || len(file.Tests[0].Unknown) == 0 || len(file.Tests[1].Unknown) == 0 || file.Tests[2].Assertions[0].Kind != "result" {
		t.Fatal(file.Tests)
	}
}
