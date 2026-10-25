package codescan

import "testing"

func TestTestMigrationsKeepSubtestAndFrameworkNames(t *testing.T) {
	before := scanSources(t, map[string]string{
		"old_test.go": "package p\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Run(\"a\",func(t *testing.T){t.Fatal(\"x\")})}\n",
		"old.test.ts": "import {test,describe} from 'vitest';describe('old',()=>{test('one',()=>{});});",
	})
	after := scanSources(t, map[string]string{
		"new_test.go": "package p\nimport \"testing\"\nfunc TestNew(t *testing.T){t.Run(\"a\",func(t *testing.T){t.Fatal(\"x\")})}\n",
		"new.test.ts": "import {test,describe} from 'vitest';describe('new',()=>{test('one',()=>{});});",
	})
	changes, err := Compare(t.Context(), before, after, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved := TestMigrations(before, after, changes)
	if moved["test:old_test.go:TestOld/a"] != "test:new_test.go:TestNew/a" || moved["test:old.test.ts:old > one"] != "test:new.test.ts:new > one" {
		t.Fatal(moved, changes)
	}
	associated, err := Associate(t.Context(), before, after, changes, []Position{{Requirement: "R1", At: "test:old_test.go:TestOld/a"}})
	if err != nil || len(associated.Pending) != 0 || associated.Positions[0].At != "test:new_test.go:TestNew/a" {
		t.Fatal(associated, err)
	}
}
