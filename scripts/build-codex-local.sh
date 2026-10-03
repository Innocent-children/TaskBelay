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

production_files='package.json
README.md
.agents/plugins/marketplace.json
bin/taskbelay-codex.mjs
lib/command.mjs
lib/artifacts-help.mjs
lib/install-experience.mjs
lib/json.mjs
lib/host-launch-contract.mjs
lib/lifecycle.mjs
lib/paths.mjs
lib/platform.mjs
lib/platform/macos/policies.mjs
lib/platform/windows/policies.mjs
lib/platform/macos/command.mjs
lib/platform/windows/command.mjs
lib/platform/macos/pet-installer.mjs
lib/provisioning-receipt.mjs
lib/task-admission.mjs
lib/task-handoff.mjs
lib/task-launch.mjs
lib/worktree-lifecycle.mjs
plugin/.codex-plugin/plugin.json
plugin/.mcp.json
plugin/hooks/hooks.json
plugin/hooks/pre-tool-use.mjs
plugin/skills/taskbelay/SKILL.md
plugin/skills/taskbelay/agents/openai.yaml
plugin/skills/taskbelay/references/admission.md
plugin/skills/taskbelay/references/artifact-contract.md
plugin/skills/taskbelay/references/connection-examples.md
plugin/skills/taskbelay/references/connection.md
plugin/skills/taskbelay/references/core-lifecycle-examples.md
plugin/skills/taskbelay/references/core-lifecycle.md
plugin/skills/taskbelay/references/launch-examples.md
plugin/skills/taskbelay/references/lifecycle-examples.md
plugin/skills/taskbelay/references/nodes/comprehension-examples.md
plugin/skills/taskbelay/references/nodes/delivery-examples.md
plugin/skills/taskbelay/references/nodes/design-examples.md
plugin/skills/taskbelay/references/nodes/implementation-examples.md
plugin/skills/taskbelay/references/nodes/refactor-examples.md
plugin/skills/taskbelay/references/nodes/requirements-examples.md
plugin/skills/taskbelay/references/nodes/tasks-examples.md
plugin/skills/taskbelay/references/nodes/test-examples.md
plugin/skills/taskbelay/references/recovery-examples.md
plugin/skills/taskbelay/references/recovery.md
plugin/skills/taskbelay/references/response-examples.md
plugin/skills/taskbelay/references/worktree-launch.md
plugin/skills/taskbelay/references/artifacts.md
plugin/skills/taskbelay/references/host-lifecycle.md
plugin/skills/taskbelay/references/method-profiles.md
plugin/skills/taskbelay/references/node-payloads.md
plugin/skills/taskbelay/references/nodes/comprehension.md
plugin/skills/taskbelay/references/nodes/delivery.md
plugin/skills/taskbelay/references/nodes/design.md
plugin/skills/taskbelay/references/nodes/implementation.md
plugin/skills/taskbelay/references/nodes/refactor.md
plugin/skills/taskbelay/references/nodes/requirements.md
plugin/skills/taskbelay/references/nodes/tasks.md
plugin/skills/taskbelay/references/nodes/test.md
plugin/skills/taskbelay/references/task-handoff.md
plugin/skills/taskbelay/references/tool-results.md
plugin/skills/taskbelay/references/transport.md
plugin/skills/taskbelay/references/verification.md
plugin/skills/taskbelay/references/successes/taskbelay_abandon_task-abandon.md
plugin/skills/taskbelay/references/successes/taskbelay_cancel_task-cancel.md
plugin/skills/taskbelay/references/successes/taskbelay_get_next_action-guarded-read.md
plugin/skills/taskbelay/references/successes/taskbelay_get_task-read.md
plugin/skills/taskbelay/references/successes/taskbelay_prepare_task_branch_rename-prepare-branch-rename.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-rename-complete.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-rename-cancel.md
plugin/skills/taskbelay/references/successes/taskbelay_get_task-history-page.md
plugin/skills/taskbelay/references/successes/host-supersede-replace.md
plugin/skills/taskbelay/references/successes/host-supersede-resume-recover.md
plugin/skills/taskbelay/references/successes/taskbelay_open_task-create.md
plugin/skills/taskbelay/references/successes/taskbelay_open_task-multiple.md
plugin/skills/taskbelay/references/successes/taskbelay_open_task-resume.md
plugin/skills/taskbelay/references/successes/taskbelay_prepare_task_relocation-prepare.md
plugin/skills/taskbelay/references/successes/taskbelay_recover_action-saved-operation.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-allow_once.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-expand_scope.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-history.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-reject.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-relocation.md
plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-verification-or-recovery.md
plugin/skills/taskbelay/references/successes/taskbelay_server_info-handshake.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-code_too_complex.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-comprehension_passed.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-design_too_complex.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-evidence_insufficient.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-implementation_defect.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-requirement_unclear.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_complete.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_comprehension.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_design.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_implementation.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_requirements.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_test.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_design-design_ready.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_design-design_requires_requirements.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_needs_refactor.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_ready_for_test.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_requires_design.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_requires_requirements.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_ready_for_test.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_requires_design.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_requires_requirements.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_requirements-requirements_ready.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_plan_saved.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_ready.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_require_design.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_require_requirements.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_accepted_with_known_failures.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_expose_design_issue.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_expose_requirement_issue.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_failed_implementation.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_passed.md
plugin/skills/taskbelay/references/successes/taskbelay_submit_test-verification_budget_increased.md
plugin/skills/taskbelay/references/successes/host-bootstrap-managed.md
plugin/skills/taskbelay/references/successes/host-cleanup-branch-remove-branch.md
plugin/skills/taskbelay/references/successes/host-cleanup-decision-keep.md
plugin/skills/taskbelay/references/successes/host-cleanup-worktree-remove-worktree.md
plugin/skills/taskbelay/references/successes/host-cli-provision-single.md
plugin/skills/taskbelay/references/successes/host-dispatch-call-managed.md
plugin/skills/taskbelay/references/successes/host-dispatch-reconcile-lookup.md
plugin/skills/taskbelay/references/successes/host-dispatch-recover-not-called.md
plugin/skills/taskbelay/references/successes/host-dispatch-result-queued.md
plugin/skills/taskbelay/references/successes/host-dispatch-result-ready.md
plugin/skills/taskbelay/references/successes/host-dispatch-start-managed.md
plugin/skills/taskbelay/references/successes/host-handoff-result-record.md
plugin/skills/taskbelay/references/successes/host-handoff-start-start.md
plugin/skills/taskbelay/references/successes/host-handoff-status-pending.md
plugin/skills/taskbelay/references/successes/host-handoff-status-succeeded.md
plugin/skills/taskbelay/references/successes/host-inspect-single.md
plugin/skills/taskbelay/references/successes/host-local-provision-current-session.md
plugin/skills/taskbelay/references/successes/host-prepare-local-branch.md
plugin/skills/taskbelay/references/successes/host-prepare-local-managed.md
plugin/skills/taskbelay/references/successes/host-prepare-remote-cli.md
plugin/skills/taskbelay/references/successes/host-scope-multiple.md
plugin/skills/taskbelay/references/successes/host-scope-single.md
plugin/skills/taskbelay/references/successes/host-status-launch.md
lib/worktree-snapshot.mjs'

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
PACK_REPORT=$pack_report node <<'NODE'
const packed = JSON.parse(process.env.PACK_REPORT);
const report = Array.isArray(packed) ? packed[0] : packed;
const actual = (report.files ?? [])
  .map((entry) => typeof entry === "string" ? entry : entry.path ?? entry.name)
  .sort();
