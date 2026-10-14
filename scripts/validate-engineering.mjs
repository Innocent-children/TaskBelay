import { spawnSync } from "node:child_process";
import { mkdirSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const go = process.env.GO_EXECUTABLE ?? "go";
const output = join(root, ".evaluation");
mkdirSync(output, { recursive: true });

function run(program, args, extra = {}) {
  process.stdout.write(`${program} ${args.join(" ")}\n`);
  const result = spawnSync(program, args, {
    cwd: root,
    stdio: "inherit",
    env: { ...process.env, ...extra },
    windowsHide: true,
  });
  if (result.error || result.status !== 0)
    throw new Error(`Check failed: ${program} ${args.join(" ")}`, {
      cause: result.error,
    });
}
function testFiles(directory, suffix = ".test.mjs") {
  return readdirSync(join(root, directory))
    .filter((file) => file.endsWith(suffix))
    .sort()
    .map((file) => `${directory}/${file}`);
}

run(process.execPath, ["scripts/sync-skill-references.mjs", "--check"]);
run(process.execPath, ["scripts/sync-host-commands.mjs", "--check"]);
run(process.execPath, ["scripts/check-versions.mjs"]);
run("git", ["diff", "--check"]);
const formatting = spawnSync("gofmt", ["-l", "cmd", "internal", "tests"], {
  cwd: root,
  encoding: "utf8",
  windowsHide: true,
});
if (formatting.status !== 0 || formatting.stdout.trim())
  throw new Error(
    `Go formatting differs: ${formatting.stdout || formatting.stderr}`,
  );
run(go, ["vet", "./..."]);
run(go, ["test", "-count=1", "-timeout=90s", "./..."]);
run(process.execPath, ["scripts/build-webui.mjs"]);
run(process.execPath, ["--test", ...testFiles("packages/webui/tests")]);

const executable = join(
  output,
  process.platform === "win32" ? "taskbelay.exe" : "taskbelay",
);
run(go, ["build", "-o", executable, "./cmd/taskbelay"]);
run(
  process.execPath,
  [
    "--test",
    "tests/journeys/engineering.test.mjs",
    "tests/host-engineering.test.mjs",
  ],
  { ENGINEERING_CORE: executable },
);

// Build and release tests use isolated fixtures; no release or installation is performed.
const tests = [
  ...testFiles("scripts"),
  ...testFiles("tests").filter(
    (file) => file !== "tests/host-engineering.test.mjs",
  ),
  "tests/comprehensive/package_contract.test.mjs",
  ...testFiles("release"),
  ...testFiles("release/taskbelay"),
];
for (const host of ["codex", "deepseek", "claude", "zcode", "taskbelay"])
  tests.push(...testFiles(`packages/${host}/tests`));
tests.push(...testFiles("packages/desktop-pet/windows/tests", ".test.cjs"));
run(process.execPath, ["--test", ...tests]);
process.stdout.write(
  "Repository validation passed. Native Host chats and other-platform execution remain separate checks.\n",
);
