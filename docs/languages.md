# 语言支持

TaskBelay 用统一的任务、声明、引用、入口和用例概念检查各种语言。每种语言提供语法规则与 Runner，项目提供自己的运行环境和命令约定。

## 三级支持

| 级别 | 范围 | 能力 |
| --- | --- | --- |
| 文件级 | 所有文件 | 累计 diff、文件归属、依赖、方案范围、验证记录和任务接续 |
| 代码级 | 内置语法与 tags 查询的语言 | 声明、引用、用法和静态规则；具体语法按本语言规则确定 |
| 测试级 | Go、Python、JavaScript、TypeScript、Java、Rust 的已支持框架 | 用例枚举、逐条运行、覆盖和变异；动态部分按项目设置开启 |

能力按操作确定：能够识别测试但没有覆盖率工具时，仍可做静态检查和记录验证；只有覆盖采集、单例选择等条件齐备时才做对应动态分析。每次检查列出实际完成的项目。

C、C++、C#、Ruby、PHP 等具有内置语法与 tags 查询的语言，采用代码级通用分析。项目使用其他框架时，保留文件和代码能力，通过 Runner 扩展测试能力。

## 文件分类

文件按扩展名匹配规则，其他内置语言采用 gotreesitter 的登记信息。

- 生成代码：开头五行含 `@generated`，或同时含 generated 与 do not edit，不分大小写；参与快照，不做代码精简分析。
- 依赖目录：vendor、node_modules 等目录中的依赖源码不展开分析。
- 锁文件：与依赖清单一起记录，D2 和 S2 不单独提示；依赖变化读清单。
- 测试代码：按语言规则识别，通用约定包括 test、tests、__tests__、spec 目录和带 test/spec 分隔后缀的文件名。一个生产文件内的测试模块单独标记范围，不把整个文件当作测试，例如 Rust 的 `#[cfg(test)]` 模块。
- 测试数据：测试目录中非源码文件，与对应测试记录。

## 代码级的通用做法

- 声明采用 tags 查询的具名定义，包括函数、方法、类、接口、类型以及顶层常量和变量。方法名字带所属类型；函数内部的局部定义属于外层声明。
- 相同位置以文件、类型和名字定位。移动和改名采用编辑记录与唯一结构对应，纯删除保留编辑前声明。
- 比较语法树叶子序列，区分格式、字面量及注释、名字变化。简单函数没有分支和循环，只读写参数、字段、字面量及本语言允许的转换调用。
- 引用按标识符名字建立候选关系。字符串和注释不算普通引用，按名字调用另查。候选关系供归属和用法分析，不把同名认作精确的类型解析。
- 调用节点按位置或命名参数对应；常量包括字面量和可解析的顶层常量。隐式默认值也进入值集合。
- 参数和字段的使用包括读写、构造、默认值、复合赋值和自增减。别名、反射、解码或间接写入无法解析时标记具体范围，不推断为永远不变。
- 接口实现先用本语言显式关系；没有显式关系时按方法候选匹配。函数参数被调用或标注为函数类型时，统计传入函数。
- 分支点包括条件、case、循环、条件表达式、逻辑与或，语言规则可补充。

## 测试级的通用做法

用例名采用框架可定位的一条用例标识。参数组、子测试各算一条；运行时名字与源码无法唯一对应时，把外层测试作为整体记录，不据此拆出删除候选。Runner 转义测试名与命令参数，核对实际只运行了所选用例，避免正则或前缀误选。

行覆盖是执行过的需求代码行集合，不能替代分支和断言。覆盖无效、执行的是外部进程或没有对应源代码时，返回采集情况，供 T1 使用。

动态检查从固定快照导出临时副本。Runner 选择项目已有工具、虚拟环境、命令前缀和准备步骤，覆盖、缓存与构建输出定向到临时目录。需要复用的依赖按下表及项目配置提供；基线先核对实际执行的是副本代码。外部服务和共享状态按项目约定准备，默认串行。

基线成功后才做变异。返回结果分为断言失败、编译失败、环境失败、超时等，只有有效变异导致断言失败才计入发现集合。预算和候选使用方式见 [过度测试判定](over-testing.md)。

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
| 文件 | `.py`。类型存根 `.pyi` 按文件级检查 |
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
| 文件 | `.js`、`.jsx`、`.mjs`、`.cjs`、`.ts`、`.tsx`、`.mts`、`.cts`。类型声明文件 `.d.ts` 按文件级检查 |
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
