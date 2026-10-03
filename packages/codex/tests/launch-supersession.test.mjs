import assert from "node:assert/strict";
import test from "node:test";
import {mkdtemp, mkdir, realpath, rm} from "node:fs/promises";
import {tmpdir} from "node:os";
import {join} from "node:path";
import {admissionFixture, writeHandoffFixture} from "./fixtures/task-handoff.mjs";
import {inspectAdmissionAnchor} from "../lib/task-admission.mjs";
import {defaultRunGit} from "../lib/worktree-lifecycle.mjs";
import {prepareTaskLaunch, provisionLocalTask, supersedeTaskLaunch, resumeLaunchSupersession, beginManagedTaskDispatch, claimManagedTaskDispatch} from "../lib/task-launch.mjs";
import {receiptDigest, provisioningReceiptPath, readProvisioningReceipt, writeProvisioningReceiptAtomic, updateProvisioningReceipt, withProvisioningReceiptLock} from "../lib/provisioning-receipt.mjs";

async function fixture(t, managed = false) {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-supersede-")));
  t.after(() => rm(root, {recursive:true, force:true}));
  const source = join(root,"source");
  await mkdir(source);
  await defaultRunGit(["init","-b","main",source]);
  await defaultRunGit(["-C",source,"-c","user.name=Fixture","-c","user.email=fixture@example.invalid","commit","--allow-empty","-m","base"]);
  const request = "Test launch supersession";
  const anchor = await inspectAdmissionAnchor({request,repositories:[{key:"primary",repository_path:source}]});
  const input = {request,...admissionFixture(anchor),launch_id:"before",repository_key:"primary",repository_path:source,workspace_mode:"new_branch",source_type:"local",carry_changes:false,remote_name:"",base_branch:"main",target_branch:"before",surface:"current_session",worktree_path:source,handoff_file:null};
  if (managed) Object.assign(input,{workspace_mode:"dedicated_worktree",surface:"managed_worktree",worktree_path:null,handoff_file:await writeHandoffFixture(root,request)});
  const options = {productSupportRoot:join(root,"support"),checkWorkspaceAvailable:async root => ({available:true,repository_path:root})};
  const prepared = await prepareTaskLaunch(input,options);
  const replacement = {...input,launch_id:"after",target_branch:"after"};
  const change = {launch_id:"before",repository_key:"primary",expected_receipt_digest:receiptDigest(prepared.receipt),reason:"Confirmed corrected branch choice",replacement};
  return {root,source,input,options,prepared,change};
}

test("supersession retains a unique successor and permanently rejects every old worker", async t => {
  const f = await fixture(t);
  const result = await supersedeTaskLaunch(f.change,f.options);
  assert.equal(result.predecessor.operation_status.phase,"superseded");
  assert.equal(result.receipt.predecessor.launch_id,"before");
  await assert.rejects(prepareTaskLaunch(f.input,f.options), /permanently superseded/);
  await assert.rejects(provisionLocalTask({launch_id:"before",repository_key:"primary"},f.options), /permanently superseded/);
  await assert.rejects(beginManagedTaskDispatch({launch_id:"before",repository_key:"primary",project_id:"p"},f.options), /permanently superseded/);
  await assert.rejects(claimManagedTaskDispatch({launch_id:"before",repository_key:"primary",dispatch_attempt_id:"old"},f.options), /permanently superseded/);
  await assert.rejects(supersedeTaskLaunch({...f.change,replacement:{...f.change.replacement,launch_id:"other"}},f.options), /already superseded/);
  const prepared = await prepareTaskLaunch(result.prepare_input,f.options);
  const done = await provisionLocalTask({launch_id:prepared.receipt.launch_id,repository_key:"primary"},f.options);
  assert.equal(done.receipt.operation_status.phase,"provisioned");
  assert.equal((await defaultRunGit(["-C",f.source,"branch","--show-current"])).trim(),"after");
});

