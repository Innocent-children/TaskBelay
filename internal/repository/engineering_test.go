package repository

import (
	"strings"
	"testing"
)

func TestDependencyFactsIncludeReplacementAndToolchain(t *testing.T) {
	m, e := dependencies("go.mod", []byte("module example\ngo 1.26\ntoolchain go1.27.0\nrequire (\n example/pkg v1.0.0\n)\nreplace example/pkg => ../replacement\nexclude example/old v0.1.0\n"))
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"module", "go", "toolchain", "example/pkg", "replace:example/pkg", "exclude:example/old"} {
		if m[key] == "" {
			t.Fatal("missing dependency fact", key)
		}
	}
	if _, e = dependencies("go.mod", []byte("future_directive secret\n")); e == nil {
		t.Fatal("unknown directive silently ignored")
	}
}
func TestExportedSignaturesSeparateBodiesFromInterfaces(t *testing.T) {
	a, e := exported([]byte("package example\nfunc Exported(v int) int {return v+1}\n"))
	if e != nil {
		t.Fatal(e)
	}
	b, _ := exported([]byte("package example\nfunc Exported(v int) int {return v+2}\n"))
	if a != b || strings.Contains(a, "return") {
		t.Fatal("body was an interface change")
	}
	c, _ := exported([]byte("package example\nfunc Exported(v string) int {return 1}\n"))
	if c == a {
		t.Fatal("signature change missed")
	}
}
