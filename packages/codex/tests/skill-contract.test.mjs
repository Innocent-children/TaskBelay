import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
const root=new URL('../plugin/skills/taskbelay/',import.meta.url);
test('codex installed engineering references are complete',async()=>{const m=JSON.parse(await readFile(new URL('../package.json',import.meta.url),'utf8'));for(const f of ['engineering.md','protocol.md','workspace.md']){const text=await readFile(new URL('references/'+f,root),'utf8');assert.ok(text.length>200);assert.ok(m.files.includes('plugin/skills/taskbelay/references/'+f));assert.ok(!text.includes('taskbelay_submit_'))}const skill=await readFile(new URL('SKILL.md',root),'utf8');assert.match(skill,/existing (?:user )?(?:authorization|authority)/iu);});
