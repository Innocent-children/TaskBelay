package codescan

import (
	"slices"
	"strings"
	"testing"
)

func TestTypeScriptAbstractionErrorsExportsAndCompatibility(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"package.json": `{"name":"library"}`,
		"src/types.ts": `export interface Reader { read(value: string): number; }
export abstract class Base { abstract save(value: string): void; }
export class Concrete extends Base implements Reader {
 @mapped enabled = false;
 constructor(public count = 1) {super();}
 read(value: string){return value.length;}
 save(value: string){}
 toString(){return 'concrete';}
 toJSON(){return {};}
}
const object: Reader = {read(value: string){return value.length;}};
export class Fault extends TypeError {}
export class Specific extends Fault {}
function check(error: unknown){if(error instanceof Fault){}; if(error instanceof Specific){};}
/** @deprecated use Concrete */
export class OldClass {}
export const Old = Concrete;
export {Concrete as Compat};
export type OldReader = Reader;
function main(value=1){return value;}
export default main;
class Internal {constructor(value=1){}}
export const Alias = Internal;
`,
		"tests/types.test.ts": `import {Reader} from '../src/types'; class Fake implements Reader {read(value:string){return 0;}}`,
		"pages/view.ts":       `export const render = (value=1) => value;`,
	})
	d := snapshot.Declarations()
	reader := d["src/types.ts:Reader"].Usage
	if reader == nil || !reader.Abstract || !slices.Equal(reader.Methods, []string{"read"}) {
		t.Fatal(reader)
	}
	var real, fake []Use
	for _, use := range reader.Implementations {
		if use.Test {
			fake = append(fake, use)
		} else {
			real = append(real, use)
		}
	}
	if len(real) != 2 || len(fake) != 1 || fake[0].Value != "tests/types.test.ts:Fake" {
		t.Fatal(real, fake, snapshot.Missing)
	}
	base := d["src/types.ts:Base"].Usage
	if len(base.Implementations) != 1 || base.Implementations[0].Value != "src/types.ts:Concrete" {
		t.Fatal(base)
	}
	for _, name := range []string{"Concrete.read", "Concrete.save", "Reader.read"} {
		if !d["src/types.ts:"+name].Usage.Parameters[0].Interface {
			t.Fatal(name, d["src/types.ts:"+name].Usage)
		}
	}
	for _, name := range []string{"Fault", "Specific"} {
		u := d["src/types.ts:"+name].Usage
		if !u.Error || len(u.ErrorChecks) != 1 {
			t.Fatal(name, u)
		}
	}
	for name, want := range map[string]string{"Old": "Concrete", "Compat": "Concrete", "OldReader": "Reader"} {
		u := d["src/types.ts:"+name].Usage
		if u == nil || u.Alias != want || !u.Public {
			t.Fatal(name, u)
		}
	}
	if !d["src/types.ts:OldClass"].Usage.Deprecated {
		t.Fatal("JSDoc marker lost")
	}
	for _, name := range []string{"Concrete.constructor", "Concrete.toString", "Concrete.toJSON", "main"} {
		if len(d["src/types.ts:"+name].Usage.Implicit) == 0 {
			t.Fatal(name)
		}
	}
	if len(d["pages/view.ts:render"].Usage.Implicit) == 0 || !d["src/types.ts:Internal.constructor"].Usage.Parameters[0].Public {
		t.Fatal("exported values lost their callable contract")
	}
	fields := d["src/types.ts:Concrete"].Usage.Fields
	if len(fields) != 2 || !fields[0].External || fields[1].Name != "count" || len(fields[1].Values) != 1 || fields[1].Values[0].Text != "1" {
		t.Fatal(fields)
	}
	private := scanSources(t, map[string]string{"package.json": `{"private":true}`, "a.ts": `export class State { constructor(value=1){} } export default function run(value=1){return value;}`})
	for _, declaration := range private.Declarations() {
		if declaration.Usage.Public {
			t.Fatal(declaration.ID)
		}
	}
}

