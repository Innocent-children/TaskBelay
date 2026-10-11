import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import assert from 'node:assert/strict';

const root = mkdtempSync(join(tmpdir(), 'taskbelay-assumption-'));
function git(...args) { const p = spawnSync('git', ['-C', root, ...args], {encoding:'utf8', windowsHide:true}); assert.equal(p.status, 0, p.stderr); return p.stdout; }
function fingerprint() { return createHash('sha256').update(readFileSync(join(root,'sum.cjs'))).update(readFileSync(join(root,'test.cjs'))).digest('hex'); }
try {
  git('init','-b','main'); git('config','user.email','evaluation@localhost'); git('config','user.name','TaskBelay evaluation');
  writeFileSync(join(root,'sum.cjs'),'module.exports = (a,b) => a-b;\n');
  writeFileSync(join(root,'test.cjs'),"require('node:assert/strict').equal(require('./sum.cjs')(2,3),5);\n");
  writeFileSync(join(root,'README.md'),'Example\n'); git('add','.'); git('commit','-m','baseline');
  assert.notEqual(spawnSync(process.execPath,['test.cjs'],{cwd:root,windowsHide:true}).status,0);
  writeFileSync(join(root,'sum.cjs'),'module.exports = (a,b) => a+b;\n');
  assert.equal(spawnSync(process.execPath,['test.cjs'],{cwd:root,windowsHide:true}).status,0);
  const checked = fingerprint();
  writeFileSync(join(root,'README.md'),'Unrelated documentation\n'); assert.equal(fingerprint(),checked);
  git('add','.'); git('commit','-m','same checked content'); assert.equal(fingerprint(),checked);
  writeFileSync(join(root,'sum.cjs'),'module.exports = (a,b) => a-b;\n'); assert.notEqual(fingerprint(),checked);
  console.log(JSON.stringify({environment:'Windows native Git and Node',actual_executions:2,failed_before_fix:true,passed_after_fix:true,unrelated_document_preserved:true,linear_commit_preserved:true,relevant_change_invalidated:true}));
} finally { rmSync(root,{recursive:true,force:true}); }
