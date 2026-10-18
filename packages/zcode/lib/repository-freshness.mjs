// Generated from packages/host-workspace/repository-freshness.mjs; edit the shared source.
import { lstat } from "node:fs/promises";
import { inspectSourceRepository, defaultRunGit } from "./worktree-lifecycle.mjs";
import { integrationReceipt, ownerRunning, requestDigest } from "./integration-receipt.mjs";

// Bound callers supply the original source selection even after the task branch gets its own upstream.
export async function selectRepositoryUpstream(item, source, runGit) {
  const selection = { source_type: item.source_type ?? "local", remote_name: item.remote_name ?? "", base_branch: item.base_branch ?? source.branch };
  if (!["local", "remote"].includes(selection.source_type) || selection.source_type === "local" && selection.remote_name !== "" || selection.source_type === "remote" && (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u.test(selection.remote_name) || !item.base_branch)) throw new Error("explicit source_type/remote_name/base_branch selection is invalid");
  if (!selection.base_branch) return { selection, state: "detached" };
  const git = (...args) => runGit(["-C", source.canonical_root, ...args]);
  await git("check-ref-format", "--branch", selection.base_branch);
  if (selection.source_type === "remote") {
    const remotes = String(await git("remote")).trim().split(/\r?\n/u);
    if (!remotes.includes(selection.remote_name)) throw new Error("bound source remote no longer exists");
    return { selection, upstream: `refs/remotes/${selection.remote_name}/${selection.base_branch}`, remote: selection.remote_name, remote_ref: `refs/heads/${selection.base_branch}` };
  }
  const ref = `refs/heads/${selection.base_branch}`;
  const records = String(await git("for-each-ref", "--format=%(refname)%00%(upstream)%00%(upstream:remotename)%00%(upstream:remoteref)", ref)).trim().split(/\r?\n/u).map(line=>line.split("\0"));
  const config=records.find(fields=>fields[0]===ref)??[];
  const [, upstream, remote, remoteRef] = config;
  if (!upstream) return { selection, state: "no_upstream" };
  if (!upstream.startsWith("refs/remotes/") || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u.test(remote ?? "") || !remoteRef?.startsWith("refs/heads/")) return { selection, state: "not_fetched" };
  return { selection, upstream, remote, remote_ref: remoteRef };
}

