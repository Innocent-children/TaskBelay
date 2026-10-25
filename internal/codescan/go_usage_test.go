package codescan

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGoInterfacesGenericsErrorsAndCompatibility(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"lib/types.go": `package lib
import "errors"
type Closer interface { Close() error }
type Reader interface { Closer; Read([]byte)(int,error) }
type File struct{}
func (f *File) Close() error {return nil}
func (f *File) Read(p []byte)(int,error){return len(p),nil}
type Partial struct{}
func (p Partial) Read([]byte)(int,error){return 0,nil}
type Fault struct{}
func (f Fault) Error() string {return "fault"}
var ErrBad=errors.New("bad")
// Deprecated: use File.
type Old = File
func Map[T any](item T) T { return item }
func check(err error) {
 var f *Fault
 errors.As(err,&f)
 errors.Is(err,ErrBad)
 if err==ErrBad {}
 if _,ok:=err.(Fault);ok{}
 switch err.(type) {case Fault:}
 Map[int](1)
 Map[string]("x")
 Map(true)
}

func init(){}
`,
		"lib/types_test.go": `package lib
type Fake struct{}
func (f Fake) Close() error{return nil}
func (f Fake) Read([]byte)(int,error){return 0,nil}
`,
		"internal/private/value.go": "package private\nfunc Exported(){}\n",
		"cmd/main.go":               "package main\nfunc Exported(){}\nfunc main(){}\n",
	})
	d := snapshot.Declarations()
	reader := d["lib/types.go:Reader"].Usage
	if reader == nil || !reader.Abstract || !slices.Equal(reader.Methods, []string{"Close", "Read"}) {
		t.Fatal(reader)
	}
	var real, fake []string
	for _, use := range reader.Implementations {
		if use.Test {
			fake = append(fake, use.Value)
		} else {
			real = append(real, use.Value)
		}
	}
	if !slices.Equal(real, []string{"lib/types.go:File"}) || !slices.Equal(fake, []string{"lib/types_test.go:Fake"}) {
		t.Fatal(real, fake)
	}
	if usage := d["lib/types.go:Fault"].Usage; !usage.Error || len(usage.ErrorChecks) != 3 {
		t.Fatal(usage)
	}
	if usage := d["lib/types.go:ErrBad"].Usage; !usage.Sentinel || len(usage.ErrorChecks) != 2 {
		t.Fatal(usage)
	}
	if usage := d["lib/types.go:Map"].Usage; !usage.Generic || !usage.Inferred || len(usage.TypeArguments) != 2 || len(usage.Unknown) == 0 {
		t.Fatal(usage)
	}
	if usage := d["lib/types.go:Old"].Usage; !usage.Deprecated || usage.Alias != "File" || !usage.Public {
		t.Fatal(usage)
	}
	for _, id := range []string{"internal/private/value.go:Exported", "cmd/main.go:Exported"} {
		if d[id].Usage.Public {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"lib/types.go:init", "cmd/main.go:main", "lib/types.go:File.Read", "lib/types.go:Fault.Error"} {
		if len(d[id].Usage.Implicit) == 0 {
			t.Fatal(id)
		}
	}
	if snapshot.Files["lib/types.go"].Operations["usage"].Complete || !snapshot.Files["lib/types.go"].CodeComplete {
		t.Fatal(snapshot.Files["lib/types.go"].Operations)
	}
}

