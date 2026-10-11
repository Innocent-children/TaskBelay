#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_directory=""
final_artifact=false
expected_source_commit=""
report_path=""
codex_compatibility='>=0.147.0'

usage() {
  printf '%s\n' 'usage: build-codex-local.sh --output ABSOLUTE_DIRECTORY [--final --source-commit GIT_SHA --report ABSOLUTE_FILE]' >&2
}

fail() {
  printf 'build-codex-local: %s\n' "$1" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      output_directory=$2
      shift 2
      ;;
    --final)
      final_artifact=true
      shift
      ;;
    --source-commit)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      expected_source_commit=$2
      shift 2
      ;;
    --report)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      report_path=$2
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

[ -n "$output_directory" ] || { usage; exit 2; }
case "$output_directory" in
  /*) ;;
  *) fail "output directory must be absolute" ;;
esac
[ -d "$output_directory" ] || fail "output directory must already exist"
output_directory=$(CDPATH= cd -- "$output_directory" && pwd)

if [ "$final_artifact" = true ]; then
  [ -n "$expected_source_commit" ] || fail "final artifact requires --source-commit"
  [ -n "$report_path" ] || fail "final artifact requires --report"
  case "$report_path" in
    /*) ;;
    *) fail "artifact report path must be absolute" ;;
  esac
  report_directory=$(dirname -- "$report_path")
  [ -d "$report_directory" ] || fail "artifact report parent directory must already exist"
  report_directory=$(CDPATH= cd -- "$report_directory" && pwd)
  report_path="$report_directory/$(basename -- "$report_path")"
  if [ -e "$report_path" ] || [ -L "$report_path" ]; then
    fail "artifact report already exists"
  fi
elif [ -n "$report_path" ]; then
  fail "--report is valid only with --final"
fi

existing_artifact=""
for candidate in "$output_directory"/*.tgz; do
  if [ -e "$candidate" ]; then
    existing_artifact=$candidate
    break
  fi
done
[ -z "$existing_artifact" ] || fail "output directory already contains a .tgz artifact"

source_commit=$(git -C "$repository_root" rev-parse HEAD)
case "$source_commit" in
  [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*) ;;
  *) fail "could not resolve the source commit" ;;
esac

source_dirty=false
if [ -n "$(git -C "$repository_root" status --porcelain)" ]; then
  source_dirty=true
fi
if [ "$final_artifact" = true ]; then
  [ "$source_dirty" = false ] || fail "final artifact requires a clean frozen source tree"
  [ "$expected_source_commit" = "$source_commit" ] || fail "requested source commit does not equal HEAD"
elif [ -n "$expected_source_commit" ]; then
  fail "--source-commit is valid only with --final"
fi

core_version=$(sed -n '1p' "$repository_root/CORE_VERSION")
codex_version=$(node -p 'require(process.argv[1]).version' "$repository_root/packages/codex/package.json")
[ -n "$core_version" ] || fail "CORE_VERSION is empty"
[ -n "$codex_version" ] || fail "Codex package version is empty"

node - "$repository_root" "$codex_version" "$core_version" "$codex_compatibility" <<'NODE'
const fs = require("node:fs");
const path = require("node:path");
const [root, codexVersion, coreVersion, codexCompatibility] = process.argv.slice(2);
const packageManifest = JSON.parse(fs.readFileSync(path.join(root, "packages/codex/package.json"), "utf8"));
const pluginManifest = JSON.parse(fs.readFileSync(path.join(root, "packages/codex/plugin/.codex-plugin/plugin.json"), "utf8"));
const lifecycleSource = fs.readFileSync(path.join(root, "packages/codex/lib/lifecycle.mjs"), "utf8");
const privateContract = !Object.hasOwn(packageManifest, "private") || packageManifest.private === false;
const platformContract = JSON.stringify(packageManifest.os) === JSON.stringify(["darwin", "win32"]) &&
  JSON.stringify(packageManifest.cpu) === JSON.stringify(["arm64", "x64"]);
const publishContract = packageManifest.publishConfig?.access === "public" &&
  packageManifest.publishConfig?.registry === "https://registry.npmjs.org/" &&
  JSON.stringify(Object.keys(packageManifest.publishConfig).sort()) === JSON.stringify(["access", "registry"]);
if (
  packageManifest.name !== "taskbelay-codex" ||
  !privateContract ||
  !platformContract ||
  !publishContract ||
  packageManifest.license !== "Apache-2.0"
) {
  throw new Error("Codex package does not satisfy the fixed public package contract");
}
if (packageManifest.version !== codexVersion || pluginManifest.version !== codexVersion) {
  throw new Error("Codex package and plugin versions must match");
}
const productVersion = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-beta\.(0|[1-9][0-9]*))?$/;
const coreVersionPattern = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;
if (!productVersion.test(codexVersion) || !coreVersionPattern.test(coreVersion)) {
  throw new Error("Codex version must be stable or beta and Core must be MAJOR.MINOR.PATCH");
}
if (!lifecycleSource.includes(`CODEX_COMPATIBILITY_RANGE = "${codexCompatibility}"`)) {
  throw new Error("Codex compatibility metadata does not match the selected range");
}
NODE

build_root=$(mktemp -d -t taskbelay-codex-build.XXXXXX)
trap 'rm -rf -- "$build_root"' EXIT HUP INT TERM
stage_root="$build_root/package"
mkdir -p "$stage_root"

production_files=$(node -e 'const fs=require("node:fs");const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));process.stdout.write(["package.json","README.md",...m.files.filter(x=>x!=="LICENSE"&&!x.startsWith("runtime/"))].filter((x,i,a)=>a.indexOf(x)===i).join("\n"))' "$repository_root/packages/codex/package.json")

printf '%s\n' "$production_files" | while IFS= read -r relative_path; do
  [ -n "$relative_path" ] || continue
  source_path="$repository_root/packages/codex/$relative_path"
  case "$relative_path" in
    lib/command.mjs|lib/platform/windows/command.mjs|lib/platform/macos/command.mjs)
      continue
      ;;
    lib/worktree-lifecycle.mjs|lib/worktree-snapshot.mjs)
      source_path="$repository_root/packages/host-workspace/$(basename -- "$relative_path")"
      ;;
  esac
  [ -f "$source_path" ] || fail "required production file is missing: $relative_path"
  mkdir -p "$stage_root/$(dirname -- "$relative_path")"
  cp "$source_path" "$stage_root/$relative_path"
done
node "$repository_root/scripts/sync-host-commands.mjs" --output "$stage_root/lib"
cp "$repository_root/LICENSE" "$stage_root/LICENSE"
chmod 0755 "$stage_root/bin/taskbelay-codex.mjs"

runtime_report=$(node "$repository_root/scripts/build-core-runtimes.mjs" --output "$stage_root/runtime")
RUNTIME_REPORT=$runtime_report CORE_VERSION=$core_version node <<'NODE'
const report = JSON.parse(process.env.RUNTIME_REPORT);
if (report.coreVersion !== process.env.CORE_VERSION ||
    JSON.stringify(Object.keys(report.runtimes).sort()) !== JSON.stringify(["darwin-arm64", "win32-x64"])) {
  throw new Error("Core runtime build report does not match the package contract");
}
NODE

if [ "$(uname -s)" = Darwin ] && [ "$(uname -m)" = arm64 ]; then
  reported_core=$(
    cd "$build_root"
    "$stage_root/runtime/darwin-arm64/taskbelay" version
  )
  [ "$reported_core" = "taskbelay $core_version" ] || fail "detached Core version does not match CORE_VERSION"
else
  go version -m "$stage_root/runtime/darwin-arm64/taskbelay" >/dev/null
fi

node "$repository_root/scripts/sync-skill-references.mjs" --host codex --output "$stage_root/plugin/skills/taskbelay"

pack_report=$(pnpm --config.ignore-scripts=true --dir "$stage_root" pack --dry-run --json)
STAGE_ROOT=$stage_root PACK_REPORT=$pack_report node <<'NODE'
const packed = JSON.parse(process.env.PACK_REPORT);
const report = Array.isArray(packed) ? packed[0] : packed;
const actual = (report.files ?? [])
  .map((entry) => typeof entry === "string" ? entry : entry.path ?? entry.name)
  .sort();
const expected = JSON.parse(require('node:fs').readFileSync(require('node:path').join(process.env.STAGE_ROOT,'package.json'),'utf8')).files.concat(['package.json','README.md']).filter((v,i,a)=>a.indexOf(v)===i).sort();
if (report.name !== "taskbelay-codex" || JSON.stringify(actual) !== JSON.stringify(expected)) {
  throw new Error(`unexpected staged pack contents: ${JSON.stringify(actual)}`);
}
NODE

artifact_path="$output_directory/taskbelay-codex-$codex_version.tgz"
find "$stage_root" -exec touch -t 198510260815.00 {} +
if tar --version 2>/dev/null | grep -q 'GNU tar'; then
  (
    cd "$build_root"
    find package -type f -print | LC_ALL=C sort | COPYFILE_DISABLE=1 tar \
      -cf "$build_root/taskbelay-codex.tar" \
      --format ustar \
      --owner 0 \
      --group 0 \
      --numeric-owner \
      -T -
  )
else
  (
    cd "$build_root"
    find package -type f -print | LC_ALL=C sort | COPYFILE_DISABLE=1 tar \
      -cf "$build_root/taskbelay-codex.tar" \
      --format ustar \
      --uid 0 \
      --gid 0 \
      --uname root \
      --gname root \
      -T -
  )
fi
gzip -n -c "$build_root/taskbelay-codex.tar" >"$artifact_path"
[ -f "$artifact_path" ] || fail "archive creation did not produce the expected artifact"

if command -v shasum >/dev/null 2>&1; then
  artifact_sha256=$(shasum -a 256 "$artifact_path" | awk '{print $1}')
elif command -v sha256sum >/dev/null 2>&1; then
  artifact_sha256=$(sha256sum "$artifact_path" | awk '{print $1}')
else
  fail "a SHA-256 utility is required"
fi

ARTIFACT_PATH=$artifact_path \
ARTIFACT_SHA256=$artifact_sha256 \
PACKAGE_VERSION=$codex_version \
CORE_VERSION=$core_version \
SOURCE_COMMIT=$source_commit \
SOURCE_DIRTY=$source_dirty \
FINAL_ARTIFACT=$final_artifact \
CODEX_COMPATIBILITY=$codex_compatibility \
REPORT_PATH=$report_path \
node <<'NODE'
const fs = require("node:fs");
const path = require("node:path");

const baseReport = {
  artifact_path: process.env.ARTIFACT_PATH,
  artifact_sha256: process.env.ARTIFACT_SHA256,
  package_version: process.env.PACKAGE_VERSION,
  core_version: process.env.CORE_VERSION,
  source_commit: process.env.SOURCE_COMMIT,
  source_dirty: process.env.SOURCE_DIRTY === "true",
  final_artifact: process.env.FINAL_ARTIFACT === "true",
  platforms: ["darwin-arm64", "win32-x64"],
};

if (!baseReport.final_artifact) {
  process.stdout.write(`${JSON.stringify(baseReport)}\n`);
  process.exit(0);
}

const report = {
  report_type: "taskbelay-codex-final-artifact",
  artifact_path: baseReport.artifact_path,
  artifact_sha256: baseReport.artifact_sha256,
  package_version: baseReport.package_version,
  core_version: baseReport.core_version,
  codex_compatibility: process.env.CODEX_COMPATIBILITY,
  source_commit: baseReport.source_commit,
  source_dirty: baseReport.source_dirty,
  final_artifact: true,
  platforms: baseReport.platforms,
  package_allowlist_verified: true,
  runtime_executable_verified: true,
  built_at: new Date().toISOString(),
};
const bytes = `${JSON.stringify(report)}\n`;
const reportPath = process.env.REPORT_PATH;
const directory = path.dirname(reportPath);
const temporaryPath = path.join(
  directory,
  `.${path.basename(reportPath)}.tmp-${process.pid}-${Date.now()}`,
);
let temporaryCreated = false;
try {
  const descriptor = fs.openSync(temporaryPath, "wx", 0o600);
  temporaryCreated = true;
  try {
    fs.writeFileSync(descriptor, bytes, "utf8");
    fs.fsyncSync(descriptor);
  } finally {
    fs.closeSync(descriptor);
  }
  fs.linkSync(temporaryPath, reportPath);
  fs.unlinkSync(temporaryPath);
  temporaryCreated = false;
  const directoryDescriptor = fs.openSync(directory, "r");
  try {
    fs.fsyncSync(directoryDescriptor);
  } finally {
    fs.closeSync(directoryDescriptor);
  }
} catch (error) {
  if (temporaryCreated) {
    try { fs.unlinkSync(temporaryPath); } catch {}
  }
  if (error?.code === "EEXIST") {
    throw new Error("artifact report already exists");
  }
  throw error;
}
process.stdout.write(bytes);
NODE
