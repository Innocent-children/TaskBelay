#!/usr/bin/env node
import { writeSharedSkillReferences } from "./sync-skill-references.mjs";
import { syncWorkspaceFiles, workspaceFiles } from "../packages/host-workspace/build.mjs";
import { hostCommandFiles, writeHostCommands } from "./sync-host-commands.mjs";
import { desktopSourceFiles } from "./desktop-pet-package.mjs";

import { chmod, copyFile, mkdir, mkdtemp, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { gunzipSync, gzipSync } from "node:zlib";

import { execPortableCommand } from "../packages/taskbelay/lib/command.mjs";
import { buildCoreRuntimes } from "./build-core-runtimes.mjs";

const scriptPath = fileURLToPath(import.meta.url);
const repositoryRoot = resolve(dirname(scriptPath), "..");

export function normalizeForwardedArguments(arguments_) {
  return arguments_[0] === "--" ? arguments_.slice(1) : arguments_;
}

export async function copyExecutable(source, target, dependencies = {}) {
  await (dependencies.copyFile ?? copyFile)(source, target);
  if ((dependencies.requireExecutableMode ?? true) && (dependencies.platform ?? process.platform) !== "win32") {
    await (dependencies.chmod ?? chmod)(target, 0o755);
  }
}

export async function runLocalTaskBelay(arguments_, dependencies = {}) {
  const run = dependencies.run ?? runCommand;
  const root = dependencies.repositoryRoot ?? repositoryRoot;
  const temporaryRoot = await (dependencies.mkdtemp ?? mkdtemp)(join(tmpdir(), "taskbelay-local-"));
  try {
    const artifacts = await buildPackages({ root, temporaryRoot, run });
    process.stderr.write(`taskbelay-local: built local packages in ${temporaryRoot}\n`);
    const managerRoot = join(temporaryRoot, "manager");
    await mkdir(managerRoot, { recursive: true });
    await run("tar", ["-xzf", artifacts.manager.path, "-C", managerRoot], { cwd: root });
    const packageRoot = join(managerRoot, "package");
    const lifecyclePath = join(packageRoot, "lib", "lifecycle.mjs");
    const { runMain } = await import(pathToFileURL(lifecyclePath).href);
    const forwarded = normalizeForwardedArguments(arguments_);
    const runtimeOperation = ["webui","call"].includes(forwarded[0]) || ["help", "--help", "-h", "version", "--version"].includes(forwarded[0]);
    const result = runtimeOperation
      ? await (await import(pathToFileURL(join(packageRoot, "lib", "runtime.mjs")).href)).runTaskBelay(forwarded)
      : await runMain(forwarded, {
        localPackages: {
          codex: { path: artifacts.codex.path, version: artifacts.codex.version },
          deepseek: { path: artifacts.deepseek.path, version: artifacts.deepseek.version },
          claude: { path: artifacts.claude.path, version: artifacts.claude.version },
          zcode: { path: artifacts.zcode.path, version: artifacts.zcode.version },
        },
      });
    return result.code;
  } finally {
    await (dependencies.rm ?? rm)(temporaryRoot, { recursive: true, force: true });
  }
}

async function buildPackages({ root, temporaryRoot, run }) {
  const outputRoot = join(temporaryRoot, "artifacts");
  const stageRoot = join(temporaryRoot, "stages");
  await mkdir(outputRoot, { recursive: true });
  await mkdir(stageRoot, { recursive: true });

  const runtimeReport = await buildCoreRuntimes({
    repositoryRoot: root,
    outputRoot: join(temporaryRoot, "runtimes"),
    run,
  });
  const coreArtifacts = new Map(
    Object.values(runtimeReport.runtimes).map((runtime) => [runtime.relativePath, runtime]),
  );

  const codex = await stageAndPack("codex", { root, stageRoot, outputRoot, coreArtifacts, run });
  const deepseek = await stageAndPack("deepseek", { root, stageRoot, outputRoot, coreArtifacts, run });
  const claude = await stageAndPack("claude", { root, stageRoot, outputRoot, coreArtifacts, run });
  const zcode = await stageAndPack("zcode", { root, stageRoot, outputRoot, coreArtifacts, run });
  const manager = await stageAndPack("taskbelay", { root, stageRoot, outputRoot, coreArtifacts: null, run });
  return { codex, deepseek, claude, zcode, manager };
}

export async function stageAndPack(product, { root, stageRoot, outputRoot, coreArtifacts, run }) {
  const packageRoot = join(root, "packages", product);
  const manifest = JSON.parse(await readFile(join(packageRoot, "package.json"), "utf8"));
  if (product === "taskbelay") manifest.files = desktopSourceFiles(manifest);
  const productStageRoot = join(stageRoot, product);
  const destination = join(productStageRoot, "package");
  const files = [...new Set(["package.json", "README.md", ...manifest.files])];
  for (const relativePath of files) {
    const target = join(destination, relativePath);
    await mkdir(dirname(target), { recursive: true });
    if (relativePath === "package.json") await writeFile(target, `${JSON.stringify(manifest, null, 2)}\n`);
    else if (relativePath === "LICENSE") await copyFile(join(root, "LICENSE"), target);
    else if (coreArtifacts?.has(relativePath)) {
      const runtime = coreArtifacts.get(relativePath);
      await copyExecutable(runtime.path, target, {
        requireExecutableMode: runtime.requireExecutableMode,
      });
    }
    else if (["codex", "deepseek", "claude", "zcode"].includes(product) && workspaceFiles.some(name => relativePath === `lib/${name}`)) {
      await syncWorkspaceFiles(root, product, join(destination, "lib"));
    }
    else if (["codex", "claude", "zcode"].includes(product) && hostCommandFiles.some(name => relativePath === `lib/${name}`)) {
      continue;
    }
    else await copyFile(join(packageRoot, relativePath), target);
  }
  if (["codex", "claude", "zcode"].includes(product)) {
    await writeHostCommands({ root, destination: join(destination, "lib") });
  }
  if (["codex", "deepseek", "claude", "zcode"].includes(product)) {
    const skillPath = ["deepseek", "zcode"].includes(product) ? "skills/taskbelay" : "plugin/skills/taskbelay";
    await writeSharedSkillReferences({ root, host: product, destination: join(destination, skillPath) });
  }
  let artifactPath;
  if (coreArtifacts) {
    const filename = `${manifest.name.replace(/^@/u, "").replaceAll("/", "-")}-${manifest.version}.tgz`;
    artifactPath = join(outputRoot, filename);
    const tarPath = join(outputRoot, `${filename}.tar`);
    await run("tar", ["-cf", tarPath, "--format", "ustar", "-C", productStageRoot, "package"], { cwd: root });
    const executablePaths = packageExecutablePaths(manifest, coreArtifacts);
    const normalized = normalizeUstarArchive(await readFile(tarPath), executablePaths);
    await writeFile(artifactPath, gzipSync(normalized, { level: 9, mtime: 0 }), { mode: 0o644 });
    artifactPath = await realpath(artifactPath);
  } else {
    const result = await run("pnpm", ["--config.ignore-scripts=true", "--dir", destination, "pack", "--pack-destination", outputRoot, "--json"], { cwd: root });
    const reportValue = JSON.parse(result.stdout);
    const report = Array.isArray(reportValue) ? reportValue[0] : reportValue;
    artifactPath = await realpath(resolve(outputRoot, report.filename));
    if (report.name !== manifest.name || report.version !== manifest.version) throw new Error(`local ${product} package identity is invalid`);
  }
  if (!(await stat(artifactPath)).isFile()) throw new Error(`local ${product} package is missing`);
  if (coreArtifacts) {
    const modes = ustarEntryModes(gunzipSync(await readFile(artifactPath)));
    for (const [relativePath, runtime] of coreArtifacts) {
      const archivedPath = `package/${relativePath}`;
      const expectedMode = runtime.requireExecutableMode ? 0o755 : 0o644;
      if (modes.get(archivedPath) !== expectedMode) {
        throw new Error(`local ${product} packaged Core mode is invalid (${modes.get(archivedPath)?.toString(8) ?? "missing"})`);
      }
    }
  }
  return { path: artifactPath, version: manifest.version };
}

function packageExecutablePaths(manifest, coreArtifacts) {
  const paths = new Set(
    [...coreArtifacts.values()]
      .filter((runtime) => runtime.requireExecutableMode)
      .map((runtime) => `package/${runtime.relativePath}`),
  );
  const bins = typeof manifest.bin === "string" ? [manifest.bin] : Object.values(manifest.bin ?? {});
  for (const path of bins) paths.add(`package/${path}`);
  return paths;
}

export function normalizeUstarArchive(archive, executablePaths = new Set()) {
  const normalized = Buffer.from(archive);
  const foundExecutables = new Set();
  walkUstar(normalized, ({ offset, path, type }) => {
    if (type !== "file" && type !== "directory") return;
    const executable = type === "file" && executablePaths.has(path);
    writeTarOctal(normalized, offset + 100, 8, type === "directory" || executable ? 0o755 : 0o644);
    writeTarOctal(normalized, offset + 108, 8, 0);
    writeTarOctal(normalized, offset + 116, 8, 0);
    writeTarOctal(normalized, offset + 136, 12, 0);
    normalized.fill(0, offset + 265, offset + 329);
    writeTarChecksum(normalized, offset);
    if (executable) foundExecutables.add(path);
  });
  const missing = [...executablePaths].filter((path) => !foundExecutables.has(path));
  if (missing.length !== 0) throw new Error(`archive is missing executable entries: ${missing.join(", ")}`);
  return normalized;
}

export function ustarEntryModes(archive) {
  const modes = new Map();
  walkUstar(archive, ({ offset, path, type }) => {
    if (type === "file" || type === "directory") modes.set(path, readTarOctal(archive, offset + 100, 8));
  });
  return modes;
}

function walkUstar(archive, visit) {
  let offset = 0;
  while (offset + 512 <= archive.length) {
    const header = archive.subarray(offset, offset + 512);
    if (header.every((byte) => byte === 0)) return;
    const name = readTarString(archive, offset, 100);
    const prefix = readTarString(archive, offset + 345, 155);
    const path = prefix === "" ? name : `${prefix}/${name}`;
    const typeFlag = archive[offset + 156];
    const type = typeFlag === 53 ? "directory" : typeFlag === 0 || typeFlag === 48 ? "file" : "other";
    const size = readTarOctal(archive, offset + 124, 12);
    visit({ offset, path, type, size });
    offset += 512 + Math.ceil(size / 512) * 512;
  }
  if (offset !== archive.length) throw new Error("invalid USTAR archive length");
}

function readTarString(archive, offset, length) {
  const field = archive.subarray(offset, offset + length);
  const end = field.indexOf(0);
  return field.subarray(0, end < 0 ? field.length : end).toString("utf8");
}

function readTarOctal(archive, offset, length) {
  const value = readTarString(archive, offset, length).trim();
  if (!/^[0-7]+$/u.test(value)) throw new Error("invalid USTAR octal field");
  return Number.parseInt(value, 8);
}

function writeTarOctal(archive, offset, length, value) {
  const encoded = `${value.toString(8).padStart(length - 1, "0")}\0`;
  archive.write(encoded, offset, length, "ascii");
}

function writeTarChecksum(archive, offset) {
  archive.fill(32, offset + 148, offset + 156);
  let checksum = 0;
  for (let index = offset; index < offset + 512; index += 1) checksum += archive[index];
  archive.write(`${checksum.toString(8).padStart(6, "0")}\0 `, offset + 148, 8, "ascii");
}

async function runCommand(executable, arguments_, { cwd, environment = process.env } = {}) {
  try {
    return await execPortableCommand(executable, arguments_, {
      cwd,
      env: environment,
      encoding: "utf8",
      maxBuffer: 16 * 1024 * 1024,
      timeout: 300_000,
      shell: false,
    });
  } catch (error) {
    const detail = [error.stdout, error.stderr].filter(Boolean).join("\n").trim() || error.message;
    throw new Error(`${executable} ${arguments_.join(" ")} failed${detail ? `: ${detail}` : ""}`, { cause: error });
  }
}

if (process.argv[1] && await realpath(process.argv[1]).catch(() => "") === await realpath(scriptPath)) {
  runLocalTaskBelay(process.argv.slice(2)).then(
    (code) => { process.exitCode = code; },
    (error) => {
      process.stderr.write(`taskbelay-local: ${error.message}\n`);
      process.exitCode = 1;
    },
  );
}
