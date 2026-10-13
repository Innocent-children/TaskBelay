// Generated from packages/host-workspace/integration-receipt.mjs; edit the shared source.
import { createHash, randomUUID } from "node:crypto";
import { lstat, mkdir, open, readFile, rename, unlink } from "node:fs/promises";
import { dirname, isAbsolute, join, resolve } from "node:path";

const idPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u;
export const requestDigest = value => createHash("sha256").update(JSON.stringify(value)).digest("hex");

// A Host operation receipt, not a Task. Existing records are never executed again.
export async function integrationReceipt(root, id, request) {
  if (typeof root !== "string" || !isAbsolute(root) || root !== resolve(root)) throw new Error("integration requires the Host's canonical receipt root");
  if (typeof id !== "string" || !idPattern.test(id)) throw new Error("integration requires a stable operation_id");
  await directory(root);
  const parent = join(root, "freshness");
  await directory(parent);
  const path = join(parent, id + ".json"), digest = requestDigest(request);
  const read = async () => {
    await regular(path, true);
    let record;
    try { record = JSON.parse(await readFile(path, "utf8")); }
    catch (error) { if (error.code === "ENOENT") return null; throw error; }
    if (!record || record.operation_id !== id || record.request_digest !== digest || requestDigest(record.request) !== digest || !Array.isArray(record.repositories) || record.repositories.length !== request.repositories.length || !["assessing", "integrating", "complete", "blocked", "partial", "uncertain", "reconciled"].includes(record.status) || !Number.isSafeInteger(record.owner_pid) || record.owner_pid < 0) throw new Error("integration receipt identity/request is invalid; preserve it");
    for (let i = 0; i < record.repositories.length; i++) {
      const item = record.repositories[i], selected = request.repositories[i];
      if (!item || typeof item!=="object" || item.key !== selected.key || item.repository_path !== selected.repository_path || requestDigest(item.source_selection) !== requestDigest(selected.source_selection) || !item.source || !item.integration || !["pending", "not_needed", "blocked", "applying", "applied", "unknown"].includes(item.integration.state)) throw new Error("integration receipt repository binding is invalid; preserve it");
    }
    const commit=/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u;
    for(const item of record.repositories){
      if(item.source.canonical_root!==item.repository_path || item.source.branch!==null&&typeof item.source.branch!=="string" || !commit.test(item.source.head) || !/^[0-9a-f]{64}$/u.test(item.source.instance) || typeof item.source.clean!=="boolean" || typeof item.integrated!=="boolean" || item.integrated!==(item.integration.state==="applied") || item.integration.target_head!==""&&!commit.test(item.integration.target_head)) throw new Error("integration receipt facts are invalid; preserve it");
      if(["applying","applied","unknown"].includes(item.integration.state)&&!commit.test(item.integration.target_head)) throw new Error("integration attempt has no valid frozen target; preserve it");
    }
    if(record.status==="complete"&&record.repositories.some(item=>!["applied","not_needed"].includes(item.integration.state))) throw new Error("integration receipt completion is inconsistent; preserve it");
    return record;
  };
  let activeLock;
  const save = async record => {
    if(!activeLock) throw new Error("receipt writes require the owned operation lock");
    await activeLock.verify();
    await regular(path, true);
    const temporary = path + "." + randomUUID() + ".tmp";
    const handle = await open(temporary, "wx", 0o600);
    try { await handle.writeFile(JSON.stringify(record) + "\n"); await handle.sync(); }
    finally { await handle.close(); }
    try {
      await activeLock.verify();
      await rename(temporary, path);
      if(process.platform!=="win32"){
        const dir = await open(parent, "r");
        try { await dir.sync(); } finally { await dir.close(); }
      }
    } finally { await unlink(temporary).catch(() => {}); }
  };
  return { path, digest, read, save, lock: async recovery => { activeLock=await lock(path + (recovery ? ".recovery.lock" : ".lock"),recovery);return activeLock; } };
}

export function ownerRunning(pid) {
  if (!Number.isSafeInteger(pid) || pid <= 0) return false;
  try { process.kill(pid, 0); return true; }
  catch (error) { if (error.code === "ESRCH") return false; throw error; }
}

async function lock(path, recoverStale = false) {
  await regular(path, true);
  try { return await claimLock(path); }
  catch (cause) {
    if(cause.code!=="EEXIST") throw cause;
    if(!recoverStale) throw locked(cause);
  }
  // Only read-only recovery locks can be reclaimed. A separate exclusive claim
  // prevents two reclaimers from deleting a replacement owner's lock.
  let fence;
  try { fence=await claimLock(path+".claim"); }
  catch(cause){if(cause.code!=="EEXIST")throw cause;throw locked(cause)}
  try {
    const retained=await lockFact(path);
    if(ownerRunning(retained.pid))throw new Error("recovery lock owner is still running; preserve the lock");
    await verifyLock(path,retained);
    await unlink(path);
    return await claimLock(path);
  } finally { await fence(); }
}
function locked(cause){const error=new Error("integration/recovery operation is already owned; read the retained receipt and establish its owner/identity before recovery",{cause});error.code="ELOCKED";return error}
async function claimLock(path){
  await regular(path,true);
  const handle=await open(path,"wx",0o600),token=randomUUID();
  try{await handle.writeFile(JSON.stringify({pid:process.pid,token})+"\n");await handle.sync()}
  catch(error){await handle.close();throw error} // Incomplete identity stays blocked.
  const identity=await handle.stat(),fact={pid:process.pid,token,identity};
  const release=async()=>{try{await verifyLock(path,fact);await unlink(path)}finally{await handle.close()}};
  release.verify=()=>verifyLock(path,fact);
  return release;
}
async function lockFact(path){
  await regular(path,false);
  const identity=await lstat(path),value=JSON.parse(await readFile(path,"utf8"));
  if(!Number.isSafeInteger(value.pid)||value.pid<=0||typeof value.token!=="string"||!/^[0-9a-f-]{36}$/u.test(value.token))throw new Error("recovery lock owner/identity is unknown; preserve the lock");
  return {...value,identity};
}
async function verifyLock(path,fact){
  const current=await lockFact(path);
  if(current.pid!==fact.pid||current.token!==fact.token||current.identity.dev!==fact.identity.dev||current.identity.ino!==fact.identity.ino||current.identity.birthtimeMs!==fact.identity.birthtimeMs)throw new Error("operation lock changed; preserve the replacement lock and receipt");
}

async function regular(path, missing) {
  try { const info = await lstat(path); if (!info.isFile() || info.isSymbolicLink() || typeof process.getuid === "function" && info.uid !== process.getuid()) throw new Error("receipt/lock must be an owned regular file"); }
  catch (error) { if (!missing || error.code !== "ENOENT") throw error; }
}
async function directory(path) {
  const parent = dirname(path);
  if (parent !== path) await noSymlinks(parent);
  try { await mkdir(path, { recursive: true, mode: 0o700 }); }
  catch (error) { if (error.code !== "EEXIST") throw error; }
  await noSymlinks(path);
  const info = await lstat(path);
  if (!info.isDirectory() || info.isSymbolicLink() || typeof process.getuid === "function" && info.uid !== process.getuid()) throw new Error("receipt root must be an owned directory");
}
async function noSymlinks(path) {
  const parent = dirname(path);
  if (parent !== path) await noSymlinks(parent);
  try { const info = await lstat(path); if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("receipt path contains a symbolic link"); }
  catch(error) { if(error.code!=="ENOENT") throw error; }
}