const expected = [
  ".agents/plugins/marketplace.json",
  "LICENSE",
  "README.md",
  "bin/taskbelay-codex.mjs",
  "lib/artifacts-help.mjs",
  "lib/command.mjs",
  "lib/host-launch-contract.mjs",
  "lib/install-experience.mjs",
  "lib/json.mjs",
  "lib/lifecycle.mjs",
  "lib/paths.mjs",
  "lib/platform.mjs",
  "lib/platform/macos/command.mjs",
  "lib/platform/macos/pet-installer.mjs",
  "lib/platform/macos/policies.mjs",
  "lib/platform/windows/command.mjs",
  "lib/platform/windows/policies.mjs",
  "lib/provisioning-receipt.mjs",
  "lib/task-admission.mjs",
  "lib/task-handoff.mjs",
  "lib/task-launch.mjs",
  "lib/worktree-lifecycle.mjs",
  "lib/worktree-snapshot.mjs",
  "package.json",
  "plugin/.codex-plugin/plugin.json",
  "plugin/.mcp.json",
  "plugin/hooks/hooks.json",
  "plugin/hooks/pre-tool-use.mjs",
  "plugin/skills/taskbelay/SKILL.md",
  "plugin/skills/taskbelay/agents/openai.yaml",
  "plugin/skills/taskbelay/references/admission.md",
  "plugin/skills/taskbelay/references/artifact-contract.md",
  "plugin/skills/taskbelay/references/connection-examples.md",
  "plugin/skills/taskbelay/references/connection.md",
  "plugin/skills/taskbelay/references/core-lifecycle-examples.md",
  "plugin/skills/taskbelay/references/core-lifecycle.md",
  "plugin/skills/taskbelay/references/launch-examples.md",
  "plugin/skills/taskbelay/references/lifecycle-examples.md",
  "plugin/skills/taskbelay/references/nodes/comprehension-examples.md",
  "plugin/skills/taskbelay/references/nodes/delivery-examples.md",
  "plugin/skills/taskbelay/references/nodes/design-examples.md",
  "plugin/skills/taskbelay/references/nodes/implementation-examples.md",
  "plugin/skills/taskbelay/references/nodes/refactor-examples.md",
  "plugin/skills/taskbelay/references/nodes/requirements-examples.md",
  "plugin/skills/taskbelay/references/nodes/tasks-examples.md",
  "plugin/skills/taskbelay/references/nodes/test-examples.md",
  "plugin/skills/taskbelay/references/recovery-examples.md",
  "plugin/skills/taskbelay/references/recovery.md",
  "plugin/skills/taskbelay/references/response-examples.md",
  "plugin/skills/taskbelay/references/worktree-launch.md",
  "plugin/skills/taskbelay/references/artifacts.md",
  "plugin/skills/taskbelay/references/host-lifecycle.md",
  "plugin/skills/taskbelay/references/method-profiles.md",
  "plugin/skills/taskbelay/references/node-payloads.md",
  "plugin/skills/taskbelay/references/nodes/comprehension.md",
  "plugin/skills/taskbelay/references/nodes/delivery.md",
  "plugin/skills/taskbelay/references/nodes/design.md",
  "plugin/skills/taskbelay/references/nodes/implementation.md",
  "plugin/skills/taskbelay/references/nodes/refactor.md",
  "plugin/skills/taskbelay/references/nodes/requirements.md",
  "plugin/skills/taskbelay/references/nodes/tasks.md",
  "plugin/skills/taskbelay/references/nodes/test.md",
  "plugin/skills/taskbelay/references/task-handoff.md",
  "plugin/skills/taskbelay/references/tool-results.md",
  "plugin/skills/taskbelay/references/transport.md",
  "plugin/skills/taskbelay/references/verification.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_abandon_task-abandon.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_cancel_task-cancel.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_get_next_action-guarded-read.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_get_task-read.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_prepare_task_branch_rename-prepare-branch-rename.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-rename-complete.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-rename-cancel.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_get_task-history-page.md",
  "plugin/skills/taskbelay/references/successes/host-supersede-replace.md",
  "plugin/skills/taskbelay/references/successes/host-supersede-resume-recover.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_open_task-create.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_open_task-multiple.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_open_task-resume.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_prepare_task_relocation-prepare.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_recover_action-saved-operation.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-allow_once.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-expand_scope.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-history.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-reject.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-relocation.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_resolve_blocker-verification-or-recovery.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_server_info-handshake.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-code_too_complex.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-comprehension_passed.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-design_too_complex.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-evidence_insufficient.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-implementation_defect.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_comprehension-requirement_unclear.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_complete.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_comprehension.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_design.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_implementation.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_requirements.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_delivery-delivery_needs_test.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_design-design_ready.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_design-design_requires_requirements.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_needs_refactor.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_ready_for_test.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_requires_design.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_implementation-implementation_requires_requirements.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_ready_for_test.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_requires_design.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_refactor-refactor_requires_requirements.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_requirements-requirements_ready.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_plan_saved.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_ready.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_require_design.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_tasks-tasks_require_requirements.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_accepted_with_known_failures.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_expose_design_issue.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_expose_requirement_issue.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_failed_implementation.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_test-tests_passed.md",
  "plugin/skills/taskbelay/references/successes/taskbelay_submit_test-verification_budget_increased.md",
  "plugin/skills/taskbelay/references/successes/host-bootstrap-managed.md",
  "plugin/skills/taskbelay/references/successes/host-cleanup-branch-remove-branch.md",
  "plugin/skills/taskbelay/references/successes/host-cleanup-decision-keep.md",
  "plugin/skills/taskbelay/references/successes/host-cleanup-worktree-remove-worktree.md",
  "plugin/skills/taskbelay/references/successes/host-cli-provision-single.md",
  "plugin/skills/taskbelay/references/successes/host-dispatch-call-managed.md",
  "plugin/skills/taskbelay/references/successes/host-dispatch-reconcile-lookup.md",
  "plugin/skills/taskbelay/references/successes/host-dispatch-recover-not-called.md",
  "plugin/skills/taskbelay/references/successes/host-dispatch-result-queued.md",
  "plugin/skills/taskbelay/references/successes/host-dispatch-result-ready.md",
  "plugin/skills/taskbelay/references/successes/host-dispatch-start-managed.md",
  "plugin/skills/taskbelay/references/successes/host-handoff-result-record.md",
  "plugin/skills/taskbelay/references/successes/host-handoff-start-start.md",
  "plugin/skills/taskbelay/references/successes/host-handoff-status-pending.md",
  "plugin/skills/taskbelay/references/successes/host-handoff-status-succeeded.md",
  "plugin/skills/taskbelay/references/successes/host-inspect-single.md",
  "plugin/skills/taskbelay/references/successes/host-local-provision-current-session.md",
  "plugin/skills/taskbelay/references/successes/host-prepare-local-branch.md",
  "plugin/skills/taskbelay/references/successes/host-prepare-local-managed.md",
  "plugin/skills/taskbelay/references/successes/host-prepare-remote-cli.md",
  "plugin/skills/taskbelay/references/successes/host-scope-multiple.md",
  "plugin/skills/taskbelay/references/successes/host-scope-single.md",
  "plugin/skills/taskbelay/references/successes/host-status-launch.md",
  "runtime/darwin-arm64/taskbelay",
  "runtime/win32-x64/taskbelay.exe"
].sort();
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
