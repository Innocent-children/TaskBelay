# Repository Scripts

[中文](README.md) | [English](README_en.md)

`scripts/` 保存本仓库的开发检查、源码安装包构建和独立发布工具。开发入口与
发布入口严格分离：普通验证不会安装真实 Host 产品，也不会创建 npm、Tag 或 GitHub Release。

## 日常开发

| 命令 | 用途 |
| --- | --- |
| `pnpm run validate` | 运行仓库规定的检查 |
| `pnpm run validate:contracts` | 只运行公开接口规范测试 |
| `pnpm run versions:check` | 检查 Core、Codex、DeepSeek、Claude、ZCode 版本文件与同步副本 |
| `pnpm run taskbelay:local` | 从当前源码打包四个 Adapter 和统一入口并进入与 `taskbelay` 相同的本地安装菜单 |
| `node scripts/build-claude-local.mjs --output <绝对目录>` | 将 Claude Adapter tarball 构建到仓库外，不执行安装 |
| `node scripts/build-zcode-local.mjs --output <绝对目录>` | 将自包含 ZCode Adapter tarball 构建到仓库外，不执行 UI 安装 |
| `node tests/zcode/verify-package.mjs <解包目录>` | 使用隔离配置、真实 Git 和包内原生 Core 检查最终包；不执行 ZCode UI 或模型会话 |
| `node tests/claude/verify-package.mjs <解包目录> <Claude可执行文件>` | 使用真实 CLI 和隔离配置验证最终包，不执行已认证模型会话 |
| `pnpm --dir packages/codex test` | 运行 Codex package-local tests |
| `pnpm --dir packages/deepseek test` | 运行 DeepSeek package-local tests |

`validate-repository.sh` 是 Node 验证入口的 Unix 包装层；`node scripts/validate-engineering.mjs` 检查版本/生成副本、空白、Go vet/test、WebUI 构建、真实工程旅程、独立宿主包和当前 package-local tests。先按锁文件安装依赖。此入口不发布，也不运行真实宿主模型会话。

Windows CI 将 Codex、DeepSeek、ZCode 和统一管理器的包测试分别放在独立 PowerShell 步骤中，并显式返回各自的退出码。任一套件失败都会使对应步骤失败，阻止后续默认步骤继续执行。

## 平台定向检查

测试按实际依赖的运行环境分开：

| 检查 | 环境与预期结果 |
| --- | --- |
| `node --test tests/ci_workflow.test.mjs` | 所有平台检查包测试步骤独立且不可忽略失败；Windows 额外读取实际工作流脚本，以真实 PowerShell 和替代的包命令注入退出码 7/0，检查四个套件分别失败或成功。该回归不代表远端 GitHub Actions 已执行。 |
| `node --test packages/deepseek/tests/macos-paths.test.mjs` | 仅 macOS arm64：临时包包含两套平台模块，macOS shell 样例通过预检，POSIX 权限和符号链接规则通过；其他平台跳过。 |
| `node --test packages/deepseek/tests/windows-support.test.mjs` | 仅 Windows x64，须设置 `TASKBELAY_WINDOWS_CORE` 为实际构建的 Core：移出源码目录的包能加载模块并运行 `.exe` 预检；环境不满足时跳过。 |
| `node --test packages/deepseek/tests/paths.test.mjs packages/taskbelay/tests/local-packages.test.mjs` | 跨平台：检查包路径选择与本地包摘要；路径比较使用实际路径，不执行其他平台的程序。 |
| `node --test packages/desktop-pet/windows/tests/renderer.test.cjs` | 跨平台模拟 DOM、IPC 和时钟：普通轮询保留散步；缩放取消散步并能安排后续活动；工作和庆祝动画保持进度。此检查已接入 Mac、Windows CI，不代表原生窗口测试。 |

## 本地安装测试

下面一条命令会构建 WebUI 和 bundled Core，在仓库外的临时目录生成 `taskbelay`、
`taskbelay-codex`、`taskbelay-deepseek`、`taskbelay-claude` 与 `taskbelay-zcode` tarball，再从本地 tarball 启动统一安装菜单：

```bash
pnpm run taskbelay:local
```

也可以转发现有非交互参数：

```bash
pnpm run taskbelay:local -- reinstall --host codex --yes
```

本地模式会真实替换所选 Host 的 Adapter，即使 manifest 版本与已安装版本相同；安装计划、确认、
注册、receipt 和就绪状态检查仍由现有 `taskbelay` 生命周期负责。它使用临时管理器，不升级已有的全局 `taskbelay`；源码诊断应从仓库根目录运行 `node packages/taskbelay/bin/taskbelay.mjs`。脚本退出时删除临时构建文件，不调用
`npm publish`，也不创建 Tag 或 GitHub Release。发布后仍需从 npm 下载并逐字节核对安装包，同时检查 Release 附件。

`taskbelay:local` 的 Node orchestrator 可在 macOS arm64 和 Windows 10/11 x64 运行，并同时构建、
校验、暂存 `darwin-arm64/taskbelay` 与 `win32-x64/taskbelay.exe`。Windows 开发机需要可用的 Go、
Node.js、npm 和 pnpm；不要求 Bash 来启动这个入口。

