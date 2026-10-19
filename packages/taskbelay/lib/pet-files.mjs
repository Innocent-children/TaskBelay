import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { lstat, readdir } from "node:fs/promises";
import { join } from "node:path";

/**
 * Compare the application only, excluding user settings and appearances.
 * Content comparison also detects same-version local builds and changed assets.
 */
export async function petApplicationsDiffer(source, target) {
  const sourceInfo = await lstat(source);
  const targetInfo = await lstat(target).catch(error => { if (error.code !== "ENOENT") throw error; return null; });
  if (sourceInfo.isSymbolicLink() || targetInfo?.isSymbolicLink()) throw new Error("desktop application paths must not contain links");
  if (!targetInfo) return true;
  if (sourceInfo.isDirectory() && targetInfo.isDirectory()) {
    const [left, right] = await Promise.all([readdir(source), readdir(target)]);
    left.sort(); right.sort();
    if (JSON.stringify(left) !== JSON.stringify(right)) return true;
    for (const name of left) if (await petApplicationsDiffer(join(source, name), join(target, name))) return true;
    return false;
  }
  if (!sourceInfo.isFile() || !targetInfo.isFile() || sourceInfo.size !== targetInfo.size) return true;
  const hash = async path => {
    const digest = createHash("sha256");
    for await (const bytes of createReadStream(path)) digest.update(bytes);
    return digest.digest("hex");
  };
  const [left, right] = await Promise.all([hash(source), hash(target)]);
  return left !== right;
}
