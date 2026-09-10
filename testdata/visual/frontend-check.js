// Path: testdata/visual/frontend-check.js
// File: frontend-check.js
/**
 * Canonical browser behavior runner for Glance frontend regression checks.
 *
 * Ownership: development-only frontend regression infrastructure.
 * Responsibilities: exercise deterministic Glance browser behavior and fail
 * on objective frontend contract violations.
 * Non-goals: screenshot capture, pixel comparison, browser installation, or
 * production access.
 */

'use strict';

const { chromium } = require('playwright');
const {
  coverageOutputPath,
  startCoverage,
  collectCoverage,
  checkpointCoverage,
  writeCoverage
} = require('./frontend-coverage');

const BASE_URL = (process.env.GLANCE_VISUAL_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
const CHROME = process.env.GLANCE_VISUAL_CHROME || '/usr/bin/google-chrome';
const VIEWPORT = { width: 1600, height: 1000 };

async function openPage(page, route, coveragePath = "") {
  const url = `${BASE_URL}${route}`;
  await checkpointCoverage(page, coveragePath);
  const response = await page.goto(url, {
    waitUntil: 'domcontentloaded',
    timeout: 30000
  });

  if (!response) {
    throw new Error(`No HTTP response while loading ${url}`);
  }

  if (!response.ok()) {
    throw new Error(`HTTP ${response.status()} while loading ${url}`);
  }

  const pageElement = page.locator('#page.content-ready[aria-busy="false"]');
  await pageElement.waitFor({
    state: 'attached',
    timeout: 10000
  });

  const pageContent = page.locator('#page-content');
  await pageContent.waitFor({
    state: 'attached',
    timeout: 10000
  });

  const contentLength = await pageContent.evaluate(element => element.innerHTML.trim().length);
  if (contentLength === 0) {
    throw new Error('Canonical page content is empty after initialization');
  }
}


async function getStatusBarTickerState(page) {
  return page.locator(".visual-fixture-status-bar .status-bar-mode-ticker").evaluate(statusBar => {
    const track = statusBar.querySelector(".status-bar-track");
    const style = track ? getComputedStyle(track) : null;

    return {
      initialized: statusBar.dataset.tickerInitialized,
      ready: statusBar.dataset.tickerReady,
      contentWidth: style ? parseFloat(style.getPropertyValue("--status-bar-ticker-content-width")) : 0,
      containerWidth: style ? parseFloat(style.getPropertyValue("--status-bar-ticker-container-width")) : 0,
      duration: style ? parseFloat(style.getPropertyValue("--status-bar-ticker-duration")) : 0
    };
  });
}

function assertStatusBarTickerState(state, stage) {
  if (state.initialized !== "true") {
    throw new Error(`Status Bar ticker is not initialized ${stage}`);
  }

  if (state.ready !== "true") {
    throw new Error(`Status Bar ticker is not ready ${stage}`);
  }

  if (!(state.contentWidth > 0)) {
    throw new Error(`Status Bar ticker content width is invalid ${stage}: ${state.contentWidth}`);
  }

  if (!(state.containerWidth > 0)) {
    throw new Error(`Status Bar ticker container width is invalid ${stage}: ${state.containerWidth}`);
  }

  if (!(state.duration > 0)) {
    throw new Error(`Status Bar ticker duration is invalid ${stage}: ${state.duration}`);
  }
}


async function runAuthChecks(browser, coveragePath = "") {
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1
  });
  const page = await context.newPage();
  const browserErrors = [];

  await startCoverage(page, coveragePath);
  let authenticationRequests = 0;

  page.on("pageerror", error => {
    browserErrors.push(error.message);
  });

  await page.route("**/api/authenticate", async route => {
    authenticationRequests++;

    if (authenticationRequests === 1) {
      await route.abort("failed");
      return;
    }

    await route.continue();
  });

  try {
    const response = await page.goto(`${BASE_URL}/login`, {
      waitUntil: "domcontentloaded",
      timeout: 30000
    });

    if (!response) {
      throw new Error("No HTTP response while loading login page");
    }

    if (!response.ok()) {
      throw new Error(`HTTP ${response.status()} while loading login page`);
    }

    const username = page.locator("#username");
    const password = page.locator("#password");
    const errorMessage = page.locator("#error-message");
    const loginButton = page.locator("#login-button");

    await page.locator("#login-container").waitFor({
      state: "visible",
      timeout: 10000
    });

    await username.fill("test-user");
    await password.fill("test-password");

    if (await loginButton.isDisabled()) {
      throw new Error("Login button did not enable for valid fixture credentials");
    }

    await loginButton.click();

    await errorMessage.waitFor({
      state: "visible",
      timeout: 10000
    });

    const errorText = (await errorMessage.textContent())?.trim();
    if (errorText !== "An error occurred, please try again") {
      throw new Error(`Unexpected login transport failure message: ${errorText}`);
    }

    if (await loginButton.isDisabled()) {
      throw new Error("Login button remained disabled after authentication transport failure");
    }

    const passwordHasFocus = await password.evaluate(
      element => document.activeElement === element
    );
    if (!passwordHasFocus) {
      throw new Error("Password input did not receive focus after authentication transport failure");
    }

    if (browserErrors.length > 0) {
      throw new Error(
        `Unexpected login browser error(s) after transport failure: ${browserErrors.join(" | ")}`
      );
    }

    // Capture the login-page module before successful authentication navigates
    // away from /login. Chromium drops that document's V8 coverage after the
    // navigation even when resetOnNavigation is disabled.
    await collectCoverage(page, coveragePath);

    await Promise.all([
      page.waitForURL(`${BASE_URL}/`, { timeout: 10000 }),
      loginButton.click()
    ]);

    if (authenticationRequests !== 2) {
      throw new Error(
        `Expected two authentication requests, observed ${authenticationRequests}`
      );
    }

    if (browserErrors.length > 0) {
      throw new Error(`Unexpected login browser error(s): ${browserErrors.join(" | ")}`);
    }

    console.log("PASS login transport failure recovery");
    console.log("PASS login same-credential retry");
    console.log("PASS no unexpected login browser errors");
    console.log("Authentication frontend checks passed.");
  } finally {
    writeCoverage(coveragePath);
    await context.close();
  }
}


