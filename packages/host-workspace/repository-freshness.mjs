import { inspectSourceRepository, defaultRunGit } from "./worktree-lifecycle.mjs";

// Fetch updates remote refs only. Integration is separately authorized and preflighted for all roots.
export async function assessRepositoryFreshness(input, {runGit=defaultRunGit, now=()=>new Date()}={}) {
 const allowed=["repositories","fetch","integrate","authorization_ref","integration_authorization_ref"];
 if (!input || typeof input!=="object" || Array.isArray(input) || Object.keys(input).some(k=>!allowed.includes(k)) || !Array.isArray(input.repositories) || input.repositories.length<1 || input.repositories.length>8) throw new Error("freshness requires a closed request with one to eight repositories");
 const fetch=input.fetch??true, integrate=input.integrate??false;
 if(typeof fetch!=="boolean" || typeof integrate!=="boolean" || integrate&&!fetch) throw new Error("integration requires a fresh fetch");
 if(integrate && (typeof input.integration_authorization_ref!=="string" || !input.integration_authorization_ref.trim())) throw new Error("separate integration authorization reference required");
 if((fetch||integrate) && (typeof input.authorization_ref!=="string" || !input.authorization_ref.trim())) throw new Error("actual Git authorization reference required for fetch or integration");
 const roots=new Set(),keys=new Set(),reports=[];
 for(const item of input.repositories) {
  if(!item || Object.keys(item).some(k=>!["key","repository_path","base_branch"].includes(k)) || !Object.hasOwn(item,"key") || !Object.hasOwn(item,"repository_path") || !/^[a-z0-9][a-z0-9._-]{0,127}$/.test(item.key) || keys.has(item.key)) throw new Error("invalid or duplicate repository key/path");
  const source=await inspectSourceRepository(item.repository_path,{runGit});
  if(roots.has(source.canonical_root)) throw new Error("duplicate canonical worktree");roots.add(source.canonical_root);keys.add(item.key);
  reports.push({key:item.key,repository_path:source.canonical_root,source,remote_freshness:{head:source.head,upstream:"",remote_head:"",state:"not_checked",fetched:false,remote_name:"",remote_ref:"",checked_at:now().toISOString(),basis:""},action:"inspect",integrated:false});
 }
 for(const report of reports) {
  const source=report.source,f=report.remote_freshness,git=(...args)=>runGit(["-C",source.canonical_root,...args]);
  if(!source.branch) {f.state="detached";f.basis="No current branch; no corresponding upstream was selected.";continue}
  const baseBranch=input.repositories.find(r=>r.key===report.key).base_branch??source.branch;
  if(typeof baseBranch!=="string" || !baseBranch) throw new Error("base_branch must name the retained task source branch");
  await git("check-ref-format","--branch",baseBranch);
  if(baseBranch!==source.branch) await git("rev-parse","--verify",`refs/heads/${baseBranch}^{commit}`);
  const config=String(await git("for-each-ref","--format=%(upstream)%00%(upstream:remotename)%00%(upstream:remoteref)",`refs/heads/${baseBranch}`)).trim().split("\0");
  const [upstream,remote,remoteRef]=config;
  if(!upstream) {f.state="no_upstream";f.basis="Selected task source branch has no configured upstream; local work may continue without claiming remote freshness.";continue}
  if(!upstream.startsWith("refs/remotes/") || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(remote??"") || !remoteRef?.startsWith("refs/heads/")) {f.state="not_fetched";f.basis="Upstream is local or unsupported; assess it explicitly without claiming a remote fetch.";continue}
  f.upstream=upstream;f.remote_name=remote;f.remote_ref=remoteRef;
  if(!fetch) {f.state="not_fetched";f.basis="Fetch was explicitly skipped; cached remote refs do not establish current remote freshness.";continue}
  try {await git("fetch","--no-tags",remote,`+${remoteRef}:${upstream}`);f.fetched=true;f.fetched_at=now().toISOString()}
  catch(error) {f.checked_at=now().toISOString();f.state="fetch_failed";f.basis="Fetch failed or was denied; preserve local work and inspect the original failure. "+String(error.message).slice(0,300);continue}
  f.remote_head=String(await git("rev-parse","--verify",`${upstream}^{commit}`)).trim();
  if(!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(f.remote_head)) throw new Error("invalid fetched commit");
  const counts=String(await git("rev-list","--left-right","--count",`HEAD...${upstream}`)).trim().split(/\s+/).map(Number);
  if(counts.length!==2 || counts.some(n=>!Number.isSafeInteger(n)||n<0)) throw new Error("invalid upstream comparison");
  const [ahead,behind]=counts;f.state=ahead&&behind?"diverged":behind?"behind":ahead?"ahead":"current";
  f.basis=`Fetched the configured upstream of ${baseBranch}; local ahead ${ahead}, behind ${behind}.`;
  report.action=f.state==="behind"?(source.clean?"fast_forward_available":"preserve_dirty"):f.state==="diverged"?"preserve_divergence":"continue";
  f.checked_at=now().toISOString();
 }
 // Never partially integrate an unsafe multi-repository selection.
 const unsafe=reports.some(r=>!r.source.clean || !["current","ahead","behind","no_upstream"].includes(r.remote_freshness.state));
 if(integrate&&!unsafe) {
  for(const report of reports) {
   if(report.remote_freshness.state!=="behind") continue;
   const current=await inspectSourceRepository(report.repository_path,{runGit});
   if(!current.clean || current.head!==report.source.head || current.branch!==report.source.branch || current.status_digest!==report.source.status_digest) throw new Error("workspace changed before fast-forward; preserve and inspect");
   const git=(...args)=>runGit(["-C",report.repository_path,...args]);
   const actual=String(await git("for-each-ref","--format=%(upstream)",`refs/heads/${input.repositories.find(r=>r.key===report.key).base_branch??current.branch}`)).trim();
   if(actual!==report.remote_freshness.upstream) throw new Error("upstream changed before integration");
   await git("merge","--ff-only","--no-edit","--no-autostash",report.remote_freshness.remote_head);
   const after=await inspectSourceRepository(report.repository_path,{runGit});
   if(after.head!==report.remote_freshness.remote_head || after.branch!==current.branch || !after.clean) throw new Error("fast-forward result is uncertain; inspect original operation before retrying");
   report.integrated=true;report.remote_freshness.head=after.head;report.remote_freshness.state="current";report.remote_freshness.checked_at=now().toISOString();report.action="continue";
  }
 }
 return {repositories:reports.map(({source,...report})=>report),integration_blocked:integrate&&unsafe};
}
