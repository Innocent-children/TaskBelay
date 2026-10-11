# Repository Scripts

[中文](README.md) | [English](README_en.md)

`scripts/` contains repository development validation, source-package builders, and standalone
release tooling. Development and publication entrypoints are separate: ordinary validation does not
install real Host products or create npm, Tag, or GitHub Release state.

## Daily development

| Command | Purpose |
| --- | --- |
| `pnpm run validate` | Run the repository's required checks |
| `pnpm run validate:contracts` | Run public contract tests only |
| `pnpm run versions:check` | Verify Core, Codex, DeepSeek, Claude and ZCode version files and mirrors |
| `pnpm run taskbelay:local` | Pack four Adapters and the manager from current source and open the normal `taskbelay` install menu |
| `node scripts/build-claude-local.mjs --output <absolute-directory>` | Build a Claude Adapter tarball outside the repository; does not install it |
| `node scripts/build-zcode-local.mjs --output <absolute-directory>` | Build a self-contained ZCode Adapter tarball outside the repository; does not perform UI installation |
| `node tests/zcode/verify-package.mjs <extracted-package>` | Check the final package with isolated configuration, real Git and native packaged Core; does not run ZCode UI or model sessions |
| `node tests/claude/verify-package.mjs <extracted-package> <claude-executable>` | Verify a final package with an actual Claude CLI and isolated configuration; does not authenticate a model session |
| `pnpm --dir packages/codex test` | Run Codex package-local tests |
| `pnpm --dir packages/deepseek test` | Run DeepSeek package-local tests |

`validate-repository.sh` wraps the portable Node entry. `node scripts/validate-engineering.mjs` checks versions/generated copies, whitespace, Go vet/test, WebUI build, native engineering journeys, independent Host packages and current package-local tests. Install locked dependencies first. It neither publishes nor runs authenticated Host model sessions.

Windows CI runs the Codex, DeepSeek, ZCode and lifecycle manager package suites in separate PowerShell steps, each explicitly returning its own exit code. A failed suite fails its step and prevents later default steps from running.

## Platform-targeted checks

Tests are separated by the environment they actually require:

| Check | Environment and expected result |
| --- | --- |
| `node --test tests/ci_workflow.test.mjs` | All platforms check that package suites have independent steps and cannot ignore failure. Windows also executes the actual workflow scripts in real PowerShell with a substituted package command returning 7 or 0, checking failure and success for all four suites. This regression does not represent a remote GitHub Actions run. |
| `node --test packages/deepseek/tests/macos-paths.test.mjs` | macOS arm64 only: the detached package includes both platform modules, the macOS shell fixture passes preflight, and POSIX permissions and symlink rules pass; skipped elsewhere. |
| `node --test packages/deepseek/tests/windows-support.test.mjs` | Windows x64 only, with `TASKBELAY_WINDOWS_CORE` pointing to a built Core: the detached package loads its modules and runs the `.exe` preflight; skipped when prerequisites are absent. |
| `node --test packages/deepseek/tests/paths.test.mjs packages/taskbelay/tests/local-packages.test.mjs` | Cross-platform package-path selection and local artifact hashes; comparisons use canonical paths and execute no foreign-platform programs. |
| `node --test packages/desktop-pet/windows/tests/renderer.test.cjs` | Simulated DOM, IPC and clock on either platform: ordinary polls preserve walking, resizing cancels walking and allows later activities, and work and celebration playback retain their progress. Both Mac and Windows CI run this check; it is not a native window test. |

## Local installation testing

This one command builds the WebUI and bundled Core, creates `taskbelay`, `taskbelay-codex`,
`taskbelay-deepseek`, `taskbelay-claude` and `taskbelay-zcode` tarballs in a temporary directory outside the repository, and starts the
unified install menu from the local tarball:

```bash
pnpm run taskbelay:local
```

Existing non-interactive arguments can be forwarded unchanged:

```bash
pnpm run taskbelay:local -- reinstall --host codex --yes
```

Local mode really replaces the selected Host Adapter even when its manifest version matches the
installed version. The existing `taskbelay` lifecycle still owns plans, confirmation, registration,
receipts, and readiness read-back. The launcher removes temporary artifacts when it exits; it never
runs `npm publish` or creates a Tag or GitHub Release. Registry byte read-back and Release asset
checks still require the publication workflow.

