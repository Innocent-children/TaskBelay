import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { exampleSchema } from './interface-schema.mjs';

const root=resolve(import.meta.dirname,'..');
const check=process.argv.includes('--check');
const notes=JSON.parse(await readFile(join(root,'contracts/field-notes.json'),'utf8'));
const read=name=>readFile(join(root,'contracts',name),'utf8').then(JSON.parse);
const core=await read('core-methods.json');
const host=await read('host-examples.json');
const management=await read('management-examples.json');
const runtime=await read('runtime-examples.json');
const source=await readFile(join(root,'packages/host-interface/reference.mjs'),'utf8');
async function save(path,value) {
  const text=typeof value==='string'?value:JSON.stringify(value,null,2)+'\n';
  const target=join(root,path);
  if(check) {if(await readFile(target,'utf8')!==text)throw new Error('Generated interface reference differs: '+path);return}
  await mkdir(dirname(target),{recursive:true});await writeFile(target,text);
}
const sourceFor=entry=>entry.name.startsWith('taskbelay.')?['packages/taskbelay/lib/'+(entry.name.includes('.pet.')?'pet.mjs':'lifecycle.mjs')]:entry.name.startsWith('core.')?['cmd/taskbelay/main.go']:entry.name.startsWith('GET ')||entry.name.startsWith('POST ')?['internal/webui/server.go','internal/webui/session.go']:entry.name.startsWith('deepseek.')?['packages/deepseek/lib/workspace-tool.mjs','packages/deepseek/lib/workspace-coordinator.mjs']:[`packages/${entry.name.split('.')[0]}/${entry.name.includes('.hook.')?(entry.name.startsWith('zcode.')?'hooks':'plugin/hooks')+'/pre-tool-use.mjs':'lib/'+(entry.name.includes('.lifecycle.')?'lifecycle':'workspace')+'.mjs'}`];
const prepared=[...host,...management,...runtime].map(entry=>{
  const failures=entry.examples.filter(e=>e.name!=='success');
  const failureSchemas=failures.map(e=>exampleSchema(e.output,notes));
  const implementation=entry.name.startsWith('codex.host-launch.')?['packages/codex/bin/taskbelay-codex.mjs','packages/codex/lib/task-launch.mjs']:sourceFor(entry);
  return {...entry,implementation,read_only:!["GET /api/tasks/{task_id}","core.webui.status"].includes(entry.name) && /\.status$|\.doctor$|\.inspect$|^GET |\.help$|\.version$|\.config\.validate$/.test(entry.name),output_schema:{description:'Complete transport result. Success and failure are separate branches; read the method transport instructions before parsing.',anyOf:[entry.output_schema,...failureSchemas]}};
});
const all=[...core.map(entry=>({...entry,implementation:['internal/protocol/handler.go','internal/protocol/examples_test.go']})),...prepared];
if(new Set(all.map(x=>x.name)).size!==100||all.length!==100)throw new Error('Current interface coverage must contain exactly the 100 registered method/operation branches');
await save('contracts/interfaces.json',all);
await save('internal/protocol/runtime-contracts.json',prepared.filter(e=>e.name.startsWith('core.')||e.name.startsWith('GET ')||e.name.startsWith('POST ')));
for(const product of ['codex','claude','zcode','deepseek','taskbelay']) {
  const entries=product==='taskbelay'?all.filter(e=>e.name.startsWith('taskbelay.')||e.name.startsWith('core.')||e.name.startsWith('GET ')||e.name.startsWith('POST ')):prepared.filter(e=>e.name.startsWith(product+'.'));
  await save(`packages/${product}/contracts/interfaces.json`,[...core,...entries]);
  await save(`packages/${product}/lib/interface-reference.mjs`,'// Generated from packages/host-interface/reference.mjs; edit the shared source.\n'+source);
  const path=`packages/${product}/package.json`,manifest=JSON.parse(await readFile(join(root,path),'utf8'));
  for(const item of ['lib/interface-reference.mjs','contracts/interfaces.json'])if(!manifest.files.includes(item))manifest.files.push(item);
  await save(path,manifest);
}
process.stdout.write(check?'Packaged interface references match.\n':'Packaged 100 current interface descriptions and actual producer examples.\n');
