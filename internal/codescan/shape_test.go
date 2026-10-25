package codescan

import "testing"

func TestDocumentedSimpleConversionsBranchesAndForwarders(t *testing.T) {
	for _, test := range []struct {
		file, source, id string
		simple           bool
		branches         int
		forwarder        string
	}{
		{"a.go", "package p\nfunc get(n int)int{return n+1}", "get", true, 0, ""},
		{"a.go", "package p\nfunc convert(n int)int64{return int64(n)}", "convert", true, 0, ""},
		{"a.go", "package p\ntype Count int\nfunc convert(n int)Count{return Count(n)}", "convert", true, 0, ""},
		{"a.go", "package p\nfunc forward(a,b int)int{return real(a,b)}", "forward", false, 0, "real"},
		{"a.go", "package p\nfunc reverse(a,b int)int{return real(b,a)}", "reverse", false, 0, ""},
		{"a.go", "package p\nfunc branch(a,b bool)int{if a&&b{return 1};return 0}", "branch", false, 2, ""},
		{"a.go", "package p\nfunc convert(n int, int64 func(int) int)int{return int64(n)}", "convert", false, 0, ""},
		{"a.py", "def convert(n):\n return str(n)\n", "convert", true, 0, "str"},
		{"a.py", "def forward(a,b):\n return real(a,b)\n", "forward", false, 0, "real"},
		{"a.py", "def branch(n):\n if n:\n  return 1\n elif n is None:\n  return 2\n return 0\n", "branch", false, 2, ""},
		{"a.py", "def convert(n,str):\n return str(n)\n", "convert", false, 0, ""},
		{"a.py", "from custom import str\ndef convert(n):\n return str(n)\n", "convert", false, 0, "str"},
		{"a.py", "from custom import *\ndef convert(n):\n return str(n)\n", "convert", false, 0, "str"},
		{"a.js", "export function convert(n){return Array.from(n)}", "convert", true, 0, "Array.from"},
		{"a.ts", "export const convert=(n:number)=>String(n)", "convert", true, 0, "String"},
		{"a.ts", "function branch(n?:string){return n ?? 'default'}", "branch", false, 1, ""},
		{"a.java", "class Box { int forward(int n){return real(n);} }", "Box.forward", false, 0, "real"},
		{"a.rs", "fn convert(n:String)->String { n.clone() }", "convert", true, 0, ""},
		{"a.rs", "fn branch(n:Result<i32,E>)->Result<i32,E>{let x=n?;loop{break};Ok(x)}", "branch", false, 2, ""},
		{"a.c", "int get(int n){return n;}", "get", true, 0, ""},
		{"a.cpp", "class Box { public: int get(int n){return n;} };", "Box.get", true, 0, ""},
		{"a.cs", "class Box { int Get(int n){return n;} }", "Box.Get", true, 0, ""},
		{"a.php", "<?php function get($n){return $n;}", "get", true, 0, ""},
		{"a.php", "<?php function get(){include 'side-effects.php';}", "get", false, 0, ""},
		{"a.rb", "def get(n)\n n\nend\n", "get", true, 0, ""},
	} {
		t.Run(test.file+"/"+test.id, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{test.file: test.source})
			if !snapshot.Files[test.file].CodeComplete {
				t.Fatal(snapshot.Missing)
			}
			shape := snapshot.Declarations()[test.file+":"+test.id].Shape
			if shape == nil || shape.Simple != test.simple || len(shape.Branches) != test.branches || shape.Forwarder != test.forwarder {
				t.Fatalf("got %+v; want simple=%v branches=%d forward=%q", shape, test.simple, test.branches, test.forwarder)
			}
		})
	}
}