export async function assessRepositoryFreshness(input, { runGit = defaultRunGit, now = () => new Date(), receiptRoot } = {}) {
  const allowed = ["repositories", "fetch", "integrate", "authorization_ref", "integration_authorization_ref", "operation_id", "recover"];
  if (!input || typeof input !== "object" || Array.isArray(input) || Object.keys(input).some(k => !allowed.includes(k)) || !Array.isArray(input.repositories) || input.repositories.length < 1 || input.repositories.length > 8) throw new Error("freshness requires a closed request with one to eight repositories");
  const fetch = input.fetch ?? true, integrate = input.integrate ?? false, recover = input.recover ?? false;
  if (typeof fetch !== "boolean" || typeof integrate !== "boolean" || typeof recover !== "boolean" || integrate && !fetch || recover && !integrate) throw new Error("integration requires a fresh fetch request; recovery requires the original integration request");
  if (integrate && (typeof input.integration_authorization_ref !== "string" || !input.integration_authorization_ref.trim())) throw new Error("separate integration authorization reference required");
  if ((fetch || integrate) && (typeof input.authorization_ref !== "string" || !input.authorization_ref.trim())) throw new Error("actual Git authorization reference required for fetch or integration");
  if (integrate && (typeof input.operation_id !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/u.test(input.operation_id))) throw new Error("integration requires a stable operation_id");
  const roots = new Set(), keys = new Set(), reports = [];
  for (const item of input.repositories) {
    if (!item || typeof item !== "object" || Array.isArray(item) || !Object.hasOwn(item,"key") || !Object.hasOwn(item,"repository_path") || typeof item.key !== "string" || ["source_type","remote_name","base_branch"].some(k=>Object.hasOwn(item,k)&&typeof item[k]!=="string") || Object.keys(item).some(k => !["key", "repository_path", "source_type", "remote_name", "base_branch"].includes(k)) || !/^[a-z0-9][a-z0-9._-]{0,127}$/u.test(item.key) || keys.has(item.key)) throw new Error("invalid or duplicate repository key/path");
    const source = await inspectSourceRepository(item.repository_path, { runGit });
    if (roots.has(source.canonical_root)) throw new Error("duplicate canonical worktree");
    roots.add(source.canonical_root); keys.add(item.key);
    const selected = await selectRepositoryUpstream(item, source, runGit);
    reports.push({ key: item.key, repository_path: source.canonical_root, source: { ...source, instance: await instance(source) }, source_selection: selected.selection,
      remote_freshness: { head: source.head, upstream: "", remote_head: "", state: "not_checked", fetched: false, remote_name: "", remote_ref: "", checked_at: now().toISOString(), basis: "Source selected; no fetch completed." }, action: "inspect", integrated: false,
      integration: { state: "pending", target_head: "", basis: "No integration attempted." } });
  }
  let receipt, record, release;
  if (integrate) {
    const request = { repositories: reports.map(r => ({ key: r.key, repository_path: r.repository_path, source_selection: r.source_selection })), fetch, integrate, authorization_ref: input.authorization_ref, integration_authorization_ref: input.integration_authorization_ref };
    receipt = await integrationReceipt(receiptRoot, input.operation_id, request);
    const retained = await receipt.read();
    if (retained) {
      if (!recover) return output(retained, receipt.path, true);
      return await reconcile(retained, receipt, { runGit, now });
    }
    if (recover) throw new Error("no retained integration intent; recovery cannot create or execute an operation");
    release = await receipt.lock(false);
    try {
      if (await receipt.read()) throw new Error("integration identity was claimed concurrently; read the saved receipt");
      record = { operation_id: input.operation_id, request_digest: receipt.digest, request, status: "assessing", owner_pid: process.pid, created_at: now().toISOString(), repositories: reports };
      await receipt.save(record); // Durable intent precedes even the first fetch.
    } catch (error) { await release(); throw error; }
  }
  const save = async () => { if (receipt) await receipt.save(record); };
  try {
    for (const report of reports) {
      const source = report.source, f = report.remote_freshness, git = (...args) => runGit(["-C", report.repository_path, ...args]);
      const selected = await selectRepositoryUpstream(report.source_selection, source, runGit);
      if (selected.state) {
        f.state = selected.state;
        f.basis = selected.state === "detached" ? "No selected named source; remote freshness is unknown." : "The selected source has no supported configured remote upstream; local work is preserved without claiming latest.";
        await save(); continue;
      }
      f.upstream = selected.upstream; f.remote_name = selected.remote; f.remote_ref = selected.remote_ref;
      if (!fetch) { f.state = "not_fetched"; f.basis = "Fetch was explicitly skipped; cached refs do not establish current remote freshness."; continue; }
      report.fetch_status = "attempting"; await save();
      try { await git("fetch", "--no-tags", selected.remote, `+${selected.remote_ref}:${selected.upstream}`); f.fetched = true; f.fetched_at = now().toISOString(); report.fetch_status = "completed"; }
      catch (error) { f.state = "fetch_failed"; f.checked_at = now().toISOString(); f.basis = "Fetch failed or was denied; preserve work and inspect the original failure. " + error.message.slice(0, 300); report.fetch_status = "failed"; report.failure=errorFact(error); await save(); continue; }
      f.remote_head = String(await git("rev-parse", "--verify", `${selected.upstream}^{commit}`)).trim();
      if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u.test(f.remote_head)) throw new Error("invalid fetched commit");
      const counts = String(await git("rev-list", "--left-right", "--count", `HEAD...${selected.upstream}`)).trim().split(/\s+/u).map(Number);
      if (counts.length !== 2 || counts.some(n => !Number.isSafeInteger(n) || n < 0)) throw new Error("invalid upstream comparison");
      const [ahead, behind] = counts; f.state = ahead && behind ? "diverged" : behind ? "behind" : ahead ? "ahead" : "current";
      f.checked_at = now().toISOString(); f.basis = `Fetched explicitly selected ${selected.selection.source_type} source ${selected.selection.base_branch}; local ahead ${ahead}, behind ${behind}.`;
      report.action = f.state === "behind" ? (source.clean ? "fast_forward_available" : "preserve_dirty") : f.state === "diverged" ? "preserve_divergence" : "continue";
      report.integration.target_head = f.remote_head; await save();
    }
    if (!integrate) return { repositories: reports.map(publicReport), integration_blocked: false };
    const unsafe = reports.some(r => !r.source.clean || !["current", "ahead", "behind", "no_upstream"].includes(r.remote_freshness.state));
    if (unsafe) {
      for (const r of reports) { r.integration.state = "blocked"; r.integration.basis = "Whole selection is unsafe; no integration attempted."; }
      record.status = "blocked";
    } else {
      // Recheck every root before the first mutation, then again immediately before each mutation.
      for (const r of reports) await unchanged(r, runGit);
      record.status = "integrating"; await save();
      for (const r of reports) {
        if (r.remote_freshness.state !== "behind") { r.integration.state = "not_needed"; await save(); continue; }
        try {
          await unchanged(r, runGit);
          r.integration.state = "applying"; r.integration.attempted_at = now().toISOString(); await save();
          await runGit(["-C", r.repository_path, "merge", "--ff-only", "--no-edit", "--no-autostash", r.integration.target_head]);
          const after = await inspectSourceRepository(r.repository_path, { runGit });
          if (!after.clean || after.head !== r.integration.target_head || after.branch !== r.source.branch || await instance(after) !== r.source.instance) throw new Error("fast-forward result is uncertain; inspect the original attempt");
          r.integrated = true; r.remote_freshness.head = after.head; r.remote_freshness.state = "current"; r.remote_freshness.checked_at = now().toISOString(); r.action = "continue";
          r.integration.state = "applied"; r.integration.finished_at = now().toISOString(); r.integration.basis = "Actual fast-forward returned and the bound clean target was verified."; await save();
        } catch (error) {
          r.integration.state = r.integration.state === "applying" ? "unknown" : r.integration.state === "applied" ? "applied" : "blocked";
          r.integration.basis = error.message; r.integration.failure=errorFact(error); r.action = "preserve_and_inspect";
          record.status = r.integration.state === "unknown" ? "uncertain" : reports.some(x => x.integrated) ? "partial" : "blocked";
          record.error = error.message; break;
        }
      }
      if (record.status === "integrating") record.status = "complete";
    }
    record.owner_pid = 0; await save(); return output(record, receipt.path, false);
  } catch (error) {
    if (!receipt) throw error;
    record.status = "uncertain"; record.error = error.message;
    try { record.owner_pid = 0; await save(); }
    catch (failure) { record.receipt_error = failure.message; }
    return output(record, receipt.path, false);
  } finally { if (release) await release(); }
}

