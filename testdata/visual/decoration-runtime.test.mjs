import assert from 'node:assert/strict';
import test from 'node:test';
import runtime from './decoration-runtime.js';

test('runtime profiles cannot select arbitrary or production files', () => {
    for (const value of ['../glance.yml','/etc/glance.yml','testdata/decorations/../../glance.yml','testdata/decorations/missing.yml']) {
        assert.throws(() => runtime.validateTestConfig(value));
    }
});
test('runtime selection only restarts for a changed profile and uses Make', () => {
    const calls = [];
    const select = runtime.createRuntimeSelector((...args) => calls.push(args));
    select('test-instance.yml');
    select('testdata/decorations/halloween.yml');
    select('testdata/decorations/halloween.yml');
    select('test-instance.yml');
    assert.equal(calls.length, 2);
    assert.equal(calls[0][0], 'make');
    assert.deepEqual(calls[0][1], ['--no-print-directory','test-instance-start','TEST_CONFIG=testdata/decorations/halloween.yml']);
});
test('failed runtime start is not recorded as an active profile', () => {
    let calls = 0;
    const select = runtime.createRuntimeSelector(() => { calls++; throw new Error('start failed'); });
    assert.throws(() => select('testdata/decorations/halloween.yml'));
    assert.throws(() => select('testdata/decorations/halloween.yml'));
    assert.equal(calls, 2);
});
