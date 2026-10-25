package codescan

import (
	"slices"
	"testing"
)

func genericValueTexts(slot Slot) []string {
	var result []string
	for _, value := range slot.Values {
		if !slices.Contains(result, value.Text) {
			result = append(result, value.Text)
		}
	}
	return result
}

func TestGenericCParametersFunctionsAndShadowing(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.c": `const int COUNT=2;
int add(int value,int unused){{int unused=3;consume(unused);}return value;}
int wrap(int value){return add(value,0);}
int identity(int value){return value;}
int apply(int (*callback)(int),int value){return callback(value);}
int mutate(int value){value++;return value;}
int run(){add(COUNT,0);wrap(3);apply(identity,4);mutate(2);return 0;}
`})
	d := snapshot.Declarations()
	u := d["a.c:add"].Usage
	if u == nil || len(u.Parameters) != 2 || !u.Parameters[0].Read || u.Parameters[1].Read {
		t.Fatal(u, snapshot.Files["a.c"].Missing)
	}
	values := genericValueTexts(u.Parameters[0])
	if len(values) != 2 || !slices.Contains(values, "2") || !slices.Contains(values, "3") {
		t.Fatal(values, u)
	}
	callback := d["a.c:apply"].Usage.Parameters[0]
	if !callback.Function || !callback.Read || len(callback.Functions) != 1 {
		t.Fatal(callback)
	}
	mutate := d["a.c:mutate"].Usage.Parameters[0]
	if !mutate.Written || len(mutate.Unknown) == 0 {
		t.Fatal(mutate)
	}
	if !snapshot.Files["a.c"].CodeComplete || snapshot.Files["a.c"].Operations["usage"].Complete {
		t.Fatal(snapshot.Files["a.c"].Operations)
	}
}

func TestGenericCPPAbstractionsDefaultsAndFields(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.cpp": `class Reader{public:virtual int read(int value)=0;};
class Real:public Reader{int count=1;public:int read(int value)override{count=value;return count;}};
int read(int value=2){return value;}int run(){read();return read(3);}
`})
	d := snapshot.Declarations()
	reader := d["a.cpp:Reader"].Usage
	if reader == nil || !reader.Abstract || !slices.Contains(reader.Methods, "read") || len(reader.Implementations) != 1 {
		t.Fatal(reader, d)
	}
	real := d["a.cpp:Real"].Usage
	if len(real.Fields) != 1 || !real.Fields[0].Read || !real.Fields[0].Written || len(real.Fields[0].Unknown) == 0 {
		t.Fatal(real)
	}
	values := genericValueTexts(d["a.cpp:read"].Usage.Parameters[0])
	if len(values) != 2 || !slices.Contains(values, "2") || !slices.Contains(values, "3") {
		t.Fatal(values)
	}
}

func TestGenericCSharpExplicitAndStructuralInterfaces(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.cs": `interface Reader{int Read(int value);}
class Real:Reader{int count=1;public int Read(int value){count=value;return count;}int Apply(int value=2){return Read(value);}int Run(){Apply();return Apply(3);}}
class Candidate{int Read(int value){return value;}}
`})
	d := snapshot.Declarations()
	reader := d["a.cs:Reader"].Usage
	if reader == nil || !reader.Abstract || len(reader.Implementations) != 2 {
		t.Fatal(reader)
	}
	if !d["a.cs:Real.Read"].Usage.Parameters[0].Interface {
		t.Fatal(d["a.cs:Real.Read"])
	}
	field := d["a.cs:Real"].Usage.Fields[0]
	values := genericValueTexts(field)
	if !field.Read || !field.Written || len(values) != 3 || !slices.Contains(values, "1") || !slices.Contains(values, "2") || !slices.Contains(values, "3") {
		t.Fatal(field, snapshot.Files["a.cs"].Missing)
	}
}

func TestGenericRubyDefaultsKeywordsAndInstanceFields(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.rb": `def add(value, other: 2)
 value
end
add(1,other: 3)
class Box
 def initialize(value=1)
  @count=value
 end
 def read(value)
  @count+=value
  @count
 end
end
`})
	d := snapshot.Declarations()
	add := d["a.rb:add"].Usage
	if len(add.Parameters) != 2 || !add.Parameters[0].Read || add.Parameters[1].Read || !slices.Contains(genericValueTexts(add.Parameters[1]), "3") {
		t.Fatal(add)
	}
	box := d["a.rb:Box"].Usage
	if len(box.Fields) != 1 || !box.Fields[0].Read || !box.Fields[0].Written || len(box.Fields[0].Unknown) == 0 {
		t.Fatal(box, snapshot.Files["a.rb"].Missing)
	}
}

func TestGenericPHPNamedArgumentsAndProperties(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.php": `<?php interface Reader{function read($value);}class Real implements Reader{private int $count=1;function read($value){$this->count=$value;return $this->count;}}
function add($left=1,$right=2){return $left;}add(right:4,left:3);`})
	d := snapshot.Declarations()
	add := d["a.php:add"].Usage
	if len(add.Parameters) != 2 || !add.Parameters[0].Read || add.Parameters[1].Read || !slices.Contains(genericValueTexts(add.Parameters[0]), "3") || !slices.Contains(genericValueTexts(add.Parameters[1]), "4") {
		t.Fatal(add)
	}
	reader := d["a.php:Reader"].Usage
	if !reader.Abstract || len(reader.Implementations) != 1 {
		t.Fatal(reader)
	}
	field := d["a.php:Real"].Usage.Fields[0]
	if !field.Read || !field.Written || len(field.Unknown) == 0 {
		t.Fatal(field)
	}
}

func TestGenericForwardersAndUnspecifiedCapabilities(t *testing.T) {
	for _, sample := range []struct{ file, source, name, target string }{
		{"a.c", `int forward(int value){return target(value);}`, "forward", "target"},
		{"a.cpp", `int forward(int value){return target(value);}`, "forward", "target"},
		{"a.cs", `class Box{int Forward(int value){return Target(value);}}`, "Box.Forward", "Target"},
		{"a.php", `<?php function forward($value){return target($value);}`, "forward", "target"},
		{"a.rb", "def forward(value)\n target(value)\nend\n", "forward", "target"},
	} {
		t.Run(sample.file, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{sample.file: sample.source})
			file := snapshot.Files[sample.file]
			d := snapshot.Declarations()[sample.file+":"+sample.name]
			if d.Shape == nil || d.Shape.Forwarder != sample.target {
				t.Fatal(d.Shape)
			}
			for _, operation := range []string{"entries", "risks", "tests"} {
				if file.Operations[operation].Complete || file.Operations[operation].Reason == "" {
					t.Fatal(file.Operations)
				}
			}
		})
	}
}

func TestGenericShadowedCallsReferencesAndNamedKeys(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"a.cpp": `int compute(int value){return value;}void modify(int& value){value++;}int run(int input,int unused){{int other=0,unused;consume(unused);}int(*compute)(int)=other;compute(9);modify(input);return 0;}`,
		"a.cs":  `class Box{int Add(int left,int right){return left;}int Run(int left){return Add(right:2,left:1);}}`,
	})
	d := snapshot.Declarations()
	compute := d["a.cpp:compute"].Usage.Parameters[0]
	if len(compute.Values) != 0 || len(compute.Unknown) == 0 {
		t.Fatal(compute)
	}
	run := d["a.cpp:run"].Usage
	if run.Parameters[1].Read || len(run.Parameters[0].Unknown) == 0 {
		t.Fatal(run)
	}
	if d["a.cs:Box.Run"].Usage.Parameters[0].Read {
		t.Fatal(d["a.cs:Box.Run"].Usage)
	}
	add := d["a.cs:Box.Add"].Usage
	if !slices.Equal(genericValueTexts(add.Parameters[0]), []string{"1"}) || !slices.Equal(genericValueTexts(add.Parameters[1]), []string{"2"}) {
		t.Fatal(add)
	}
}

func TestGenericBranchesInheritedImplementationsAndEscapes(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"a.rb":  "def branch(value)\n if value\n  1\n else\n  2\n end\nend\n",
		"a.cs":  `interface Reader{int Read();}class Base:Reader{int Read(){return 1;}}class Child:Base{}`,
		"a.php": `<?php function use_value($value){return $value;}use_value('\x61');use_value('a');`,
	})
	d := snapshot.Declarations()
	shape := d["a.rb:branch"].Shape
	if shape == nil || shape.Simple || len(shape.Branches) == 0 {
		t.Fatal(shape)
	}
	if len(d["a.cs:Reader"].Usage.Implementations) != 2 {
		t.Fatal(d["a.cs:Reader"].Usage)
	}
	value := d["a.php:use_value"].Usage.Parameters[0]
	if len(value.Unknown) == 0 || !slices.Equal(genericValueTexts(value), []string{"a"}) {
		t.Fatal(value)
	}
}

func TestGenericCPPReferenceDeclarationNames(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.cpp": `int value=1;int& alias=value;int&& moved=2;int forward(int& value){return target(value);}`})
	if !snapshot.Files["a.cpp"].CodeComplete {
		t.Fatal(snapshot.Files["a.cpp"].Missing)
	}
	d := snapshot.Declarations()
	for _, name := range []string{"value", "alias", "moved", "forward"} {
		if d["a.cpp:"+name].Name != name {
			t.Fatal(name, d)
		}
	}
	if d["a.cpp:forward"].Shape.Forwarder != "target" {
		t.Fatal(d["a.cpp:forward"].Shape)
	}
}

func TestGenericSimpleFieldsDoNotIncludeNewOrConversion(t *testing.T) {
	for _, sample := range []struct{ file, source, read, other string }{
		{"a.cpp", `class Box{int value;int read(){return value;}int other(int n){return static_cast<int>(n);}};`, "Box.read", "Box.other"},
		{"a.cs", `class Box{int value;int Read(){return value;}object Other(){return new Box();}}`, "Box.Read", "Box.Other"},
		{"a.php", `<?php class Box{private int $value;function read(){return $this->value;}function other(){return new Box();}}`, "Box.read", "Box.other"},
	} {
		t.Run(sample.file, func(t *testing.T) {
			d := scanSources(t, map[string]string{sample.file: sample.source}).Declarations()
			if d[sample.file+":"+sample.read].Shape == nil || !d[sample.file+":"+sample.read].Shape.Simple {
				t.Fatal(d[sample.file+":"+sample.read])
			}
			if d[sample.file+":"+sample.other].Shape == nil || d[sample.file+":"+sample.other].Shape.Simple {
				t.Fatal(d[sample.file+":"+sample.other])
			}
		})
	}
}
