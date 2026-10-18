import assert from "node:assert/strict";
import { cp, mkdir, mkdtemp, readFile, readdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { checkVersions } from "../scripts/check-versions.mjs";
import { syncPublicReleaseVersions } from "../scripts/sync-public-release-versions.mjs";

test("Core and all five package versions are independent", async (t) => {
  const root = await fixtureRoot(t);
  await writeFile(join(root, "CORE_VERSION"), "1.2.3\n");
  await setVersion(join(root, "packages/codex/package.json"), "2.3.4");
  await setVersion(join(root, "packages/codex/plugin/.codex-plugin/plugin.json"), "2.3.4");
  await setVersion(join(root, "packages/deepseek/package.json"), "3.4.5");
  await setVersion(join(root, "packages/taskbelay/package.json"), "4.5.6");
  await setVersion(join(root, "packages/claude/package.json"), "5.6.7");
  await setVersion(join(root, "packages/claude/.claude-plugin/plugin.json"), "5.6.7");
  await setVersion(join(root, "packages/zcode/package.json"), "6.7.8");
  await setVersion(join(root, "packages/zcode/.zcode-plugin/plugin.json"), "6.7.8");
  const marketplacePath = join(root, "packages/zcode/marketplace.json");
  const marketplace = JSON.parse(await readFile(marketplacePath, "utf8"));
  marketplace.plugins[0].version = "6.7.8";
  await writeFile(marketplacePath, JSON.stringify(marketplace));
  assert.deepEqual(await checkVersions(root), { core: "1.2.3", codex: "2.3.4", deepseek: "3.4.5", taskBelay: "4.5.6", claude: "5.6.7", zcode: "6.7.8" });
});

test("Core version authority accepts one Windows CRLF terminator", async (t) => {
  const root = await fixtureRoot(t);
  const expected = (await readFile(join(root, "CORE_VERSION"), "utf8")).trim();
  await writeFile(join(root, "CORE_VERSION"), `${expected}\r\n`);
  assert.equal((await checkVersions(root)).core, expected);
});

test("each plugin manifest mirrors its own Host package version", async (t) => {
  for (const [product, path] of [
    ["Codex", "packages/codex/plugin/.codex-plugin/plugin.json"],
    ["Claude", "packages/claude/.claude-plugin/plugin.json"],
    ["ZCode", "packages/zcode/.zcode-plugin/plugin.json"],
  ]) await t.test(product, async t => {
    const root = await fixtureRoot(t);
    await setVersion(join(root, path), "9.9.9");
    await assert.rejects(checkVersions(root), new RegExp(`${product} plugin version must equal ${product} package version`, "u"));
    const rootPackage = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
    assert.equal(Object.hasOwn(rootPackage, "version"), false);
  });
});

for (const product of ["codex", "deepseek", "claude", "zcode"]) {
  test(`${product} stable release updates only its public identity and the released Core`, async t => {
    const root = await fixtureRoot(t), metadataPath = join(root, "release/public-versions.json");
    const previous = JSON.parse(await readFile(metadataPath, "utf8"));
    const result = await syncPublicReleaseVersions(root, { product, version: "9.8.7", coreVersion: "8.7.6" });
    assert.deepEqual(result.previous, previous);
    assert.deepEqual(result.current, { ...previous, core_version: "8.7.6", [product]: { version: "9.8.7", core_version: "8.7.6" } });
    assert.deepEqual(JSON.parse(await readFile(metadataPath, "utf8")), result.current);
    assert.deepEqual(result.changedPaths, ["release/public-versions.json"]);
    await assert.rejects(syncPublicReleaseVersions(root, { product, version: "9.8.7", coreVersion: "8.7.6" }), /already matches/);
  });

  test(`public version updates reject invalid existing ${product} metadata before writing`, async t => {
    const root = await fixtureRoot(t), metadataPath = join(root, "release/public-versions.json");
    const previous = JSON.parse(await readFile(metadataPath, "utf8"));
    previous[product] = { version: "invalid", core_version: "8.7.6" };
    const contents = JSON.stringify(previous);
    await writeFile(metadataPath, contents);
    await assert.rejects(syncPublicReleaseVersions(root, { product: "codex", version: "9.8.7", coreVersion: "8.7.6" }), new RegExp(`public ${product} version metadata is invalid`, "u"));
    assert.equal(await readFile(metadataPath, "utf8"), contents);
  });
}

test("beta and non-Host selections cannot overwrite public stable versions", async t => {
  const root = await fixtureRoot(t), metadataPath = join(root, "release/public-versions.json");
  const contents = await readFile(metadataPath, "utf8");
  for (const product of ["codex", "deepseek", "claude", "zcode"]) {
    await assert.rejects(syncPublicReleaseVersions(root, { product, version: "9.8.7-beta.1", coreVersion: "8.7.6" }), /strict MAJOR.MINOR.PATCH/);
  }
  await assert.rejects(syncPublicReleaseVersions(root, { product: "taskbelay", version: "9.8.7", coreVersion: "8.7.6" }), /product must equal/);
  assert.equal(await readFile(metadataPath, "utf8"), contents);
});

test("current product surfaces contain no internal version system except the database version", async () => {
  const root = new URL("../", import.meta.url);
  const files = [];
  for (const path of ["internal", "packages/codex/bin", "packages/codex/lib", "packages/codex/plugin", "packages/deepseek/lib", "packages/deepseek/skills", "scripts", "release"]) {
    await walk(new URL(`${path}/`, root), files);
  }

  const forbidden = /core_contract_version|core_limits_version|storage_schema_version|schema_version|snapshot_version|process_version|build_profile|standard-development@\d+|(?:requirements|design|tasks|implementation|test|comprehension|refactor|delivery)-result@\d+|blocker-resolution@\d+|ProcessActionV\d+|persistedTaskV\d+|taskbelay\/(?:git-common-dir|repository-identity|worktree-fingerprint|repository-binding)\/v\d+/iu;
  const violations = [];
  for (const file of files) {
    const path = fileURLPath(file);
    if (path.endsWith("_test.go") || path.includes("/testdata/") || path.includes("/tests/")) continue;
    if (forbidden.test(await readFile(file, "utf8"))) violations.push(path);
  }
  assert.deepEqual(violations, []);
});

async function fixtureRoot(t) {
  const root = await mkdtemp(join(tmpdir(), "taskbelay-versions-"));
  t.after(() => import("node:fs/promises").then(({ rm }) => rm(root, { recursive: true, force: true })));
  for (const path of [
    "CORE_VERSION", "package.json", "packages/codex/package.json",
    "packages/codex/plugin/.codex-plugin/plugin.json", "packages/deepseek/package.json",
    "packages/taskbelay/package.json",
    "packages/claude/package.json", "packages/claude/.claude-plugin/plugin.json",
    "packages/zcode/package.json", "packages/zcode/.zcode-plugin/plugin.json", "packages/zcode/marketplace.json",
    "release/public-versions.json",
  ]) {
    await mkdir(join(root, path, ".."), { recursive: true });
    await cp(new URL(`../${path}`, import.meta.url), join(root, path), { recursive: true });
  }
  return root;
}

async function walk(directory, files) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (["node_modules", "testdata", "tests"].includes(entry.name)) continue;
    const child = new URL(`${entry.name}${entry.isDirectory() ? "/" : ""}`, directory);
    if (entry.isDirectory()) await walk(child, files);
    else if (/\.(?:go|mjs|js|json|sh)$/u.test(entry.name)) files.push(child);
  }
}

function fileURLPath(url) {
  return decodeURIComponent(url.pathname);
}

async function setVersion(path, version) {
  const value = JSON.parse(await readFile(path, "utf8"));
  value.version = version;
  await writeFile(path, `${JSON.stringify(value, null, 2)}\n`);
}
