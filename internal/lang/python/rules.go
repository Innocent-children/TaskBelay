package python

import "github.com/Innocent-children/taskbelay/internal/lang"

var Rules = lang.Rules{
	SimpleCalls: []string{"str", "bool", "list", "dict", "tuple", "set"}, BranchNodes: []string{"elif_clause", "except_clause"},
	AbstractBases: []string{"abc.ABC", "typing.Protocol"}, StructuralBases: []string{"typing.Protocol"}, AbstractDecorators: []string{"abc.abstractmethod"},
	ErrorBases:         []string{"Exception", "ArithmeticError", "AssertionError", "AttributeError", "BufferError", "EOFError", "ExceptionGroup", "ImportError", "LookupError", "MemoryError", "NameError", "OSError", "ReferenceError", "RuntimeError", "StopAsyncIteration", "StopIteration", "SyntaxError", "SystemError", "TypeError", "ValueError", "Warning", "FloatingPointError", "OverflowError", "ZeroDivisionError", "ModuleNotFoundError", "IndexError", "KeyError", "UnboundLocalError", "BlockingIOError", "ChildProcessError", "ConnectionError", "FileExistsError", "FileNotFoundError", "InterruptedError", "IsADirectoryError", "NotADirectoryError", "PermissionError", "ProcessLookupError", "TimeoutError", "NotImplementedError", "RecursionError", "IndentationError", "TabError", "UnicodeError", "UnicodeDecodeError", "UnicodeEncodeError", "UnicodeTranslateError", "DeprecationWarning", "PendingDeprecationWarning"},
	ExternalFieldBases: []string{"pydantic.BaseModel", "pydantic.BaseSettings", "pydantic_settings.BaseSettings"},
	ConfigBases:        []string{"pydantic.BaseSettings", "pydantic_settings.BaseSettings"},
	IgnoredDecorators:  []string{"builtins.staticmethod", "builtins.classmethod", "builtins.property", "functools.*"},
	Constructors:       map[string]string{"argparse.ArgumentParser": "argparse.ArgumentParser", "pathlib.Path": "pathlib.Path", "sqlite3.connect": "sqlite3.Connection", "sqlite3.Cursor": "sqlite3.Cursor", "unittest.mock.Mock": "unittest.mock.Mock", "unittest.mock.MagicMock": "unittest.mock.MagicMock", "unittest.mock.AsyncMock": "unittest.mock.AsyncMock", "unittest.mock.create_autospec": "unittest.mock.Mock"},
	ReceiverMethods:    map[string]string{"argparse.ArgumentParser.add_subparsers": "argparse.SubParsersAction", "sqlite3.Connection.cursor": "sqlite3.Cursor"},
	Entries: []lang.CallRule{
		{Target: "argparse.ArgumentParser.add_argument", Kind: "flag"}, {Target: "click.option", Kind: "flag"},
		{Target: "argparse.SubParsersAction.add_parser", Kind: "cmd"}, {Target: "click.command", Kind: "cmd"}, {Target: "click.Group.command", Kind: "cmd"},
		{Target: "os.getenv", Kind: "env"}, {Target: "os.environ.get", Kind: "env"},
		{Target: "django.urls.path", Kind: "route"}, {Target: "django.urls.re_path", Kind: "route"},
	},
	ConcurrentImports: []string{"threading", "multiprocessing", "concurrent.futures"},
	Risks: []lang.CallRule{
		{Target: "threading.*", Kind: "concurrency"}, {Target: "multiprocessing.*", Kind: "concurrency"}, {Target: "concurrent.futures.*", Kind: "concurrency"}, {Target: "asyncio.gather", Kind: "concurrency"}, {Target: "asyncio.create_task", Kind: "concurrency"},
		{Target: "pathlib.Path.write_text", Kind: "write"}, {Target: "pathlib.Path.write_bytes", Kind: "write"}, {Target: "pathlib.Path.unlink", Kind: "write"}, {Target: "pathlib.Path.mkdir", Kind: "write"}, {Target: "pathlib.Path.rename", Kind: "write"}, {Target: "pathlib.Path.rmdir", Kind: "write"},
		{Target: "os.remove", Kind: "write"}, {Target: "os.rename", Kind: "write"}, {Target: "os.makedirs", Kind: "write"}, {Target: "os.mkdir", Kind: "write"},
		{Target: "shutil.copy", Kind: "write"}, {Target: "shutil.copy2", Kind: "write"}, {Target: "shutil.copyfile", Kind: "write"}, {Target: "shutil.copyfileobj", Kind: "write"}, {Target: "shutil.copytree", Kind: "write"}, {Target: "shutil.move", Kind: "write"}, {Target: "shutil.rmtree", Kind: "write"},
		{Target: "sqlite3.Cursor.execute", Kind: "write"}, {Target: "sqlite3.Cursor.executemany", Kind: "write"},
		{Target: "builtins.int", Kind: "input"}, {Target: "builtins.float", Kind: "input"}, {Target: "json.loads", Kind: "input"}, {Target: "json.load", Kind: "input"}, {Target: "yaml.safe_load", Kind: "input"}, {Target: "yaml.load", Kind: "input"}, {Target: "tomllib.loads", Kind: "input"}, {Target: "tomllib.load", Kind: "input"}, {Target: "datetime.datetime.strptime", Kind: "input"}, {Target: "datetime.datetime.fromisoformat", Kind: "input"}, {Target: "datetime.date.fromisoformat", Kind: "input"}, {Target: "datetime.time.fromisoformat", Kind: "input"}, {Target: "urllib.parse.urlparse", Kind: "input"},
		{Target: "subprocess.*", Kind: "command"}, {Target: "os.system", Kind: "command"},
	},
}

const Tags = `
(assignment left: (_) @binding) @definition.variable
`
