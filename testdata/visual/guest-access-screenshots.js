// Deterministic guest login and authorized dashboard screenshots.
// Run only against the Makefile-managed guest-access test instance.
'use strict';
const { chromium } = require('playwright');
const fs = require('node:fs');
const path = require('node:path');
const base = (process.env.GLANCE_VISUAL_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
const output = path.join(__dirname, 'screenshots', 'guest-access');
const SCREENSHOTS = Object.freeze(['guest-login-desktop.png', 'guest-dashboard-desktop.png', 'guest-login-mobile.png']);
async function captureGuestScreenshots(browser, destination = output, selected = SCREENSHOTS) {
  const requested = new Set(selected);
  const unknown = [...requested].filter(name => !SCREENSHOTS.includes(name));
  if (unknown.length) throw new Error("Unknown guest screenshot: " + unknown.join(", "));
  if (requested.size === 0) return;
  fs.mkdirSync(destination, { recursive: true });

  if (requested.has("guest-login-desktop.png") || requested.has("guest-dashboard-desktop.png")) {
    const page = await browser.newPage({ viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 1 });
    try {
      const login = await page.goto(base + "/login");
      if (!login || login.status() !== 200) throw new Error("Guest login page unavailable");
      const button = page.getByRole("button", { name: "CONTINUE AS GUEST" });
      await button.waitFor({ state: "visible" });
      if (requested.has("guest-login-desktop.png")) {
        await page.screenshot({ path: path.join(destination, "guest-login-desktop.png"), fullPage: true, animations: "disabled" });
      }
      await button.click();
      await page.waitForURL(url => !url.pathname.endsWith("/login"));
      await page.getByText("Public guest dashboard fixture.").waitFor({ state: "visible" });
      const privateNavigation = page.locator("a[href=\"/private\"], a[href=\"/default/private\"]");
      if (await privateNavigation.count() !== 0) throw new Error("Restricted guest page appeared in dashboard navigation");
      if (requested.has("guest-dashboard-desktop.png")) {
        await page.screenshot({ path: path.join(destination, "guest-dashboard-desktop.png"), fullPage: true, animations: "disabled" });
      }
      const denied = await page.goto(base + "/default/private");
      if (!denied || denied.status() !== 404) throw new Error("Restricted guest page returned " + (denied ? denied.status() : "no response"));
    } finally {
      await page.close();
    }
  }

  if (requested.has("guest-login-mobile.png")) {
    const mobile = await browser.newPage({ viewport: { width: 430, height: 900 }, deviceScaleFactor: 1 });
    try {
      const login = await mobile.goto(base + "/login");
      if (!login || login.status() !== 200) throw new Error("Mobile guest login page unavailable");
      await mobile.getByRole("button", { name: "CONTINUE AS GUEST" }).waitFor({ state: "visible" });
      await mobile.screenshot({ path: path.join(destination, "guest-login-mobile.png"), fullPage: true, animations: "disabled" });
    } finally {
      await mobile.close();
    }
  }

  console.log("Guest screenshots staged in " + destination);
}

module.exports = { captureGuestScreenshots, SCREENSHOTS };

if (require.main === module) {
  (async () => {
    const browser = await chromium.launch({
      headless: true,
      executablePath: process.env.GLANCE_VISUAL_CHROME || '/usr/bin/google-chrome'
    });
    try {
      await captureGuestScreenshots(browser);
    } finally {
      await browser.close();
    }
  })().catch(err => {
    console.error(err);
    process.exitCode = 1;
  });
}