test("supersede versus dispatch claim has exactly one permitted winner", async t => {
  const f = await fixture(t,true);
  const ready = await beginManagedTaskDispatch({launch_id:"before",repository_key:"primary",project_id:"p"},f.options);
  const results = await Promise.allSettled([
    supersedeTaskLaunch({...f.change,expected_receipt_digest:receiptDigest(ready.receipt)},f.options),
    claimManagedTaskDispatch({launch_id:"before",repository_key:"primary",dispatch_attempt_id:ready.receipt.operation_status.dispatch_attempt_id},f.options),
  ]);
  const replaced = results[0].status === "fulfilled";
  const dispatched = results[1].status === "fulfilled" && results[1].value.should_dispatch === true;
  assert.notEqual(replaced,dispatched);
  const prior = await readProvisioningReceipt(f.prepared.receipt_path,f.options);
  assert.equal(prior.operation_status.phase,replaced?"superseded":"dispatching");
  if (dispatched) await assert.rejects(supersedeTaskLaunch({...f.change,expected_receipt_digest:receiptDigest(prior)},f.options),/positive proof/);
});

test("crash after permanent mark resumes only its create-only successor seed", async t => {
  const f = await fixture(t);
  await assert.rejects(supersedeTaskLaunch(f.change,{...f.options,afterSupersession:()=>{throw new Error("crash after mark");}}),/crash after mark/);
  const prior = await readProvisioningReceipt(f.prepared.receipt_path,f.options);
  assert.equal(prior.operation_status.phase,"superseded");
  const input = {launch_id:"before",repository_key:"primary",seed_digest:prior.supersession.seed_digest};
  const resumed = await resumeLaunchSupersession(input,f.options);
  const reread = await resumeLaunchSupersession(input,f.options);
  assert.deepEqual(resumed.receipt,reread.receipt);
  await assert.rejects(resumeLaunchSupersession({...input,seed_digest:"0".repeat(64)},f.options),/matching retained/);
});

test("supersession shares the active worker lock and claims block replacement", async t => {
  const f = await fixture(t);
  await withProvisioningReceiptLock(f.prepared.receipt_path,f.options,async()=>{
    await assert.rejects(supersedeTaskLaunch(f.change,f.options), /already in progress/);
  });
  await assert.rejects(supersedeTaskLaunch(f.change,{...f.options,checkWorkspaceAvailable:async root=>({available:false,repository_path:root})}),/active TaskBelay Task/);
  const results = await Promise.allSettled([supersedeTaskLaunch(f.change,f.options),supersedeTaskLaunch({...f.change,replacement:{...f.change.replacement,launch_id:"other",target_branch:"other"}},f.options)]);
  assert.equal(results.filter(result=>result.status==="fulfilled").length,1);
});

test("pre-target failure can be corrected but an invoked Git failure cannot", async t => {
  for (const invoked of [false,true]) await t.test(String(invoked),async t => {
    const f = await fixture(t);
    const runGit = async (args,options) => {
      if (invoked ? args.includes("switch") : args.includes("check-ref-format")) throw new Error("injected boundary failure");
      return await defaultRunGit(args,options);
    };
    await assert.rejects(provisionLocalTask({launch_id:"before",repository_key:"primary"},{...f.options,runGit}),/injected boundary/);
    const failed = await readProvisioningReceipt(f.prepared.receipt_path,f.options);
    assert.equal(failed.operation_status.target_effects,invoked?"may_have_effects":"not_invoked");
    const change = {...f.change,expected_receipt_digest:receiptDigest(failed)};
    if (invoked) await assert.rejects(supersedeTaskLaunch(change,f.options),/positive proof/);
    else assert.equal((await supersedeTaskLaunch(change,f.options)).receipt.launch_id,"after");
  });
});

test("historical failed status without call evidence is insufficient", async t => {
  const f = await fixture(t);
  const failed = updateProvisioningReceipt(f.prepared.receipt,{phase:"failed",values:{}});
  delete failed.operation_status.target_effects;
  await writeProvisioningReceiptAtomic(f.prepared.receipt_path,failed,f.options);
  await assert.rejects(supersedeTaskLaunch({...f.change,expected_receipt_digest:receiptDigest(failed)},f.options),/positive proof/);
});