## 本地源码构建

- `build-webui.mjs`：跨平台构建并同步嵌入式 WebUI；`build-webui.sh` 是 Unix 包装层；
- `build-core-runtimes.mjs`：唯一的双 Core runtime 构建入口，输出按 runtime key 命名的 JSON 报告；
- `build-codex-local.sh`：使用统一 runtime 报告构建 Codex 源码 tarball；
- `build-deepseek-local.mjs`：在系统临时 staging 中使用统一 runtime 报告构建 DeepSeek 源码 tarball；
- `build-host-release.mjs --product <codex|deepseek|claude|zcode>`：从两份独立冻结源码准备 Host 正式发布产物；`build-codex-release.sh`、`build-deepseek-release.sh` 调用同一实现。

各 Host Adapter 源码 package 均不保存预编译 Core。各自的 `package.json` 仍声明最终 npm 包内的
两个 runtime 路径；本地构建和 release staging 现场生成这些文件后再打包。

最终安装包和测试记录必须写入仓库外、由操作者选择的目录。

## 发布入口

推荐在 GitHub Actions 手工运行 `publish-npm` 工作流，选择 `codex`、`deepseek`、`claude`、`zcode` 或 `taskbelay`。五个 npm 包须分别把
`Innocent-children/taskbelay` 的 `publish-npm.yml` 配置为允许 `npm publish` 的 GitHub Actions
Trusted Publisher；运行时只选择产品、channel 和目标版本。工作流使用固定的发布检查，通过 OIDC
获取短期 npm 发布凭据，使用
ARM64 runner（四个 Host Adapter 使用 `macos-15`，TaskBelay 桌面包使用带 Xcode 27 的 `xcode-27` 预览镜像）、Go `1.26.5`、Node.js `24.18.0` 和 pnpm `11.24.0`，所有产品共用发布队列串行执行，
再调用下列入口。四个 Host Adapter 制备交叉构建并校验两个平台的 Core；CLI 制备包含两个平台的桌面应用。发布 runner 的操作系统只是构建基础设施，
不缩小构建产物运行时范围。npm 发布不创建依赖 `NODE_AUTH_TOKEN` 的 registry 认证配置。
版本提交、Tag 和 GitHub Release 使用安装到当前仓库、加入 `main` ruleset bypass list 的专用
GitHub App 短期 token；仓库变量 `RELEASE_APP_CLIENT_ID` 和 secret `RELEASE_APP_PRIVATE_KEY`
分别提供 App Client ID 与完整 PEM 私钥。

Claude/ZCode 的新增入口不代表对应 npm 包已经首次发布或配置 Trusted Publisher。首次启用前由维护者确认包名所有权、npm 首次发布条件和认证方式，并完成每个包的 Trusted Publisher 配置；仓库不自动修改 npm 设置。

```bash
pnpm run release:codex -- \
  [--channel stable|beta] \
  --version "<CODEX_VERSION>" \
  --output "<ABSOLUTE_DIRECTORY>" \
  --confirm "codex-v<CODEX_VERSION>"
```

```bash
pnpm run release:deepseek -- \
  [--channel stable|beta] \
  --version "<DEEPSEEK_VERSION>" \
  --output "<ABSOLUTE_DIRECTORY>" \
  --confirm "deepseek-v<DEEPSEEK_VERSION>"
```

```bash
pnpm run release:claude -- \
  [--channel stable|beta] \
  --version "<CLAUDE_VERSION>" \
  --output "<ABSOLUTE_DIRECTORY>" \
  --confirm "claude-v<CLAUDE_VERSION>"
```

```bash
pnpm run release:zcode -- \
  [--channel stable|beta] \
  --version "<ZCODE_VERSION>" \
  --output "<ABSOLUTE_DIRECTORY>" \
  --confirm "zcode-v<ZCODE_VERSION>"
```

```bash
pnpm run release:taskbelay -- \
  --version "<TASKBELAY_VERSION>" \
  --output "<ABSOLUTE_DIRECTORY>" \
  --confirm "taskbelay-v<TASKBELAY_VERSION>"
```

四个 Host Adapter 默认使用 `stable` channel，只接受稳定 SemVer，并要求 `main` 与 `origin/main` 一致。`beta` 只接受
`MAJOR.MINOR.PATCH-beta.N`，允许任意干净的命名分支，version commit 推回当前分支；npm 固定使用
`beta` dist-tag，GitHub Release 固定为 prerelease，稳定版 `latest` 保持不变。
已有 GitHub Release 的 prerelease 属性必须与所选 channel 一致，否则停止发布。CLI 只支持 stable，并要求干净且同步的 `main`。

Host 发布命令只更新所选 package manifest、对应 plugin/marketplace 版本副本，以及 stable 的 `release/public-versions.json` 条目；CLI 只更新 `packages/taskbelay/package.json`。发布命令不读取或改写 Markdown。首次 stable 发布可以使用当前源码版本并新增公开版本条目；四个 Host 均不依赖历史 Tag。

