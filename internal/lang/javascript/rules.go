package javascript

import "github.com/Innocent-children/taskbelay/internal/lang"

var Rules = lang.Rules{
	SimpleCalls: []string{"String", "Boolean", "Array.from"}, BranchNodes: []string{"catch_clause"}, BranchOperators: []string{"??"},
	Constructors:      map[string]string{"commander.Command": "commander.Command", "yargs": "yargs.Argv", "yargs/yargs": "yargs.Argv"},
	ConcurrentImports: []string{"worker_threads"},
	ErrorBases:        []string{"Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "AggregateError"},
	ImplicitMethods:   []string{"constructor", "toString", "toJSON"},
	Entries: []lang.CallRule{
		{Target: "commander.Command.option", Kind: "flag"}, {Target: "commander.Command.command", Kind: "cmd"},
		{Target: "yargs.Argv.option", Kind: "flag"}, {Target: "yargs.Argv.options", Kind: "flag"}, {Target: "yargs.Argv.command", Kind: "cmd"},
		{Target: "util.parseArgs", Kind: "flag"},
	},
	Risks: []lang.CallRule{
		{Target: "worker_threads.*", Kind: "concurrency"}, {Target: "global.Worker", Kind: "concurrency"},
		{Target: "global.Promise.all", Kind: "concurrency"}, {Target: "global.Promise.race", Kind: "concurrency"}, {Target: "global.Promise.allSettled", Kind: "concurrency"}, {Target: "global.Promise.any", Kind: "concurrency"},
		{Target: "global.JSON.parse", Kind: "input"}, {Target: "global.parseInt", Kind: "input"}, {Target: "global.parseFloat", Kind: "input"}, {Target: "global.Number", Kind: "input"}, {Target: "global.URL", Kind: "input"}, {Target: "global.Date.parse", Kind: "input"},
		{Target: "child_process.*", Kind: "command"},
	},
}

var FileWrites = []string{"writeFile", "writeFileSync", "appendFile", "appendFileSync", "write", "writeSync", "writev", "writevSync", "copyFile", "copyFileSync", "cp", "cpSync", "truncate", "truncateSync", "ftruncate", "ftruncateSync", "unlink", "unlinkSync", "rm", "rmSync", "rmdir", "rmdirSync", "rename", "renameSync", "mkdir", "mkdirSync", "mkdtemp", "mkdtempSync", "createWriteStream"}

const Tags = `
(variable_declarator name: (_) @binding) @definition.variable
(export_specifier alias: (identifier) @name) @definition.alias
`

const TypeScriptTags = `
(abstract_class_declaration name: (type_identifier) @name) @definition.class
(abstract_method_signature name: (property_identifier) @name) @definition.method
(type_alias_declaration name: (type_identifier) @name) @definition.type
(method_signature name: (property_identifier) @name) @definition.method
(function_signature name: (identifier) @name) @definition.function
`
