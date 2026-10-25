package codescan

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRustLongBuilderChainsKeepFactsWithinBudget(t *testing.T) {
	for _, sample := range []struct {
		name     string
		length   int
		complete bool
	}{{"known receiver", 28, true}, {"beyond depth limit", 60, false}} {
		t.Run(sample.name, func(t *testing.T) {
			source := `use clap::Arg; fn run(){let flag=Arg::new("flag")` + strings.Repeat(`.help("description")`, sample.length) + `.long("ready");}`
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			snapshot := Analyze(ctx, "fixed-tree", []Source{{Path: "src/lib.rs", Data: []byte(source), Mode: "100644"}})
			if err := ctx.Err(); err != nil {
				t.Fatalf("builder analysis did not finish: %v", err)
			}
			file := snapshot.Files["src/lib.rs"]
			if file == nil || !file.CodeComplete {
				t.Fatal("fixed source was not parsed", snapshot.Missing)
			}
			var locations []string
			for _, entry := range file.Entries {
				locations = append(locations, entry.Location())
			}
			if slices.Contains(locations, "flag:--ready") != sample.complete || file.Operations["entries"].Complete != sample.complete {
				t.Fatal(locations, file.Operations, file.Missing)
			}
			if !slices.Contains(locations, "flag:flag") {
				t.Fatal("the known inner call lost its entry", locations)
			}
		})
	}
}

func TestRustDocumentedEntriesUseImportsDerivesAndBuilders(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"Cargo.toml": "[package]\nname='example'\nversion='0.1.0'\n",
		"src/lib.rs": `use std::env::{var as env_var,var_os};
use clap::{Arg,Command,Parser,Subcommand};
use axum::{Router,routing::{get,post}};
use actix_web::{get as http_get,post as http_post};
pub const KEY:&str="ROOT";
#[derive(Parser)]
struct Opt {#[arg(long="save",env="SAVE")] output:String,#[clap(long="force")] force:bool}
#[derive(Subcommand)]
enum Cmd {#[command(name="upload")] Upload,#[clap(name="download")] Pull}
#[http_get("/health")]
async fn health(){}
#[http_post(r#"/save"#)]
async fn save(){}
fn run(){let root=env_var(KEY);let other=var_os(r##"OTHER"##);let arg=Arg::new("mode").long("kind").env("ARG_ENV");
let sub=Command::new("sub").about("subcommand");let root=Command::new("root").subcommand(sub).subcommands([Command::new("one"),Command::new("two").name("renamed")]);
let router=Router::new().route("/health",get(health).post(save)).route("/save",post(save));}
mod nested;
mod inline {use super::KEY;pub fn read(){std::env::var(KEY);}}
`,
		"src/nested.rs": `use crate::KEY;pub fn load(){std::env::var(KEY);}`,
	})
	file := snapshot.Files["src/lib.rs"]
	var locations []string
	for _, entry := range file.Entries {
		locations = append(locations, entry.Location())
		if entry.Declaration == "" || entry.Evidence == "" || entry.Span.End <= entry.Span.Start {
			t.Fatal(entry)
		}
	}
	for _, want := range []string{"flag:--save", "flag:--force", "flag:mode", "flag:--kind", "env:SAVE", "env:ROOT", "env:OTHER", "env:ARG_ENV", "cmd:upload", "cmd:download", "cmd:sub", "cmd:one", "cmd:renamed", "route:GET /health", "route:POST /health", "route:POST /save"} {
		if !slices.Contains(locations, want) {
			t.Fatal(want, locations, file.Missing)
		}
	}
	if slices.Contains(locations, "cmd:root") || slices.Contains(locations, "cmd:two") || !file.Operations["entries"].Complete {
		t.Fatal(locations, file.Missing)
	}
	nested := snapshot.Files["src/nested.rs"]
	if len(nested.Entries) != 1 || nested.Entries[0].Name != "ROOT" || !nested.Operations["entries"].Complete {
		t.Fatal(nested.Entries, nested.Missing)
	}
	if !file.CodeComplete || !file.Operations["usage"].Complete {
		t.Fatal(file.Operations, file.Missing)
	}
}

