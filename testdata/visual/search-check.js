'use strict';

// Runs within the maintained deterministic frontend workflow. Popup interception
// keeps the test on Glance and prevents navigation to any external destination.
async function checkSearch(page) {
  const viewport = page.viewportSize();
  const widget = page.locator('.visual-fixture-search .search');
  const input = widget.locator('.search-input');
  const rows = widget.locator('.search-result');
  const waitFor = async (predicate, message) => {
    for (let attempt = 0; attempt < 50; attempt++) {
      if (await predicate()) return;
      await page.waitForTimeout(100);
    }
    throw new Error(message);
  };
  await page.evaluate(() => {
    window.__searchOpened = [];
    window.__originalSearchOpen = window.open;
    window.open = (url, target) => {
      window.__searchOpened.push({ url, target });
      return null; // A blocked popup must not throw when focus is attempted.
    };
  });
  const lastURL = () => page.evaluate(() => window.__searchOpened.at(-1)?.url);
  const neighborStyle = await widget.evaluate(element => {
    const next = element.closest('.widget').nextElementSibling;
    if (!next) throw new Error('Search overlay fixture requires a following widget');
    const previous = next.getAttribute('style');
    // Reproduce a neighboring card's stacking context rather than checking
    // only that result text exists somewhere in the DOM.
    next.style.position = 'relative';
    next.style.zIndex = '1';
    return previous;
  });
  try {
    await input.fill('qa');
    await input.press('Enter');
    if (await lastURL() !== 'https://example.invalid/search-shortcut') throw new Error('Search exact alias did not navigate');

    await input.fill('QA');
    const shortcutResult = rows.filter({ hasText: 'QA destination' });
    await shortcutResult.waitFor({ state: 'visible' });
    await shortcutResult.scrollIntoViewIfNeeded();
    const onTop = await shortcutResult.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      return element.contains(document.elementFromPoint(
        bounds.left + bounds.width / 2,
        bounds.top + bounds.height / 2
      ));
    });
    if (!onTop) throw new Error('Search result is covered by a neighboring widget');
    await shortcutResult.click();
    if (await lastURL() !== 'https://example.invalid/search-shortcut') throw new Error('Search pointer result activation failed');

    await input.fill('QA dest');
    await input.press('Enter');
    if (await lastURL() !== 'https://www.google.com/search?q=QA%20dest') throw new Error('Fuzzy result hijacked ordinary Search submission');

    await input.fill('!path owner/repo');
    await input.press('Enter');
    if (await lastURL() !== 'https://github.com/owner/repo') throw new Error('Raw bang path was encoded');
    await input.fill('!gh a/b &c');
    await input.press('Enter');
    if (await lastURL() !== 'https://github.com/search?q=a%2Fb%20%26c') throw new Error('Normal bang query was not encoded');

    await input.fill('Glance');
    await waitFor(async () => (await rows.allTextContents()).some(text => text.includes('bookmarks')), 'Local bookmark result missing');
    await input.fill('needle');
    await waitFor(async () => (await rows.allTextContents()).some(text => text.includes('needle suggestion')), 'Provider suggestion missing');
    if (await widget.locator('.search-results img').count() !== 0) throw new Error('Suggestion text was interpreted as HTML');
    await input.press('ArrowDown');
    if (!await input.getAttribute('aria-activedescendant')) throw new Error('Search keyboard selection has no active descendant');
    await input.press('Escape');
    if (await input.getAttribute('aria-expanded') !== 'false') throw new Error('Escape did not dismiss Search results');

    // Return a delayed old response after a newer response. The controller must
    // reject it even if the browser/network cannot cancel the old request.
    await page.route('**/api/search/*/suggestions?*', async route => {
      const query = new URL(route.request().url()).searchParams.get('q');
      if (query === 'oldquery') await new Promise(resolve => setTimeout(resolve, 700));
      await route.fulfill({ json: [`${query} result`] }).catch(() => {});
    });
    await input.fill('oldquery');
    await page.waitForTimeout(300);
    await input.fill('newquery');
    await waitFor(async () => (await rows.allTextContents()).some(text => text.includes('newquery result')), 'Latest Search result missing');
    await page.waitForTimeout(800);
    if ((await rows.allTextContents()).some(text => text.includes('oldquery result'))) throw new Error('Stale Search response replaced current results');
    await page.unroute('**/api/search/*/suggestions?*');

    await input.fill('');
    await waitFor(async () => (await rows.allTextContents()).some(text => text.includes('QA dest')), 'Search history did not retain submission');
    await input.press('ArrowDown');
    const selectedHistory = await widget.locator('[aria-selected="true"] .search-result-title').textContent();
    await widget.locator('.search-remove-history').click();
    await waitFor(async () => !(await widget.locator('.search-result-title').allTextContents()).includes(selectedHistory), 'Individual history removal failed');
    await widget.locator('.search-clear-history').click();
    await waitFor(async () => !(await rows.allTextContents()).some(text => text.includes('history')), 'Clear Search history failed');

    await page.setViewportSize({ width: 390, height: 844 });
    // Glance displays only the selected column on mobile. Search is in this
    // page's small column, while its full-width column is selected by default.
    const columnIndex = await widget.evaluate(element =>
      Array.from(document.querySelectorAll('.page-columns > .page-column'))
        .indexOf(element.closest('.page-column'))
    );
    if (columnIndex < 0) throw new Error('Search fixture is not in a page column');
    await page.locator('.mobile-navigation-input').nth(columnIndex).locator('xpath=..').click();
    await input.waitFor({ state: 'visible' });
    await input.fill('needle');
    await waitFor(async () => await rows.count() > 0, 'Mobile Search results missing');
    const bounds = await widget.locator('.search-dropdown').boundingBox();
    if (!bounds || bounds.x < 0 || bounds.x + bounds.width > 391) throw new Error('Search dropdown escapes mobile viewport');
    await input.press('Escape');
    await input.press('Tab');
    if (await input.evaluate(element => document.activeElement === element)) throw new Error('Search trapped Tab navigation');
    await page.setViewportSize(viewport);
    console.log('PASS Search aliases, bangs, local results, suggestions, stale responses, history, keyboard and mobile layout');
  } finally {
    // Cleanup must not depend on visibility or replace the original assertion
    // with a fill timeout when the failed scenario left a mobile column hidden.
    await page.setViewportSize(viewport);
    await widget.evaluate((element, previous) => {
      const next = element.closest('.widget').nextElementSibling;
      if (previous === null) next.removeAttribute('style');
      else next.setAttribute('style', previous);
    }, neighborStyle);
    await page.unroute('**/api/search/*/suggestions?*');
    await page.evaluate(() => { window.open = window.__originalSearchOpen; delete window.__searchOpened; delete window.__originalSearchOpen; });
    await input.evaluate(element => {
      element.value = '';
      element.dispatchEvent(new Event('input', { bubbles: true }));
      element.blur();
    });
  }
}

module.exports = { checkSearch };
