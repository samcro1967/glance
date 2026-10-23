import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import test from 'node:test';

const source = await fs.readFile('internal/glance/static/js/widget-loading.js', 'utf8');
const { reconcileLoadingWidgets } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);

const pending = id => ({ closest: () => id === null ? null : { dataset: { widgetId: id } } });
const root = items => ({ querySelectorAll: selector => {
    assert.equal(selector, '[data-widget-state="loading"]');
    return items;
} });

test('pending cards, footers and repeated bar items reconcile their render targets once', () => {
    const calls = [];
    assert.equal(reconcileLoadingWidgets(root([pending('1'), pending('2'), pending('3'), pending('3')]), id => calls.push(id)), 3);
    assert.deepEqual(calls, ['1', '2', '3']);
});

test('completed pages and detached or invalid placeholders do not request renders', () => {
    const calls = [];
    assert.equal(reconcileLoadingWidgets(root([pending(null), pending('undefined'), pending('../config')]), id => calls.push(id)), 0);
    assert.equal(reconcileLoadingWidgets(root([]), id => calls.push(id)), 0);
    assert.deepEqual(calls, []);
});

test('reconnection rechecks remaining pending targets, without rechecking completed widgets', () => {
    const calls = [];
    const items = [pending('1'), pending('2')];
    reconcileLoadingWidgets(root(items), id => calls.push(id));
    items.shift();
    reconcileLoadingWidgets(root(items), id => calls.push(id));
    items.length = 0;
    reconcileLoadingWidgets(root(items), id => calls.push(id));
    assert.deepEqual(calls, ['1', '2', '2']);
});
