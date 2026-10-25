package codescan

import (
	"slices"
	"strings"
	"testing"
)

func TestRustTraitsErrorsVisibilityAliasesAndImplicitUses(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"Cargo.toml": "[package]\nname='sample'\nversion='0.1.0'\n",
		"src/lib.rs": `use std::error::Error as StdError;
pub trait Reader {fn read(&self, input:&str)->String;}
pub struct ReaderImpl {count:u32}
impl Reader for ReaderImpl {fn read(&self,input:&str)->String{input.into()}}
pub(crate) struct Internal;
#[derive(Deserialize)] pub struct Config {pub enabled:bool}
#[derive(Error)] enum Fault {Missing,Other}
struct Failure;
fn check(err:Fault, failure:Failure){match err {Fault::Missing=>{},_=>{}};if let Fault::Other=err{};matches!(err,Fault::Missing|Fault::Other);failure.downcast_ref::<Failure>();}
impl StdError for Failure {}
#[deprecated] pub type Old=ReaderImpl;
pub use ReaderImpl as Legacy;
pub use crate::{ReaderImpl as Ancient,Config};
use ReaderImpl as Private;
#[no_mangle] pub fn exported(value:i32){}
#[export_name="entry"] fn entry(){}
#[cfg(test)] mod tests {struct Fake;impl super::Reader for Fake{fn read(&self,input:&str)->String{input.into()}}}
`,
		"app/Cargo.toml":  "[package]\nname='app'\nversion='0.1.0'\n",
		"app/src/main.rs": "pub struct Application;fn main(){}",
	})
	d := snapshot.Declarations()
	reader := d["src/lib.rs:Reader"].Usage
	if reader == nil || !reader.Abstract || !reader.Public || !slices.Equal(reader.Methods, []string{"read"}) || len(reader.Implementations) != 2 {
		t.Fatal(reader, snapshot.Files["src/lib.rs"].Missing)
	}
	if reader.Implementations[0].Test == reader.Implementations[1].Test {
		t.Fatal(reader.Implementations)
	}
	method := d["src/lib.rs:ReaderImpl.read"].Usage
	if method == nil || len(method.Implicit) != 1 || len(method.Parameters) != 1 || !method.Parameters[0].Interface || !method.Parameters[0].Read {
		t.Fatal(method)
	}
	if d["src/lib.rs:Internal"].Usage.Public || d["app/src/main.rs:Application"].Usage.Public {
		t.Fatal("restricted or application visibility")
	}
	config := d["src/lib.rs:Config"].Usage
	if len(config.Fields) != 1 || !config.Fields[0].External {
		t.Fatal(config)
	}
	fault, failure := d["src/lib.rs:Fault"].Usage, d["src/lib.rs:Failure"].Usage
	if !fault.Error || len(fault.ErrorChecks) != 3 || !failure.Error || len(failure.ErrorChecks) != 1 {
		t.Fatal(fault, failure)
	}
	for _, name := range []string{"Old", "Legacy", "Ancient"} {
		u := d["src/lib.rs:"+name].Usage
		if u == nil || !u.Public || u.Alias == "" || (name == "Old") != u.Deprecated {
			t.Fatal(name, u, d)
		}
	}
	if _, exists := d["src/lib.rs:Private"]; exists {
		t.Fatal("private import is not a compatibility declaration")
	}
	for _, name := range []string{"src/lib.rs:exported", "src/lib.rs:entry", "app/src/main.rs:main"} {
		if len(d[name].Usage.Implicit) == 0 {
			t.Fatal(name, d[name])
		}
	}
	if !snapshot.Files["src/lib.rs"].CodeComplete {
		t.Fatal(snapshot.Files["src/lib.rs"].Missing)
	}
}

