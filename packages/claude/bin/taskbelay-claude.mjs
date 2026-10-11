#!/usr/bin/env node
import { readSync, realpathSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { core } from "../lib/runtime.mjs";
import * as lifecycle from "../lib/lifecycle.mjs";
import * as workspace from "../lib/workspace.mjs";
import { runHook } from "../plugin/hooks/pre-tool-use.mjs";
import { assertNoDuplicateJSONMembers } from "../lib/json.mjs";
const operations = ["inspect", "prepare", "provision", "status", "scope", "bind-task", "launch", "retry-launch", "resume", "record-session", "cleanup-worktree", "cleanup-branch"];
async function input() {
  const chunks = []; let size = 0;
  for (;;) {
    const chunk = Buffer.alloc(65536);
    const count = readSync(0, chunk, 0, chunk.length, null);
    if (!count) break;
    size += count;
    if (size > 1024 * 1024) throw new Error("Input exceeds 1 MiB");
    chunks.push(chunk.subarray(0, count));
  }
  const bytes = Buffer.concat(chunks);
  const raw = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  assertNoDuplicateJSONMembers(raw);
  const value = JSON.parse(raw);
  if (!value || Array.isArray(value) || typeof value !== "object") throw new Error("One closed JSON object required");
  return value;
}
export async function runCLI(args) {
  if (args.length === 0 || args.includes("--help")) {
    process.stdout.write("taskbelay-claude status|setup|remove [--json]\ntaskbelay-claude --version\ntaskbelay-claude mcp\ntaskbelay-claude host-check pre-file-write|workspace-available\ntaskbelay-claude hook pre-tool-use\ntaskbelay-claude host-launch " + operations.join("|") + "\nHost-launch input: one JSON object on closed stdin. inspect: {request,repositories:[{key,repository_path}]}; prepare: {request,authorization_ref,repositories:[{key,repository_path,workspace_mode,source_type,remote_name,base_branch,target_branch,carry_changes,worktree_path}],handoff}. Other operations: {launch_id,...operation inputs}. Read the Skill for exact operation contracts.\n");
    return 0;
  }
  if (args.length === 1 && args[0] === "--version") { process.stdout.write((await core(["version"])).stdout); return 0; }
  if (args.length === 2 && args[0] === "call") { const result = await core(["call",args[1]], {stream:true}); return result?.code ?? 0; }
  if (args.length === 1 && args[0] === "mcp") { await core(["mcp", "--stdio"], { stream: true }); return 0; }
  if (["status", "setup", "remove"].includes(args[0]) && (args.length === 1 || args.length === 2 && args[1] === "--json")) {
    process.stdout.write(JSON.stringify(await lifecycle[args[0]]()) + "\n"); return 0;
  }
  if (args.length === 2 && args[0] === "hook" && args[1] === "pre-tool-use") {
    try { const result = await runHook(await input()); if (result) process.stdout.write(JSON.stringify(result)); return 0; }
    catch (e) { process.stderr.write("TaskBelay write check failed: " + e.message); return 2; }
  }
  if (args.length === 2 && (args[0] === "host-check" && ["pre-file-write", "workspace-available"].includes(args[1]))) {
    try { process.stdout.write((await core(args, { input: await input() })).stdout); return 0; }
    catch (error) { if (error.stdout) { process.stdout.write(error.stdout); return error.code || 1; } throw error; }
  }
  if (args.length === 2 && args[0] === "host-launch" && operations.includes(args[1])) {
    const value = await input(), operation = args[1];
    let result;
    if (["inspect", "prepare"].includes(operation)) result = await workspace[operation](value);
    else {
      const { launch_id, ...rest } = value;
      if (["status", "scope", "provision"].includes(operation)) {
        if (Object.keys(rest).length) throw new Error("Unknown operation fields");
        result = await workspace[operation](launch_id);
      } else if (operation === "bind-task") result = await workspace.bindTask(launch_id, rest);
      else if (operation.startsWith("cleanup-")) result = await workspace.cleanup(launch_id, operation, rest);
      else result = await workspace.session(launch_id, operation, rest);
    }
    process.stdout.write(JSON.stringify(result) + "\n"); return 0;
  }
  throw new Error("Invalid command; use --help");
}
function isMainModule() {
  if (!process.argv[1]) return false;
  try {
    return import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href;
  } catch {
    return false;
  }
}

if (isMainModule()) {
  runCLI(process.argv.slice(2)).then(code => { process.exitCode = code; }, error => { process.stderr.write("taskbelay-claude: " + error.message + "\n"); process.exitCode = 1; });
}
