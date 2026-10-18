import { execFile as execFileCallback } from "node:child_process";
import { createHash } from "node:crypto";
import { realpath } from "node:fs/promises";
import { isAbsolute, resolve } from "node:path";
import { promisify } from "node:util";
import { nativeGitPath } from "./platform.mjs";

const execFile = promisify(execFileCallback);

const digestPattern = /^[0-9a-f]{64}$/u;

export function requestDigest(request) {
  if (typeof request !== "string" || request.trim() === "") {
    throw new Error("workspace request must be a non-empty string");
  }
  return createHash("sha256").update(request, "utf8").digest("hex");
}

export async function inspectAdmissionAnchor({
  request,
  repositories,
  runGit = defaultRunGit,
} = {}) {
  if (!Array.isArray(repositories) || repositories.length === 0 || repositories.length > 8) {
    throw new Error("workspace repositories must contain one to eight entries");
  }
  const seenKeys = new Set();
  const seenRoots = new Set();
  const observed = [];
  for (const repository of repositories) {
    assertExactKeys(repository, ["key", "repository_path"], "workspace repository");
    assertRepositoryKey(repository.key);
    if (seenKeys.has(repository.key)) throw new Error(`duplicate repository key ${repository.key}`);
    seenKeys.add(repository.key);
    assertAbsolutePath(repository.repository_path, "repository_path");
    const requestedPath = resolve(repository.repository_path);
    const topLevel = nativeGitPath(normalizeLine(await runGit(["-C", requestedPath, "rev-parse", "--show-toplevel"])));
    assertAbsolutePath(topLevel, "Git worktree root");
    const canonicalRoot = await realpath(topLevel);
    if (seenRoots.has(canonicalRoot)) throw new Error("workspace repositories must identify distinct worktrees");
    seenRoots.add(canonicalRoot);
    const head = normalizeLine(await runGit(["-C", canonicalRoot, "rev-parse", "--verify", "HEAD"]));
    assertCommit(head, "HEAD");
    const status = await runGit([
      "-C", canonicalRoot, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=none",
    ], { encoding: "buffer" });
    const statusBuffer = Buffer.isBuffer(status) ? status : Buffer.from(status);
    observed.push(Object.freeze({
      repository_key: repository.key,
      canonical_root: canonicalRoot,
      head,
      status_digest: createHash("sha256").update(statusBuffer).digest("hex"),
      dirty_paths: Object.freeze(parsePorcelainV2Paths(statusBuffer).slice(0, 64)),
      dirty_paths_truncated: parsePorcelainV2Paths(statusBuffer).length > 64,
    }));
  }
  return Object.freeze({
    request_digest: requestDigest(request),
    repositories: Object.freeze(observed),
  });
}

function parsePorcelainV2Paths(value) {
  const records = value.toString("utf8").split("\0");
  const paths = [];
  for (let index = 0; index < records.length; index += 1) {
    const record = records[index];
    if (!record) continue;
    if (record.startsWith("? ") || record.startsWith("! ")) {
      paths.push(record.slice(2));
      continue;
    }
    if (record.startsWith("1 ") || record.startsWith("u ")) {
      paths.push(record.split(" ").slice(record.startsWith("1 ") ? 8 : 10).join(" "));
      continue;
    }
    if (record.startsWith("2 ")) {
      paths.push(record.split(" ").slice(9).join(" "));
      if (records[index + 1]) paths.push(records[index + 1]);
      index += 1;
    }
  }
  return [...new Set(paths.filter(Boolean))].sort();
}

async function defaultRunGit(arguments_, { encoding = "utf8" } = {}) {
  const options = {
    encoding: encoding === "buffer" ? null : encoding,
    env: { ...process.env, GIT_TERMINAL_PROMPT: "0" },
    maxBuffer: 4 * 1024 * 1024,
    timeout: 30_000,
    windowsHide: true,
  };
  try {
    const { stdout } = await execFile("git", arguments_, options);
    return stdout;
  } catch (cause) {
    throw new Error(`read-only Git assessment failed${cause?.killed ? " or timed out" : ""}`);
  }
}

function normalizeLine(value) {
  const line = String(value).replace(/\r?\n$/u, "");
  if (line === "" || line.includes("\n") || line.includes("\r") || line.includes("\0")) {
    throw new Error("Git returned an invalid single-line value");
  }
  return line;
}

function assertCommit(value, label) {
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u.test(value)) throw new Error(`${label} is not a complete Git commit`);
}

function assertRepositoryKey(value) {
  if (typeof value !== "string" || !/^[a-z0-9][a-z0-9._-]{0,127}$/u.test(value)) {
    throw new Error("repository key is invalid");
  }
}

function assertAbsolutePath(value, label) {
  if (typeof value !== "string" || value.includes("\0") || !isAbsolute(value) || resolve(value) !== value) {
    throw new Error(`${label} must be a normalized absolute path`);
  }
}

function assertStringArray(value, label) {
  if (!Array.isArray(value) || value.some((entry) => typeof entry !== "string" || entry.trim() === "")) {
    throw new Error(`${label} must be an array of non-empty strings`);
  }
}

function assertExactKeys(value, keys, label) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error(`${label} must be an object`);
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  if (stableJSON(actual) !== stableJSON(expected)) throw new Error(`${label} has an invalid closed shape`);
}

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

// The launch decision records the user's choice after the complete assessment was shown.
