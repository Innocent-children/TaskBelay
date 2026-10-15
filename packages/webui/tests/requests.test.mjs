import assert from 'node:assert/strict';
import test from 'node:test';
import { SelectionRequests } from '../src/lib/requests.ts';
import { callTask } from '../src/lib/api.ts';

test('switching tasks or refreshing discards earlier read and mutation responses', () => {
  const requests = new SelectionRequests();
  const first = requests.select('first');
  const mutation = requests.capture();
  requests.select('second');
  assert.equal(requests.current(first, 'first'), false);
  assert.equal(requests.current(mutation.sequence, mutation.id), false);
  const newer = requests.select('first');
  assert.equal(requests.current(first, 'first'), false);
  assert.equal(requests.current(newer, 'first'), true);
});

test('a lost mutation response is never retried or reported as saved', async () => {
  const original = globalThis.fetch;
  const document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: 'fixture-csrf' }) };
  let calls = 0;
  globalThis.fetch = async () => { calls++; throw new Error('response lost'); };
  try {
    await assert.rejects(callTask('taskbelay_resolve_issue', { operation_id: 'stable-fixture-operation' }), /response was lost/);
    assert.equal(calls, 1);
  } finally { globalThis.fetch = original; globalThis.document = document; }
});

test('failure refreshing a saved mutation preserves its outcome and never repeats it', async () => {
  const original = globalThis.fetch;
  const document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: 'fixture-csrf' }) };
  const methods = [];
  globalThis.fetch = async (_, options) => {
    methods.push(options?.method ?? 'GET');
    if (methods.length === 1) return { ok: true, json: async () => ({ ok: true, result: { identity: { task_id: 'fixture' } } }) };
    throw new Error('display refresh failed');
  };
  try {
    await assert.rejects(callTask('taskbelay_resolve_issue', { operation_id: 'stable-fixture-operation' }), /Task saved/);
    assert.deepEqual(methods, ['POST', 'GET']);
  } finally { globalThis.fetch = original; globalThis.document = document; }
});


test('run recovery refreshes the task identified by its view without repeating recovery', async () => {
  const original = globalThis.fetch, document = globalThis.document;
  globalThis.document = { querySelector: () => ({ content: 'fixture-csrf' }) };
  const requests = [];
  globalThis.fetch = async (url, options) => {
    requests.push({url, method: options?.method ?? 'GET'});
    if (requests.length === 1) return {ok:true,json:async()=>({ok:true,result:{view:{identity:{task_id:'recovered-task'}},run:{run_id:'original-run',status:'interrupted_unknown'}}})};
    return {ok:true,json:async()=>({ok:true,result:{identity:{task_id:'recovered-task'},decision:{pending_runs:[]}}})};
  };
  try {
    const result=await callTask('taskbelay_recover_run',{operation_id:'recover-once'});
    assert.equal(result.identity.task_id,'recovered-task');
    assert.deepEqual(requests,[{url:'/api/core/taskbelay_recover_run',method:'POST'},{url:'/api/tasks/recovered-task?refresh=1',method:'GET'}]);
  } finally {globalThis.fetch=original;globalThis.document=document}
});
