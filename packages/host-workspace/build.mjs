import { readFile, writeFile, mkdir } from "node:fs/promises";
import { dirname, join } from "node:path";
export const workspaceFiles = ["worktree-lifecycle.mjs", "worktree-snapshot.mjs", "repository-freshness.mjs", "integration-receipt.mjs"];
export async function syncWorkspaceFiles(root, host, destination = join(root, "packages", host, "lib")) {
  for (const name of workspaceFiles) {
    const source = await readFile(join(root, "packages/host-workspace", name), "utf8");
    const target = join(destination, name);
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, "// Generated from packages/host-workspace/" + name + "; edit the shared source.\n" + source);
  }
}
