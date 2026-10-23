'use strict';

// Uses only the Makefile-managed local fixture and test runtime. Hold SSE until
// all initial fetches finish, making the otherwise intermittent subscription gap
// deterministic. No screenshots or production configuration are accessed.
const assert = require('node:assert/strict');
const { chromium } = require('playwright');

async function main() {
    const base = 'http://127.0.0.1:18080';
    const fixture = 'http://127.0.0.1:18089';
    const browser = await chromium.launch({
        executablePath: process.env.GLANCE_VISUAL_CHROME || '/usr/bin/google-chrome',
        headless: true,
    });
    try {
        for (const { viewport, lateSSE } of [true, false].flatMap(lateSSE =>
            [{ width: 1600, height: 1000 }, { width: 390, height: 844 }].map(viewport => ({ viewport, lateSSE })))) {
            await fetch(`${fixture}/widget-loading/reset`);
            const context = await browser.newContext({ viewport, reducedMotion: 'reduce' });
            try {
                // Observe the native open event while every fixture fetch is
                // held. A header-only response must not pass this check.
                await context.addInitScript(() => {
                    const NativeEventSource = window.EventSource;
                    window.EventSource = class extends NativeEventSource {
                        constructor(...args) {
                            super(...args);
                            this.addEventListener('open', () => { window.__widgetLoadingStreamOpen = true; });
                        }
                    };
                });
                let releaseCalculator;
                const calculatorGate = new Promise(resolve => { releaseCalculator = resolve; });
                const failCalculator = !lateSSE && viewport.width === 390;
                await context.route(/\/calculator\.js(?:\?|$)/, async route => {
                    await calculatorGate;
                    if (failCalculator) await route.fulfill({ status: 503, contentType: 'text/javascript', body: '' });
                    else await route.continue();
                });
                let connect;
                const gate = new Promise(resolve => { connect = resolve; });
                await context.route(/\/api\/live-updates(?:\?|$)/, async route => {
                    await gate;
                    await route.continue();
                });
                if (!lateSSE) connect();
                const page = await context.newPage();
                const unhandledErrors = [];
                page.on('pageerror', error => unhandledErrors.push(error.message));
                const calculatorRequested = page.waitForRequest(/\/calculator\.js(?:\?|$)/, { timeout: 10000 });
                // Restart through Make so each viewport begins with cold widgets.
                require('node:child_process').execFileSync('make', ['--no-print-directory', 'test-instance-start', 'TEST_CONFIG=testdata/widget-loading.yml'], { stdio: 'inherit' });
                // Restarting the fixture resets its response gate as well.
                await page.goto(base, { waitUntil: 'domcontentloaded' });
                await page.locator('#page.content-ready').waitFor({ timeout: 10000 });
                await calculatorRequested;
                assert.ok(await page.locator('#page-content').isVisible(), 'slow enhancement hid the page');
                assert.equal(await page.locator('.page-loading-container').isVisible(), false);
                assert.equal(await page.locator('#page').getAttribute('data-enhancements-ready'), null);
                await page.getByRole('tab', { name: 'Delayed failure' }).click();
                assert.equal(await page.getByRole('tab', { name: 'Delayed failure' }).getAttribute('aria-selected'), 'true');
                await page.getByRole('tab', { name: 'Delayed success' }).click();
                await page.waitForFunction(() => document.querySelector('[data-unit-converter-result]')?.textContent.trim().length > 0, undefined, { timeout: 5000 });
                const converterResult = page.locator('[data-unit-converter-result]');
                const previousResult = await converterResult.textContent();
                await page.locator('[data-unit-converter-value]').fill('2');
                await page.waitForFunction(previous => document.querySelector('[data-unit-converter-result]').textContent !== previous, previousResult, { timeout: 5000 });
                assert.ok(await page.locator('[data-widget-state="loading"]').count() >= 4);
                assert.equal(await page.locator('.widget-error-header').count(), 0);
                assert.ok((await page.locator('#page-content').innerText()).includes('Static fixture is ready.'));
                assert.equal(await page.locator('.widget-loading-icon').first().evaluate(el => getComputedStyle(el).animationName), 'none');
                const expand = page.getByRole('button', { name: 'Expand Delayed success', exact: true });
                assert.ok(await expand.isDisabled(), 'group expansion should wait for initial content');
                if (!lateSSE) {
                    await page.waitForFunction(() => window.__widgetLoadingStreamOpen === true, undefined, { timeout: 5000 });
                    assert.ok(await page.locator('[data-widget-state="loading"]').count() >= 4,
                        'initial SSE must open while provider results are still pending');
                }
                await fetch(`${fixture}/widget-loading/release`);
                let committed = false;
                for (let i = 0; i < 50; i++) {
                    const html = await (await fetch(`${base}/api/pages/home/content/`)).text();
                    if (html.includes('Ready fixture') && html.includes('widget-error-header') && !html.includes('data-widget-state="loading"')) {
                        committed = true;
                        break;
                    }
                    await new Promise(resolve => setTimeout(resolve, 100));
                }
                assert.ok(committed, 'provider results did not commit before SSE subscription');
                connect();
                await page.waitForFunction(() => document.querySelectorAll('[data-widget-state="loading"]').length === 0, undefined, { timeout: 10000 });
                assert.ok((await page.locator('.status-bar').innerText()).includes('Ready compact'));
                assert.ok((await page.locator('.footer-micro-layer').innerText()).includes('Ready compact'));
                assert.equal(await page.locator('.widget-error-header').count(), 1);
                await page.waitForFunction(() => {
                    const button = document.querySelector('[data-widget-expand-title="Delayed success"]');
                    return button !== null && !button.disabled;
                }, undefined, { timeout: 5000 });
                await expand.click();
                const dialog = page.getByRole('dialog', { name: 'Expanded Delayed success' });
                await dialog.getByText('Expanded Ready fixture', { exact: true }).waitFor({ timeout: 5000 });
                await dialog.getByRole('button', { name: 'Close expanded widget' }).click();
                await page.getByRole('tab', { name: 'Delayed failure' }).click();
                assert.ok(await page.locator('.widget-error-header').isVisible());
                // Live replacements and group expansion worked while a separate
                // module was still held. Release it, or fail it once to verify isolation.
                assert.equal(await page.locator('#page').getAttribute('data-enhancements-ready'), null);
                releaseCalculator();
                await page.locator('#page[data-enhancements-ready="true"]').waitFor({ timeout: 10000 });
                if (!failCalculator) {
                    await page.locator('[data-calculator-digit="7"]').click();
                    assert.equal((await page.locator('[data-calculator-result]').textContent()).trim(), '7');
                }
                assert.deepEqual(unhandledErrors, []);
                console.log(`PASS visible page, usable tabs/converter and live updates during delayed enhancement${failCalculator ? ' failure' : ''} (${viewport.width}px)`);
                console.log(`PASS widget loading, nested content, footer, status bar, failure, group expansion, reduced motion and ${lateSSE ? "late" : "early"} SSE (${viewport.width}px)`);
            } finally {
                await context.close();
            }
        }
    } finally {
        await browser.close();
    }
}

main().catch(error => {
    console.error(error);
    process.exitCode = 1;
});
