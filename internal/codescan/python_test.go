package codescan

import (
	"slices"
	"strings"
	"testing"
)

func TestPythonAbstractionsErrorsAndDeclarationExceptions(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"pyproject.toml": "[project]\nname='example'\nversion='1'\n",
		"src/pkg/types.py": `from abc import ABC, abstractmethod
from typing import Protocol
from pydantic import BaseModel
from pydantic_settings import BaseSettings
import functools
import warnings
class Reader(ABC):
 @abstractmethod
 def read(self, amount): ...
class Concrete(Reader):
 def read(self, amount): return amount
class Pending(Reader): pass
class Shape(Protocol):
 def save(self, value): ...
class Structural:
 def save(self, value): return value
class Fault(Exception): pass
class Specific(Fault): pass
class Model(BaseModel):
 name: str
class Config(BaseSettings):
 enabled: bool = False
class NextConfig(Config):
 count: int = 1
class Plain:
 def __len__(self): return 1
 @staticmethod
 def static(value): return value
 @classmethod
 def make(cls, value): return value
 @property
 def name(self): return "name"
 @functools.cache
 def cached(self): return 1
 @custom
 def routed(self): return 1
def old():
 warnings.warn("use new", category=DeprecationWarning)
@deprecated
def obsolete(): pass
def _private(): pass
Old = Concrete
def check(error):
 try: old()
 except (Fault, Specific): pass
 try: old()
 except Specific as failure: pass
 return isinstance(error, Specific)
`,
		"tests/test_types.py": `from pkg.types import Reader
class Fake(Reader):
 def read(self, amount): return amount
`,
	})
	d := snapshot.Declarations()
	reader := d["src/pkg/types.py:Reader"].Usage
	if reader == nil || !reader.Abstract || !slices.Equal(reader.Methods, []string{"read"}) {
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
	if !slices.Equal(real, []string{"src/pkg/types.py:Concrete"}) || !slices.Equal(fake, []string{"tests/test_types.py:Fake"}) {
		t.Fatal(real, fake, snapshot.Missing)
	}
	if !d["src/pkg/types.py:Pending"].Usage.Abstract {
		t.Fatal("inherited abstract method lost")
	}
	shape := d["src/pkg/types.py:Shape"].Usage
	if len(shape.Implementations) != 1 || shape.Implementations[0].Value != "src/pkg/types.py:Structural" {
		t.Fatal(shape)
	}
	if !d["src/pkg/types.py:Concrete.read"].Usage.Parameters[0].Interface {
		t.Fatal("implementation signature lost")
	}
	for name, checks := range map[string]int{"Fault": 1, "Specific": 3} {
		u := d["src/pkg/types.py:"+name].Usage
		if !u.Error || len(u.ErrorChecks) != checks {
			t.Fatalf("%s: %+v", name, u)
		}
	}
	for _, name := range []string{"Model", "Config", "NextConfig"} {
		fields := d["src/pkg/types.py:"+name].Usage.Fields
		if len(fields) != 1 || !fields[0].External {
			t.Fatalf("%s: %+v", name, fields)
		}
	}
	for _, name := range []string{"old", "obsolete"} {
		if !d["src/pkg/types.py:"+name].Usage.Deprecated {
			t.Fatal(name)
		}
	}
	if old := d["src/pkg/types.py:Old"].Usage; old.Alias != "Concrete" || !old.Public {
		t.Fatal(old)
	}
	for _, name := range []string{"Plain.__len__", "Plain.routed"} {
		if len(d["src/pkg/types.py:"+name].Usage.Implicit) == 0 {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"Plain.static", "Plain.make", "Plain.name", "Plain.cached"} {
		if len(d["src/pkg/types.py:"+name].Usage.Implicit) != 0 {
			t.Fatal(name)
		}
	}
	if d["src/pkg/types.py:_private"].Usage.Public || d["src/pkg/types.py:Config.enabled"].Usage.Public || !d["src/pkg/types.py:Plain.static"].Usage.Public {
		t.Fatal("public scope differs from package rule")
	}
}

func TestPythonParameterFieldsAndInvocationValues(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"constants.py": "LIMIT = 3\n",
		"a.py": `from constants import LIMIT
from typing import Callable
def plus(value): return value + 1
def identity(value): return value
class State:
 enabled: bool = False
 count: int
 def __init__(self, count=LIMIT, callback: Callable=plus):
  self.count = count
  self.callback = callback
 def compute(self, value=1): return self.callback(value) + self.count
 @staticmethod
 def static(value): return value
 @classmethod
 def make(cls, value): return value
def invoke(flag=False, count=LIMIT, *, callback=plus, unused=0):
 if flag: count += 1
 return callback(count)
def variadic(*items, **options): return items, options
def positional(value, /, *, named=1): return value, named
state = State(count=4, callback=identity)
state.count = 5
state.enabled = True
state.compute(value=2)
State.compute(state, 3)
State.static(4)
State.make(5)
invoke(True, callback=identity)
invoke(count=4)
variadic(1, 1, a=2)
variadic(1, a=2, b=2)
positional(2, named=3)
`,
	})
	d := snapshot.Declarations()
	state := d["a.py:State"].Usage
	field := func(name string) Slot {
		t.Helper()
		for _, f := range state.Fields {
			if f.Name == name {
				return f
			}
		}
		t.Fatalf("missing %s: %+v", name, state)
		return Slot{}
	}
	count := field("count")
	var counts []string
	for _, value := range count.Values {
		counts = append(counts, value.Text)
	}
	slices.Sort(counts)
	if !slices.Equal(counts, []string{"3", "4", "5"}) || !count.Read || !count.Written {
		t.Fatal(count)
	}
	callback := field("callback")
	if !callback.Function || !callback.Read || len(callback.Functions) != 2 {
		t.Fatal(callback)
	}
	parameters := d["a.py:invoke"].Usage.Parameters
	if len(parameters) != 4 || !parameters[0].Read || !parameters[1].Written || len(parameters[1].Unknown) == 0 || !parameters[2].Function || len(parameters[2].Functions) != 2 || parameters[3].Read {
		t.Fatal(parameters)
	}
	for name, want := range map[string][]string{"State.compute": {"1", "2", "3"}, "State.static": {"4"}, "State.make": {"5"}} {
		u := d["a.py:"+name].Usage.Parameters
		if len(u) != 1 {
			t.Fatal(name, u)
		}
		var got []string
		for _, v := range u[0].Values {
			got = append(got, v.Text)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatal(name, got, want)
		}
	}
	for _, slot := range d["a.py:variadic"].Usage.Parameters {
		if !slot.Variadic || len(slot.Values) != 2 || slot.Values[0].Text == slot.Values[1].Text {
			t.Fatal(slot)
		}
	}
	if p := d["a.py:positional"].Usage.Parameters; len(p) != 2 || len(p[0].Unknown) != 0 || len(p[1].Values) != 2 {
		t.Fatal(p)
	}
}

