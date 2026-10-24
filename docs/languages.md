# 语言支持

TaskBelay 用于任何语言的开发任务。[任务和需求](requirements.md)、[过度设计判定](over-design.md)、[过度测试判定](over-testing.md) 和 [半自动模式](semi-auto.md) 的规则，用声明、引用、接口、测试用例、对外入口、依赖这些和语言无关的概念写成。本文写这些概念在各种语言里对应什么写法，以及一个文件能做哪些检查。

## 三级支持

一个文件能做哪些检查，取决于它所用的语言支持到哪一级。高一级的语言包括低一级的全部检查。

| 级别 | 哪些语言 | 能做的检查 |
| --- | --- | --- |
| 文件级 | 所有文件，包括文档、配置，以及 TaskBelay 解析不了语法的源文件 | 改动按整个文件归属需求；D2、D8；S1、S2；V1、V3；任务接续 |
| 代码级 | TaskBelay 内置了语法的语言：gotreesitter 自带语法和 tags 查询的语言，包括本文“各语言的规则”一节写到的 Go、Python、JavaScript、TypeScript、Java、Rust，以及 C、C++、C#、Ruby、PHP 等 | 改动按声明归属需求；D1、D5、D6，D4、D7 和 E1 的通用部分。“各语言的规则”一节写到的语言，再加上那里列出的各项所对应的检查：D3，D4 的泛型和错误类型，D5 的字段，D7 的弃用标记和别名 |
| 测试级 | “各语言的规则”一节写了测试框架的语言：Go、Python、JavaScript、TypeScript、Java、Rust，并且本机装有那里列出的工具 | T1 到 T6；V2 |

规则需要的东西这门语言没有时，这条规则不检查这个文件，检查结果的“未完成的部分”里写明哪些文件、哪些规则、因为什么没检查，任务完成报告里也会列出来。没检查的部分不会因此拦截。

## 文件归哪种语言

文件按扩展名归到一种语言。“各语言的规则”一节写了每种语言的扩展名，其余语言按 gotreesitter 登记的扩展名。

下面这些文件不做代码级和测试级的检查：

- **生成的代码**：文件开头 5 行里出现 `@generated`，或者同时出现 `generated` 和 `do not edit`，不分大小写。Go 的 `// Code generated ... DO NOT EDIT.` 也在其中。
- **依赖目录**：`vendor/`、`node_modules/` 下的文件。
- **依赖清单的锁文件**：见“依赖清单”一节。它们随依赖清单一起变，D2 和 S2 不检查，D8 读的是依赖清单本身。

**测试代码**：“各语言的规则”一节写了这门语言的测试代码在哪里，就按那里的写法；其余语言按通用的写法：路径里有名为 `test`、`tests`、`__tests__`、`spec` 的目录，或者文件名去掉扩展名之后以 `test`、`spec` 开头或结尾并且用 `_`、`.`、`-` 隔开，或者以 `Test`、`Tests`、`Spec` 结尾（比如 `FooTest.kt`）。测试代码由 [过度测试判定](over-testing.md) 检查，过度设计的规则不检查它。测试目录下不是代码的文件是**测试数据**，比如 Go 的 `testdata/`，和测试代码一样处理。

## 代码级的通用做法

代码级的分析只看语法，不看类型，所有语言用同一套做法。各语言的规则表可以对其中几项另作规定，没有规定的就按这里的做法。

