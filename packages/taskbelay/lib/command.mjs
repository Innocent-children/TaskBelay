import { execFile as execFileCallback } from "node:child_process";
import * as macos from "./platform/macos/command.mjs";
import * as windows from "./platform/windows/command.mjs";

const implementations = Object.freeze({ darwin: macos, win32: windows });
function commands(platform = process.platform) {
  const implementation = implementations[platform];
  if (!implementation) throw new Error(`unsupported command platform ${platform}`);
  return implementation;
}

export async function execPortableCommand(executable, arguments_, options = {}) {
  const environment = options.env ?? process.env;
  const invocation = await commands().resolveCommandInvocation(executable, environment, arguments_);
  return execFileWithClosedInput(invocation.executable, [...invocation.prefixArguments, ...(invocation.arguments ?? arguments_)], options);
}

function execFileWithClosedInput(executable, arguments_, { input, ...options }) {
  return new Promise((resolvePromise, reject) => {
    const child = execFileCallback(executable, arguments_, options, (error, stdout, stderr) => {
      if (error) { error.stdout = stdout; error.stderr = stderr; reject(error); }
      else resolvePromise({ stdout, stderr });
    });
    child.stdin?.on("error", () => {});
    child.stdin?.end(input);
  });
}

export function formatCommand(arguments_, platform = process.platform) {
  return arguments_.map(argument => commands(platform).quoteArgument(String(argument))).join(" ");
}
