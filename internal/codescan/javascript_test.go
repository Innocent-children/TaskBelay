package codescan

import (
	"slices"
	"strings"
	"testing"
)

func TestJavaScriptAndTypeScriptDocumentedEntries(t *testing.T) {
	for _, extension := range []string{"js", "ts", "tsx"} {
		t.Run(extension, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{
				"constants." + extension: `export const FLAG = '--mode'; export const ROUTE = '/items';`,
				"a." + extension: `import {FLAG as mode, ROUTE} from './constants';
import {Command, program} from 'commander';
import yargs from 'yargs';
import {parseArgs} from 'node:util';
const name = 'export';
const command = new Command();
command.option('-v, --verbose').command(name + ' <file>');
program.option(mode).command('clear');
yargs.option('limit', {alias:['l', 'maximum']}).command('run <path>');
yargs.options({enabled:{type:'boolean', alias:'e'}});
parseArgs({options:{format:{type:'string', short:'f'}, x:{type:'boolean'}}});
process.env.MODE;
process.env['LIMIT'];
import.meta.env.PUBLIC_URL;
app.get(ROUTE, handler);
router.post('/save', handler);
app.all('/any', handler);
app.route('/group');
const cjs = require('commander');
cjs.option('--common');
const {option: setOption} = require('yargs');
setOption('common', {alias:'c'});
const argv = require('yargs/yargs')([]);
argv.command('inspect');
`,
				"src/pages/api/item." + extension: `export default function handler(req,res){return res;}`,
				"app/orders/route.ts":             `export function GET(){return null;}`,
			})
			var got []string
			for _, file := range snapshot.Files {
				for _, entry := range file.Entries {
					got = append(got, entry.Location())
				}
				if !file.Operations["entries"].Complete {
					t.Fatal(file.Path, file.Missing)
				}
			}
			want := []string{"flag:-v", "flag:--verbose", "cmd:export", "flag:--mode", "cmd:clear", "flag:--limit", "flag:-l", "flag:--maximum", "cmd:run", "flag:--enabled", "flag:-e", "flag:--format", "flag:-f", "flag:--x", "env:MODE", "env:LIMIT", "env:PUBLIC_URL", "route:GET /items", "route:POST /save", "route:/any", "route:/group", "flag:--common", "flag:--common", "flag:-c", "cmd:inspect", "route:src/pages/api/item." + extension, "route:app/orders/route.ts"}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatal(got, want)
			}
		})
	}
}

