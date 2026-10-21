import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import test from 'node:test';

// Load the production module without its browser diagnostic transport. The
// actual exported date functions are tested, not a reimplementation.
let source = await fs.readFile('internal/glance/static/js/decorations.js', 'utf8');
source = source.replace("import { frontendDiagnosticError } from './diagnostics.js';", 'const frontendDiagnosticError = () => {};');
const { selectDecoration, nextDecorationCheck, setupDecorations, decorationBulbTiming } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);
test("bulbs have stable, varied durations and phases across the perimeter", () => {
    const timings = Array.from({ length: 84 }, (_, index) => decorationBulbTiming(index));
    assert.deepEqual(timings, Array.from({ length: 84 }, (_, index) => decorationBulbTiming(index)));
    assert.ok(new Set(timings.map(value => value.duration)).size > 20);
    assert.ok(new Set(timings.map(value => value.delay)).size > 20);
    for (const value of timings) {
        assert.ok(value.duration >= 3 && value.duration <= 6);
        assert.ok(value.delay <= 0 && value.delay > -2 * value.duration);
    }
    assert.notDeepEqual(timings[0], timings[24]);
    assert.notDeepEqual(timings[24], timings[42]);
    assert.notDeepEqual(timings[42], timings[66]);
});

const halloween = { colors: ['#ff7518'] };
const winter = { colors: ['#ffffff'] };
const config = { auto: true, schedule: [
    { start: '10-01', end: '10-31', design: halloween },
    { start: '12-31', end: '01-05', design: winter },
] };

test('manual selection preserves the custom or preset design', () => {
    assert.equal(selectDecoration({ auto: false, design: halloween }), halloween);
});
test('inclusive ranges, gaps, and year crossing', () => {
    for (const [month, day, expected] of [[9,30,null],[10,1,halloween],[10,31,halloween],[11,1,null],[12,31,winter],[1,1,winter],[1,5,winter],[1,6,null]]) {
        assert.equal(selectDecoration(config, new Date(2026, month - 1, day)), expected);
    }
});
test('leap-day entries do not drift into March', () => {
    const leap = { auto: true, schedule: [{ start:'02-29', end:'02-29', design:winter }] };
    assert.equal(selectDecoration(leap, new Date(2028,1,29)), winter);
    assert.equal(selectDecoration(leap, new Date(2027,2,1)), null);
});
test('checks reach midnight exactly and remain bounded', () => {
    assert.equal(nextDecorationCheck(new Date(2026,9,31,23,59,59,500)), 500);
    assert.equal(nextDecorationCheck(new Date(2026,9,31,12)), 60000);
});
test('disabled/absent decoration markup creates no controller', () => {
    assert.equal(setupDecorations({ querySelector: () => null }), undefined);
});

test('one controller owns timers, visibility, history restoration and disposal', () => {
    const saved = Object.fromEntries(['document','window','setTimeout','clearTimeout'].map(key => [key, globalThis[key]]));
    const doc = new EventTarget();
    const win = new EventTarget();
    doc.hidden = false;
    const host = { isConnected:true, replaceChildren() {}, classList:{ toggle() {} } };
    const payload = {auto:true,lights:false,footerIcons:false,animation:false,schedule:[{start:'01-01',end:'12-31',design:{colors:[]}}]};
    const root = {querySelector: selector => selector === '#decorations' ? host : {textContent:JSON.stringify(payload)}, querySelectorAll:()=>[]};
    const timers = new Map();
    let serial = 0;
    try {
        Object.assign(globalThis, {document:doc,window:win,setTimeout:fn=>{timers.set(++serial,fn);return serial;},clearTimeout:id=>timers.delete(id)});
        const controller = setupDecorations(root);
        assert.equal(timers.size,1);
        assert.equal(setupDecorations(root),controller);
        assert.equal(timers.size,1);
        doc.hidden = true;
        doc.dispatchEvent(new Event('visibilitychange'));
        assert.equal(timers.size,0);
        doc.hidden = false;
        doc.dispatchEvent(new Event('visibilitychange'));
        assert.equal(timers.size,1);
        win.dispatchEvent(new Event('pagehide'));
        assert.equal(timers.size,0);
        win.dispatchEvent(new Event('pageshow'));
        assert.equal(timers.size,1);
        controller.dispose();
        assert.equal(timers.size,0);
        win.dispatchEvent(new Event('pageshow'));
        doc.dispatchEvent(new Event('visibilitychange'));
        assert.equal(timers.size,0);
        const replacement = setupDecorations(root);
        assert.notEqual(replacement,controller);
        replacement.dispose();
    } finally { Object.assign(globalThis,saved); }
});