func TestPythonAPIsUseImportsBindingsAndLiteralNames(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"api.py": `import argparse as ap
import click
import os as env
from pathlib import Path
from pydantic_settings import BaseSettings
import asyncio
import threading
import concurrent.futures
import json, yaml, tomllib, sqlite3, subprocess, shutil
from datetime import datetime, date
from urllib.parse import urlparse
FLAG = "--" + "mode"
parser = ap.ArgumentParser()
parser.add_argument(FLAG, "-m")
parser.add_argument("input")
sub = parser.add_subparsers()
sub.add_parser("export")
@click.group()
def cli(): pass
@cli.command("run")
@click.option("--all", "-a")
def run(): pass
@click.command(name="clear")
def clear(): pass
class Settings(BaseSettings):
 enabled: bool = False
env.getenv("MODE")
env.environ.get("LIMIT")
env.environ["HOME"]
@app.get("/items")
def items(): pass
@app.route("/save", methods=["POST", "PUT"])
def save(): pass
open("read", "r")
open("write", mode="w+")
Path("name").write_text("value")
env.makedirs("dir")
shutil.copy("a", "b")
db = sqlite3.connect(":memory:")
cursor = db.cursor()
cursor.execute("update rows set n=1")
cursor.executemany("insert into rows values (?)", values)
asyncio.create_task(work())
asyncio.gather(work())
int("1")
float("1")
json.loads("{}")
yaml.safe_load("a: 1")
tomllib.loads("a=1")
datetime.strptime("2020", "%Y")
date.fromisoformat("2020-01-01")
urlparse("https://example.test")
subprocess.run(["echo", "test"])
env.system("echo test")
`,
		"urls.py": `from django.urls import path, re_path
urlpatterns = [path("api/", view), re_path(r"^items/$", view)]
`,
	})
	want := []string{"flag:--mode", "flag:-m", "cmd:export", "cmd:run", "cmd:clear", "flag:--all", "flag:-a", "config:enabled", "env:MODE", "env:LIMIT", "env:HOME", "route:GET /items", "route:POST /save", "route:PUT /save", "route:api/", "route:^items/$"}
	var got []string
	for _, file := range snapshot.Files {
		for _, entry := range file.Entries {
			got = append(got, entry.Location())
		}
		if !file.Operations["entries"].Complete || !file.Operations["risks"].Complete {
			t.Fatal(file.Path, file.Missing, file.Operations)
		}
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatal(got, want)
	}
	counts := map[string]int{}
	for _, risk := range snapshot.Files["api.py"].Risks {
		counts[risk.Kind]++
		if strings.Contains(risk.Evidence, "open(\"read\"") {
			t.Fatal(risk)
		}
	}
	for kind, want := range map[string]int{"concurrency": 4, "write": 6, "input": 8, "command": 2} {
		if counts[kind] != want {
			t.Fatal(kind, counts, snapshot.Files["api.py"].Risks)
		}
	}
}

