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

async function captureBackendDiagnostics(page) {
  const response = await page.request.get(`${BASE_URL}/api/diagnostics`);
  if (!response.ok()) {
    throw new Error(`Unable to load backend diagnostics: HTTP ${response.status()}`);
  }

  const diagnostics = await response.json();
  const rendering = diagnostics.rendering;
  const outbound = diagnostics.outbound_http;

  if (!rendering || !outbound) {
    throw new Error("Backend diagnostics response is missing performance sections");
  }

  const widgets = Array.isArray(diagnostics.widgets) ? diagnostics.widgets : [];
  const widgetActivity = (widget) => ({
    id: widget.id,
    type: widget.type,
    title: widget.title || "",
    degraded: widget.degraded,
    failure_class: widget.failure_class || "",
    failure_cause: widget.failure_cause || "",
    last_duration_ms: widget.last_duration_ms,
    refresh_started_at: widget.refresh_started_at || null,
    last_attempt: widget.last_attempt || null,
    attempts: widget.attempts,
    successes: widget.successes,
    failures: widget.failures,
  });

  const refreshing = widgets
    .filter((widget) => widget.refresh_started_at)
    .sort((a, b) => a.id - b.id);

  const refreshingIDs = new Set(refreshing.map((widget) => widget.id));
  const slowestRecent = widgets
    .filter((widget) => !refreshingIDs.has(widget.id) && widget.last_attempt)
    .sort((a, b) =>
      (b.last_duration_ms - a.last_duration_ms) ||
      (a.id - b.id))
    .slice(0, 5);

  console.log("=== CORRELATED BACKEND DIAGNOSTICS ===");
  console.log(JSON.stringify({
    generated_at: diagnostics.generated_at,
    refresh_widgets: diagnostics.refresh_widgets,
    refreshing_widgets: diagnostics.refreshing_widgets,
    degraded_widgets: diagnostics.degraded_widgets,
    total_attempts: diagnostics.total_attempts,
    total_successes: diagnostics.total_successes,
    total_failures: diagnostics.total_failures,
    total_lock_skips: diagnostics.total_lock_skips,
    widget_refresh_activity: {
      refreshing: refreshing.map(widgetActivity),
      slowest_recent: slowestRecent.map(widgetActivity),
    },
    rendering: {
      widget_calls: rendering.widget_calls,
      widget_snapshot_hits: rendering.widget_snapshot_hits,
      widget_refresh_lock_waits: rendering.widget_refresh_lock_waits,
      widget_refresh_lock_wait_total_ms: rendering.widget_refresh_lock_wait_total_ms,
      widget_refresh_lock_wait_max_ms: rendering.widget_refresh_lock_wait_max_ms,
      widget_refresh_lock_wait_max_widget: rendering.widget_refresh_lock_wait_max_widget,
      widget_renders: rendering.widget_renders,
      widget_render_total_ms: rendering.widget_render_total_ms,
      widget_render_max_ms: rendering.widget_render_max_ms,
      widget_render_max_widget: rendering.widget_render_max_widget,
      page_template_executions: rendering.page_template_executions,
      page_template_failures: rendering.page_template_failures,
      page_lock_wait_total_ms: rendering.page_lock_wait_total_ms,
      page_lock_wait_average_ms: rendering.page_lock_wait_average_ms,
      page_lock_wait_max_ms: rendering.page_lock_wait_max_ms,
      page_template_execution_total_ms: rendering.page_template_execution_total_ms,
      page_template_execution_average_ms: rendering.page_template_execution_average_ms,
      page_template_execution_max_ms: rendering.page_template_execution_max_ms,
    },
    outbound_http: {
      exchanges: outbound.exchanges,
      transport_errors: outbound.transport_errors,
      average_round_trip_duration_ms: outbound.average_round_trip_duration_ms,
      max_round_trip_duration_ms: outbound.max_round_trip_duration_ms,
    },
  }, null, 2));
  process.stdout.write("PERFORMANCE_BACKEND_DIAGNOSTICS_COMPLETE\n");
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

    await new Promise((resolve, reject) => {
      let captureInFlight = false;

      process.on("SIGUSR1", async () => {
        if (captureInFlight) return;
        captureInFlight = true;

        try {
          await captureBackendDiagnostics(page);
        } catch (error) {
          reject(error);
        } finally {
          captureInFlight = false;
        }
      });

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