- **声明**：tags 查询标为定义的语法节点，包括函数、方法、类、结构体、接口、类型、模块，以及顶层的常量和变量。声明的范围是整个节点。定义在另一个函数里面的函数和类，属于外层的声明，不单独算。类型里面定义的方法，名字前面加上所属类型的名字，写成 `类型.方法`；类型嵌套时逐层加上。所属类型默认是包围它的类、结构体或接口。
- **只改格式、只改字面量和注释、只改名字**：把起点快照里的版本和当前版本各自展开成语法树的叶子节点序列，按顺序比较。两边的叶子节点完全相同，说明只差空白和换行，是只改格式；去掉字面量和注释之后相同，是只改字面量和注释；只有标识符不同，是只改名字。
- **引用**：一个声明的语法树里有一个标识符，和另一个声明的名字相同，或者和 `类型.方法` 里的方法名相同，就说前者引用后者。字符串和注释里的文字不算标识符。引用只按名字对应，不看作用域、导入和类型，所以同名的声明都会连上。
- **调用处和传入的值**：调用处是语法树里的调用节点，被调用的是这个名字，或者是以 `.名字` 结尾的成员访问。按位置传的参数按位置对应，按名字传的参数按名字对应。传入的值是常量，指字面量（包括 `nil`、`null`、`None`、`true`、`false`），或者一个指向顶层常量声明的名字。
- **接口类抽象**：tags 查询标为接口的定义，以及规则表写明的抽象类写法。实现它的具体类型，是声明头部（名字和主体之间，比如 `implements`、`extends`、基类列表、冒号之后）出现它的名字的类型。
- **函数参数**：在函数体里被当作函数调用的参数，比如 `f(x)` 里的 `f`；有类型标注的语言，类型是函数类型的参数也算。传进来的不同函数，具名函数按名字算一个，每个匿名函数各算一个。
- **分支点**：语法树里的条件语句、条件分支（每个 `case`）、循环、条件表达式、逻辑与和逻辑或。规则表可以补充这门语言特有的写法。

## 测试级的通用做法

- **测试用例**：一条可以单独运行的测试，写法见各语言的规则表。参数化的用例，每一组参数算一条；本次新增的参数组是新增用例。
- **用例名**：实现位置里 `test:` 后面写测试文件的路径、冒号和用例名，比如 `test:internal/export/range_test.go:TestParseRange/5-5`。用例名的写法见各语言的规则表。
- **单独运行一条用例**：按规则表的方式运行，行覆盖写到临时目录，不写进工作区。
- **路径**：单独运行一条用例时，它在需求代码里执行过的行的集合。
- **变异**：在工作区的临时副本里做。副本里是当前工作区状态的全部文件，再把规则表列出的、被 `.gitignore` 忽略的依赖目录（比如 `node_modules`）链接过去。改过的源文件只写进副本，用户工作区里的文件始终不变。在副本里先不加变异，把组里的用例运行一遍，同样统计行覆盖：用例全部通过，并且覆盖结果显示执行的是副本里的需求代码，才做变异。有用例失败，说明副本里运行不了这些用例；执行的不是副本里的文件，说明测试导入的仍是原工作区的代码，比如以可编辑方式安装的包，改了副本也测不出来。这两种情况下，这一组标为未检查。变异方式写在 [过度测试判定](over-testing.md) 的 T2 里，每种语言的运算符写法按它的语法，比如逻辑非在 Python 里是 `not`。

## 依赖清单

D8 和 S2 按下表找出新增的直接依赖。`dep:` 后面写依赖的名字，写法和清单里的一致。

| 清单 | 直接依赖 | 依赖的名字 | 锁文件 |
| --- | --- | --- | --- |
| `go.mod` | 不带 `// indirect` 的 `require` 项 | 模块路径 | `go.sum` |
| `package.json` | `dependencies`、`devDependencies`、`peerDependencies`、`optionalDependencies` | 包名 | `package-lock.json`、`npm-shrinkwrap.json`、`yarn.lock`、`pnpm-lock.yaml`、`bun.lock` |
| `pyproject.toml` | `[project]` 的 `dependencies` 和 `optional-dependencies`，`[dependency-groups]`，`[tool.poetry]` 下的各类 dependencies | 包名 | `poetry.lock`、`uv.lock`、`pdm.lock` |
| `requirements*.txt` | 每一行一个包，`-r`、`-e` 和注释行除外 | 包名 | |
| `Cargo.toml` | `[dependencies]`、`[dev-dependencies]`、`[build-dependencies]`、`[target.*.dependencies]`、`[workspace.dependencies]` | crate 名 | `Cargo.lock` |
| `pom.xml` | `<dependency>` | `groupId:artifactId` | |
| `build.gradle`、`build.gradle.kts` | `implementation`、`api`、`compileOnly`、`runtimeOnly` 以及它们的 `test` 前缀版本里的坐标 | `group:name` | `gradle.lockfile` |
| `Gemfile` | `gem` | gem 名 | `Gemfile.lock` |
| `composer.json` | `require`、`require-dev` | 包名 | `composer.lock` |
| `*.csproj` | `<PackageReference>` | 包名 | `packages.lock.json` |