func TestRustDocumentedRisksAndTypedReceivers(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `use std::{fs::{self,File,OpenOptions},sync::{Mutex,RwLock,mpsc}};
fn run(text:&str){std::thread::spawn(||{});tokio::spawn(async{});let mutex=Mutex::new(0);let lock=RwLock::new(0);std::sync::atomic::AtomicUsize::new(0);mpsc::channel();
fs::write("a","x");fs::remove_file("a");fs::remove_dir_all("a");fs::rename("a","b");fs::create_dir_all("a");fs::copy("a","b");File::create("a");OpenOptions::new().write(true).open("a");
text.parse::<u32>();"1".parse::<u32>();String::from("1").parse::<u32>();serde_json::from_str::<u32>("1");serde_json::from_slice::<u32>(b"1");serde_json::from_reader::<_,u32>(text);toml::from_str::<u32>("1");serde_yaml::from_str::<u32>("1");
std::process::Command::new("echo").arg("x").status();}
`})
	file := snapshot.Files["src/lib.rs"]
	kinds := map[string]bool{}
	evidence := map[string]bool{}
	for _, risk := range file.Risks {
		kinds[risk.Kind] = true
		evidence[risk.Evidence] = true
	}
	if len(kinds) != 4 || !file.Operations["risks"].Complete {
		t.Fatal(kinds, file.Missing)
	}
	for _, want := range []string{`fs::write("a","x")`, `File::create("a")`, `text.parse::<u32>()`, `String::from("1").parse::<u32>()`, `serde_json::from_reader::<_,u32>(text)`, `std::process::Command::new("echo")`} {
		if !evidence[want] {
			t.Fatal(want, file.Risks)
		}
	}
}

func TestRustShadowingDefaultNamesDynamicBuildersAndMacros(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `use clap::{Arg,Command,Parser,Subcommand};
use axum::{Router,routing::get};
use std::env::var;
#[derive(Parser)] struct Opt{#[arg(long,env)] dry_run:bool}
#[derive(Subcommand)]enum Cmd{Upload}
struct Fake;impl Fake {fn route(self,path:&str,handler:i32)->Self{self}fn parse(self)->i32{0}}
fn run(name:&str){let var=|x|x;var("SHADOWED");Fake{}.route("/fake",0);Fake{}.parse();std::env::var(name);let arg=Arg::new(name);let command=Command::new("root").subcommand(dynamic());let router=Router::new().nest("/v1",Router::new().route("/x",get(handler)));assert!(std::fs::write("x","x").is_ok());custom!();}
#[cfg(unix)] fn platform(){std::env::var("UNIX");}
`})
	file := snapshot.Files["src/lib.rs"]
	for _, entry := range file.Entries {
		if entry.Name == "SHADOWED" || entry.Name == "/fake" || entry.Name == "Upload" || entry.Name == "dry_run" || entry.Name == "root" {
			t.Fatal(entry)
		}
	}
	for _, risk := range file.Risks {
		if strings.Contains(risk.Evidence, "Fake") {
			t.Fatal(risk)
		}
	}
	reasons := []string{}
	for _, missing := range file.Missing {
		reasons = append(reasons, missing.Reason)
	}
	for _, want := range []string{"默认命名", "字符串常量", "子命令", "嵌套", "宏调用", "条件属性"} {
		if !strings.Contains(strings.Join(reasons, "\n"), want) {
			t.Fatal(want, reasons)
		}
	}
	if file.Operations["entries"].Complete || file.Operations["risks"].Complete || !file.CodeComplete {
		t.Fatal(file.Operations, file.Missing)
	}
}

