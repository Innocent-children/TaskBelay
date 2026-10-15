import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, mkdtemp, mkdir, cp, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';
import { HOST_LAUNCH_OPERATIONS } from '../packages/codex/lib/host-launch-contract.mjs';
import { operations as zcodeOperations } from '../packages/zcode/bin/taskbelay-zcode.mjs';
import { OPERATIONS } from '../packages/taskbelay/lib/cli.mjs';
const root=resolve(import.meta.dirname,'..');
const all=JSON.parse(await readFile(join(root,'contracts/interfaces.json'),'utf8'));
function described(schema,path) {
  for(const [key,value] of Object.entries(schema.properties??{})) {
    assert.ok(typeof value.description==='string'&&value.description.trim().length>0,`${path}.${key} lacks usable field notes`);
    assert.ok(!value.description.startsWith('Returned '),`${path}.${key} has placeholder notes`);
    described(value,path+'.'+key);
  }
  for(const value of [...(schema.anyOf??[]),...(schema.oneOf??[])])described(value,path);
  if(schema.items)described(schema.items,path+'[]');
}
test('current public method/operation branches have complete discoverable contracts',()=>{
  assert.equal(all.length,100);assert.equal(new Set(all.map(x=>x.name)).size,100);
  const expected=[...HOST_LAUNCH_OPERATIONS.map(x=>'codex.host-launch.'+x),...zcodeOperations.map(x=>'zcode.host-launch.'+x),...OPERATIONS.map(x=>'taskbelay.lifecycle.'+x)];
  for(const name of expected)assert.ok(all.some(x=>x.name===name),name);
  for(const entry of all){
    assert.ok(entry.description.length>50,entry.name);assert.ok(entry.implementation.length,entry.name);
    assert.ok(entry.examples.some(x=>x.name==='success'),entry.name);
    assert.ok(entry.examples.some(x=>x.name!=='success'),entry.name);
    described(entry.input_schema,entry.name+'.input');described(entry.output_schema,entry.name+'.output');
    for(const e of entry.examples){assert.ok(e.preconditions,entry.name);assert.notEqual(e.input,undefined);assert.notEqual(e.output,undefined)}
  }
});
test('method help works from isolated package files without Core, Git, stdin or a Task data directory',async t=>{
  const folder=await mkdtemp(join(tmpdir(),'taskbelay-installed-help-'));t.after(()=>rm(folder,{recursive:true,force:true}));
  for(const host of ['codex','claude','zcode','taskbelay']){
    const target=join(folder,host);await mkdir(target);
    for(const name of ['bin','lib','contracts','package.json'])await cp(join(root,'packages',host,name),join(target,name),{recursive:true});
    if(host==='codex'||host==='claude')await cp(join(root,'packages',host,'plugin/hooks'),join(target,'plugin/hooks'),{recursive:true});
    if(host==='zcode')await cp(join(root,'packages',host,'hooks'),join(target,'hooks'),{recursive:true});
    const binary=join(target,'bin',host==='taskbelay'?'taskbelay.mjs':`taskbelay-${host}.mjs`);
    const invocations=host==='taskbelay'?[['status','--help','--json'],['pet','start','--help','--json']]:[['host-launch','inspect','--help','--json'],['setup','--help','--json'],['hook','pre-tool-use','--help','--json']];
    invocations.push(['call','taskbelay_checkpoint','--help','--json'],['check','run','--help','--json']);
    for(const args of invocations){
      const result=spawnSync(process.execPath,[binary,...args],{input:'not JSON',encoding:'utf8',timeout:5000,env:{...process.env,TASKBELAY_DATA_DIR:'deliberately-relative',HOME:folder,USERPROFILE:folder}});
      assert.equal(result.status,0,result.stderr);const contract=JSON.parse(result.stdout);assert.ok(contract.output_schema);assert.ok(contract.examples.length>=2);
    }
  }
});