## 各语言的规则

每种语言一张表。表里没写的项目，按上面的通用做法；通用做法也没有的项目，对应的检查不做，见“三级支持”一节。

### Go

| 项目 | 写法 |
| --- | --- |
| 文件 | `.go` |
| 测试代码 | `_test.go` 文件；测试数据是 `testdata/` 目录 |
| 方法的所属类型 | 接收者的类型 |
| 接口的实现 | Go 不写明实现关系。一个具体类型的方法里包含接口的全部方法名，就算实现了这个接口 |
| 泛型的一种用法 | 写明的一组类型实参，比如 `Map[int, string]`。有一处用法省略类型实参、靠推断时，这个泛型声明不检查 |
| 错误类型 | 有 `Error() string` 方法的类型；赋给包级变量的 `errors.New(...)`、`fmt.Errorf(...)`，也就是哨兵错误 |
| 区分错误的写法 | `errors.Is`、`errors.As`、类型断言、类型 switch、`==`、`!=` |
| 字段和字段的赋值 | 结构体字段；结构体字面量里给它的值，以及 `x.f = ...` |
| 从外部数据写入的字段 | 带结构体标签的字段 |
| 库的公开声明 | 既不在任何 `internal/` 目录下、也不是 `main` 的包里，大写字母开头的声明 |
| 弃用标记 | `// Deprecated:` 注释 |
| 保留旧名字的别名 | `type Old = New` |
| 隐式调用的声明 | `main`、`init`；带 `//go:linkname`、`//export` 指令的声明；和仓库里某个接口的方法同名的方法；标准库接口的方法：`String`、`GoString`、`Format`、`Error`、`Unwrap`、`Is`、`As`、`MarshalJSON`、`UnmarshalJSON`、`MarshalText`、`UnmarshalText`、`ServeHTTP`、`Read`、`Write`、`Close`、`Len`、`Less`、`Swap`、`Scan`、`Value` |
| 命令行参数 | 标准库 `flag` 的定义函数，如 `flag.String`、`fs.BoolVar`；`github.com/spf13/pflag`，以及 `github.com/spf13/cobra` 的 `Flags()`、`PersistentFlags()` 上的定义函数；`github.com/urfave/cli` 的各种 Flag 结构体字面量 |
| 子命令 | `cobra.Command`、`cli.Command` 结构体字面量 |
| 环境变量 | `os.Getenv`、`os.LookupEnv`，参数是字符串常量 |
| HTTP 路由 | `http.Handle`、`http.HandleFunc` 和 `ServeMux` 的同名方法；`chi`、`gin`、`echo` 的路由注册方法。路径是字符串常量 |
| 配置字段 | 新增的、带 `yaml`、`toml` 或 `mapstructure` 标签的结构体字段 |
| 简单声明里允许的调用 | 类型转换 |
| 风险：并发 | `go` 语句、channel 操作、`select`、`sync` 和 `sync/atomic` 包 |
| 风险：写文件或数据库 | `os.WriteFile`、`os.Create`、`os.OpenFile`、`os.Remove`、`os.RemoveAll`、`os.Rename`、`os.Mkdir`、`os.MkdirAll`、`*os.File` 的写方法，`database/sql` 的 `Exec` 系列方法 |
| 风险：解析外部输入 | `strconv` 的 `Parse` 系列和 `Atoi`、`encoding/json` 和 `encoding/xml` 的 `Unmarshal` 与 `Decode`、YAML 和 TOML 的 `Unmarshal`、`time.Parse`、`url.Parse`、`fmt.Sscan` 系列 |
| 风险：执行外部命令 | `os/exec` 包 |
| 测试用例 | `func TestXxx(t *testing.T)` 是一条用例。函数里用 `t.Run` 跑子测试的，每个子测试算一条，外层函数本身不算。表格驱动的测试用 `t.Run` 逐行运行时，表里每一行是一个子测试；子测试的名字取自表格行里的某个字段，按这个字段的值把子测试和表格行对上，对不上时整个测试函数算一条。不用 `t.Run`、在一个函数里循环检查所有行的，整个函数算一条。基准测试、模糊测试和示例函数不算 |
| 用例名 | 测试函数名，子测试写成“函数名/子测试名”，比如 `TestParseRange/5-5` |
| mock 的调用检查 | gomock 的 `EXPECT()`、`Finish()`；testify/mock 的 `AssertExpectations`、`AssertCalled`、`AssertNotCalled`、`AssertNumberOfCalls` |
| 检查结果的语句 | `t.Error`、`t.Errorf`、`t.Fatal`、`t.Fatalf`、`t.Fail`、`t.FailNow`；testify 的 `assert`、`require` 包里除 mock 调用检查以外的函数 |
| 单独运行一条用例 | 每个测试包用 `go test -c -cover -covermode=set -coverpkg=<需求代码所在的包>` 编译一次测试程序，再用 `-test.run` 选中一条用例运行，`-test.coverprofile` 写出覆盖到的代码块，换算成行。表格行和子测试的名字先用 `-test.v` 运行一次外层函数得到 |
| 需要的工具 | Go 工具链 |
| 受影响的测试范围 | 改动过 Go 文件的包，加上直接或间接导入它们的包，测试文件里的导入也算，用 `go list` 算出。缩小后的命令是 `go test <这些包>` |
| 副本里链接的目录 | 不需要 |