func TestRustRawStringsAliasesGlobImportsAndUnknownSources(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"src/lib.rs": `use std::env::*;use clap as cli;use actix_web::get;
const KEY:&str="R\u{4f}_\x4f\0T";
#[derive(cli::Parser)]struct Opt{#[arg(long=r##"raw-key"##)] name:String}
#[get("/a\
  /b")]fn handler(){}
fn run(){var(KEY);var_os(r#"raw\key"#);let call=std::env::var;call("ALIAS");}
`,
		"src/unknown.rs": `use external::*;fn run(){var("UNKNOWN");Arg::new("unknown");value.parse();}`,
		"src/shadow.rs":  `mod std{pub mod env{pub fn var(value:&str){}}}use std::env::var;fn run(){var("LOCAL");}`,
	})
	file := snapshot.Files["src/lib.rs"]
	var entries []string
	for _, entry := range file.Entries {
		entries = append(entries, entry.Location())
	}
	for _, want := range []string{"flag:--raw-key", "route:GET /a/b", "env:RO_O\x00T", `env:raw\key`, "env:ALIAS"} {
		if !slices.Contains(entries, want) {
			t.Fatal(want, entries, file.Missing)
		}
	}
	unknown := snapshot.Files["src/unknown.rs"]
	if len(unknown.Entries) != 0 || unknown.Operations["entries"].Complete || unknown.Operations["risks"].Complete {
		t.Fatal(unknown.Entries, unknown.Missing)
	}
	if len(snapshot.Files["src/shadow.rs"].Entries) != 0 {
		t.Fatal(snapshot.Files["src/shadow.rs"].Entries)
	}
}

func TestRustMutableBorrowDoesNotKeepOldCommandName(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `use clap::Command;
fn run(){let mut sub=Command::new("old");change(&mut sub);Command::new("root").subcommand(sub);
let mut other=Command::new("before");let alias=&mut other;change(alias);Command::new("root").subcommand(other);}
`})
	file := snapshot.Files["src/lib.rs"]
	if len(file.Entries) != 0 || file.Operations["entries"].Complete {
		t.Fatal(file.Entries, file.Operations, file.Missing)
	}
}

func TestRustSimpleCallsAndAdditionalBranches(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `fn copy(value:String)->String{value.clone()}
fn branch(value:Option<i32>)->Option<i32>{let n=value?;loop{if n>0{return Some(n)}}}
`})
	d := snapshot.Declarations()
	if shape := d["src/lib.rs:copy"].Shape; shape == nil || !shape.Simple {
		t.Fatal(shape)
	}
	if shape := d["src/lib.rs:branch"].Shape; shape == nil || shape.Simple || len(shape.Branches) < 3 {
		t.Fatal(shape)
	}
}

func TestRustBuildersStopAtGettersAndKeepImportEvidenceLocal(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `use clap::{Arg,Command};use std::sync::atomic::*;use std::process::*;
fn run(){let matches=clap::Command::new("root").get_matches();matches.subcommand();let value=Arg::new("id").get_long();value.long("NOT_A_FLAG");clap::Command::new("root").about("description").subcommand(clap::Command::new("child"));}
`})
	file := snapshot.Files["src/lib.rs"]
	for _, entry := range file.Entries {
		if entry.Name == "NOT_A_FLAG" || entry.Name == "root" {
			t.Fatal(entry)
		}
	}
	child := false
	for _, entry := range file.Entries {
		child = child || entry.Location() == "cmd:child"
	}
	if !child {
		t.Fatal(file.Entries, file.Missing)
	}
	for _, risk := range file.Risks {
		if strings.Contains(risk.Evidence, "use clap") {
			t.Fatal("unrelated import used as risk evidence", risk)
		}
	}
	if file.Operations["entries"].Complete {
		t.Fatal("unresolved getter treated as builder")
	}
}

func TestRustPatternScopesShadowImportedFunctions(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"src/lib.rs": `use std::env::var;
struct Holding<T>{var:T}
fn run(){for var in [|x|x]{var("LOOP");}if let Some(var)=Some(|x|x){var("IF");}else{var("ELSE");}match Some(|x|x){Some(var)=>{var("MATCH");},None=>{}}
let Holding{var}=Holding{var:|x|x};var("STRUCT");}
fn repeated(){let var=|x|x;let var=var;var("SHADOW");}
`})
	file := snapshot.Files["src/lib.rs"]
	var names []string
	for _, entry := range file.Entries {
		names = append(names, entry.Name)
	}
	if !slices.Equal(names, []string{"ELSE"}) {
		t.Fatal(names, file.Missing)
	}
}
