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

const selectedFilters = [dashboardFilter, pageFilter, imageFilter]
  .filter(Boolean);

if (selectedFilters.length > 1) {
  throw new Error('--dashboard, --page, and --image are mutually exclusive');
}

if (imageFilter && MODE !== 'docs') {
  throw new Error('--image is only supported in docs mode');
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

async function captureQa(browser) {
  const qaPages = selectedQaPages();
  const selective = Boolean(dashboardFilter || pageFilter);

  const qaOutputRoot = qaViewportName === 'desktop'
    ? SCREENSHOT_ROOT
    : path.join(SCREENSHOT_ROOT, qaViewportName);

  if (selective) {
    fs.mkdirSync(qaOutputRoot, { recursive: true });
  } else {
    ensureCleanDirectory(qaOutputRoot);
  }

  const context = await browser.newContext({ viewport: qaViewport, deviceScaleFactor: 1 });
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', error => pageErrors.push(error.message));

  for (const [dashboard, name, route] of qaPages) {
    const directory = path.join(qaOutputRoot, dashboard);
    fs.mkdirSync(directory, { recursive: true });
    await openPage(page, route);

    if (qaViewportName !== 'desktop') {
      await page.locator('.mobile-navigation-page-links-input').evaluate(input => {
        input.checked = false;
      });

      await page.waitForFunction(() => {
        const navigation = document.querySelector('.mobile-navigation');
        if (navigation === null) {
          return false;
        }

        const navigationHeight = parseFloat(
          getComputedStyle(document.documentElement).getPropertyValue('--mobile-navigation-height')
        );

        if (!Number.isFinite(navigationHeight)) {
          return false;
        }

        const top = navigation.getBoundingClientRect().top;
        return Math.abs(top - (window.innerHeight - navigationHeight)) < 1;
      });
    }

    const output = path.join(directory, `${name}.png`);
    await page.screenshot({
      path: output,
      fullPage: qaViewportName === 'desktop'
    });
    console.log(`QA   ${dashboard}/${name}.png`);
  }

  if (qaViewportName !== 'desktop') {
    console.log('');
    console.log(`Page screenshots:   ${qaPages.length}`);
    console.log('Widget screenshots: skipped for non-desktop QA viewport');
    console.log(`Total screenshots:  ${qaPages.length}`);

    await context.close();

    if (pageErrors.length) {
      console.warn(`Browser page errors observed: ${pageErrors.length}`);
    }

    return;
  }

  const widgetMappings = JSON.parse(fs.readFileSync(WIDGET_MAP, 'utf8'));
  const widgetDirectory = path.join(SCREENSHOT_ROOT, 'widgets');
  fs.mkdirSync(widgetDirectory, { recursive: true });

  const allowedRoutes = selectedRoutes();
  const selectedWidgetMappings = Object.fromEntries(
    Object.entries(widgetMappings).filter(([, recipe]) =>
      !allowedRoutes || allowedRoutes.has(recipe.route)
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
        await locator.waitFor({ state: 'visible', timeout: 5000 });

        const output = path.join(widgetDirectory, `${widgetType}.png`);
        await locator.screenshot({ path: output });
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
  console.log(`Page screenshots:   ${qaPages.length}`);
  console.log(`Widget screenshots: ${capturedWidgets}/${expectedWidgets}`);
  console.log(`Total screenshots:  ${qaPages.length + capturedWidgets}`);

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
    const context = await browser.newContext({
      viewport,
      deviceScaleFactor: 1
    });
    const page = await context.newPage();

    try {
      await openPage(page, recipe.route);

      const output = path.join(DOCS_STAGING, filename);
      fs.mkdirSync(path.dirname(output), { recursive: true });

      if (recipe.capture === 'element') {
        const locator = page.locator(recipe.selector).nth(recipe.nth || 0);
        await locator.waitFor({ state: 'visible', timeout: 15000 });
        await locator.screenshot({ path: output });
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
