import { receiptAdmissionFixture } from "./fixtures/task-handoff.mjs";
import assert from "node:assert/strict";
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  createProvisioningReceipt as rawCreateProvisioningReceipt,
  provisioningReceiptPath,
  readProvisioningReceipt,
  updateProvisioningReceipt,
  validateProvisioningReceipt,
  withProvisioningReceiptLock,
  writeProvisioningReceiptAtomic,
} from "../lib/provisioning-receipt.mjs";

test("provisioning receipts use one closed secret-free shape and immutable launch identity", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-codex-receipt-")));
  const support = join(root, "support");
  await mkdir(support);
  t.after(() => rm(root, { recursive: true, force: true }));
  const receipt = fixtureReceipt();
  assert.deepEqual(validateProvisioningReceipt(receipt), receipt);
  assert.throws(() => updateProvisioningReceipt(receipt, { phase: "confirmed", values: { handoff_digest: "e".repeat(64) } }), /cannot change handoff_digest/u);
  assert.throws(() => validateProvisioningReceipt({ ...receipt, remote_url: "secret" }), /closed shape/u);
  assert.throws(() => createProvisioningReceipt({
    launchId: "launch-1",
    requestDigest: "a".repeat(64),
    handoffDigest: "d".repeat(64),
    sourceRepositoryIdentity: "b".repeat(64),
    repositoryKey: "primary",
    sourceType: "remote", carryChanges: false, remoteName: "ssh://private.example/repository",
    baseBranch: "main",
    targetBranch: "codex/task",
    surface: "managed_worktree",
  }), /remote_name/u);

  const path = provisioningReceiptPath(support, receipt.launch_id, receipt.repository_key);
  await writeProvisioningReceiptAtomic(path, receipt, { productSupportRoot: support, createOnly: true });
  assert.deepEqual(await readProvisioningReceipt(path, { productSupportRoot: support }), receipt);
  await assert.rejects(
    writeProvisioningReceiptAtomic(path, receipt, { productSupportRoot: support, createOnly: true }),
    /already exists/u,
  );
  const fetching = updateProvisioningReceipt(receipt, { phase: "resolving", values: {} });
  const fetched = updateProvisioningReceipt(fetching, {
    phase: "prepared",
    values: { base_commit: "c".repeat(40) },
  });
  assert.equal(fetched.operation_status.phase, "prepared");
  assert.equal(receipt.base_commit, null);
  assert.throws(
    () => updateProvisioningReceipt(receipt, { phase: "provisioned", values: { base_commit: "c".repeat(40), worktree_path: join(root, "worktree") } }),
    /invalid provisioning phase transition/u,
  );

  let entered;
  let release;
  const inside = new Promise((resolve) => { entered = resolve; });
  const wait = new Promise((resolve) => { release = resolve; });
  const firstLock = withProvisioningReceiptLock(path, { productSupportRoot: support }, async () => {
    entered();
    await wait;
  });
  await inside;
  await assert.rejects(
    withProvisioningReceiptLock(path, { productSupportRoot: support }, async () => {}),
    /already in progress/u,
  );
  release();
  await firstLock;
  await writeFile(`${path}.lock`, `${JSON.stringify({ pid: 2147483647, created_at: "2026-09-03T00:00:00.000Z" })}\n`);
  let reclaimed = false;
  await withProvisioningReceiptLock(path, { productSupportRoot: support }, async () => { reclaimed = true; });
  assert.equal(reclaimed, true);
});

test("provisioning receipt writes reject symbolic-link parents", async (t) => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "taskbelay-codex-receipt-link-")));
  const support = join(root, "support");
  const outside = join(root, "outside");
  await Promise.all([mkdir(support), mkdir(outside)]);
  t.after(() => rm(root, { recursive: true, force: true }));
  await symlink(outside, join(support, "provisioning"), process.platform === "win32" ? "junction" : undefined);
  const receipt = fixtureReceipt();
  const path = provisioningReceiptPath(support, receipt.launch_id, receipt.repository_key);
  await assert.rejects(
    writeProvisioningReceiptAtomic(path, receipt, { productSupportRoot: support }),
    /symbolic link/u,
  );
});

function fixtureReceipt() {
  return createProvisioningReceipt({
    launchId: "launch-1",
    requestDigest: "a".repeat(64),
    handoffDigest: "d".repeat(64),
    sourceRepositoryIdentity: "b".repeat(64),
    repositoryKey: "primary",
    sourceType: "remote", carryChanges: false, remoteName: "origin",
    baseBranch: "main",
    targetBranch: "codex/task",
    surface: "managed_worktree",
    createdAt: "2026-09-03T00:00:00.000Z",
  });
}

function createProvisioningReceipt(input) { return rawCreateProvisioningReceipt({workspaceMode:"dedicated_worktree", ...input, authorizationRef:receiptAdmissionFixture(input)}); }