### Python

| 项目 | 写法 |
| --- | --- |
| 文件 | `.py`。类型存根 `.pyi` 不检查 |
| 测试代码 | 文件名是 `test_*.py`、`*_test.py` 或 `conftest.py`，或者在 `tests/`、`test/` 目录下 |
| 接口类抽象 | 继承 `abc.ABC`、`typing.Protocol` 的类，以及带 `abc.abstractmethod` 方法的类 |
| 接口的实现 | 基类列表里有它的名字的类。`Protocol` 不写明实现关系，一个类有它的全部方法名，就算实现了它 |
| 错误类型 | 继承 `Exception` 或另一个错误类型的类 |
| 区分错误的写法 | `except X`、`except (X, Y)`、`isinstance(e, X)` |
| 字段和字段的赋值 | 类属性、带类型标注的类变量，以及方法里的 `self.x = ...`；构造时传给它的值 |
| 从外部数据写入的字段 | 继承 pydantic `BaseModel`、`BaseSettings` 的类的字段 |
| 库的公开声明 | 工作区是一个可发布的包（`pyproject.toml` 里有 `[project]` 表，或者有 `setup.py`）时，名字不以 `_` 开头的模块级声明，以及这些类里名字不以 `_` 开头的方法 |
| 弃用标记 | 带 `DeprecationWarning` 或 `PendingDeprecationWarning` 的 `warnings.warn(...)`；`@deprecated` 装饰器 |
| 保留旧名字的别名 | 模块级的 `Old = New`，`Old` 是起点快照里已有的名字 |
| 隐式调用的声明 | 名字形如 `__xxx__` 的方法；带装饰器的函数和方法，装饰器是 `staticmethod`、`classmethod`、`property`、`functools` 里的那些除外 |
| 命令行参数 | argparse 的 `add_argument`，参数以 `-` 开头；click 的 `@click.option` |
| 子命令 | argparse 的 `add_parser`；click 的 `@click.command`、`@<命令组>.command` |
| 环境变量 | `os.getenv`、`os.environ.get`、`os.environ[...]`，参数是字符串常量 |
| HTTP 路由 | 对象上的 `route`、`get`、`post`、`put`、`patch`、`delete` 装饰器，第一个参数是以 `/` 开头的字符串常量，覆盖 Flask 和 FastAPI；Django `urls.py` 里的 `path`、`re_path` |
| 配置字段 | 新增的、继承 `BaseSettings` 的类的字段 |
| 简单声明里允许的调用 | `str`、`bool`、`list`、`dict`、`tuple`、`set` |
| 分支点 | 另外加上 `elif`、`except` |
| 风险：并发 | `threading`、`multiprocessing`、`concurrent.futures` 包，`asyncio.gather`、`asyncio.create_task` |
| 风险：写文件或数据库 | 模式里带 `w`、`a`、`x`、`+` 的 `open`；`pathlib.Path` 的 `write_text`、`write_bytes`、`unlink`、`mkdir`、`rename`、`rmdir`；`os.remove`、`os.rename`、`os.makedirs`、`os.mkdir`；`shutil` 的复制、移动和删除函数；数据库游标的 `execute`、`executemany` |
| 风险：解析外部输入 | `int`、`float`、`json.loads`、`json.load`、`yaml.safe_load`、`yaml.load`、`tomllib.loads`、`tomllib.load`、`datetime.strptime`、`fromisoformat`、`urllib.parse.urlparse` |
| 风险：执行外部命令 | `subprocess` 包、`os.system` |
| 测试用例 | pytest：测试文件里名字以 `test` 开头的函数，以及名字以 `Test` 开头的类里以 `test` 开头的方法；`@pytest.mark.parametrize` 的每一组参数算一条。unittest：`unittest.TestCase` 子类里以 `test` 开头的方法 |
| 用例名 | pytest 的 node id 去掉文件路径的部分，比如 `TestRange::test_parse[5-5]` |
| mock 的调用检查 | `unittest.mock` 的 `assert_called`、`assert_called_once`、`assert_called_with`、`assert_called_once_with`、`assert_any_call`、`assert_has_calls`、`assert_not_called`；`assert` 语句里只检查 mock 的 `called`、`call_count`、`call_args` |
| 检查结果的语句 | 其余的 `assert` 语句；`unittest.TestCase` 的 `assert` 系列方法；`pytest.raises`、`pytest.fail` |
| 单独运行一条用例 | `python -m coverage run --data-file=<临时文件> -m pytest <node id>`，再用 `coverage json` 读出执行过的行。python 用工作区里的虚拟环境 `.venv` 或 `venv`，没有时用 PATH 上的 `python3`。运行时设置 `PYTHONDONTWRITEBYTECODE=1`，不在工作区里写 `.pyc` 文件 |
| 需要的工具 | 项目环境里装有 pytest 和 coverage.py |
| 受影响的测试范围 | 直接或间接 `import` 了本次改动过的模块的测试文件。模块名按工作区根目录和 `src/` 目录换算成文件。缩小后的命令是 `pytest <这些测试文件>` |
| 副本里链接的目录 | `.venv`、`venv`。在副本里运行时，把副本的根目录和 `src/` 放在 `PYTHONPATH` 最前面，让副本里的源文件优先于以可编辑方式安装的包 |

