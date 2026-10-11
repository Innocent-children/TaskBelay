import test from 'node:test';import assert from 'node:assert/strict';import {requestDigest} from '../lib/task-admission.mjs';
test('workspace inspection retains exact request identity without a suitability gate',()=>{assert.equal(requestDigest('authorized request').length,64);assert.notEqual(requestDigest('one'),requestDigest('two'));assert.throws(()=>requestDigest(''));});