func TestGoParameterAndFieldValueEvidence(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package main
const Limit = 1 + 2
type State struct {
 Flag bool
 Count int
 Callback func(int) int
 Tagged bool ` + "`json:\"flag\"`" + `
}
func callback(n int)int{return n}
func run(flag bool,count int,unused int,fn func(int)int)int {
 if flag {count+=1}
 return fn(count)
}
func shadow(value int)int {
 {value:=2;value++}
 return value
}
func reuse(value int)int { value,other:=2,3; _=other; return value }
func main(){
 state:=State{Flag:true, Count:Limit, Callback:callback, Tagged:true}
 state.Flag=false
 state.Count++
 state.Callback=func(n int)int{return n+1}
 run(true,Limit,7,callback)
 shadow(1)
 reuse(1)
 _=state.Flag
}
`})
	d := snapshot.Declarations()
	fields := d["a.go:State"].Usage.Fields
	if len(fields) != 4 {
		t.Fatal(fields)
	}
	flag := fields[0]
	var values []string
	for _, value := range flag.Values {
		values = append(values, value.Text)
	}
	if !slices.Contains(values, "false") || !slices.Contains(values, "true") || !flag.Read || !flag.Written || len(flag.Unknown) != 0 {
		t.Fatal(flag)
	}
	if len(fields[1].Unknown) == 0 || !fields[1].Read || !fields[1].Written {
		t.Fatal(fields[1])
	}
	if len(fields[2].Functions) != 2 || !fields[2].Function || !fields[3].External {
		t.Fatal(fields)
	}
	parameters := d["a.go:run"].Usage.Parameters
	if len(parameters) != 4 || !parameters[0].Read || parameters[0].Written || parameters[2].Read || parameters[2].Written {
		t.Fatal(parameters)
	}
	if len(parameters[0].Values) != 1 || parameters[0].Values[0].Text != "true" || len(parameters[1].Values) != 1 || parameters[1].Values[0].Text != "3" {
		t.Fatal(parameters)
	}
	if len(parameters[1].Unknown) == 0 || !parameters[3].Function || len(parameters[3].Functions) != 1 {
		t.Fatal(parameters)
	}
	if p := d["a.go:shadow"].Usage.Parameters[0]; !p.Read || p.Written {
		t.Fatal(p)
	}
	if p := d["a.go:reuse"].Usage.Parameters[0]; !p.Read || !p.Written || len(p.Values) != 2 {
		t.Fatal(p)
	}
}

func TestGoUnknownWritesAndInferredFunctionParameters(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package main
import "encoding/json"
type State struct {Flag bool}
func use(fn func(bool),value bool){fn(value)}
func action(value bool){}
func main(){
 s:=State{Flag:true}
 alias:=&s.Flag
 *alias=false
 json.Unmarshal(nil,&s)
 use(action,true)
 callback:=action
 callback(false)
}
`})
	d := snapshot.Declarations()
	flag := d["a.go:State"].Usage.Fields[0]
	if len(flag.Unknown) < 2 {
		t.Fatal(flag)
	}
	if p := d["a.go:action"].Usage.Parameters[0]; len(p.Unknown) == 0 {
		t.Fatal(p)
	}
	if p := d["a.go:use"].Usage.Parameters[0]; !p.Function || len(p.Functions) != 1 {
		t.Fatal(p)
	}
	if snapshot.Files["a.go"].Operations["usage"].Complete {
		t.Fatal(snapshot.Files["a.go"].Operations)
	}
	for _, unknown := range flag.Unknown {
		if unknown.File != "a.go" || unknown.Span.StartLine == 0 || !strings.Contains(unknown.Reason, "Flag") {
			t.Fatal(unknown)
		}
	}
}

func TestGoEmbeddedFieldsFunctionAliasesAndVariadicValues(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package main
type Handler func(int) int
type Base struct{ Flag bool }
type State struct{ *Base; Count int; Handle Handler }
func action(n int)int{return n}
func use(fn Handler){_ = fn(1)}
func many(values ...int){}
func Map[T any](value T)T{return value}
type Box[T any] struct{ Value T }
var _ = 1
func main(){
 state:=State{&Base{Flag:true},3,action}
 state.Flag=false
 state.Handle=action
 use(action)
 many()
 many(1)
 many(1,1)
 args:=[]int{1};many(args...)
 f:=Map[int];_ = f(1)
 _=Box[int]{Value:1}
}
`})
	d := snapshot.Declarations()
	if _, exists := d["a.go:_"]; exists {
		t.Fatal("blank identifier became a declaration")
	}
	base := d["a.go:Base"].Usage.Fields[0]
	if !base.Written || len(base.Values) != 3 || len(base.Unknown) != 0 {
		t.Fatal(base)
	}
	state := d["a.go:State"].Usage.Fields
	if len(state) != 3 || state[0].Name != "Base" || state[0].Values[0].Kind != "nil" || !state[2].Function || len(state[2].Functions) != 2 {
		t.Fatal(state)
	}
	if use := d["a.go:use"].Usage.Parameters[0]; !use.Function || len(use.Functions) != 1 {
		t.Fatal(use)
	}
	many := d["a.go:many"].Usage.Parameters[0]
	if len(many.Values) != 3 || many.Values[0].Kind != "nil" || many.Values[1].Text == many.Values[2].Text || len(many.Unknown) != 1 {
		t.Fatal(many)
	}
	if mapped := d["a.go:Map"].Usage; len(mapped.TypeArguments) != 1 || mapped.Inferred || len(mapped.Parameters[0].Unknown) == 0 {
		t.Fatal(mapped)
	}
	if box := d["a.go:Box"].Usage; len(box.TypeArguments) != 1 || box.TypeArguments[0].Value != "[int]" {
		t.Fatal(box)
	}
}

