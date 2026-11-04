import assert from "node:assert/strict";
import test from "node:test";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { captureGuestScreenshots, SCREENSHOTS } = require("./guest-access-screenshots.js");

function mockBrowser() {
  const calls = [];
  const page = {
    goto: async url => { calls.push(["goto", url]); return { status: () => url.endsWith("/default/private") ? 404 : 200 }; },
    getByRole: () => ({ waitFor: async () => {}, click: async () => {} }),
    getByText: () => ({ waitFor: async () => {} }),
    waitForURL: async () => {},
    locator: () => ({ count: async () => 0 }),
    screenshot: async options => { calls.push(["screenshot", options.path]); },
    close: async () => { calls.push(["close"]); }
  };
  return { calls, browser: { newPage: async () => { calls.push(["newPage"]); return page; } } };
}

test("guest screenshot registry contains the three supported captures", () => {
  assert.deepEqual([...SCREENSHOTS], ["guest-login-desktop.png", "guest-dashboard-desktop.png", "guest-login-mobile.png"]);
});

test("selecting only mobile login avoids desktop authentication and capture", async () => {
  const { calls, browser } = mockBrowser();
  await captureGuestScreenshots(browser, "/tmp", ["guest-login-mobile.png"]);
  assert.equal(calls.filter(c => c[0] === "newPage").length, 1);
  assert.deepEqual(calls.filter(c => c[0] === "screenshot").map(c => c[1].split("/").at(-1)), ["guest-login-mobile.png"]);
  assert.equal(calls.filter(c => c[0] === "close").length, 1);
});

test("selecting only desktop dashboard still checks private-page denial", async () => {
  const { calls, browser } = mockBrowser();
  await captureGuestScreenshots(browser, "/tmp", ["guest-dashboard-desktop.png"]);
  assert.deepEqual(calls.filter(c => c[0] === "screenshot").map(c => c[1].split("/").at(-1)), ["guest-dashboard-desktop.png"]);
  assert.ok(calls.some(c => c[0] === "goto" && c[1].endsWith("/default/private")));
  assert.equal(calls.filter(c => c[0] === "close").length, 1);
});

test("unknown guest screenshot fails before creating browser pages", async () => {
  const { calls, browser } = mockBrowser();
  await assert.rejects(captureGuestScreenshots(browser, "/tmp", ["unknown.png"]), /Unknown guest screenshot/);
  assert.equal(calls.length, 0);
});

test("empty selection does not create browser pages", async () => {
  const { calls, browser } = mockBrowser();
  await captureGuestScreenshots(browser, "/tmp", []);
  assert.equal(calls.length, 0);
});