### JavaScript 和 TypeScript

| 项目 | 写法 |
| --- | --- |
| 文件 | `.js`、`.jsx`、`.mjs`、`.cjs`、`.ts`、`.tsx`、`.mts`、`.cts`。类型声明文件 `.d.ts` 不检查 |
| 测试代码 | 文件名里有 `.test.` 或 `.spec.`，或者在 `__tests__/` 目录下 |
| 接口类抽象 | TypeScript 的 `interface`、`abstract class` |
| 接口的实现 | 类的 `implements`、`extends` 后面出现它的名字；类型标注为这个接口的对象字面量，每一个算一种 |
| 泛型的一种用法 | TypeScript 写明的一组类型实参，比如 `Cache<string>`。有一处用法靠推断时，这个泛型声明不检查 |
| 错误类型 | `extends Error` 或另一个错误类型的类 |
| 区分错误的写法 | `instanceof X` |
| 字段和字段的赋值 | 类字段声明，以及 `this.x = ...` |
| 从外部数据写入的字段 | 带装饰器的类字段 |
| 库的公开声明 | `package.json` 里没有 `"private": true` 时，带 `export` 的声明 |
| 弃用标记 | JSDoc 的 `@deprecated` |
| 保留旧名字的别名 | `export const Old = New`、`export { New as Old }`、`export type Old = New`，`Old` 是起点快照里已有的名字 |
| 隐式调用的声明 | `export default` 的声明；`pages/`、`app/`、`routes/` 目录下的文件里导出的声明，这些目录由框架按文件路由加载；类的 `constructor`、`toString`、`toJSON`；带装饰器的声明 |
| 命令行参数 | commander 的 `.option(...)`；yargs 的 `.option(...)`、`.options(...)`；`util.parseArgs` 的 `options` 里的键 |
| 子命令 | commander、yargs 的 `.command(...)` |
| 环境变量 | `process.env.X`、`process.env["X"]`、`import.meta.env.X` |
| HTTP 路由 | 对象上的 `get`、`post`、`put`、`patch`、`delete`、`all`、`route` 调用，第一个参数是以 `/` 开头的字符串常量，覆盖 Express、Koa Router、Fastify；`pages/api/` 下新增的文件，以及新增的 `app/**/route.js`、`app/**/route.ts` 文件 |
| 简单声明里允许的调用 | `String`、`Boolean`、`Array.from` |
| 分支点 | 另外加上 `??`、`catch` |
| 风险：并发 | `worker_threads`、`Worker`、`Promise.all`、`Promise.race`、`Promise.allSettled`、`Promise.any` |
| 风险：写文件或数据库 | `fs` 和 `fs/promises` 里写入、追加、删除、改名、建目录的函数 |
| 风险：解析外部输入 | `JSON.parse`、`parseInt`、`parseFloat`、`Number(...)`、`new URL(...)`、`Date.parse` |
| 风险：执行外部命令 | `child_process` 模块 |
| 测试用例 | Jest、Vitest 的 `test`、`it` 调用；`test.each`、`it.each` 表格的每一行算一条 |
| 用例名 | 外层各个 `describe` 的名字和用例的名字，用 ` > ` 连起来，比如 `parseRange > 5-5` |
| mock 的调用检查 | `toHaveBeenCalled`、`toHaveBeenCalledTimes`、`toHaveBeenCalledWith`、`toHaveBeenLastCalledWith`、`toHaveBeenNthCalledWith`，以及对应的 `toBeCalled` 系列 |
| 检查结果的语句 | 其余的 `expect(...)` 断言；`node:assert` 模块 |
| 单独运行一条用例 | 用工作区 `node_modules/.bin` 里的测试程序。Jest：`jest <文件> -t <用例名> --coverage --coverageReporters=json --coverageDirectory=<临时目录>`。Vitest：`vitest run <文件> -t <用例名> --coverage.enabled --coverage.reporter=json --coverage.reportsDirectory=<临时目录>`。从生成的 `coverage-final.json` 读出执行过的行 |
| 需要的工具 | 项目里装有 Jest，或者装有 Vitest 和它的覆盖率插件 `@vitest/coverage-v8` |
| 受影响的测试范围 | Jest 用 `jest --findRelatedTests <改动的文件>`，Vitest 用 `vitest related <改动的文件>`，由测试程序自己算出 |
| 副本里链接的目录 | `node_modules` |