func TestTypeScriptExplicitGenericGroupsAndInference(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": `interface Cache<T> {get(value:T):T;}
class Box<T> { value:T; constructor(value:T){this.value=value;} }
function convert<T>(value:T):T{return value;}
function pair<A,B>(left:A,right:B){return left;}
const object: Cache<string> = {get(value){return value;}};
const inferred: Cache = other;
const stringBox = new Box<string>('a');
const numberBox = new Box<number>(1);
convert<string>('a');
convert<number>(1);
const stringConvert = convert<string>;
stringConvert('b');
convert(true);
pair<string>('x',1);
`})
	d := snapshot.Declarations()
	groups := func(u *Usage) []string {
		t.Helper()
		var values []string
		for _, use := range u.TypeArguments {
			values = append(values, use.Value)
		}
		slices.Sort(values)
		return slices.Compact(values)
	}
	box := d["a.ts:Box"].Usage
	if !box.Generic || box.Inferred || !slices.Equal(groups(box), []string{"<number>", "<string>"}) {
		t.Fatal(box)
	}
	convert := d["a.ts:convert"].Usage
	if !convert.Generic || !convert.Inferred || len(convert.Unknown) == 0 || !slices.Equal(groups(convert), []string{"<number>", "<string>"}) {
		t.Fatal(convert)
	}
	if cache := d["a.ts:Cache"].Usage; !cache.Inferred || len(cache.Implementations) != 1 {
		t.Fatal(cache)
	}
	if pair := d["a.ts:pair"].Usage; !pair.Inferred || len(pair.TypeArguments) != 0 {
		t.Fatal(pair)
	}
}

func TestJavaScriptParameterFieldsDefaultsAndCallValues(t *testing.T) {
	for _, extension := range []string{"js", "ts"} {
		t.Run(extension, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{"a." + extension: `function identity(value){return value;}
function plus(value){return value+1;}
class State {
 enabled = false;
 count;
 callback;
 constructor(count=1, callback=identity){this.count=count;this.callback=callback;}
 run(value=2){return this.callback(value)+this.count;}
}
function invoke(flag=false,count=2,fn=identity,unused){if(flag){count++;}return fn(count);}
function variadic(...items){return items;}
const convert = (value=1) => value;
const state = new State(3,plus);
state.count = 4;
state.enabled = true;
state.run(5);
invoke(true,3,plus);
invoke();
variadic();variadic(1);variadic(1,1);
convert(2);
`})
			d := snapshot.Declarations()
			prefix := "a." + extension + ":"
			field := func(name string) Slot {
				t.Helper()
				for _, slot := range d[prefix+"State"].Usage.Fields {
					if slot.Name == name {
						return slot
					}
				}
				t.Fatalf("missing %s: %+v", name, d[prefix+"State"].Usage)
				return Slot{}
			}
			values := func(slot Slot) []string {
				t.Helper()
				var got []string
				for _, v := range slot.Values {
					got = append(got, v.Text)
				}
				slices.Sort(got)
				return slices.Compact(got)
			}
			if count := field("count"); !count.Read || !count.Written || !slices.Equal(values(count), []string{"1", "3", "4", "undefined"}) {
				t.Fatal(count)
			}
			if enabled := field("enabled"); !slices.Equal(values(enabled), []string{"false", "true"}) {
				t.Fatal(enabled)
			}
			if callback := field("callback"); !callback.Function || !callback.Read || len(callback.Functions) != 2 {
				t.Fatal(callback)
			}
			parameters := d[prefix+"invoke"].Usage.Parameters
			if len(parameters) != 4 || !parameters[0].Read || !parameters[1].Written || len(parameters[1].Unknown) == 0 || !parameters[2].Function || len(parameters[2].Functions) != 2 || parameters[3].Read || !slices.Equal(values(parameters[3]), []string{"undefined"}) {
				t.Fatal(parameters)
			}
			if slot := d[prefix+"variadic"].Usage.Parameters[0]; !slot.Variadic || len(slot.Values) != 3 || len(values(slot)) != 3 {
				t.Fatal(slot)
			}
			if slot := d[prefix+"convert"].Usage.Parameters[0]; !slot.Read || !slices.Equal(values(slot), []string{"1", "2"}) {
				t.Fatal(slot)
			}
		})
	}
}

func TestJavaScriptUncertainWritesScopesAndOverloads(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": `function unused(value){ {let value=2;return value;} }
function closure(value){ function read(){return value;} return read(); }
function nested(value){ function read(value){return value;} return read(2); }
function update(value){value += 1; return value;}
function use(value=1){return value;}
function overload(value:string):string;
function overload(value:number):number;
function overload(value:any){return value;}
class State { count=1; }
const state = new State();
state.count++;
delete state.count;
unknown(state);
state[key]=2;
use(...args);
overload(1);
`})
	d := snapshot.Declarations()
	for name, read := range map[string]bool{"unused": false, "closure": true, "nested": false} {
		if slot := d["a.ts:"+name].Usage.Parameters[0]; slot.Read != read {
			t.Fatal(name, slot)
		}
	}
	if len(d["a.ts:update"].Usage.Parameters[0].Unknown) == 0 || len(d["a.ts:use"].Usage.Parameters[0].Unknown) == 0 || len(d["a.ts:State"].Usage.Fields[0].Unknown) < 3 {
		t.Fatal(snapshot.Missing)
	}
	count := 0
	for id, decl := range d {
		if strings.HasPrefix(id, "a.ts:overload") {
			count++
			if len(decl.Usage.Parameters[0].Unknown) == 0 {
				t.Fatal(id, decl.Usage)
			}
		}
	}
	if count != 3 {
		t.Fatal(count, d)
	}
	if snapshot.Files["a.ts"].Operations["usage"].Complete || !snapshot.Files["a.ts"].CodeComplete {
		t.Fatal(snapshot.Files["a.ts"].Operations)
	}
}

func TestJavaScriptUsageAcrossFilesAliasesAndNestedBlocks(t *testing.T) {
	for _, extension := range []string{"js", "ts"} {
		t.Run(extension, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{
				"values." + extension: `export function original(value=1){return value;}
export function changed(value=1){return value;}
export class Fault extends TypeError {}
export class Base { count=1; read(value=2){return this.count+value;} }
`,
				"use." + extension: `import {original as imported,changed as renamed,Fault,Base} from './values';
const alias=imported;
alias(3);
renamed(4);renamed=other;
class Specific extends Fault {}
function check(e){if(e instanceof Specific){} if(e instanceof Fault){}}
class Derived extends Base {}
const state=new Derived();state.count=3;state.read(4);
function nested(value){if(value){let hidden=1;function inner(){return hidden;} value++;}return value;}
function shadow(value){if(true){let value=1;value++;}return 1;}
function undefinedValue(undefined=7,other=undefined){return undefined+other;}
undefinedValue(2,3);
const Store=class { #secret=1; ['visible']=false; read(){return this.#secret+this.visible;} };
const store=new Store();store.visible=true;
`,
			})
			d := snapshot.Declarations()
			valuesPrefix, usePrefix := "values."+extension+":", "use."+extension+":"
			original := d[valuesPrefix+"original"].Usage.Parameters[0]
			if len(original.Unknown) != 0 || len(original.Values) != 2 || original.Values[1].Text != "3" {
				t.Fatal(original)
			}
			if len(d[valuesPrefix+"changed"].Usage.Parameters[0].Unknown) == 0 {
				t.Fatal("renamed import lost its unresolved call evidence")
			}
			for _, name := range []string{valuesPrefix + "Fault", usePrefix + "Specific"} {
				if u := d[name].Usage; !u.Error || len(u.ErrorChecks) != 1 {
					t.Fatal(name, u)
				}
			}
			if slot := d[valuesPrefix+"Base"].Usage.Fields[0]; !slot.Read || len(slot.Values) != 2 || slot.Values[1].Text != "3" {
				t.Fatal(slot)
			}
			if slot := d[valuesPrefix+"Base.read"].Usage.Parameters[0]; len(slot.Values) != 2 || slot.Values[1].Text != "4" {
				t.Fatal(slot)
			}
			if slot := d[usePrefix+"nested"].Usage.Parameters[0]; !slot.Read || !slot.Written || len(slot.Unknown) == 0 {
				t.Fatal(slot)
			}
			if slot := d[usePrefix+"shadow"].Usage.Parameters[0]; slot.Read || slot.Written {
				t.Fatal(slot)
			}
			for id := range d {
				if strings.HasSuffix(id, ":hidden") || strings.HasSuffix(id, ":inner") {
					t.Fatal("local definition escaped its function", id)
				}
			}
			params := d[usePrefix+"undefinedValue"].Usage.Parameters
			if len(params) != 2 || !params[0].Read || len(params[1].Unknown) == 0 {
				t.Fatal(params)
			}
			for _, value := range params[1].Values {
				if value.Kind == "undefined" {
					t.Fatal("shadowed undefined was treated as the global value", params)
				}
			}
			fields := d[usePrefix+"Store"].Usage.Fields
			if len(fields) != 2 || !fields[0].Read || !fields[1].Read || len(fields[1].Values) != 2 || d[usePrefix+"Store.read"].ID == "" {
				t.Fatal(fields)
			}
		})
	}
}

func TestTypeScriptFunctionTypesAndExplicitGenericAliases(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": `interface Callable {(value:number):number;}
type Handler=(value:number)=>number;
function identity(value){return value;}
function first(fn:Handler){}
function second(fn:Callable){}
function third(fn:Function){}
first(identity);second(value=>value+1);third(Number);
function generic<T>(value:T){return value;}
const alias=generic;
alias<string>('a');
const specialized=generic<number>;
specialized(1);
class Box<T>{constructor(value:T){}}
const BoxAlias=Box;
new BoxAlias<string>('a');
`})
	d := snapshot.Declarations()
	for _, name := range []string{"first", "second", "third"} {
		slot := d["a.ts:"+name].Usage.Parameters[0]
		if !slot.Function || len(slot.Functions) != 1 || len(slot.Unknown) != 0 {
			t.Fatal(name, slot)
		}
	}
	for _, name := range []string{"generic", "Box"} {
		if usage := d["a.ts:"+name].Usage; usage.Inferred || len(usage.TypeArguments) == 0 {
			t.Fatal(name, usage)
		}
	}
	if slot := d["a.ts:generic"].Usage.Parameters[0]; len(slot.Unknown) != 0 || len(slot.Values) != 2 {
		t.Fatal(slot)
	}
}

func TestJavaScriptLiteralKindsAndIndirectObjectUpdates(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": `function numeric(value=-1){return value;}
numeric(1);numeric(1.0);numeric(0x1);numeric(9007199254740993);numeric(9007199254740992);numeric(1n);numeric(-1n);numeric(-0);
class Inner {count=1;}
class Outer {inner:Inner; constructor(inner:Inner){this.inner=inner;} read(){return this.inner.count;} }
const outer=new Outer(new Inner());outer.inner.count=2;
class Escape { count=1; read(value=1){return this.count+value;} }
function factory(){return new Escape();}
class Mutation {read(value=1){return value;}}
const instance=new Mutation();instance.read=replacement;instance.read(2);
class Local {
 count=1;
 read(){const obj={method(){this.count=2;}};function inner(){this.count=3;}class Hidden {count=4;}return this.count;}
}
class Collision {count=1;static count=2;}
`})
	d := snapshot.Declarations()
	values := map[string]bool{}
	for _, value := range d["a.ts:numeric"].Usage.Parameters[0].Values {
		values[value.Kind+":"+value.Text] = true
	}
	for _, want := range []string{"Number:-1", "Number:1", "Number:9.007199254740992e+15", "Number:-0", "BigInt:1", "BigInt:-1"} {
		if !values[want] {
			t.Fatal(want, values)
		}
	}
	if len(values) != 6 {
		t.Fatal(values)
	}
	if slot := d["a.ts:Inner"].Usage.Fields[0]; !slot.Read || len(slot.Values) != 2 || slot.Values[1].Text != "2" {
		t.Fatal(slot)
	}
	if len(d["a.ts:Escape"].Usage.Fields[0].Unknown) == 0 || len(d["a.ts:Escape.read"].Usage.Parameters[0].Unknown) == 0 || len(d["a.ts:Mutation.read"].Usage.Parameters[0].Unknown) == 0 {
		t.Fatal("escaped objects or overwritten methods lost uncertainty")
	}
	if slot := d["a.ts:Local"].Usage.Fields[0]; len(slot.Values) != 1 || slot.Values[0].Text != "1" {
		t.Fatal(slot)
	}
	if slot := d["a.ts:Collision"].Usage.Fields[0]; len(slot.Unknown) == 0 {
		t.Fatal(slot)
	}
}
