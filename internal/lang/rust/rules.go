package rust

import "github.com/Innocent-children/taskbelay/internal/lang"

var Rules = lang.Rules{
	SimpleCalls: []string{"into", "from", "clone"}, BranchNodes: []string{"try_expression", "loop_expression"},
	Entries: []lang.CallRule{
		{Target: "std::env::var", Kind: "env"}, {Target: "std::env::var_os", Kind: "env"},
		{Target: "clap::Arg::new", Kind: "flag"}, {Target: "clap::Arg::long", Kind: "flag"}, {Target: "clap::Arg::env", Kind: "env"},
		{Target: "axum::Router::route", Kind: "route"},
	},
	Risks: []lang.CallRule{
		{Target: "std::thread::*", Kind: "concurrency"}, {Target: "tokio::spawn", Kind: "concurrency"},
		{Target: "std::sync::Mutex::*", Kind: "concurrency"}, {Target: "std::sync::RwLock::*", Kind: "concurrency"}, {Target: "std::sync::atomic::*", Kind: "concurrency"}, {Target: "std::sync::mpsc::*", Kind: "concurrency"},
		{Target: "tokio::sync::Mutex::*", Kind: "concurrency"}, {Target: "tokio::sync::RwLock::*", Kind: "concurrency"}, {Target: "tokio::sync::mpsc::*", Kind: "concurrency"},
		{Target: "std::fs::write", Kind: "write"}, {Target: "std::fs::remove_file", Kind: "write"}, {Target: "std::fs::remove_dir_all", Kind: "write"}, {Target: "std::fs::rename", Kind: "write"}, {Target: "std::fs::create_dir_all", Kind: "write"}, {Target: "std::fs::copy", Kind: "write"}, {Target: "std::fs::File::create", Kind: "write"}, {Target: "std::fs::OpenOptions::*", Kind: "write"},
		{Target: "str::parse", Kind: "input"}, {Target: "std::string::String::parse", Kind: "input"},
		{Target: "serde_json::from_str", Kind: "input"}, {Target: "serde_json::from_slice", Kind: "input"}, {Target: "serde_json::from_reader", Kind: "input"}, {Target: "toml::from_str", Kind: "input"}, {Target: "serde_yaml::from_str", Kind: "input"},
		{Target: "std::process::Command::*", Kind: "command"},
	},
	ReceiverMethods: map[string]string{
		"clap::Arg::new": "clap::Arg", "clap::Command::new": "clap::Command", "axum::Router::new": "axum::Router",
		"clap::Command::get_matches": "clap::ArgMatches", "clap::Command::get_name": "str",
		"std::fs::File::create": "std::fs::File", "std::fs::OpenOptions::new": "std::fs::OpenOptions", "std::process::Command::new": "std::process::Command",
		"std::string::String::new": "std::string::String", "std::string::String::from": "std::string::String", "std::env::var": "std::string::String",
	},
}

var KnownPaths = []string{
	"rstest", "rstest::rstest", "mockall", "mockall::automock", "mockall::mock", "tokio::test",
	"std", "std::env", "std::fs", "std::thread", "std::sync", "std::sync::atomic", "std::sync::mpsc", "std::process",
	"std::env::var", "std::env::var_os", "std::fs::File", "std::fs::OpenOptions", "std::sync::Mutex", "std::sync::RwLock",
	"std::string::String", "std::process::Command", "clap", "clap::Arg", "clap::Command", "clap::Parser", "clap::Args", "clap::Subcommand",
	"axum", "axum::Router", "axum::routing", "tokio", "tokio::sync", "tokio::sync::Mutex", "tokio::sync::RwLock", "tokio::sync::mpsc",
	"actix_web", "serde_json", "serde_yaml", "toml",
}

var HTTPMethods = []string{"get", "post", "put", "delete", "patch", "head", "options", "trace", "connect"}

var BuilderMethods = map[string][]string{
	"clap::Arg":             {"long", "short", "env", "help", "long_help", "required", "action", "value_parser", "num_args", "default_value"},
	"clap::Command":         {"arg", "args", "subcommand", "subcommands", "name", "about", "long_about", "version", "author", "subcommand_required", "arg_required_else_help"},
	"axum::Router":          {"route", "nest", "merge"},
	"std::process::Command": {"arg", "args", "env", "envs", "current_dir", "stdin", "stdout", "stderr"},
	"std::fs::OpenOptions":  {"read", "write", "append", "truncate", "create", "create_new"},
}

const Tags = `
(const_item name: (identifier) @name) @definition.constant
(static_item name: (identifier) @name) @definition.variable
(type_item name: (type_identifier) @name) @definition.type
(use_as_clause alias: (identifier) @name) @definition.type
`