async function instance(source) {
  const info = await lstat(source.worktree_git_dir, { bigint: true });
  return requestDigest([source.canonical_root, source.worktree_git_dir, String(info.dev), String(info.ino), String(info.birthtimeNs)]);
}
async function unchanged(report, runGit) {
  const current = await inspectSourceRepository(report.repository_path, { runGit });
  const selected = await selectRepositoryUpstream(report.source_selection, current, runGit);
  if ((selected.upstream ?? "") !== report.remote_freshness.upstream || (selected.remote ?? "") !== report.remote_freshness.remote_name || (selected.remote_ref ?? "") !== report.remote_freshness.remote_ref) throw new Error("bound source upstream changed before integration");
  if (!current.clean || current.head !== report.source.head || current.branch !== report.source.branch || current.status_digest !== report.source.status_digest || await instance(current) !== report.source.instance) throw new Error("workspace changed before fast-forward; preserve and inspect");
}
function publicReport({ source, ...report }) { return report; }
function output(record, path, replayed) {
  return { repositories: record.repositories.map(publicReport), integration_blocked: record.status !== "complete", operation: { operation_id: record.operation_id, receipt_path: path, status: record.status, replayed, error: record.error, receipt_error: record.receipt_error } };
}
async function reconcile(record, receipt, { runGit, now }) {
  if (["complete", "blocked", "partial", "reconciled"].includes(record.status)) return output(record, receipt.path, true);
  if (ownerRunning(record.owner_pid)) throw new Error("original integration owner is still running; inspect the retained receipt");
  const release = await receipt.lock(true);
  try {
    record = await receipt.read();
    if (ownerRunning(record.owner_pid)) throw new Error("original integration owner is still running");
    for (const r of record.repositories) {
      const current = await inspectSourceRepository(r.repository_path, { runGit });
      const same = await instance(current) === r.source.instance && current.branch === r.source.branch;
      const observed = same && current.clean && current.head === r.integration.target_head ? "target_present" : same && current.head === r.source.head && current.status_digest === r.source.status_digest ? "original_state_present" : "workspace_drift";
      r.integration.recovery = { checked_at: now().toISOString(), head: current.head, clean: current.clean, observation: observed, basis: "Read-only reconciliation of current Git facts. It does not prove the original command outcome, repeat a fetch/merge, continue remaining roots or roll back work." };
      if (r.integration.state === "applying") r.integration.state = "unknown";
      if(r.fetch_status==="attempting")r.fetch_status="unknown";
      r.remote_freshness.head = current.head; r.remote_freshness.fetched = false; delete r.remote_freshness.fetched_at;
      r.remote_freshness.state = "not_fetched"; r.remote_freshness.checked_at = now().toISOString(); r.remote_freshness.basis = "Recovered local Git state; no new fetch occurred. Read the retained frozen target and per-root result before any new operation.";
    }
    record.status = "reconciled"; record.owner_pid = 0; await receipt.save(record);
    return output(record, receipt.path, true);
  } finally { await release(); }
}

function errorFact(error){return {message:String(error.message).slice(0,1000),code:error.code??error.cause?.code??null,stderr:String(error.stderr??error.cause?.stderr??"").slice(0,2048)}}
