import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import test from 'node:test';

const source = await fs.readFile('internal/glance/static/js/page-readiness.js', 'utf8');
const { createPageReadiness, runPageEnhancements } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);

function fixture() {
    const classes = new Set();
    const attributes = new Map();
    const errors = [];
    const page = { classList: { add: name => classes.add(name) }, setAttribute: (name, value) => attributes.set(name, value) };
    return { classes, attributes, errors, readiness: createPageReadiness(page, error => errors.push(error)) };
}

test('reveal makes content accessible and runs queued observers exactly once', () => {
    const state = fixture();
    let calls = 0;
    state.readiness.afterReady(() => { assert.ok(state.classes.has('content-ready')); calls++; });
    assert.equal(calls, 0);
    state.readiness.reveal();
    state.readiness.reveal();
    assert.equal(calls, 1);
    assert.equal(state.attributes.get('aria-busy'), 'false');
});

test('late observers run immediately and a broken observer does not block its siblings', () => {
    const state = fixture();
    const failure = new Error('observer failed');
    let calls = 0;
    state.readiness.afterReady(() => { throw failure; });
    state.readiness.afterReady(() => { calls++; });
    state.readiness.reveal();
    state.readiness.afterReady(() => { calls++; });
    assert.equal(calls, 2);
    assert.deepEqual(state.errors, [failure]);
});

test('pending and failed enhancements do not block independent controls or page visibility', async () => {
    const state = fixture();
    const failure = new Error('module unavailable');
    const failures = [];
    let release;
    let siblingReady = false;
    const pending = new Promise(resolve => { release = resolve; });
    const work = runPageEnhancements([
        ['pending', () => pending],
        ['failed', () => { throw failure; }],
        ['ready', () => { siblingReady = true; return ['cleanup']; }],
    ], (name, error) => failures.push([name, error]));
    state.readiness.reveal();
    assert.ok(siblingReady);
    assert.ok(state.classes.has('content-ready'));
    assert.deepEqual(failures, [['failed', failure]]);
    release([]);
    assert.deepEqual(await work, [[], [], ['cleanup']]);
});