func TestJavaScriptRisksAndAliasReceivers(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": `import * as fs from 'node:fs';
import {appendFile as append} from 'node:fs/promises';
import {Worker as NodeWorker} from 'node:worker_threads';
import cp from 'node:child_process';
const {writeFile: write} = require('fs');
fs.writeFile('file', data, callback);
fs.promises.rename('old', 'new');
append('log', data);
write('output', data, callback);
fs.readFile('input', callback);
fs.copyFile('from', 'to', callback);
fs.promises.truncate('file');
new Worker('web.js');
new NodeWorker('node.js');
Promise.all([]); Promise.race([]); Promise.allSettled([]); Promise.any([]);
JSON.parse('{}'); parseInt('1'); parseFloat('1'); Number('1'); new URL('https://example.test'); Date.parse('2020');
cp.spawn('cmd');
const child = require('child_process');
child.exec('cmd');
`})
	file := snapshot.Files["a.ts"]
	if !file.Operations["risks"].Complete {
		t.Fatal(file.Missing)
	}
	counts := map[string]int{}
	for _, risk := range file.Risks {
		counts[risk.Kind]++
		if strings.Contains(risk.Evidence, "readFile") {
			t.Fatal(risk)
		}
	}
	for kind, want := range map[string]int{"write": 6, "concurrency": 7, "input": 6, "command": 2} {
		if counts[kind] != want {
			t.Fatal(kind, counts, file.Risks)
		}
	}
}

func TestJavaScriptShadowedDynamicAndMutatedAPIs(t *testing.T) {
	for _, source := range []string{
		`import fs from 'fs'; function run(fs,process,Number,Worker){fs.writeFile('x',data);process.env.SECRET;Number('1');new Worker('x');}`,
		`import {Command} from 'commander'; let program = new Command(); program = other; program.option('--unknown');`,
		`import fs from 'fs'; fs.writeFile = other; fs.writeFile('x',data);`,
		`import {parseArgs} from 'util'; parseArgs({options:{...dynamic}}); process.env[key]; app.get(path, handler);`,
		`const fs = require('fs'); function run(){ fs.writeFile('x',data); let fs = other; }`,
		`function run(Number: (x:string)=>number, process: unknown) { Number('1'); process.env.X; }`,
		`const run = (process, Number) => {Number('1');return process.env.X;};`,
		`import {Command} from 'commander'; const [program] = new Command(); program.option('--fake');`,
		`import fs from 'fs'; fs[key] = other; fs.writeFile('x', data);`,
		`import fs from 'fs'; fs[key]('x', data);`,
		`import fs from 'fs'; fs.writeFile.call(null, 'x', data);`,
		`let env = process.env; env = other; env.SECRET;`,
		`const external = require(name);`,
		`let convert = Number; convert = other; convert('1');`,
		`import {writeFile as save} from 'fs'; save = other; save('x', data);`,
	} {
		t.Run(source, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{"a.ts": source})
			file := snapshot.Files["a.ts"]
			if len(file.Entries) != 0 || len(file.Risks) != 0 || (file.Operations["entries"].Complete && file.Operations["risks"].Complete) {
				t.Fatal(file.Entries, file.Risks, file.Operations, file.Missing)
			}
		})
	}
}

func TestTypeScriptAbstractDeclarationsPreserveOwners(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": `export abstract class Reader { abstract read(value:string):number; concrete(value:string){return value.length;} }`})
	for _, id := range []string{"a.ts:Reader", "a.ts:Reader.read", "a.ts:Reader.concrete"} {
		if _, ok := snapshot.Declarations()[id]; !ok {
			t.Fatal(id, snapshot.Declarations())
		}
	}
	if !snapshot.Files["a.ts"].Operations["declarations"].Complete {
		t.Fatal(snapshot.Missing)
	}
}

func TestJavaScriptUnknownLocalExportsAndLiteralNames(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"a.js":      `import {save} from './bridge'; save('x', data);`,
		"bridge.js": `export * from 'fs';`,
	})
	if file := snapshot.Files["a.js"]; len(file.Risks) != 0 || file.Operations["risks"].Complete {
		t.Fatal(file.Risks, file.Missing)
	}
	known := scanSources(t, map[string]string{"a.js": `import {save} from './bridge'; save('x', data);`, "bridge.js": `export {writeFile as save} from 'fs';`})
	if file := known.Files["a.js"]; len(file.Risks) != 1 || file.Risks[0].Kind != "write" || !file.Operations["risks"].Complete {
		t.Fatal(file.Risks, file.Missing)
	}
	constants := scanSources(t, map[string]string{"config.js": `export const KEY = 'MODE';`, "bridge.js": `export {KEY as NAME} from './config';`, "a.js": `import {NAME} from './bridge'; const {KEY} = require('./config'); process.env[NAME]; process.env[KEY];`})
	if file := constants.Files["a.js"]; len(file.Entries) != 2 || !file.Operations["entries"].Complete {
		t.Fatal(file.Entries, file.Missing)
	}
	for _, test := range []struct{ source, want string }{
		{`process.env['MO\x44E']`, "env:MODE"},
		{"process.env[`MODE`]", "env:MODE"},
		{`process.env['\uD83D\uDE00']`, "env:😀"},
		{`process.env['\u{1F600}']`, "env:😀"},
	} {
		file := scanSources(t, map[string]string{"a.js": test.source}).Files["a.js"]
		if len(file.Entries) != 1 || file.Entries[0].Location() != test.want {
			t.Fatal(test, file.Entries, file.Missing)
		}
	}
	file := scanSources(t, map[string]string{"a.js": "process.env[`MODE_${name}`]"}).Files["a.js"]
	if len(file.Entries) != 0 || file.Operations["entries"].Complete {
		t.Fatal(file.Entries, file.Missing)
	}
}