### Java

| 项目 | 写法 |
| --- | --- |
| 文件 | `.java` |
| 测试代码 | `src/test/` 下的文件 |
| 接口类抽象 | `interface`、`abstract class` |
| 接口的实现 | 类的 `implements`、`extends` 后面出现它的名字；`new X() { ... }` 形式的匿名类，每一个算一种 |
| 泛型的一种用法 | 写明的一组类型实参，比如 `Cache<String>`。有一处用法用 `<>` 或者靠推断时，这个泛型声明不检查 |
| 错误类型 | 继承 `Exception`、`RuntimeException`、`Throwable` 或另一个错误类型的类 |
| 区分错误的写法 | `catch (X e)`、`instanceof X` |
| 字段和字段的赋值 | 字段声明，以及 `this.x = ...`、`x.f = ...` |
| 从外部数据写入的字段 | 带注解的字段 |
| 库的公开声明 | 所在模块里没有 `main` 方法、也没有 `@SpringBootApplication` 时，`public` 的类和它们的 `public` 成员 |
| 弃用标记 | `@Deprecated` 注解、Javadoc 的 `@deprecated` |
| 隐式调用的声明 | `main` 方法；带注解的类和方法；`toString`、`equals`、`hashCode`、`compareTo`、`close`、`run`、`call` |
| 命令行参数 | picocli 的 `@Option` |
| 子命令 | picocli 的 `@Command` |
| 环境变量 | `System.getenv`，参数是字符串常量 |
| HTTP 路由 | Spring 的 `@RequestMapping`、`@GetMapping`、`@PostMapping`、`@PutMapping`、`@PatchMapping`、`@DeleteMapping`；JAX-RS 的 `@Path` |
| 配置字段 | `@Value("${...}")` 里的配置名；`@ConfigurationProperties` 类里新增的字段 |
| 简单声明里允许的调用 | 没有 |
| 风险：并发 | `Thread`、`ExecutorService`、`CompletableFuture`、`synchronized`、`java.util.concurrent` 包 |
| 风险：写文件或数据库 | `Files` 的 `write`、`writeString`、`delete`、`deleteIfExists`、`move`、`copy`、`createDirectories`；`FileOutputStream`、`FileWriter`；JDBC 的 `execute`、`executeUpdate`；`EntityManager` 的 `persist`、`merge`、`remove` |
| 风险：解析外部输入 | `Integer.parseInt`、`Long.parseLong`、`Double.parseDouble`、Jackson 的 `readValue`、`LocalDate.parse`、`LocalDateTime.parse`、`URI.create` |
| 风险：执行外部命令 | `ProcessBuilder`、`Runtime.exec` |
| 测试用例 | JUnit 的 `@Test` 方法；`@ParameterizedTest` 的每一组参数算一条 |
| 用例名 | 类名、`#`、方法名，比如 `RangeTest#parsesRange`；参数化用例后面加 `[序号]` |
| mock 的调用检查 | Mockito 的 `verify`、`verifyNoInteractions`、`verifyNoMoreInteractions` |
| 检查结果的语句 | JUnit 的 `assert` 系列方法和 `fail`；AssertJ、Hamcrest 的 `assertThat` |
| 单独运行一条用例 | Maven：`mvn test -Dtest=<类名>#<方法名>`。Gradle：`gradle test --tests <类名>.<方法名>`。工作区里有 `mvnw`、`gradlew` 时用它们。行覆盖用 JaCoCo 的 agent 统计，结果写到临时文件 |
| 需要的工具 | JDK，Maven 或 Gradle，本机 Maven 仓库或 Gradle 缓存里有 JaCoCo |
| 受影响的测试范围 | Maven 多模块项目里改动过的模块，加上依赖它们的模块。缩小后的命令是 `mvn test -pl <这些模块> -amd` |
| 副本里链接的目录 | 不需要 |