func TestRustExplicitGenericGroupsAndInference(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `struct Cache<T>{value:T}
struct Inferred<T>{value:T}
struct Raw<T>{value:T}
fn explicit<T,const N:usize>(value:T)->T{value}
fn inferred<T>(value:T)->T{value}
fn run(){let a:Cache<String>=Cache::<String>{value:String::new()};let b:Cache<u32>=Cache::<u32>{value:1};let c=Inferred{value:1};let d:Raw=dynamic();explicit::<i32,3>(1);inferred(1);}
`})
	d := snapshot.Declarations()
	cache := d["src/lib.rs:Cache"].Usage
	groups := map[string]bool{}
	for _, use := range cache.TypeArguments {
		groups[use.Value] = true
	}
	if !cache.Generic || cache.Inferred || len(groups) != 2 || !groups["<std::string::String>"] || !groups["<u32>"] {
		t.Fatal(cache, snapshot.Files["src/lib.rs"].Missing)
	}
	for _, name := range []string{"Inferred", "Raw", "inferred"} {
		u := d["src/lib.rs:"+name].Usage
		if u == nil || !u.Generic || !u.Inferred || len(u.Unknown) == 0 {
			t.Fatal(name, u)
		}
	}
	explicit := d["src/lib.rs:explicit"].Usage
	if !explicit.Generic || explicit.Inferred || len(explicit.TypeArguments) != 1 || explicit.TypeArguments[0].Value != "<i32,3>" {
		t.Fatal(explicit)
	}
}

func TestRustParameterFieldValuesAndFunctions(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"src/lib.rs": `mod constants;use constants::COUNT;
struct State {count:i32,enabled:bool,marker:char}
struct Pair(i32,bool);
impl State {fn make(count:i32)->Self{Self{count,enabled:false,marker:'_'}} fn set(&mut self,count:i32){self.count=count;} fn read(&self)->i32{self.count} fn bump(&mut self){self.count+=1;}}
fn identity(value:i32)->i32{value}
fn apply(value:i32, callback:fn(i32)->i32, unused:bool)->i32{let unused=0;callback(value)}
fn forwarded(value:i32)->State{State::make(value)}
fn run(){let mut state=forwarded(COUNT);let other=forwarded(2);state.set(7);state.enabled=true;state.read();state.bump();let pair=Pair(4,false);apply(1,identity,true);apply(2,|x|x,false);}
`,
		"src/constants.rs": "pub const COUNT:i32=1;",
	})
	d := snapshot.Declarations()
	state := d["src/lib.rs:State"].Usage
	fields := map[string]Slot{}
	for _, slot := range state.Fields {
		fields[slot.Name] = slot
		for _, value := range slot.Values {
			if value.Implicit {
				t.Fatal("Rust has no implicit zero initialization", slot)
			}
		}
	}
	values := map[string]bool{}
	for _, value := range fields["count"].Values {
		values[value.Text] = true
	}
	if !values["1"] || !values["2"] || !values["7"] || !fields["count"].Read || !fields["count"].Written || len(fields["count"].Unknown) == 0 {
		t.Fatal(fields["count"], snapshot.Files["src/lib.rs"].Missing)
	}
	if len(fields["enabled"].Values) != 2 || len(fields["marker"].Values) != 1 || fields["marker"].Values[0].Text != "'_'" {
		t.Fatal(fields)
	}
	pair := d["src/lib.rs:Pair"].Usage
	if len(pair.Fields) != 2 || len(pair.Fields[0].Values) != 1 || pair.Fields[0].Values[0].Text != "4" {
		t.Fatal(pair)
	}
	apply := d["src/lib.rs:apply"].Usage
	if len(apply.Parameters) != 3 || !apply.Parameters[1].Function || len(apply.Parameters[1].Functions) != 2 || !apply.Parameters[0].Read || apply.Parameters[2].Read {
		t.Fatal(apply)
	}
}

func TestRustAttributeChangesBelongToDeclaration(t *testing.T) {
	before := scanSources(t, map[string]string{"src/lib.rs": "/// text\n#[deprecated]\n#[export_name=\"before\"]\npub fn entry(){}"})
	after := scanSources(t, map[string]string{"src/lib.rs": "/// text\n#[deprecated]\n#[export_name=\"after\"]\npub fn entry(){}"})
	x, y := before.Declarations()["src/lib.rs:entry"], after.Declarations()["src/lib.rs:entry"]
	if x.CodeHash == y.CodeHash || x.LiteralFreeHash != y.LiteralFreeHash || !strings.Contains(x.Text, "#[deprecated]") || strings.TrimSpace(x.Documentation) != "/// text" || x.Span.Start != 0 {
		t.Fatal(x, y)
	}
	plain := scanSources(t, map[string]string{"src/lib.rs": "/// text\npub fn entry(){}"}).Declarations()["src/lib.rs:entry"]
	if plain.Usage.Deprecated || plain.CodeHash == x.CodeHash {
		t.Fatal(plain)
	}
}