发布命令使用一套固定检查。只有上述 exact-confirmation 入口可以修改产品版本、commit/push、Tag、
npm、GitHub Release 与 assets。

五个产品共用同一个 Publisher。Publisher 使用仓库外的 `release-manifest.json` 绑定 source、
版本和安装包摘要；重跑时回读并复用匹配的远端状态。
Publisher 最多等待十分钟并重试真正的 `npm pack <package>@<version>` tarball 下载与内容核对；只对
`ETARGET`、`E404` 这类 registry 传播延迟继续等待，认证失败和字节不一致立即停止。

Actions 会在成功或失败后上传 runner 临时发布目录；用同一组 workflow 输入重跑时，publisher 会先回读 npm、Tag 和 GitHub Release，不会盲目重复
不可逆操作。临时目录本身不会跨 workflow run 自动复用。

具体操作要求见 [Release Ownership](../release/README.md)。

## 桌面宠物本地构建

`node scripts/build-desktop-pet.mjs --output "/absolute/pet-build"` 在 macOS arm64 上编译 Swift、装配已有
素材和语言资源、ad-hoc 签名并生成本地统一入口 tarball。源码包的 JS 清单和加入原生应用的 staging
清单分别检查；现有 USTAR 工具保留包内原生执行权限，解包后再核对签名。此入口不重编 Core、不修改
Adapter 安装、不发布 npm。安装、确认运行路径和替换已有应用的步骤见[桌面宠物指南](../docs/DESKTOP-PETS.md#本地构建与安装)。
`taskbelay:local` 仍使用临时 lifecycle 管理器；桌面宠物使用这里生成并安装的持久 package。

构建通过 `scripts/desktop-pet-artwork.mjs` 从 `packages/desktop-pet/default-appearance/` 复制默认蓝色海洋精灵形象，并逐文件核对内容；该素材包包含九类动作、57 个 PNG 帧。构建结果的 `frames` 和 `asset_bytes` 记录默认动画帧数与素材文件大小。其他自定义形象通过外部素材包导入。生成的应用包和外部素材目录不纳入 Git 跟踪。

Windows 桌面包由 `build-desktop-pet-windows.mjs` 构建；在仓库根目录先执行 `npm ci --prefix packages/desktop-pet/windows`，再执行 `node scripts/build-desktop-pet-windows.mjs --output "C:\pet-build"`。输出必须在仓库外。该入口装配 Windows 桌面应用、统一入口及四个 Adapter 安装包；复用 Core 构建目标表，不运行 Mac 程序或测试，也不执行发布。

Windows 桌面开发包同时携带完整的 Codex、DeepSeek、Claude 与 ZCode 安装包；构建复用 buildCoreRuntimes 和 stageAndPack，不生成缺少另一平台 Core 的临时特制 Adapter 包。安装统一入口后，`taskbelay install --host all --yes` 准备四个 Adapter 与桌面应用；ZCode 仍需按返回说明在 UI 中安装和启用。

WebUI 类型和构建由 build-webui.mjs 检查；HTTP 契约、CSRF 和 Core 投影由 go test ./internal/webui 检查。原生浏览器显示另行核验，不以构建或模拟输入替代。

## 共享 Host 命令

Codex、Claude、ZCode 的共同命令入口和 Windows、macOS 命令实现只在 `packages/host-command/` 维护。
执行 `node scripts/sync-host-commands.mjs`，将 `command.mjs`、`platform/windows/command.mjs` 和
`platform/macos/command.mjs` 生成到三个 Host 的 `lib/` 下；文件头标明共享来源。
`node scripts/sync-host-commands.mjs --check` 只读检查这九个副本是否与共享源一致。修改共享源后，
同步并一同提交生成副本，包内导入路径保持不变。

`node scripts/sync-host-commands.mjs --output <ABSOLUTE_LIB_DIRECTORY>` 将同样的三个文件生成到指定
的绝对 `lib` 目录。`taskbelay-local.mjs` 的 `stageAndPack` 和 `build-codex-local.sh` 均直接从共享源
生成 staging 内容，安装包不依赖仓库中的共享目录。统一管理器与 DeepSeek 不使用这组生成副本。

## 共享 Skill 引用

Core 通用说明和示例只在 `skills/taskbelay/core/` 编辑。执行 `node scripts/sync-skill-references.mjs` 生成 Codex、DeepSeek、Claude 与 ZCode 包内副本；副本供源码阅读和本地加载，文件头标明来源。`node scripts/sync-skill-references.mjs --check` 检查副本是否与共享源一致。Codex 本地构建和 `stageAndPack` 都在临时 staging 中重新生成引用，安装包不依赖仓库外的共享目录。共享说明只替换 `host` 值，Host 操作说明分别维护。校验覆盖四个 Host 的当前 MCP 契约、授权与执行边界及完整包内引用。

共享 protocol.md 的完整请求旁保留同工具错误示例。internal/mcp/server_test.go 通过实际生产者比较完整错误编码，并校验输出 Schema；四个独立宿主包测试由 tests/host-engineering.test.mjs 维护。旧阶段样例和更新器已删除。