The `taskbelay:local` Node orchestrator runs on macOS arm64 and Windows 10/11 x64, and builds,
verifies, and stages both `darwin-arm64/taskbelay` and `win32-x64/taskbelay.exe`. A Windows development
host needs Go, Node.js, npm, and pnpm; this entry does not require Bash to launch.

The local installer runs a temporary manager and does not upgrade an existing global `taskbelay`. Diagnose source installations from the repository root with `node packages/taskbelay/bin/taskbelay.mjs`.

## Local source builds

- `build-webui.mjs`: build and synchronize the embedded WebUI cross-platform;
  `build-webui.sh` is its Unix wrapper;
- `build-core-runtimes.mjs`: the only dual-Core runtime builder, returning one JSON report keyed by
  runtime pair;
- `build-codex-local.sh`: build the Codex source-local tarball from that runtime report;
- `build-deepseek-local.mjs`: build the DeepSeek source-local tarball from that runtime report in a
  system-temporary staging directory;
- `build-host-release.mjs --product <codex|deepseek|claude|zcode>`: prepare formal Host release artifacts
  from two independent frozen-source builds; `build-codex-release.sh` and `build-deepseek-release.sh` delegate to it.

Host Adapter source packages do not store precompiled Core executables. Each `package.json` still
declares the two runtime paths required in the final npm package; local builds and release staging
create those files before packing.

Final packages and test records must be written to an operator-selected directory outside the
repository.

## Release entrypoints

The usual maintainer entrypoint is the manually dispatched `publish-npm` GitHub Actions workflow,
selecting `codex`, `deepseek`, `claude`, `zcode` or `taskbelay`.
For each npm package, configure `publish-npm.yml` from `Innocent-children/taskbelay` as a GitHub Actions
Trusted Publisher allowed to run `npm publish`. Then select only the product, channel, and exact
version. The workflow uses one fixed release check and obtains a short-lived npm
publish credential through OIDC, uses ARM64 runners (`macos-15` for all four Host Adapters and the `xcode-27` preview image with Xcode 27 for the TaskBelay desktop package), Go `1.26.5`, Node.js `24.18.0`, and pnpm
`11.24.0`, serializes all products through one release queue, and invokes the commands below.
All four Host preparations cross-builds and verifies both Core targets; CLI preparation includes
both desktop applications. The release runner OS is build
infrastructure rather than an artifact-runtime restriction. npm publication does not create
registry authentication configuration that depends on `NODE_AUTH_TOKEN`.
Version commits, Tags, and GitHub Releases use a short-lived token from a dedicated GitHub App that is
installed on this repository and added to the `main` ruleset bypass list. Repository variable
`RELEASE_APP_CLIENT_ID` and secret `RELEASE_APP_PRIVATE_KEY` provide the App Client ID and complete PEM
private key respectively.

The new Claude/ZCode entrypoints do not mean their npm packages have completed an initial publication
or Trusted Publisher setup. Before first use, maintainers must resolve package ownership, npm
initial-publication requirements and authentication, then configure each package’s Trusted Publisher.
The repository does not change npm settings.

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

For all four Host Adapters, `stable` is the default channel. It accepts stable SemVer and requires `main` to equal
`origin/main`. `beta` accepts only `MAJOR.MINOR.PATCH-beta.N`, may use any clean named branch, and
pushes the version commit back to that branch. It always uses npm dist-tag `beta`, marks the GitHub
Release as a prerelease, and preserves stable `latest`.
An existing GitHub Release must have the prerelease status selected by the channel; a mismatch stops
publication. The CLI supports stable only and requires clean synchronized `main`.

Host release flows update only the selected package manifest, its plugin/marketplace version mirrors
and its stable `release/public-versions.json` entry; the CLI updates only `packages/taskbelay/package.json`.
Release commands neither read nor rewrite Markdown. A first stable release can use the current source
version and add its public identity; none of the four Hosts requires a previous Tag.

The release command uses one fixed check set. Only these exact-confirmation entrypoints may change a
product version, commit and push, create a Tag, publish npm, or mutate GitHub Release assets.