func TestRustUnknownMutationsDefaultsAndExternalFields(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `#[derive(Default,Deserialize)] struct Data {value:i32}
struct State {value:i32}
fn run(){let mut state=State{value:1};change(&mut state);let other=State{..state};}
`})
	d := snapshot.Declarations()
	data := d["src/lib.rs:Data"].Usage.Fields[0]
	state := d["src/lib.rs:State"].Usage.Fields[0]
	if !data.External || len(data.Values) != 0 || len(data.Unknown) < 2 || len(state.Unknown) < 2 || snapshot.Files["src/lib.rs"].Operations["usage"].Complete {
		t.Fatal(data, state, snapshot.Files["src/lib.rs"].Missing)
	}
}

func TestRustCompleteUsageOnFixedCrossFileSources(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `mod constants;use constants::VALUE;struct Data{value:i32}fn new(value:i32)->Data{Data{value}}fn read()->i32{let data=new(VALUE);data.value}`,
		"src/constants.rs": "pub const VALUE:i32=4;"})
	file := snapshot.Files["src/lib.rs"]
	if !file.Operations["usage"].Complete {
		t.Fatal(file.Missing)
	}
	field := snapshot.Declarations()["src/lib.rs:Data"].Usage.Fields[0]
	if !field.Read || !field.Written || len(field.Values) != 1 || field.Values[0].Text != "4" {
		t.Fatal(field)
	}
}

func TestRustGenericOwnersBoundsAndTypeParameterShadowing(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `struct Cache<T>{value:T}
impl<T> Cache<T>{fn new(value:T)->Self{Self{value}} fn value(&self)->&T{&self.value}}
struct Inferred<T>{value:T}
impl<T> Inferred<T>{fn new(value:T)->Self{Self{value}}}
struct T;
fn bounded<F:Fn(i32)->i32>(callback:F){}
fn shadow<String>(value:String){value.parse();}
fn run(){let a=Cache::<i32>::new(1);a.value();Inferred::new(2);bounded::<fn(i32)->i32>(|x|x);}
`})
	d := snapshot.Declarations()
	cache := d["src/lib.rs:Cache"].Usage
	if cache.Inferred || len(cache.TypeArguments) == 0 || len(cache.Fields[0].Values) != 1 || cache.Fields[0].Values[0].Text != "1" || !cache.Fields[0].Read {
		t.Fatal(cache, snapshot.Files["src/lib.rs"].Missing)
	}
	if !d["src/lib.rs:Inferred"].Usage.Inferred {
		t.Fatal(d["src/lib.rs:Inferred"].Usage)
	}
	if !d["src/lib.rs:bounded"].Usage.Parameters[0].Function {
		t.Fatal(d["src/lib.rs:bounded"].Usage)
	}
	for _, risk := range snapshot.Files["src/lib.rs"].Risks {
		if strings.Contains(risk.Evidence, "value.parse") {
			t.Fatal(risk)
		}
	}
}

func TestRustCompoundTypeArgumentsAliasesAndNestedFields(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `struct Inner{count:i32}
struct Outer{inner:Inner}
type Old=Inner;
fn choose<T>(){}
fn bounded<F>(callback:F) where F:Fn(i32)->i32{}
fn run(){let old:Old=Inner{count:1};let outer=Outer{inner:old};outer.inner.count=3;let value=outer.inner.count;
choose::<(i32,bool)>();choose::<[i32;3]>();choose::<fn(i32)->i32>();choose::<()>();}
`})
	d := snapshot.Declarations()
	u := d["src/lib.rs:choose"].Usage
	if u.Inferred || len(u.TypeArguments) != 4 {
		t.Fatal(u)
	}
	if !d["src/lib.rs:bounded"].Usage.Parameters[0].Function {
		t.Fatal(d["src/lib.rs:bounded"].Usage)
	}
	field := d["src/lib.rs:Inner"].Usage.Fields[0]
	if !field.Read || !field.Written || len(field.Values) != 2 {
		t.Fatal(field, snapshot.Files["src/lib.rs"].Missing)
	}
}

func TestRustIncompleteSyntaxAndMacroUsageRemainUnknown(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `struct State{value:i32} fn run(){custom!();} fn broken(value:) {`})
	file := snapshot.Files["src/lib.rs"]
	if file.CodeComplete || file.Operations["usage"].Complete {
		t.Fatal(file.Missing, file.Operations)
	}
}