async function main() {
  const browserErrors = [];
  const coveragePath = coverageOutputPath();
  let browser;

  try {
    browser = await chromium.launch({
      executablePath: CHROME,
      headless: true
    });

    if (process.argv.includes("--auth")) {
      await runAuthChecks(browser, coveragePath);
      return;
    }

    const context = await browser.newContext({
      viewport: VIEWPORT,
      deviceScaleFactor: 1
    });
    const page = await context.newPage();
    await startCoverage(page, coveragePath);

    page.on('pageerror', error => {
      browserErrors.push(error.message);
    });

    await page.addInitScript(() => {
      const instances = [];

      class ControlledEventSource {
        static CONNECTING = 0;
        static OPEN = 1;
        static CLOSED = 2;

        constructor(url) {
          this.url = url;
          this.readyState = ControlledEventSource.OPEN;
          this.listeners = new Map();
          instances.push(this);

          queueMicrotask(() => {
            this.dispatch('open', { type: 'open' });
          });
        }

        addEventListener(type, callback) {
          const callbacks = this.listeners.get(type) || [];
          callbacks.push(callback);
          this.listeners.set(type, callbacks);
        }

        close() {
          this.readyState = ControlledEventSource.CLOSED;
        }

        dispatch(type, event) {
          for (const callback of this.listeners.get(type) || []) {
            callback(event);
          }
        }
      }

      window.EventSource = ControlledEventSource;
      window.__glanceTestLiveUpdate = widgetID => {
        const source = instances.at(-1);
        if (!source || source.readyState !== ControlledEventSource.OPEN) {
          throw new Error('No open EventSource is available for the live-update regression');
        }

        source.dispatch('widget', {
          type: 'widget',
          data: String(widgetID)
        });
      };
    });

    await openPage(page, '/layout-composition', coveragePath);

    const statusBar = page.locator('.visual-fixture-status-bar');
    await statusBar.waitFor({ state: 'attached', timeout: 10000 });

    const statusBarWidgetID = await statusBar.getAttribute('data-widget-id');
    if (!statusBarWidgetID || !/^\d+$/.test(statusBarWidgetID)) {
      throw new Error(`Status Bar fixture has invalid widget ID: ${statusBarWidgetID}`);
    }

    assertStatusBarTickerState(
      await getStatusBarTickerState(page),
      'after initial page initialization'
    );

    for (let refresh = 1; refresh <= 2; refresh++) {
      const previousStatusBar = page.locator(`.visual-fixture-status-bar[data-widget-id="${statusBarWidgetID}"]`);
      const previousHandle = await previousStatusBar.elementHandle();

      if (!previousHandle) {
        throw new Error(`Status Bar widget is missing before live refresh ${refresh}`);
      }

      await page.evaluate(widgetID => {
        window.__glanceTestLiveUpdate(widgetID);
      }, statusBarWidgetID);

      await page.waitForFunction(
        ({ widgetID, previousElement }) => {
          const current = document.querySelector(
            `.visual-fixture-status-bar[data-widget-id="${widgetID}"]`
          );
          return current !== null && current !== previousElement;
        },
        {
          widgetID: statusBarWidgetID,
          previousElement: previousHandle
        },
        { timeout: 10000 }
      );

      await page.waitForFunction(
        widgetID => {
          const ticker = document.querySelector(
            `.visual-fixture-status-bar[data-widget-id="${widgetID}"] .status-bar-mode-ticker`
          );
          return ticker?.dataset.tickerInitialized === 'true' &&
            ticker?.dataset.tickerReady === 'true';
        },
        statusBarWidgetID,
        { timeout: 10000 }
      );

      assertStatusBarTickerState(
        await getStatusBarTickerState(page),
        `after live refresh ${refresh}`
      );

      await previousHandle.dispose();
    }

    console.log('PASS Status Bar repeated live replacement lifecycle');

    const horizontalOverflow = await page.evaluate(() =>
      document.documentElement.scrollWidth - document.documentElement.clientWidth
    );
    if (horizontalOverflow > 0) {
      throw new Error(`Desktop document overflows horizontally by ${horizontalOverflow}px`);
    }

    const headerGeometry = await page.evaluate(() => {
      const header = document.querySelector('.header-container');
      const spacer = document.querySelector('.header-container-spacer');
      const pageElement = document.getElementById('page');

      if (!header || !spacer || !pageElement) {
        return null;
      }

      const headerRect = header.getBoundingClientRect();
      const spacerRect = spacer.getBoundingClientRect();
      const pageRect = pageElement.getBoundingClientRect();

      return {
        position: getComputedStyle(header).position,
        headerTop: headerRect.top,
        headerHeight: headerRect.height,
        spacerHeight: spacerRect.height,
        spacerBottom: spacerRect.bottom,
        pageTop: pageRect.top
      };
    });

    if (headerGeometry === null) {
      throw new Error('Desktop header geometry elements are missing');
    }

    if (headerGeometry.position !== 'fixed') {
      throw new Error(`Desktop header position is ${headerGeometry.position}, expected fixed`);
    }

    if (Math.abs(headerGeometry.headerHeight - headerGeometry.spacerHeight) > 1) {
      throw new Error(
        `Desktop header/spacer height mismatch: ${headerGeometry.headerHeight}px vs ${headerGeometry.spacerHeight}px`
      );
    }

    if (headerGeometry.pageTop + 1 < headerGeometry.spacerBottom) {
      throw new Error(
        `Desktop page begins above reserved header boundary: page=${headerGeometry.pageTop}px spacer=${headerGeometry.spacerBottom}px`
      );
    }

    await page.evaluate(() => window.scrollTo(0, Math.min(600, document.documentElement.scrollHeight)));
    await page.waitForFunction(() => window.scrollY > 0);

    const scrolledHeaderTop = await page.locator('.header-container').evaluate(
      element => element.getBoundingClientRect().top
    );
    if (Math.abs(scrolledHeaderTop - headerGeometry.headerTop) > 1) {
      throw new Error(
        `Desktop fixed header moved while scrolling: ${headerGeometry.headerTop}px to ${scrolledHeaderTop}px`
      );
    }

    await page.evaluate(() => window.scrollTo(0, 0));

    const group = page.locator('.visual-fixture-group');
    const groupTabs = group.locator('[role="tab"]');
    const groupPanels = group.locator('[role="tabpanel"]');

    if (await groupTabs.count() < 2 || await groupPanels.count() < 2) {
      throw new Error('Canonical group fixture does not contain at least two tabs and panels');
    }

    const firstTab = groupTabs.nth(0);
    const secondTab = groupTabs.nth(1);
    const firstPanel = groupPanels.nth(0);
    const secondPanel = groupPanels.nth(1);

    if (
      await firstTab.getAttribute('aria-selected') !== 'true' ||
      await secondTab.getAttribute('aria-selected') !== 'false' ||
      await firstPanel.getAttribute('aria-hidden') !== 'false' ||
      await secondPanel.getAttribute('aria-hidden') !== 'true'
    ) {
      throw new Error('Canonical group fixture has incorrect initial tab state');
    }

    await secondTab.click();

    if (
      await firstTab.getAttribute('aria-selected') !== 'false' ||
      await secondTab.getAttribute('aria-selected') !== 'true' ||
      await firstPanel.getAttribute('aria-hidden') !== 'true' ||
      await secondPanel.getAttribute('aria-hidden') !== 'false'
    ) {
      throw new Error('Canonical group fixture did not switch tab state correctly');
    }

    const initialTheme = await page.locator('html').getAttribute('data-theme');
    const desktopThemePreset = page.locator(
      '.header-container .theme-choices .theme-preset:not(.current)'
    ).first();

    if (await desktopThemePreset.count() !== 1) {
      throw new Error('No non-current desktop theme preset is available');
    }

    const targetTheme = await desktopThemePreset.getAttribute('data-key');
    if (!targetTheme || targetTheme === initialTheme) {
      throw new Error(`Invalid target theme preset: ${targetTheme}`);
    }

    const themePicker = page.locator('.header-container .theme-picker');

    await themePicker.hover();
    await page.waitForFunction(() =>
      document.querySelector('.header-container .theme-picker')?.classList.contains('popover-active')
    );

    const visibleThemePreset = page.locator(
      `.theme-choices .theme-preset[data-key="${targetTheme}"]:visible`
    ).first();
    await visibleThemePreset.waitFor({ state: 'visible' });

    const themeResponsePromise = page.waitForResponse(response =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `/api/set-theme/${targetTheme}`
    );

    await visibleThemePreset.click();

    const themeResponse = await themeResponsePromise;
    if (!themeResponse.ok()) {
      throw new Error(
        `Theme change request failed: ${themeResponse.status()} ${themeResponse.statusText()}`
      );
    }

    await page.waitForFunction(
      theme => document.documentElement.dataset.theme === theme,
      targetTheme
    );

    const changedTheme = await page.locator('html').getAttribute('data-theme');
    const changedScheme = await page.locator('html').getAttribute('data-scheme');
    const currentTargetPresets = page.locator(
      `.theme-choices .theme-preset.current[data-key="${targetTheme}"]`
    );

    if (changedTheme !== targetTheme) {
      throw new Error(`Theme state did not change: ${initialTheme} -> ${changedTheme}`);
    }

    if (!changedScheme) {
      throw new Error(`Theme scheme state is missing after change to ${changedTheme}`);
    }

    if (await currentTargetPresets.count() < 1) {
      throw new Error(`Theme picker did not mark ${targetTheme} as current`);
    }

    await openPage(page, '/themes/theme-components', coveragePath);

    const themeSemanticContract = await page.evaluate(() => {
      const root = document.documentElement;
      const header = document.querySelector('.header-container');
      const widgetSurface = document.querySelector(
        '.widget > .widget-content:not(.widget-content-frameless)'
      );
      const widgetHeader = document.querySelector('.widget > .widget-header');
      const widgetTitle = document.querySelector('.widget > .widget-header > h2');
      const card = document.querySelector('.glance-card');

      if (!header || !widgetSurface || !widgetHeader || !widgetTitle || !card) {
        return null;
      }

      const rootStyle = getComputedStyle(root);

      return {
        headerShadowVariable: rootStyle.getPropertyValue('--theme-header-shadow').trim(),
        widgetShadowVariable: rootStyle.getPropertyValue('--theme-widget-shadow').trim(),
        widgetHeaderFontSizeVariable:
          rootStyle.getPropertyValue('--theme-widget-header-font-size').trim(),
        cardShadowVariable: rootStyle.getPropertyValue('--theme-card-shadow').trim(),
        headerShadow: getComputedStyle(header).boxShadow,
        widgetShadow: getComputedStyle(widgetSurface).boxShadow,
        widgetHeaderFontSize: getComputedStyle(widgetHeader).fontSize,
        widgetTitleFontSize: getComputedStyle(widgetTitle).fontSize,
        cardShadow: getComputedStyle(card).boxShadow
      };
    });

    if (themeSemanticContract === null) {
      throw new Error('Theme semantic fixture elements are missing');
    }

    if (
      themeSemanticContract.headerShadowVariable !== 'none' ||
      themeSemanticContract.widgetShadowVariable !== 'none' ||
      themeSemanticContract.cardShadowVariable !== 'none'
    ) {
      throw new Error(
        `Theme shadow variables are incorrect: header=${themeSemanticContract.headerShadowVariable} widget=${themeSemanticContract.widgetShadowVariable} card=${themeSemanticContract.cardShadowVariable}`
      );
    }

    if (themeSemanticContract.widgetHeaderFontSizeVariable !== '1.2rem') {
      throw new Error(
        `Widget-header theme font-size variable is incorrect: ${themeSemanticContract.widgetHeaderFontSizeVariable}`
      );
    }

    if (
      themeSemanticContract.headerShadow !== 'none' ||
      themeSemanticContract.widgetShadow !== 'none' ||
      themeSemanticContract.cardShadow !== 'none'
    ) {
      throw new Error(
        `Theme shadows are not respected: header=${themeSemanticContract.headerShadow} widget=${themeSemanticContract.widgetShadow} card=${themeSemanticContract.cardShadow}`
      );
    }

    if (
      themeSemanticContract.widgetTitleFontSize !==
      themeSemanticContract.widgetHeaderFontSize
    ) {
      throw new Error(
        `Widget title does not inherit widget-header theme size: title=${themeSemanticContract.widgetTitleFontSize} header=${themeSemanticContract.widgetHeaderFontSize}`
      );
    }

    const themedWidgetSurface = page.locator(
      '.widget > .widget-content:not(.widget-content-frameless)'
    ).first();
    await themedWidgetSurface.hover();

    const widgetHoverShadow = await themedWidgetSurface.evaluate(
      element => getComputedStyle(element).boxShadow
    );

    if (widgetHoverShadow !== 'none') {
      throw new Error(
        `Widget hover overrides configured theme shadow: ${widgetHoverShadow}`
      );
    }

    const themedCard = page.locator('.glance-card').first();
    await themedCard.hover();

    const cardHoverShadow = await themedCard.evaluate(
      element => getComputedStyle(element).boxShadow
    );

    if (cardHoverShadow !== 'none') {
      throw new Error(
        `Card hover overrides configured theme shadow: ${cardHoverShadow}`
      );
    }

    console.log('PASS theme semantic styling contract');

    const themeCookies = await context.cookies(BASE_URL);
    const selectedThemeCookie = themeCookies.find(cookie => cookie.name === 'theme');

    async function assertCrossSchemePageTheme(
      route,
      baseTheme,
      expectedScheme,
      expectedHeadingColor,
      expectedWidgetHeaderColor
    ) {
      await context.addCookies([{
        name: 'theme',
        value: baseTheme,
        url: BASE_URL
      }]);

      await openPage(page, route, coveragePath);

      const themeState = await page.evaluate(() => {
        const rootStyle = getComputedStyle(document.documentElement);
        const widgetHeading = document.querySelector('.widget-header > h2');

        if (!widgetHeading) {
          return null;
        }

        return {
          scheme: document.documentElement.dataset.scheme,
          headingColor: rootStyle.getPropertyValue('--theme-heading-text-color').trim(),
          widgetHeaderColor: rootStyle.getPropertyValue('--theme-widget-header-text-color').trim(),
          computedWidgetHeadingColor: getComputedStyle(widgetHeading).color,
          computedWidgetHeaderColor: getComputedStyle(widgetHeading.parentElement).color
        };
      });

      if (themeState === null) {
        throw new Error(`Cross-scheme theme page ${route} has no widget heading`);
      }

      if (themeState.scheme !== expectedScheme) {
        throw new Error(
          `Cross-scheme theme page ${route} resolved scheme=${themeState.scheme}, want ${expectedScheme}`
        );
      }

      if (themeState.headingColor !== expectedHeadingColor) {
        throw new Error(
          `Cross-scheme theme page ${route} heading semantic=${themeState.headingColor}, want ${expectedHeadingColor}`
        );
      }

      if (themeState.widgetHeaderColor !== expectedWidgetHeaderColor) {
        throw new Error(
          `Cross-scheme theme page ${route} widget-header semantic=${themeState.widgetHeaderColor}, want ${expectedWidgetHeaderColor}`
        );
      }

      const expectedComputedWidgetHeaderColor = await page.evaluate(color => {
        const probe = document.createElement('span');
        probe.style.color = color;
        document.body.appendChild(probe);
        const computed = getComputedStyle(probe).color;
        probe.remove();
        return computed;
      }, expectedWidgetHeaderColor);

      if (
        themeState.computedWidgetHeadingColor !== expectedComputedWidgetHeaderColor ||
        themeState.computedWidgetHeaderColor !== expectedComputedWidgetHeaderColor
      ) {
        throw new Error(
          `Cross-scheme theme page ${route} widget heading does not consume its semantic color: ` +
          `heading=${themeState.computedWidgetHeadingColor} ` +
          `header=${themeState.computedWidgetHeaderColor} ` +
          `want=${expectedComputedWidgetHeaderColor}`
        );
      }
    }

    try {
      await assertCrossSchemePageTheme(
        '/themes/theme-light-page',
        'glance-dark',
        'light',
        'hsl(220.0, 20.0%, 12.0%)',
        'hsl(220.0, 18.0%, 14.0%)'
      );

      await assertCrossSchemePageTheme(
        '/themes/theme-dark-page',
        'glance-light',
        'dark',
        'hsl(220.0, 20.0%, 96.0%)',
        'hsl(220.0, 18.0%, 93.0%)'
      );
    } finally {
      if (selectedThemeCookie) {
        await context.addCookies([selectedThemeCookie]);
      } else {
        await context.clearCookies({ name: 'theme' });
      }
    }

    console.log('PASS cross-scheme page theme semantics');

    const utilitiesContentURL = `${BASE_URL}/api/pages/utilities/content/`;

    await page.route(utilitiesContentURL, async route => {
      const response = await route.fetch();
      const body = await response.text();
      const pattern = /(<script type="application\/json" data-unit-converter-catalog>)[\s\S]*?(<\/script>)/;
      const matches = body.match(pattern);

      if (!matches) {
        throw new Error('Unit converter catalog was not found in utilities page content');
      }

      const corruptedBody = body.replace(pattern, `$1{invalid-json$2`);
      await route.fulfill({
        response,
        body: corruptedBody
      });
    });

    let unitConverterDiagnostic;

    const diagnosticResponsePromise = page.waitForResponse(response => {
      if (
        response.request().method() !== 'POST' ||
        new URL(response.url()).pathname !== '/api/frontend-diagnostics' ||
        response.status() !== 204
      ) {
        return false;
      }

      const body = response.request().postData();
      if (!body) return false;

      try {
        const batch = JSON.parse(body);
        unitConverterDiagnostic = batch.events?.find(
          event => event.event === 'unit_converter_catalog_parse_error'
        );
        return unitConverterDiagnostic !== undefined;
      } catch {
        return false;
      }
    });

    await openPage(page, '/utilities', coveragePath);
    await diagnosticResponsePromise;
    await page.unroute(utilitiesContentURL);

    if (!unitConverterDiagnostic) {
      throw new Error('Unit converter parse failure did not reach frontend diagnostics');
    }

    if (!unitConverterDiagnostic.detail) {
      throw new Error('Unit converter parse diagnostic is missing error detail');
    }

    await page.evaluate(() => {
      localStorage.removeItem('todo-visual-qa-todo');
    });

    await openPage(page, '/utilities', coveragePath);

    const calculator = page.locator('.visual-fixture-calculator');
    await calculator.locator('[data-calculator-digit="7"]').click();
    await calculator.locator('[data-calculator-operator="*"]').click();
    await calculator.locator('[data-calculator-digit="6"]').click();
    await calculator.locator('[data-calculator-action="equals"]').click();

    const calculatorResult = (
      await calculator.locator('[data-calculator-result]').textContent()
    )?.trim();

    if (calculatorResult !== '42') {
      throw new Error(
        `Calculator interaction produced ${calculatorResult}, want 42`
      );
    }

    const converter = page.locator('.visual-fixture-unit-converter');
    const converterCategory = converter.locator(
      '[data-unit-converter-category]'
    );
    const converterFrom = converter.locator('[data-unit-converter-from]');
    const converterTo = converter.locator('[data-unit-converter-to]');
    const converterValue = converter.locator(
      '[data-unit-converter-value]'
    );
    const converterResult = converter.locator(
      '[data-unit-converter-result]'
    );

    await converterCategory.selectOption('temperature');
    await converterFrom.selectOption('F');
    await converterTo.selectOption('C');
    await converterValue.fill('32');
    await converterValue.dispatchEvent('input');

    const convertedTemperature = (await converterResult.textContent())?.trim();

    if (convertedTemperature !== '0 °C') {
      throw new Error(
        `Unit Converter produced ${convertedTemperature}, want 0 °C`
      );
    }

    const todo = page.locator('.visual-fixture-to-do');
    const todoInput = todo.locator('.todo-input textarea');
    await todoInput.fill('Frontend regression task');
    await todoInput.press('Enter');

    const todoItem = todo.locator('.todo-item', {
      hasText: 'Frontend regression task'
    });
    await todoItem.waitFor({ state: 'visible', timeout: 10000 });

    const persistedTodo = await page.evaluate(() =>
      JSON.parse(localStorage.getItem('todo-visual-qa-todo') || '[]')
    );

    if (
      persistedTodo.length !== 1 ||
      persistedTodo[0].text !== 'Frontend regression task'
    ) {
      throw new Error(
        `To-do interaction did not persist expected item: ${JSON.stringify(persistedTodo)}`
      );
    }

    console.log('PASS calculator interaction');
    console.log('PASS unit converter interaction');
    console.log('PASS to-do persistence interaction');

    await page.evaluate(() => {
      localStorage.removeItem('timer-visual-qa-timer');
    });

    await openPage(page, '/date-time-weather', coveragePath);

    const timer = page.locator('.visual-fixture-timer');
    await timer.locator('.timer-add').click();

    await timer.locator('.timer-form-name').fill('Frontend regression timer');
    await timer.locator('.timer-form-date').fill('2099-12-31');
    await timer.locator('.timer-form-time').fill('23:59');
    await timer.locator('.timer-form-save').click();

    const timerItem = timer.locator('.timer-item', {
      hasText: 'Frontend regression timer'
    });
    await timerItem.waitFor({ state: 'visible', timeout: 10000 });

    const timerCountdown = (
      await timerItem.locator('[data-timer-countdown]').textContent()
    )?.trim();

    if (!timerCountdown) {
      throw new Error('Timer interaction did not render a countdown');
    }

    const persistedTimers = await page.evaluate(() =>
      JSON.parse(localStorage.getItem('timer-visual-qa-timer') || '[]')
    );

    if (
      persistedTimers.length !== 1 ||
      persistedTimers[0].title !== 'Frontend regression timer' ||
      persistedTimers[0].date !== '2099-12-31' ||
      persistedTimers[0].time !== '23:59'
    ) {
      throw new Error(
        `Timer interaction did not persist expected item: ${JSON.stringify(persistedTimers)}`
      );
    }

    const calendar = page.locator('.visual-fixture-calendar .calendar');
    await calendar.waitFor({ state: 'visible', timeout: 10000 });

    const initialCalendarMonth = await calendar.getAttribute(
      'data-calendar-displayed-month'
    );

    if (!initialCalendarMonth) {
      throw new Error('Calendar did not expose its initial displayed month');
    }

    await calendar.locator('button[aria-label="Next month"]').click();

    const advancedCalendarMonth = await calendar.getAttribute(
      'data-calendar-displayed-month'
    );

    if (
      !advancedCalendarMonth ||
      advancedCalendarMonth === initialCalendarMonth
    ) {
      throw new Error(
        `Calendar did not advance from ${initialCalendarMonth}: ${advancedCalendarMonth}`
      );
    }

    const undoCalendarButton = calendar.locator(
      'button[aria-label="Back to today"]'
    );
    await undoCalendarButton.waitFor({ state: 'visible', timeout: 10000 });
    await undoCalendarButton.click();

    const restoredCalendarMonth = await calendar.getAttribute(
      'data-calendar-displayed-month'
    );

    if (restoredCalendarMonth !== initialCalendarMonth) {
      throw new Error(
        `Calendar did not return to ${initialCalendarMonth}: ${restoredCalendarMonth}`
      );
    }

    await calendar.locator('[data-calendar-date="2026-09-23"]').click();

    const timedCalendarEvent = calendar.locator('.calendar-event', {
      hasText: 'Multi-day timed regression'
    });

    await timedCalendarEvent.waitFor({ state: 'visible', timeout: 10000 });

    if (await timedCalendarEvent.locator('.calendar-event-time').count() !== 1) {
      throw new Error('Calendar timed event did not render its time label');
    }

    if (await timedCalendarEvent.evaluate(element =>
      element.classList.contains('calendar-event-without-time')
    )) {
      throw new Error('Calendar timed event incorrectly used continuation layout');
    }

    await calendar.locator('[data-calendar-date="2026-09-24"]').click();

    const continuedCalendarEvent = calendar.locator('.calendar-event', {
      hasText: 'Multi-day timed regression'
    });

    await continuedCalendarEvent.waitFor({ state: 'visible', timeout: 10000 });

    if (await continuedCalendarEvent.locator('.calendar-event-time').count() !== 0) {
      throw new Error('Calendar continuation event rendered a time label');
    }

    if (!(await continuedCalendarEvent.evaluate(element =>
      element.classList.contains('calendar-event-without-time')
    ))) {
      throw new Error('Calendar continuation event did not use full-width layout');
    }

    console.log('PASS timer persistence interaction');
    console.log('PASS calendar month navigation interaction');
    console.log('PASS calendar continuation presentation');

    await openPage(page, '/search-custom-content', coveragePath);

    const presentationWidget = page.locator('.visual-fixture-custom-api');
    const presentationTable = presentationWidget.locator(
      '[data-glance-table="services"]'
    );
    const presentationChart = presentationWidget.locator(
      '[data-glance-chart="resources"]'
    );

    await presentationTable.waitFor({ state: 'visible', timeout: 10000 });
    await presentationChart.waitFor({ state: 'visible', timeout: 10000 });

    if (await presentationWidget.locator('.dt-container').count() !== 1) {
      throw new Error('Configured Custom API table was not enhanced by DataTables');
    }

    const presentationRows = presentationTable.locator('tbody > tr');
    if (await presentationRows.count() !== 2) {
      throw new Error(
        `Configured Custom API table rendered ${await presentationRows.count()} rows, want 2`
      );
    }

    if (
      !(await presentationRows.nth(0).innerText()).includes('Dashboard') ||
      !(await presentationRows.nth(1).innerText()).includes('Fixture API')
    ) {
      throw new Error('Configured Custom API table did not preserve deterministic fixture rows');
    }

    const chartCanvas = presentationChart.locator('canvas[role="img"][aria-label="Resource usage"]');
    await chartCanvas.waitFor({ state: 'visible', timeout: 10000 });

    if (await presentationChart.evaluate(element =>
      element.classList.contains('glance-chart-failed')
    )) {
      throw new Error('Configured Custom API chart entered its failure state');
    }

    console.log('PASS Custom API configured table enhancement');
    console.log('PASS Custom API configured chart enhancement');

    await openPage(page, '/feeds-content', coveragePath);

    const disclosureWidget = page.locator('.visual-fixture-hacker-news');
    const disclosureList = disclosureWidget.locator('.collapsible-container');
    const disclosureButton = disclosureWidget.locator('.expand-toggle-button');
    const disclosureItems = disclosureList.locator('.collapsible-item');

    await disclosureButton.waitFor({ state: 'visible', timeout: 10000 });

    if (await disclosureButton.getAttribute('aria-expanded') !== 'false') {
      throw new Error('Disclosure control does not report collapsed initial state');
    }

    if ((await disclosureButton.textContent())?.trim().startsWith('Show more') !== true) {
      throw new Error('Disclosure control does not show the initial Show more label');
    }

    const disclosureItemCount = await disclosureItems.count();
    if (disclosureItemCount !== 3) {
      throw new Error(`Expected 3 collapsed Hacker News items, found ${disclosureItemCount}`);
    }

    for (let i = 0; i < disclosureItemCount; i++) {
      if (await disclosureItems.nth(i).isVisible()) {
        throw new Error(`Disclosure item ${i + 1} is visible before expansion`);
      }
    }

    await disclosureButton.click();

    if (await disclosureButton.getAttribute('aria-expanded') !== 'true') {
      throw new Error('Disclosure control does not report expanded state');
    }

    if ((await disclosureButton.textContent())?.trim().startsWith('Show less') !== true) {
      throw new Error('Disclosure control does not show the expanded Show less label');
    }

    for (let i = 0; i < disclosureItemCount; i++) {
      if (!await disclosureItems.nth(i).isVisible()) {
        throw new Error(`Disclosure item ${i + 1} remains hidden after expansion`);
      }
    }

    await disclosureButton.click();

    if (await disclosureButton.getAttribute('aria-expanded') !== 'false') {
      throw new Error('Disclosure control does not report collapsed state after collapse');
    }

    if ((await disclosureButton.textContent())?.trim().startsWith('Show more') !== true) {
      throw new Error('Disclosure control does not restore the Show more label');
    }

    for (let i = 0; i < disclosureItemCount; i++) {
      if (await disclosureItems.nth(i).isVisible()) {
        throw new Error(`Disclosure item ${i + 1} remains visible after collapse`);
      }
    }

    console.log('PASS shared disclosure expand/collapse contract');

    console.log('PASS contained frontend failure diagnostics');

    const failedPageContentURL = `${BASE_URL}/api/pages/layout-composition/content/`;
    const failedPageBodySentinel = 'PAGE_CONTENT_500_BODY_MUST_NOT_BE_RENDERED';

    await page.route(failedPageContentURL, async route => {
      await route.fulfill({
        status: 500,
        contentType: 'text/plain',
        body: failedPageBodySentinel
      });
    });

    let pageContentLoadDiagnostic;

    const pageContentDiagnosticResponsePromise = page.waitForResponse(response => {
      if (
        response.request().method() !== 'POST' ||
        new URL(response.url()).pathname !== '/api/frontend-diagnostics' ||
        response.status() !== 204
      ) {
        return false;
      }

      const body = response.request().postData();
      if (!body) return false;

      try {
        const batch = JSON.parse(body);
        pageContentLoadDiagnostic = batch.events?.find(
          event => event.event === 'page_content_load_error'
        );
        return pageContentLoadDiagnostic !== undefined;
      } catch {
        return false;
      }
    });

    const failedPageResponse = await page.goto(`${BASE_URL}/layout-composition`, {
      waitUntil: 'domcontentloaded',
      timeout: 30000
    });

    if (!failedPageResponse || !failedPageResponse.ok()) {
      throw new Error(
        `Page shell failed while testing page-content recovery: HTTP ${failedPageResponse?.status()}`
      );
    }

    await page.locator('#page.content-ready[aria-busy="false"]').waitFor({
      state: 'attached',
      timeout: 10000
    });

    await pageContentDiagnosticResponsePromise;
    await page.unroute(failedPageContentURL);

    const pageLoadError = page.locator('.page-load-error');
    await pageLoadError.waitFor({ state: 'visible', timeout: 10000 });

    if (await page.locator('.page-loading-container').isVisible()) {
      throw new Error('Page loading indicator remains visible after page-content failure');
    }

    const failedPageContent = await page.locator('#page-content').innerHTML();
    if (failedPageContent.includes(failedPageBodySentinel)) {
      throw new Error('Failed page-content response body was injected into the page');
    }

    if (failedPageContent.trim().length !== 0) {
      throw new Error('Page content was initialized after its HTTP request failed');
    }

    if (!pageContentLoadDiagnostic) {
      throw new Error('Page-content HTTP failure did not reach frontend diagnostics');
    }

    if (!pageContentLoadDiagnostic.detail?.includes('500')) {
      throw new Error(
        `Page-content failure diagnostic is missing HTTP status detail: ${pageContentLoadDiagnostic.detail || '<empty>'}`
      );
    }

    console.log('PASS page-content HTTP failure recovery');

    const timedOutPageContentURL = `${BASE_URL}/api/pages/feeds-content/content/`;
    let timedOutPageContentDiagnostic;

    await page.route(timedOutPageContentURL, async () => {
      await new Promise(() => {});
    });

    const timedOutPageContentDiagnosticPromise = page.waitForResponse(response => {
      if (
        response.request().method() !== 'POST' ||
        new URL(response.url()).pathname !== '/api/frontend-diagnostics' ||
        response.status() !== 204
      ) {
        return false;
      }

      const body = response.request().postData();
      if (!body) return false;

      try {
        const batch = JSON.parse(body);
        timedOutPageContentDiagnostic = batch.events?.find(
          event =>
            event.event === 'page_content_load_error' &&
            event.detail?.includes('AbortError')
        );
        return timedOutPageContentDiagnostic !== undefined;
      } catch {
        return false;
      }
    });

    const timedOutPageResponse = await page.goto(`${BASE_URL}/feeds-content`, {
      waitUntil: 'domcontentloaded',
      timeout: 30000
    });

    if (!timedOutPageResponse || !timedOutPageResponse.ok()) {
      throw new Error(
        `Page shell failed while testing page-content timeout recovery: HTTP ${timedOutPageResponse?.status()}`
      );
    }

    await page.locator('#page.content-ready[aria-busy="false"]').waitFor({
      state: 'attached',
      timeout: 10000
    });

    await timedOutPageContentDiagnosticPromise;
    await page.unroute(timedOutPageContentURL);

    const timedOutPageLoadError = page.locator('.page-load-error');
    await timedOutPageLoadError.waitFor({ state: 'visible', timeout: 10000 });

    if (await page.locator('.page-loading-container').isVisible()) {
      throw new Error('Page loading indicator remains visible after page-content timeout');
    }

    const timedOutPageContent = await page.locator('#page-content').innerHTML();
    if (timedOutPageContent.trim().length !== 0) {
      throw new Error('Page content was initialized after its HTTP request timed out');
    }

    if (!timedOutPageContentDiagnostic) {
      throw new Error('Page-content timeout did not reach frontend diagnostics');
    }

    console.log('PASS page-content timeout recovery');

    if (browserErrors.length > 0) {
      throw new Error(`Unexpected browser error(s): ${browserErrors.join(' | ')}`);
    }

    console.log('PASS desktop page initialization');
    console.log('PASS desktop horizontal overflow');
    console.log('PASS desktop fixed header geometry');
    console.log('PASS group tab interaction');
    console.log('PASS theme picker interaction');
    console.log('PASS no unexpected browser errors');

    await collectCoverage(page, coveragePath);
    await context.close();

    const mobileContext = await browser.newContext({
      viewport: { width: 430, height: 900 },
      deviceScaleFactor: 1
    });
    const mobilePage = await mobileContext.newPage();
    await startCoverage(mobilePage, coveragePath);
    const mobileBrowserErrors = [];

    mobilePage.on('pageerror', error => {
      mobileBrowserErrors.push(error.message);
    });

    await openPage(mobilePage, '/layout-composition', coveragePath);

    const mobileOverflow = await mobilePage.evaluate(() =>
      document.documentElement.scrollWidth - document.documentElement.clientWidth
    );
    if (mobileOverflow > 0) {
      throw new Error(`Mobile document overflows horizontally by ${mobileOverflow}px`);
    }

    const mobileShell = await mobilePage.evaluate(() => {
      const header = document.querySelector('.header-container');
      const navigation = document.querySelector('.mobile-navigation');

      if (!header || !navigation) {
        return null;
      }

      return {
        headerDisplay: getComputedStyle(header).display,
        navigationDisplay: getComputedStyle(navigation).display,
        navigationPosition: getComputedStyle(navigation).position
      };
    });

    if (mobileShell === null) {
      throw new Error('Mobile shell elements are missing');
    }

    if (mobileShell.headerDisplay !== 'none') {
      throw new Error(`Desktop header remains visible on mobile: display=${mobileShell.headerDisplay}`);
    }

    if (mobileShell.navigationDisplay === 'none' || mobileShell.navigationPosition !== 'fixed') {
      throw new Error(
        `Mobile navigation is not visible and fixed: display=${mobileShell.navigationDisplay} position=${mobileShell.navigationPosition}`
      );
    }

    const mobileMenuInput = mobilePage.locator('.mobile-navigation-page-links-input');
    if (await mobileMenuInput.count() !== 1) {
      throw new Error(`Expected exactly one mobile navigation menu control, found ${await mobileMenuInput.count()}`);
    }

    if (await mobileMenuInput.isChecked()) {
      throw new Error('Mobile navigation menu starts expanded on a clean load');
    }

    const mobileNavigation = mobilePage.locator('.mobile-navigation');
    const closedNavigationBox = await mobileNavigation.boundingBox();

    await mobileMenuInput.locator('xpath=..').click();

    if (!await mobileMenuInput.isChecked()) {
      throw new Error('Mobile navigation menu did not enter expanded state');
    }

    if (closedNavigationBox === null) {
      throw new Error('Mobile navigation has no collapsed geometry');
    }

    await mobilePage.waitForFunction(
      closedY => {
        const navigation = document.querySelector('.mobile-navigation');
        return navigation !== null && navigation.getBoundingClientRect().top < closedY;
      },
      closedNavigationBox.y
    );

    const openNavigationBox = await mobileNavigation.boundingBox();
    if (openNavigationBox === null || openNavigationBox.y >= closedNavigationBox.y) {
      throw new Error(
        `Mobile navigation did not expand upward: closedY=${closedNavigationBox.y} openY=${openNavigationBox?.y}`
      );
    }

    await mobileMenuInput.locator('xpath=..').click();

    if (await mobileMenuInput.isChecked()) {
      throw new Error('Mobile navigation menu did not return to collapsed state');
    }

    const mobileRadios = mobilePage.locator('.mobile-navigation-input');
    const mobileColumns = mobilePage.locator('.page-columns > .page-column');

    const radioCount = await mobileRadios.count();
    const columnCount = await mobileColumns.count();

    if (radioCount < 2 || radioCount !== columnCount) {
      throw new Error(
        `Canonical mobile fixture has invalid radio/column counts: radios=${radioCount} columns=${columnCount}`
      );
    }

    const checkedRadio = mobilePage.locator('.mobile-navigation-input:checked');
    if (await checkedRadio.count() !== 1) {
      throw new Error(`Expected exactly one selected mobile column, found ${await checkedRadio.count()}`);
    }

    const initialIndex = Number(await checkedRadio.getAttribute('value'));
    const initialVisibleColumns = [];

    for (let i = 0; i < columnCount; i++) {
      if (await mobileColumns.nth(i).isVisible()) {
        initialVisibleColumns.push(i);
      }
    }

    if (initialVisibleColumns.length !== 1 || initialVisibleColumns[0] !== initialIndex) {
      throw new Error(
        `Incorrect initial mobile column visibility: selected=${initialIndex} visible=${initialVisibleColumns.join(',')}`
      );
    }

    const nextIndex = initialIndex === 0 ? 1 : 0;
    await mobileRadios.nth(nextIndex).locator('xpath=..').click();

    const switchedVisibleColumns = [];
    for (let i = 0; i < columnCount; i++) {
      if (await mobileColumns.nth(i).isVisible()) {
        switchedVisibleColumns.push(i);
      }
    }

    if (switchedVisibleColumns.length !== 1 || switchedVisibleColumns[0] !== nextIndex) {
      throw new Error(
        `Mobile column switch failed: selected=${nextIndex} visible=${switchedVisibleColumns.join(',')}`
      );
    }

    await openPage(mobilePage, '/feeds-content', coveragePath);

    const mobileScrollGeometry = await mobilePage.evaluate(() => {
      const spacer = document.createElement('div');
      spacer.dataset.frontendCheckScrollSpacer = 'true';
      spacer.style.height = `${window.innerHeight}px`;
      spacer.style.flexShrink = '0';
      document.querySelector('.body-content')?.appendChild(spacer);

      return {
        scrollHeight: document.documentElement.scrollHeight,
        clientHeight: document.documentElement.clientHeight
      };
    });

    if (mobileScrollGeometry.scrollHeight <= mobileScrollGeometry.clientHeight) {
      throw new Error(
        `Mobile bottom-clearance test could not create scrollable geometry: scrollHeight=${mobileScrollGeometry.scrollHeight}px clientHeight=${mobileScrollGeometry.clientHeight}px`
      );
    }

    await mobilePage.evaluate(() =>
      window.scrollTo(0, document.documentElement.scrollHeight)
    );

    const mobileBottomGeometry = await mobilePage.evaluate(() => {
      const footer = document.querySelector('.footer');
      const navigation = document.querySelector('.mobile-navigation');
      const offset = document.querySelector('.mobile-navigation-offset');

      if (!footer || !navigation || !offset) {
        return null;
      }

      const footerRect = footer.getBoundingClientRect();
      const navigationRect = navigation.getBoundingClientRect();
      const offsetRect = offset.getBoundingClientRect();

      return {
        scrollY: window.scrollY,
        maximumScroll:
          document.documentElement.scrollHeight -
          document.documentElement.clientHeight,
        footerBottom: footerRect.bottom,
        navigationTop: navigationRect.top,
        navigationVisibleHeight: window.innerHeight - navigationRect.top,
        offsetHeight: offsetRect.height
      };
    });

    if (mobileBottomGeometry === null) {
      throw new Error('Mobile bottom-clearance elements are missing');
    }

    if (Math.abs(mobileBottomGeometry.scrollY - mobileBottomGeometry.maximumScroll) > 1) {
      throw new Error(
        `Mobile scroll did not reach bottom: scrollY=${mobileBottomGeometry.scrollY}px maximum=${mobileBottomGeometry.maximumScroll}px`
      );
    }

    if (mobileBottomGeometry.footerBottom > mobileBottomGeometry.navigationTop + 1) {
      throw new Error(
        `Mobile navigation obscures footer at maximum scroll: footer=${mobileBottomGeometry.footerBottom}px navigation=${mobileBottomGeometry.navigationTop}px`
      );
    }

    if (
      mobileBottomGeometry.offsetHeight + 1 <
      mobileBottomGeometry.navigationVisibleHeight
    ) {
      throw new Error(
        `Mobile navigation offset is too small: offset=${mobileBottomGeometry.offsetHeight}px visible-navigation=${mobileBottomGeometry.navigationVisibleHeight}px`
      );
    }

    if (mobileBrowserErrors.length > 0) {
      throw new Error(`Unexpected mobile browser error(s): ${mobileBrowserErrors.join(' | ')}`);
    }

    console.log('PASS mobile page initialization');
    console.log('PASS mobile horizontal overflow');
    console.log('PASS mobile navigation shell');
    console.log('PASS mobile navigation disclosure');
    console.log('PASS mobile primary column selection');
    console.log('PASS mobile column switching');
    console.log('PASS mobile bottom navigation clearance');
    console.log('PASS no unexpected mobile browser errors');
    console.log('Frontend checks passed.');

    await collectCoverage(mobilePage, coveragePath);
    await mobileContext.close();

    writeCoverage(coveragePath);
  } finally {
    if (browser) await browser.close();
  }
}

main().catch(error => {
  console.error(`Frontend check failed: ${error.message}`);
  process.exitCode = 1;
});
