// Path: testdata/visual/performance.js
// Production-representative browser performance measurement runner.

"use strict";

const { chromium } = require("playwright");

const BASE_URL = (process.env.GLANCE_PERFORMANCE_URL || "http://127.0.0.1:18080").replace(/\/$/, "");
const CHROME = process.env.GLANCE_VISUAL_CHROME || "/usr/bin/google-chrome";
const PAGE = process.env.GLANCE_PERFORMANCE_PAGE || "tech";
const DASHBOARD = process.env.GLANCE_PERFORMANCE_DASHBOARD || "admin";
const USERNAME = process.env.GLANCE_PERFORMANCE_USERNAME;
const PASSWORD = process.env.GLANCE_PERFORMANCE_PASSWORD;
const VIEWPORT = { width: 1600, height: 1000 };

async function authenticate(page) {
  const response = await page.goto(`${BASE_URL}/login`, {
    waitUntil: "domcontentloaded",
    timeout: 30000,
  });
  if (!response || !response.ok()) {
    throw new Error(`Unable to load login page: HTTP ${response?.status() ?? "none"}`);
  }

  await page.locator("#login-container").waitFor({ state: "visible", timeout: 10000 });
  await page.locator("#username").fill(USERNAME);
  await page.locator("#password").fill(PASSWORD);

  await Promise.all([
    page.waitForURL(url => !url.pathname.endsWith("/login"), { timeout: 10000 }),
    page.locator("#login-button").click(),
  ]);
}

async function openPerformancePage(page) {
  const route = PAGE === "home" && DASHBOARD === ""
    ? "/"
    : `/${encodeURIComponent(DASHBOARD)}/${encodeURIComponent(PAGE)}`;

  const response = await page.goto(`${BASE_URL}${route}`, {
    waitUntil: "domcontentloaded",
    timeout: 30000,
  });
  if (!response || !response.ok()) {
    throw new Error(`HTTP ${response?.status() ?? "none"} while loading ${route}`);
  }

  console.log(`Performance page route: ${route}`);

  await page.locator('#page.content-ready[aria-busy="false"]').waitFor({
    state: "attached",
    timeout: 30000,
  });

  const content = page.locator("#page-content");
  await content.waitFor({ state: "attached", timeout: 10000 });
  if ((await content.innerHTML()).trim().length === 0) {
    throw new Error("Performance page content is empty after initialization");
  }

  return route;
}

async function main() {
  if (!USERNAME || !PASSWORD) {
    throw new Error("GLANCE_PERFORMANCE_USERNAME and GLANCE_PERFORMANCE_PASSWORD are required");
  }

  const browserErrors = [];
  let browser;

  try {
    browser = await chromium.launch({
      executablePath: CHROME,
      headless: true,
    });

    const context = await browser.newContext({
      viewport: VIEWPORT,
      deviceScaleFactor: 1,
    });
    const page = await context.newPage();
    page.on("pageerror", error => browserErrors.push(error.message));

    await authenticate(page);
    const route = await openPerformancePage(page);

    console.log(`Performance browser ready: ${BASE_URL}${route}`);
    process.stdout.write("PERFORMANCE_BROWSER_READY\n");

    await new Promise(resolve => {
      process.once("SIGTERM", resolve);
      process.once("SIGINT", resolve);
    });

    if (browserErrors.length > 0) {
      throw new Error(`Browser error(s): ${browserErrors.join(" | ")}`);
    }
  } finally {
    if (browser) await browser.close();
  }
}

main().catch(error => {
  console.error(`Performance browser failed: ${error.message}`);
  process.exitCode = 1;
});