func TestPythonScopesDefaultsAndExternalMutation(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.py": `import os
from typing import Callable
def unused(value): return [value for value in range(3)]
def iterable(value): return [value for value in value]
def closure(value):
 def read(): return value
 return read()
def nested(value):
 def read(value): return value
 return read(2)
def walrus(value): return [(value := 1) for item in values]
def typed(callback: Callable[[int], int] = lambda value: value): pass
def rebound(value):
 def value(): return 1
 return value()
def aliases():
 with resource() as os: os.getenv("WITH")
 try: work()
 except Exception as os: os.getenv("EXCEPT")
def shadowed():
 os.getenv("BEFORE_LOCAL_ASSIGNMENT")
 os = other
def deleted(os):
 del os
 os.getenv("DELETED")
class Value:
 def __init__(self):
  self = other
  self.field = 1
class Callback:
 def __init__(self): self.callback = lambda value: value
 def run(self, value): return self.callback(value)
class Reflect:
 field = 1
item = Reflect()
item.__dict__["field"] = unknown
del item.field
`})
	d := snapshot.Declarations()
	for name, read := range map[string]bool{"unused": false, "iterable": true, "closure": true, "nested": false} {
		if slot := d["a.py:"+name].Usage.Parameters[0]; slot.Read != read {
			t.Fatal(name, slot)
		}
	}
	if slot := d["a.py:rebound"].Usage.Parameters[0]; !slot.Written || len(slot.Unknown) == 0 {
		t.Fatal(slot)
	}
	if slot := d["a.py:walrus"].Usage.Parameters[0]; !slot.Written || slot.Read || len(slot.Values) != 1 {
		t.Fatal(slot)
	}
	if slot := d["a.py:typed"].Usage.Parameters[0]; !slot.Function || len(slot.Functions) != 1 || len(slot.Unknown) != 0 {
		t.Fatal(slot)
	}
	if slot := d["a.py:Callback"].Usage.Fields[0]; !slot.Function || len(slot.Functions) != 1 || len(slot.Unknown) != 0 {
		t.Fatal(slot)
	}
	if slot := d["a.py:Reflect"].Usage.Fields[0]; !slot.Written || len(slot.Unknown) < 2 {
		t.Fatal(slot)
	}
	if len(snapshot.Files["a.py"].Entries) != 0 || snapshot.Files["a.py"].Operations["entries"].Complete || len(d["a.py:Value"].Usage.Unknown) == 0 {
		t.Fatal(snapshot.Files["a.py"].Missing)
	}
}

func TestPythonUnknownsDoNotBecomeConstantOrEntryEvidence(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.py": `import os
import click
class State:
 def __init__(self, value=1): self.value = value
def run(value=1): return value
def shadow(os, open, int):
 os.getenv("SHADOW")
 open("path", "w")
 int("1")
def dynamic(names, cursor, mode, args):
 os.getenv(names)
 cursor.execute("update")
 open("path", mode)
 run(*args)
state = State(2)
unknown(state)
state.value += 1
@click.command
def implicit_name(): pass
@click.command()
def implicit_name_two(): pass
os = other
os.getenv("REBOUND")
`})
	file := snapshot.Files["a.py"]
	if len(file.Entries) != 0 || len(file.Risks) != 0 || file.Operations["entries"].Complete || file.Operations["risks"].Complete || file.Operations["usage"].Complete {
		t.Fatal(file.Entries, file.Risks, file.Operations)
	}
	d := snapshot.Declarations()
	if d["a.py:run"].Usage.Public || len(d["a.py:run"].Usage.Parameters[0].Unknown) == 0 || len(d["a.py:State"].Usage.Fields[0].Unknown) == 0 {
		t.Fatal(d)
	}
	for _, reason := range []string{"*args", "游标", "模式", "默认命名", "入口名字", "重绑定", "复合赋值"} {
		found := false
		for _, m := range file.Missing {
			if strings.Contains(m.Reason, reason) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s: %+v", reason, file.Missing)
		}
	}
}
