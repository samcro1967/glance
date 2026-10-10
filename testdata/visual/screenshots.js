// Path: testdata/visual/screenshots.js
// File: screenshots.js
/**
 * Canonical browser capture runner for Glance visual QA and documentation.
 *
 * Ownership: development-only visual QA infrastructure.
 * Responsibilities: capture deterministic Glance pages and explicitly mapped
 * documentation images with the project-local Playwright dependency.
 * Non-goals: pixel-diff testing, browser installation, or production access.
 * Guarantees: documentation captures are staging-only; this runner never
 * modifies the checked-in docs/images directory.
 */

'use strict';

const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');

const ROOT = path.resolve(__dirname, '..', '..');
const SCREENSHOT_ROOT = path.join(__dirname, 'screenshots');
const DOCS_STAGING = path.join(__dirname, 'docs-staging');
const DOCS_MAP = path.join(__dirname, 'docs-images.json');
const WIDGET_MAP = path.join(__dirname, 'widget-screenshots.json');
const VISUAL_PAGES_MAP = path.join(__dirname, 'visual-pages.json');
const BASE_URL = (process.env.GLANCE_VISUAL_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
const CHROME = process.env.GLANCE_VISUAL_CHROME || '/usr/bin/google-chrome';
const MODE = process.argv.includes('--docs') ? 'docs' : process.argv.includes('--all') ? 'all' : 'qa';
const DEFAULT_VIEWPORT = { width: 1600, height: 1000 };
const QA_VIEWPORTS = {
  desktop: DEFAULT_VIEWPORT,
  mobile: { width: 430, height: 900 },
};

const qaPageRoutes = {
  'layout-composition': '/layout-composition',
  'feeds-content': '/feeds-content',
  'search-custom-content': '/search-custom-content',
  'date-time-weather': '/date-time-weather',
  'homelab-monitoring': '/homelab-monitoring',
  'development-releases': '/development-releases',
  'markets-streaming': '/markets-streaming',
  'utilities': '/utilities',
  'daily-discovery': '/daily-discovery',
  'theme-global': '/themes/',
  'theme-dark-page': '/themes/theme-dark-page',
  'theme-light-page': '/themes/theme-light-page',
  'theme-partial': '/themes/theme-partial',
  'theme-components': '/themes/theme-components',
  'theme-composition': '/themes/theme-composition',
};

function optionValue(name) {
  const prefix = `--${name}=`;
  const argument = process.argv.find(value => value.startsWith(prefix));
  return argument ? argument.slice(prefix.length) : '';
}

const dashboardFilter = optionValue('dashboard');
const pageFilter = optionValue('page');
const imageFilter = optionValue('image');
const widgetFilter = optionValue('widget');
const viewportFilter = optionValue('viewport');
const qaViewportName = viewportFilter || 'desktop';
const qaViewport = QA_VIEWPORTS[qaViewportName];

if (!qaViewport) {
  throw new Error(
    `Unknown QA viewport: ${qaViewportName}; expected ${Object.keys(QA_VIEWPORTS).join(', ')}`
  );
}

if (viewportFilter && MODE !== 'qa') {
  throw new Error('--viewport is only supported in QA mode');
}

const selectedFilters = [dashboardFilter, pageFilter, imageFilter, widgetFilter]
  .filter(Boolean);

if (selectedFilters.length > 1) {
  throw new Error('--dashboard, --page, --image, and --widget are mutually exclusive');
}

if (imageFilter && MODE !== 'docs') {
  throw new Error('--image is only supported in docs mode');
}

if (widgetFilter && MODE !== 'qa') {
  throw new Error('--widget is only supported in QA mode');
}

async function revealNestedGroupContent(locator) {
  const tabpanels = [];
  let current = locator;

  // A documentation target may sit behind more than one Group tab. Collect
  // each containing tabpanel from the target outward, then activate them in
  // reverse order so every inner tab is visible before Playwright clicks it.
  while (true) {
    const tabpanel = current.locator('xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " widget-group-content ")][1]');

    if ((await tabpanel.count()) === 0) break;

    const tabpanelId = await tabpanel.getAttribute('id');
    if (tabpanelId) tabpanels.push({ tabpanel, tabpanelId });

    current = tabpanel;
  }

  for (let index = tabpanels.length - 1; index >= 0; index--) {
    const { tabpanel, tabpanelId } = tabpanels[index];
    const group = tabpanel.locator('xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " widget-type-group ")][1]');
    const tab = group.locator(`.widget-group-title[aria-controls="${tabpanelId}"]`);

    if ((await tab.getAttribute('aria-selected')) !== 'true') {
      await tab.click();
    }
  }
}

function selectedQaPages() {
  const visualPages = JSON.parse(fs.readFileSync(VISUAL_PAGES_MAP, 'utf8'));
  const qaDashboards = Object.entries(visualPages)
    .filter(([dashboard]) => dashboard !== 'screenshots');

  if (dashboardFilter && !Object.prototype.hasOwnProperty.call(visualPages, dashboardFilter)) {
    throw new Error(`Unknown visual dashboard: ${dashboardFilter}`);
  }

  if (dashboardFilter === 'screenshots') {
    throw new Error(
      'The screenshots dashboard contains documentation fixtures; use docs mode'
    );
  }

  const knownPages = new Set(
    qaDashboards.flatMap(([, pages]) => pages)
  );

  if (pageFilter && !knownPages.has(pageFilter)) {
    throw new Error(`Unknown canonical QA page: ${pageFilter}`);
  }

  const selected = [];

  for (const [dashboard, pages] of qaDashboards) {
    if (dashboardFilter && dashboard !== dashboardFilter) continue;

    for (const page of pages) {
      if (pageFilter && page !== pageFilter) continue;

      const route = qaPageRoutes[page];
      if (!route) {
        throw new Error(`No QA route registered for canonical page: ${page}`);
      }

      const outputName = page.startsWith('theme-')
        ? page.slice('theme-'.length)
        : page;

      selected.push([dashboard, outputName, route, page]);
    }
  }

  return selected;
}

function selectedRoutes() {
  const visualPages = JSON.parse(fs.readFileSync(VISUAL_PAGES_MAP, 'utf8'));

  if (dashboardFilter) {
    const pages = visualPages[dashboardFilter];
    if (!pages) throw new Error(`Unknown visual dashboard: ${dashboardFilter}`);

    return new Set(
      pages
        .map(page => qaPageRoutes[page] || `/screenshots/${page}`)
    );
  }

  if (pageFilter) {
    const knownPages = Object.values(visualPages).flat();

    if (!knownPages.includes(pageFilter)) {
      throw new Error(`Unknown visual page: ${pageFilter}`);
    }

    return new Set([
      qaPageRoutes[pageFilter] || `/screenshots/${pageFilter}`
    ]);
  }

  return null;
}


function ensureCleanDirectory(directory) {
  fs.rmSync(directory, { recursive: true, force: true });
  fs.mkdirSync(directory, { recursive: true });
}

async function settle(page) {
  await page.waitForLoadState('domcontentloaded');
  await page.waitForTimeout(1800);
}

async function openPage(page, route) {
  const url = `${BASE_URL}${route}`;
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

  await settle(page);
}

async function captureMobileViewport(page, output, viewport = qaViewport) {
  const png = await page.screenshot({ path: output, animations: 'disabled' });
  const width = png.readUInt32BE(16);
  const height = png.readUInt32BE(20);
  if (width !== viewport.width || height !== viewport.height) {
    throw new Error(`Invalid mobile screenshot dimensions for ${output}: ` +
      `${width}x${height}, expected ${viewport.width}x${viewport.height}`);
  }
}

async function waitForDilbertImage(page, locator) {
  const isDilbert = await locator.evaluate(element =>
    element.classList.contains("visual-fixture-dilbert")
  );

  if (!isDilbert) return;

  const image = locator.locator(".dilbert-comic-image");
  await image.waitFor({ state: "visible", timeout: 15000 });
  await image.scrollIntoViewIfNeeded();

  try {
    await image.evaluate(element => {
      if (!element.complete) {
        element.loading = "eager";
      }
    });

    await page.waitForFunction(() => {
      const image = document.querySelector(
        ".visual-fixture-dilbert .dilbert-comic-image"
      );
      return image && image.complete &&
        image.naturalWidth > 0 && image.naturalHeight > 0;
    }, null, { timeout: 15000 });
  } catch (error) {
    throw new Error(
      `Dilbert screenshot image failed to load: ${error.message}`
    );
  }
}

async function captureMobileWidget(page, locator, output, viewport = QA_VIEWPORTS.mobile) {
  const columnIndex = await locator.evaluate(element => {
    const column = element.closest(".page-columns > .page-column");
    if (!column) return -1;
    return Array.from(column.parentElement.children)
      .filter(child => child.classList.contains("page-column"))
      .indexOf(column);
  });

  if (columnIndex >= 0) {
    const radio = page.locator(".mobile-navigation-input").nth(columnIndex);
    if (!(await radio.isChecked())) {
      await radio.locator("xpath=..").click();
    }
  }

  if (!(await locator.isVisible())) {
    await revealNestedGroupContent(locator);
  }
  await locator.waitFor({ state: "visible", timeout: 15000 });

  // Extend only the capture document so bottom widgets can scroll into view.
  // The spacer is removed after capture; production layout is unchanged.
  const spacer = await page.evaluate(height => {
    const element = document.createElement("div");
    element.dataset.visualCaptureSpacer = "true";
    element.style.height = `${height}px`;
    element.style.flexShrink = "0";
    const content = document.querySelector(".page-content");
    if (!content) throw new Error("Missing page content for mobile capture");
    content.appendChild(element);
    return true;
  }, viewport.height * 2);

  try {
    await locator.evaluate(element => element.scrollIntoView({
      block: "start", inline: "nearest", behavior: "instant"
    }));

    const geometry = await locator.evaluate(element => {
      const rect = element.getBoundingClientRect();
      const navigation = document.querySelector(".mobile-navigation");
      const navigationTop = navigation
        ? navigation.getBoundingClientRect().top
        : window.innerHeight;
      const visibleBottom = Math.min(window.innerHeight, navigationTop);
      return {
        top: rect.top,
        bottom: rect.bottom,
        left: rect.left,
        right: rect.right,
        visibleBottom,
        scrollY: window.scrollY,
        maxScroll: document.documentElement.scrollHeight - window.innerHeight
      };
    });

    const intersects = geometry.bottom > 0 &&
      geometry.top < geometry.visibleBottom &&
      geometry.right > 0 && geometry.left < viewport.width;

    if (!intersects) {
      throw new Error(
        `Mobile widget not positioned in viewport: ${JSON.stringify(geometry)}`
      );
    }

    await waitForDilbertImage(page, locator);
    await captureMobileViewport(page, output, viewport);
  } finally {
    if (spacer) {
      await page.evaluate(() => {
        document.querySelector("[data-visual-capture-spacer]")?.remove();
      });
    }
  }
}

async function captureQa(browser) {
  const qaPages = selectedQaPages();
  const widgetMappings = JSON.parse(fs.readFileSync(WIDGET_MAP, 'utf8'));

  if (widgetFilter && !Object.prototype.hasOwnProperty.call(widgetMappings, widgetFilter)) {
    throw new Error(`Unknown widget screenshot mapping: ${widgetFilter}`);
  }

  const selective = Boolean(dashboardFilter || pageFilter || widgetFilter);

  const qaOutputRoot = qaViewportName === 'desktop'
    ? SCREENSHOT_ROOT
    : path.join(SCREENSHOT_ROOT, qaViewportName);

  if (selective) {
    fs.mkdirSync(qaOutputRoot, { recursive: true });
  } else {
    ensureCleanDirectory(qaOutputRoot);
  }

  // Exercise phone viewport and touch semantics, not only narrow desktop width.
  const context = await browser.newContext({
    viewport: qaViewport,
    deviceScaleFactor: 1,
    isMobile: qaViewportName !== 'desktop',
    hasTouch: qaViewportName !== 'desktop'
  });
  const page = await context.newPage();
  let capturedPages = 0;
  const pageErrors = [];
  page.on('pageerror', error => pageErrors.push(error.message));

  if (!widgetFilter) {
    for (const [dashboard, name, route] of qaPages) {
      const directory = path.join(qaOutputRoot, dashboard);
      fs.mkdirSync(directory, { recursive: true });
      await openPage(page, route);

      if (qaViewportName !== 'desktop') {
        const radios = page.locator('.mobile-navigation-input');
        const columns = page.locator('.page-columns > .page-column');
        const count = await radios.count();
        if (count === 0 || count !== await columns.count()) {
          throw new Error(`Mobile column controls mismatch: ${route}`);
        }

        const assertColumn = async index => {
          const state = await page.evaluate(expected => {
            const inputs = [...document.querySelectorAll('.mobile-navigation-input')];
            const columns = [...document.querySelectorAll('.page-columns > .page-column')];
            const visible = columns.map((column, i) => ({
              index: i,
              visible: column.getClientRects().length > 0 && getComputedStyle(column).display !== 'none'
            })).filter(column => column.visible).map(column => column.index);
            const widgets = columns[expected]
              ? [...columns[expected].querySelectorAll('.widget')].filter(widget =>
                  widget.getClientRects().length > 0 && getComputedStyle(widget).visibility !== 'hidden'
                ).length
              : 0;
            return { checked: inputs.findIndex(input => input.checked), visible, widgets };
          }, index);
          if (state.checked !== index || state.visible.length !== 1 || state.visible[0] !== index || state.widgets === 0) {
            throw new Error(`Invalid mobile column state for ${route}: expected=${index + 1} actual=${JSON.stringify(state)}`);
          }
        };

        const initiallySelected = await radios.evaluateAll(inputs =>
          inputs.findIndex(input => input.checked)
        );
        if (initiallySelected < 0) {
          throw new Error(`No mobile column selected: ${route}`);
        }
        const menu = page.locator('.mobile-navigation-page-links-input');
        if (await menu.isChecked()) {
          await menu.locator('xpath=..').click();
        }
        await page.waitForFunction(() => {
          const navigation = document.querySelector('.mobile-navigation');
          const height = parseFloat(getComputedStyle(document.documentElement)
            .getPropertyValue('--mobile-navigation-height'));
          return navigation && Number.isFinite(height) &&
            Math.abs(navigation.getBoundingClientRect().top - (window.innerHeight - height)) < 1;
        });
        await assertColumn(initiallySelected);

        const output = path.join(directory, `${name}.png`);
        await captureMobileViewport(page, output);
        capturedPages++;
        console.log(`QA   ${dashboard}/${name}.png`);

        for (let column = 0; column < count; column++) {
          if (column === initiallySelected) continue;
          await radios.nth(column).locator('xpath=..').click();
          await columns.nth(column).waitFor({ state: 'visible', timeout: 5000 });
          await assertColumn(column);
          const filename = `${name}-column-${column + 1}.png`;
          await captureMobileViewport(page, path.join(directory, filename));
          capturedPages++;
          console.log(`QA   ${dashboard}/${filename}`);
        }

        const activeColumn = await radios.evaluateAll(inputs =>
          inputs.findIndex(input => input.checked)
        );
        await menu.locator('xpath=..').click();
        if (!(await menu.isChecked())) {
          throw new Error(`Mobile page navigation did not open: ${route}`);
        }
        // Wait until the expanded menu reaches its final viewport position.
        await page.waitForFunction(() => {
          const navigation = document.querySelector('.mobile-navigation');
          if (!navigation) return false;
          const rect = navigation.getBoundingClientRect();
          return rect.top >= -1 && Math.abs(rect.bottom - window.innerHeight) < 1;
        });
        await assertColumn(activeColumn);
        const filename = `${name}-navigation-expanded.png`;
        await captureMobileViewport(page, path.join(directory, filename));
        capturedPages++;
        console.log(`QA   ${dashboard}/${filename}`);
      } else {
        const output = path.join(directory, `${name}.png`);
        await page.screenshot({ path: output, fullPage: true });
        capturedPages++;
        console.log(`QA   ${dashboard}/${name}.png`);
      }

    }
  }

  const widgetDirectory = path.join(qaOutputRoot, 'widgets');
  fs.mkdirSync(widgetDirectory, { recursive: true });

  const allowedRoutes = selectedRoutes();
  const selectedWidgetMappings = Object.fromEntries(
    Object.entries(widgetMappings).filter(([widgetType, recipe]) =>
      (!widgetFilter || widgetType === widgetFilter) &&
      (!allowedRoutes || allowedRoutes.has(recipe.route))
    )
  );

  const widgetsByRoute = new Map();
  for (const [widgetType, recipe] of Object.entries(selectedWidgetMappings)) {
    if (!widgetsByRoute.has(recipe.route)) {
      widgetsByRoute.set(recipe.route, []);
    }
    widgetsByRoute.get(recipe.route).push([widgetType, recipe]);
  }

  console.log('');
  console.log('=== WIDGET SCREENSHOTS ===');

  const widgetFailures = [];

  for (const [route, widgets] of widgetsByRoute) {
    await openPage(page, route);
    console.log(`ROUTE  ${route}`);

    for (const [widgetType, recipe] of widgets) {
      try {
        const locator = page.locator(recipe.selector).nth(recipe.nth || 0);
        const output = path.join(widgetDirectory, `${widgetType}.png`);
        if (qaViewportName !== "desktop") {
          await captureMobileWidget(page, locator, output);
        } else {
          if (!(await locator.isVisible())) {
            await revealNestedGroupContent(locator);
          }
          await locator.waitFor({ state: "visible", timeout: 5000 });
          await locator.screenshot({ path: output });
        }
        console.log(`WIDGET ${widgetType}.png`);
      } catch (error) {
        widgetFailures.push({
          widgetType,
          route,
          selector: recipe.selector,
          error: error.message.split('\\n')[0]
        });
        console.error(`FAILED ${widgetType}: ${error.message.split('\\n')[0]}`);
      }
    }
  }

  const expectedWidgets = Object.keys(selectedWidgetMappings).length;
  const capturedWidgets = expectedWidgets - widgetFailures.length;

  console.log('');

  console.log(`Page screenshots:   ${capturedPages}`);
  console.log(`Widget screenshots: ${capturedWidgets}/${expectedWidgets}`);
  console.log(`Total screenshots:  ${capturedPages + capturedWidgets}`);

  if (widgetFailures.length) {
    console.log('');
    console.error('=== WIDGET CAPTURE FAILURES ===');

    for (const failure of widgetFailures) {
      console.error(
        `${failure.widgetType}: route=${failure.route} ` +
        `selector=${failure.selector} error=${failure.error}`
      );
    }
  }

  await context.close();

  if (widgetFailures.length) {
    throw new Error(
      `Canonical widget capture incomplete: ` +
      `${capturedWidgets}/${expectedWidgets} succeeded; ` +
      `${widgetFailures.length} failed`
    );
  }
  if (pageErrors.length) {
    console.warn(`Browser page errors observed: ${pageErrors.length}`);
    for (const error of [...new Set(pageErrors)]) console.warn(`  ${error}`);
  }
}

async function captureDocs(browser) {
  const mappings = JSON.parse(fs.readFileSync(DOCS_MAP, 'utf8'));
  const selective = Boolean(dashboardFilter || pageFilter || imageFilter);
  const allowedRoutes = imageFilter ? null : selectedRoutes();

  if (imageFilter) {
    const recipe = mappings[imageFilter];

    if (!recipe) {
      throw new Error(`Unknown documentation image: ${imageFilter}`);
    }

    if (recipe.kind !== 'browser') {
      throw new Error(
        `Documentation image is not browser-managed: ${imageFilter}`
      );
    }
  }

  if (selective) {
    fs.mkdirSync(DOCS_STAGING, { recursive: true });
  } else {
    ensureCleanDirectory(DOCS_STAGING);
  }

  const browserMappings = Object.entries(mappings)
    .filter(([filename, recipe]) =>
      recipe.kind === 'browser' &&
      (
        imageFilter
          ? filename === imageFilter
          : (!allowedRoutes || allowedRoutes.has(recipe.route))
      )
    );

  if (selective && browserMappings.length === 0) {
    throw new Error('No documentation images match the selected visual scope');
  }

  const unknownKinds = Object.entries(mappings)
    .filter(([, recipe]) => !['browser', 'static'].includes(recipe.kind));

  if (unknownKinds.length) {
    throw new Error(
      'Unsupported documentation image kinds: ' +
      unknownKinds
        .map(([filename, recipe]) => `${filename}=${recipe.kind}`)
        .join(', ')
    );
  }

  for (const [filename, recipe] of browserMappings) {
    const viewport = recipe.viewport || DEFAULT_VIEWPORT;
    const mobile = viewport.width === QA_VIEWPORTS.mobile.width &&
      viewport.height === QA_VIEWPORTS.mobile.height;
    const context = await browser.newContext({
      viewport,
      deviceScaleFactor: 1,
      isMobile: mobile,
      hasTouch: mobile
    });
    const page = await context.newPage();

    try {
      await openPage(page, recipe.route);

      if (recipe.click) {
        const trigger = page.locator(recipe.click).nth(recipe.clickNth || 0);
        await trigger.waitFor({ state: 'visible', timeout: 15000 });
        await trigger.click();
      }

      const output = path.join(DOCS_STAGING, filename);
      fs.mkdirSync(path.dirname(output), { recursive: true });

      if (recipe.capture === 'element') {
        const locator = page.locator(recipe.selector).nth(recipe.nth || 0);

        if (mobile) {
          await captureMobileWidget(page, locator, output, viewport);
        } else {
          if (!(await locator.isVisible())) {
            await revealNestedGroupContent(locator);
          }
          await locator.waitFor({ state: "visible", timeout: 15000 });
          await waitForDilbertImage(page, locator);
          await locator.screenshot({ path: output });
        }
      } else if (recipe.capture === 'page') {
        await page.screenshot({
          path: output,
          fullPage: recipe.fullPage !== false
        });
      } else {
        throw new Error(
          `Unsupported documentation capture type for ${filename}: ${recipe.capture}`
        );
      }

      console.log(
        `DOCS stage ${filename} ` +
        `(${recipe.capture}, ${viewport.width}x${viewport.height})`
      );
    } finally {
      await context.close();
    }
  }

  console.log('');
  console.log(`Documentation browser images staged: ${browserMappings.length}`);
  console.log(`Static documentation images skipped: ${Object.keys(mappings).length - browserMappings.length}`);
  console.log(`Staging directory: ${DOCS_STAGING}`);
  console.log('docs/images was not modified.');
}

async function main() {
  if (!fs.existsSync(CHROME)) throw new Error(`Chrome executable not found: ${CHROME}`);

  let browser;

  try {
    browser = await chromium.launch({
      executablePath: CHROME,
      headless: true
    });

    if (MODE === 'qa' || MODE === 'all') await captureQa(browser);
    if (MODE === 'docs' || MODE === 'all') await captureDocs(browser);
  } finally {
    if (browser) await browser.close();
  }
}

main().catch(error => {
  console.error(`Visual capture failed: ${error.message}`);
  process.exitCode = 1;
});