### Rust

| 项目 | 写法 |
| --- | --- |
| 文件 | `.rs` |
| 测试代码 | `tests/` 目录下的文件；`#[cfg(test)]` 模块里的代码 |
| 方法的所属类型 | 所在 `impl` 块的类型 |
| 接口类抽象 | `trait` |
| 接口的实现 | `impl Trait for Type`，每个不同的 `Type` 算一种 |
| 泛型的一种用法 | 写明的一组类型实参，比如 `parse::<u32>`、`Cache<String>`。有一处用法靠推断时，这个泛型声明不检查 |
| 错误类型 | 实现了 `std::error::Error` 的类型；带 `#[derive(Error)]` 的类型 |
| 区分错误的写法 | `match`、`if let`、`matches!` 里按这个类型或它的变体匹配；`downcast_ref::<X>` |
| 字段和字段的赋值 | 结构体字段；结构体字面量里给它的值，以及 `x.f = ...` |
| 从外部数据写入的字段 | 带 `#[derive(Deserialize)]` 的结构体的字段 |
| 库的公开声明 | crate 有 `src/lib.rs` 时，`pub` 的声明，`pub(crate)` 除外 |
| 弃用标记 | `#[deprecated]` |
| 保留旧名字的别名 | `pub use New as Old;`、`type Old = New;`，`Old` 是起点快照里已有的名字 |
| 隐式调用的声明 | `main`；`impl Trait for Type` 块里的方法；带 `#[no_mangle]`、`#[export_name]` 的函数 |
| 命令行参数 | clap 的 `#[arg(long ...)]`、`#[clap(long ...)]` 字段；`Arg::new(...)` |
| 子命令 | 带 `#[derive(Subcommand)]` 的枚举的变体；`Command::new(...)` 作为子命令加入的 |
| 环境变量 | `std::env::var`、`std::env::var_os`，参数是字符串常量；clap 的 `env = "..."` |
| HTTP 路由 | axum 的 `.route(...)`；actix-web 的 `#[get(...)]`、`#[post(...)]` 等属性 |
| 简单声明里允许的调用 | `into`、`from`、`clone` |
| 分支点 | 另外加上 `?` 运算符、`loop` |
| 风险：并发 | `std::thread`、`tokio::spawn`、`Mutex`、`RwLock`、`std::sync::atomic`、`mpsc` |
| 风险：写文件或数据库 | `std::fs` 的 `write`、`remove_file`、`remove_dir_all`、`rename`、`create_dir_all`、`copy`；`File::create`、`OpenOptions` |
| 风险：解析外部输入 | `parse`、`serde_json::from_str`、`from_slice`、`from_reader`、`toml::from_str`、`serde_yaml::from_str` |
| 风险：执行外部命令 | `std::process::Command` |
| 测试用例 | 带 `#[test]` 或 `#[tokio::test]` 的函数；rstest 的每个 `#[case]` 算一条 |
| 用例名 | 测试函数在 crate 里的路径，比如 `range::tests::parses_range` |
| mock 的调用检查 | mockall 的 `expect_*` 调用和它后面的 `times`、`with` |
| 检查结果的语句 | `assert!`、`assert_eq!`、`assert_ne!`、`panic!` |
| 单独运行一条用例 | `cargo llvm-cov --lcov --output-path <临时文件> -- <用例名> --exact` |
| 需要的工具 | Rust 工具链、`cargo-llvm-cov` |
| 受影响的测试范围 | workspace 里改动过的 crate，加上依赖它们的 crate。缩小后的命令是 `cargo test -p <这些 crate>` |
| 副本里链接的目录 | 不需要。副本里的编译产物写到临时目录 |
