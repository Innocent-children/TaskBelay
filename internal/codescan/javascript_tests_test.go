package codescan

import (
	"slices"
	"testing"
)

func TestJavaScriptTestIdentitiesAndAssertions(t *testing.T) {
	file := scanSources(t, map[string]string{"range.test.ts": `import {test as check, describe, expect} from 'vitest';
import assert from 'node:assert';
describe('range', () => {
  describe('parser', () => {
    check.each([['one', 1], ['two', 2]])('%s', (name, value) => { expect(parse(value)).toBe(value); });
    check.each([{label:'low', value:1}, {label:'high', value:9}])('$label', ({value}) => expect(parse(value)).toEqual(value));
    check('mock', () => { expect(save).not.toHaveBeenCalledTimes(2); });
    check('node', () => { assert.equal(parse(1), 1); });
  });
});
`}).Files["range.test.ts"]
	want := []string{"range > parser > one", "range > parser > two", "range > parser > low", "range > parser > high", "range > parser > mock", "range > parser > node"}
	var names []string
	for _, test := range file.Tests {
		names = append(names, test.Name)
		kind := "result"
		if test.Name == "range > parser > mock" {
			kind = "mock"
		}
		if len(test.Assertions) != 1 || test.Assertions[0].Kind != kind {
			t.Fatal(test)
		}
	}
	if !slices.Equal(names, want) || !file.Operations["tests"].Complete {
		t.Fatal(names, file.Missing)
	}
}

func TestJavaScriptTestUnknownAndShadowing(t *testing.T) {
	file := scanSources(t, map[string]string{"dynamic.test.js": `test.each(rows())('row %s', () => expect(work()).toBe(1));
test.each([[1], [2]])('same', () => {});
test('plain', () => {function helper(expect) { expect(x).toHaveBeenCalled(); } });
function notAFramework(test) {test('fake', () => {});}
`}).Files["dynamic.test.js"]
	if len(file.Tests) != 3 || len(file.Tests[0].Unknown) == 0 || len(file.Tests[1].Unknown) == 0 || len(file.Tests[2].Assertions) != 0 {
		t.Fatal(file.Tests, file.Missing)
	}
}

func TestJavaScriptTaggedEachKeepsUnknownOuterTest(t *testing.T) {
	file := scanSources(t, map[string]string{"table.test.ts": "import {test, expect} from 'vitest';\ntest.each`value | result\n${1} | ${2}\n${2} | ${3}`('adds $value', ({value, result}) => { expect(value + 1).toBe(result); });\n"}).Files["table.test.ts"]
	if len(file.Tests) != 1 || file.Tests[0].Name != "adds $value" || len(file.Tests[0].Unknown) == 0 || len(file.Tests[0].Assertions) != 1 {
		t.Fatal(file.Tests, file.Missing)
	}
}
