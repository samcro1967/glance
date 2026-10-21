'use strict';

const assert = require('node:assert/strict');
const { createRuntimeSelector } = require('./decoration-runtime');

// Run real configuration through the maintained source runtime. Browser clock
// control tests date transitions without test-only switches in production code.
async function runDecorationChecks(browser, baseURL) {
  const selectRuntime = createRuntimeSelector();
  selectRuntime('testdata/decorations/auto.yml');
  for (const mobile of [false, true]) {
    const context = await browser.newContext({
      viewport: mobile ? { width:430, height:900 } : { width:1600, height:1000 },
      isMobile: mobile, hasTouch: mobile, timezoneId: 'UTC', reducedMotion: 'reduce',
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    try {
      await page.clock.install({ time: new Date('2026-10-31T12:00:00Z') });
      await page.goto(`${baseURL}/screenshots/docs-overview`);
      await page.locator('#page.content-ready').waitFor();
      assert.equal(await page.locator('.decoration-edge').count(),4);
      await page.waitForFunction(() => [...document.querySelectorAll('.decoration-footer-icon')].every(img => img.complete && img.naturalWidth > 0));
      assert.equal(await page.locator('.decoration-footer-icon').count(),2);
      const state = await page.evaluate(() => ({
        overflow: document.documentElement.scrollWidth > innerWidth,
        pointerEvents: getComputedStyle(document.querySelector('#decorations')).pointerEvents,
        animation: getComputedStyle(document.querySelector('.decoration-bulb')).animationName,
        colors: [...document.querySelectorAll('.decoration-bulb')].slice(0,2).map(b => b.style.getPropertyValue('--decoration-color')),
      }));
      assert.equal(state.overflow,false);
      assert.equal(state.pointerEvents,'none');
      assert.equal(state.animation,'none');
      assert.deepEqual(state.colors,['#ff7518','#9146ff']);
      assert.equal(await page.locator(".decoration-wire").count(), 1);
      const wire = await page.locator(".decoration-wire").evaluate(el => {
        const style = getComputedStyle(el);
        const box = el.getBoundingClientRect();
        return {
          borders: [style.borderTopWidth, style.borderRightWidth, style.borderBottomWidth, style.borderLeftWidth],
          radius: parseFloat(style.borderTopLeftRadius),
          inViewport: box.left >= 0 && box.top >= 0 && box.right <= innerWidth && box.bottom <= innerHeight,
        };
      });
      assert.deepEqual(wire.borders, ["1px", "1px", "1px", "1px"]);
      assert.ok(wire.radius > 0);
      assert.equal(wire.inViewport, true);
      const timing = await page.locator(".decoration-bulb").evaluateAll(elements => elements.map(el => ({
        duration: el.style.getPropertyValue("--decoration-duration"),
        delay: el.style.getPropertyValue("--decoration-delay"),
      })));
      assert.ok(new Set(timing.map(value => value.duration)).size > 20);
      assert.ok(new Set(timing.map(value => value.delay)).size > 20);
      const strands = await page.evaluate(() => {
        return [...document.querySelectorAll('.decoration-edge')].map(edge => {
          const bulb = edge.querySelector('.decoration-bulb');
          const box = bulb.getBoundingClientRect();
          const x = box.left + box.width / 2;
          const y = box.top + box.height / 2;
          // Ordinary hit testing ignores pointer-events:none decorations.
          // Temporarily enable just this bulb to test its actual paint order,
          // then restore it before testing normal dashboard interaction.
          bulb.style.pointerEvents = 'auto';
          try {
            return {
              side: edge.className,
              inViewport: x >= 0 && x < innerWidth && y >= 0 && y < innerHeight,
              uncovered: document.elementFromPoint(x, y) === bulb,
            };
          } finally { bulb.style.removeProperty('pointer-events'); }
        });
      });
      for (const strand of strands) {
        assert.equal(strand.inViewport, true, `${strand.side} lies outside the viewport`);
        assert.equal(strand.uncovered, true, `${strand.side} is hidden behind another element`);
      }
      await page.emulateMedia({ reducedMotion:'no-preference' });
      assert.equal(await page.locator('.decoration-bulb').first().evaluate(el => getComputedStyle(el).animationName),'decoration-glow');
      await page.emulateMedia({ reducedMotion:'reduce' });
      // Clicking ordinary page controls remains possible through the overlay.
      if (mobile) {
        const menu = page.locator('.mobile-navigation-page-links-input');
        // Match the existing mobile regression: click its visible label,
        // because the checkbox itself is deliberately hidden by native CSS.
        await menu.locator('xpath=..').click();
        assert.equal(await menu.isChecked(), true);
      } else await page.locator('.desktop-navigation a').first().click();
      await page.goto(`${baseURL}/screenshots/docs-overview`);
      await page.locator('#page.content-ready').waitFor();
      await page.clock.pauseAt(new Date('2026-10-31T23:59:59Z'));
      await page.clock.runFor(2000);
      await page.waitForFunction(() => document.querySelector('.decoration-bulb')?.style.getPropertyValue('--decoration-color') === '#cc5500');
      assert.equal(await page.locator('.decoration-edge').count(),4);
      // A schedule gap removes both the lights and footer artwork.
      await page.clock.setSystemTime(new Date('2026-06-01T12:00:00Z'));
      await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
      assert.equal(await page.locator('.decoration-bulb').count(),0);
      assert.equal(await page.locator(".decoration-wire").count(), 0);
      assert.equal(await page.locator('[data-decoration-footer]:not([hidden])').count(),0);
      await page.route('**/assets/svg/*.svg', route => route.abort());
      await page.clock.setSystemTime(new Date('2026-10-15T12:00:00Z'));
      await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
      await page.waitForFunction(() => [...document.querySelectorAll('[data-decoration-footer]')].every(el => el.hidden));
      assert.equal(await page.locator('.decoration-edge').count(),4);
      assert.deepEqual(errors,[]);
      console.log(`PASS decorations ${mobile ? 'mobile' : 'desktop'} assets, layout, reduced motion, midnight and schedule gap`);
    } finally { await context.close(); }
  }
  selectRuntime('test-instance.yml');
}

module.exports = { runDecorationChecks };
