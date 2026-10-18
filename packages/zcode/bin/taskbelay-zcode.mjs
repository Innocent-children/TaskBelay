#!/usr/bin/env node
import { readSync, realpathSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { describeInterface, interfaceHelpRequest } from "../lib/interface-reference.mjs";
import { fileURLToPath } from "node:url";
import { core } from "../lib/runtime.mjs";
import * as lifecycle from "../lib/lifecycle.mjs";
import * as workspace from "../lib/workspace.mjs";
import { runHook } from "../hooks/pre-tool-use.mjs";
import { assertNoDuplicateJSONMembers } from "../lib/json.mjs";

export const operations = Object.freeze([
  "freshness", "inspect", "prepare", "provision", "status", "scope", "bind-task", "open", "resume", "cleanup-worktree", "cleanup-branch",
]);

function input() {
  const chunks = [];
  let size = 0;
  for (;;) {
    const chunk = Buffer.alloc(65536);
    const count = readSync(0, chunk, 0, chunk.length, null);
    if (!count) break;
    size += count;
    if (size > 1024 * 1024) throw new Error("Input exceeds 1 MiB");
    chunks.push(chunk.subarray(0, count));
  }
  const raw = new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks));
  assertNoDuplicateJSONMembers(raw);
  const value = JSON.parse(raw);
  if (!value || Array.isArray(value) || typeof value !== "object") throw new Error("One closed JSON object required");
  return value;
}

function lifecycleOptions(args) {
  if (!["status", "setup", "remove"].includes(args[0])) return null;
  const flags = args.slice(1);
  if (new Set(flags).size !== flags.length || flags.some(flag => flag !== "--json" && !(args[0] === "remove" && flag === "--confirm-host-removed"))) {
    throw new Error("Invalid lifecycle flags; use --help");
  }
  return { hostRemoved: flags.includes("--confirm-host-removed") };
}

export async function runCLI(args) {
  const help = interfaceHelpRequest(args);
  if (help) {
    const name = help.name ?? (help.operation ? `zcode.${help.group}.${help.operation}` : undefined);
    process.stdout.write(JSON.stringify(await describeInterface(fileURLToPath(new URL("../", import.meta.url)), name), null, 2) + "\n");
    return 0;
  }

  if (args.length === 0 || args.length === 1 && args[0] === "--help") {
    process.stdout.write("taskbelay-zcode status|setup|remove [--json]\n" +
      "taskbelay-zcode remove --confirm-host-removed [--json]\n" +
      "taskbelay-zcode --version\ntaskbelay-zcode mcp\ntaskbelay-zcode check run\n" +
      "" +
      "taskbelay-zcode host-check pre-file-write|workspace-available\n" +
      "taskbelay-zcode hook pre-tool-use\n" +
      `taskbelay-zcode host-launch ${operations.join("|")}\n` +
      "Host-launch accepts one closed JSON object on stdin. inspect: {request,repositories:[{key,repository_path}]}; prepare: {request,authorization_ref,repositories:[{key,repository_path,workspace_mode,source_type,remote_name,base_branch,target_branch,carry_changes,worktree_path}],handoff}. Other operations: {launch_id,...operation inputs}. Use host-launch <operation> --help --json for complete schemas and examples.\n" +
      "open/resume return UI guidance only. --confirm-host-removed records the user's confirmation that the plugin/marketplace were removed in ZCode and affected sessions were closed; it does not check ZCode automatically.\n");
    return 0;
  }
  if (args.length === 1 && args[0] === "--version") {
    process.stdout.write((await core(["version"])).stdout); return 0;
  }
  if (args.length === 2 && (args[0] === "call" || args[0] === "check" && args[1] === "run")) { const result = await core(args, {stream:true}); return result?.code ?? 0; }
  if (args.length === 1 && args[0] === "mcp") { return (await core(["mcp", "--stdio", "--result-text", "summary"], { stream: true })).code; }
  const settings = lifecycleOptions(args);
  if (settings) { process.stdout.write(JSON.stringify(await lifecycle[args[0]](settings)) + "\n"); return 0; }
  if (args.length === 2 && args[0] === "hook" && args[1] === "pre-tool-use") {
    try {
      const result = await runHook(input());
      if (result) process.stdout.write(JSON.stringify(result) + "\n");
      return 0;
    } catch (error) {
      process.stderr.write(`TaskBelay write check failed: ${error.message}\n`);
      return 2;
    }
  }
  if (args.length === 2 && (args[0] === "host-check" && ["pre-file-write", "workspace-available"].includes(args[1]))) {
    try { process.stdout.write((await core(args, { input: input() })).stdout); return 0; }
    catch (error) {
      if (error.stdout) { process.stdout.write(error.stdout); return error.code || 1; }
      throw error;
    }
  }
  if (args.length === 2 && args[0] === "host-launch" && operations.includes(args[1])) {
    const value = input(), operation = args[1];
    let result;
    if (["freshness", "inspect", "prepare"].includes(operation)) result = await workspace[operation](value);
    else {
      const { launch_id, ...rest } = value;
      if (["status", "scope", "provision", "open", "resume"].includes(operation)) {
        if (Object.keys(rest).length) throw new Error("Unknown operation fields");
        result = ["open", "resume"].includes(operation)
          ? await workspace.session(launch_id, operation) : await workspace[operation](launch_id);
      } else if (operation === "bind-task") result = await workspace.bindTask(launch_id, rest);
      else result = await workspace.cleanup(launch_id, operation, rest);
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
  runCLI(process.argv.slice(2)).then(code => { process.exitCode = code; }, error => {
    process.stderr.write(`taskbelay-zcode: ${error.message}\n`);
    if (error.launch_id && error.receipt_path) {
      process.stderr.write(`Saved launch_id: ${error.launch_id}\nReceipt: ${error.receipt_path}\n`);
    }
    process.exitCode = 1;
  });
}