All five products share one Publisher. The external `release-manifest.json` binds source, version, and
artifact digests; reruns reread and reuse matching remote state.
The Publisher retries the actual `npm pack <package>@<version>` tarball read-back for up to ten
minutes. Only registry propagation responses such as `ETARGET` and `E404` keep waiting;
authentication failures and byte mismatches stop immediately.

Actions uploads the temporary release directory after both successful and failed runs. Rerunning the same workflow inputs makes the
publisher reread npm, Tag, and GitHub Release state before another irreversible operation; the runner
directory itself is not automatically reused across workflow runs.

See [Release Ownership](../release/README.md) for the exact operator contract.

## Desktop pet local build

`node scripts/build-desktop-pet.mjs --output "/absolute/pet-build"` compiles Swift on macOS arm64,
assembles existing artwork and language resources, signs ad hoc, and creates a local unified-entry
tarball. Source JS files and the staging manifest including the app are checked separately. Existing
USTAR helpers preserve native executable permissions, followed by extracted-signature verification.
This entry does not rebuild Core, change Adapter installations, or publish npm. See the [desktop pet guide](../docs/DESKTOP-PETS_en.md#local-build-and-installation) for installation, identifying the running executable, and replacing an existing app.
`taskbelay:local` retains its temporary lifecycle manager; the pet uses the persistent installed package
built by this entry.

The build uses `scripts/desktop-pet-artwork.mjs` to copy the default blue sea sprite appearance from `packages/desktop-pet/default-appearance/` and compare each delivered file with its source. The pack contains nine clips and 57 PNG frames; the `frames` and `asset_bytes` result fields record its animation frame count and artwork file size. Other custom appearances are imported from external artwork packs. Generated application bundles and external artwork directories are not tracked by Git.

Build the Windows desktop package with `build-desktop-pet-windows.mjs`. From the repository root, run `npm ci --prefix packages/desktop-pet/windows`, then `node scripts/build-desktop-pet-windows.mjs --output "C:\pet-build"`. Output must be outside the repository. This entry assembles the Windows desktop, launcher and all four Adapter packages through the Core target catalog; it does not execute Mac programs, Mac tests or publication.

The Windows desktop development distribution carries complete Codex, DeepSeek, Claude and ZCode packages through buildCoreRuntimes and stageAndPack. It does not create special Adapter archives missing the other Core runtime. After launcher bootstrap, `taskbelay install --host all --yes` prepares all four Adapters and the desktop app; ZCode still requires UI installation and enablement as instructed in the result.

WebUI type checking and bundling run through build-webui.mjs. go test ./internal/webui checks actual HTTP contracts, CSRF and Core projection. Native browser inspection is separate.

## Shared Host commands

Maintain the common Codex/Claude/ZCode command entry point and its Windows/macOS implementations only in
`packages/host-command/`. Run `node scripts/sync-host-commands.mjs` to generate `command.mjs`,
`platform/windows/command.mjs` and `platform/macos/command.mjs` under each Host's `lib/`, with headers
identifying the shared source. `node scripts/sync-host-commands.mjs --check` checks all nine copies
against the source without writing files. After changing the source, synchronize and commit the
generated copies together; package-local import paths stay unchanged.

`node scripts/sync-host-commands.mjs --output <ABSOLUTE_LIB_DIRECTORY>` generates the same three files
into the specified absolute `lib` directory. Both `stageAndPack` in `taskbelay-local.mjs` and
`build-codex-local.sh` generate staging contents directly from the shared source, so installed packages
do not depend on the repository's shared directory. The unified manager and DeepSeek do not use these
generated copies.

## Shared Skill references

Edit common Core instructions and examples only in `skills/taskbelay/core/`. Run `node scripts/sync-skill-references.mjs` to generate the Codex, DeepSeek, Claude and ZCode copies. `--check` detects stale copies. Local staging also renders them, so installed packages need no repository shared directory. Core text substitutes only the Host value; actual Host authorization, permissions and workspace instructions remain separately authored. Checks cover current MCP contracts and complete packaged references.

Shared protocol.md keeps same-tool errors beside complete requests. internal/mcp/server_test.go compares actual producer encoding and validates output Schema. tests/host-engineering.test.mjs checks independent Host packages. Retired stage examples and updaters are removed.