func TestGoUnknownAggregateAndDynamicReceiver(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package main
type State struct{Flag bool}
func mutate(value any){value.(*State).Flag=false}
func main(){var state State;state=unknown();_=state.Flag}
`})
	field := snapshot.Declarations()["a.go:State"].Usage.Fields[0]
	if len(field.Unknown) != 2 || snapshot.Files["a.go"].Operations["usage"].Complete {
		t.Fatal(field, snapshot.Files["a.go"].Operations)
	}
}

func BenchmarkGoUsage50KLines(b *testing.B) {
	var source strings.Builder
	source.WriteString("package main\n")
	for i := 0; i < 2500; i++ {
		fmt.Fprintf(&source, "func value%d(n int)int {\n", i)
		for j := 0; j < 18; j++ {
			source.WriteString("n += 1\n")
		}
		source.WriteString("return n }\n")
	}
	files := []Source{{Path: "a.go", Data: []byte(source.String()), Mode: "100644"}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(b.Context(), 20*time.Second)
		snapshot := Analyze(ctx, "fixed", files)
		cancel()
		if len(snapshot.Declarations()) != 2500 || !snapshot.Files["a.go"].CodeComplete {
			b.Fatal("analysis did not finish", len(snapshot.Declarations()))
		}
		for _, missing := range snapshot.Missing {
			if strings.Contains(missing.Reason, "deadline") {
				b.Fatal(missing)
			}
		}
	}
}

func TestGoPartialTypeInferenceAndShadowedSentinel(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"lib/generic.go": `package lib
func Pair[A,B any](first A,second B){}
`,
		"lib/use.go": `package lib
import "errors"
var ErrBad=errors.New("bad")
func use(err error){
 Pair[int](1,"x")
 ErrBad:=errors.New("another")
 if errors.Is(err,ErrBad){}
}
`,
	})
	declarations := snapshot.Declarations()
	pair := declarations["lib/generic.go:Pair"].Usage
	if !pair.Inferred || len(pair.TypeParameters) != 2 || len(pair.TypeArguments) != 0 || len(pair.Unknown) != 1 || snapshot.Files["lib/generic.go"].Operations["usage"].Complete {
		t.Fatal(pair, snapshot.Files["lib/generic.go"].Operations)
	}
	if bad := declarations["lib/use.go:ErrBad"].Usage; len(bad.ErrorChecks) != 0 {
		t.Fatal(bad)
	}
}

func TestGoParameterNamesAreNotStructKeysOrRangeBindings(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package main
type State struct{Flag bool}
func keyed(Flag bool)State{return State{Flag:true}}
func shadow(value int){for value:=range []int{1}{_ = value}}
func assigned(value int){for value=range []int{1}{_ = value}}
`})
	declarations := snapshot.Declarations()
	if slot := declarations["a.go:keyed"].Usage.Parameters[0]; slot.Read || slot.Written {
		t.Fatal(slot)
	}
	if slot := declarations["a.go:shadow"].Usage.Parameters[0]; slot.Read || slot.Written {
		t.Fatal(slot)
	}
	if slot := declarations["a.go:assigned"].Usage.Parameters[0]; !slot.Read || !slot.Written {
		t.Fatal(slot)
	}
}
