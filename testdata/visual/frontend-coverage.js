// Path: testdata/visual/frontend-coverage.js
// File: frontend-coverage.js
/**
 * Native Chromium/V8 execution coverage support for Glance frontend checks.
 *
 * Ownership: development-only frontend regression infrastructure.
 * Measures named-function execution for Glance-owned JavaScript. This is not
 * statement, branch, or line coverage and intentionally has no pass threshold.
 */

"use strict";

const fs = require("fs");
const path = require("path");

const SOURCE_DIR = path.resolve(__dirname, "../../internal/glance/static/js");
const collectedCoverage = [];

function coverageOutputPath() {
  const argument = process.argv.find(value => value.startsWith("--coverage="));
  return argument ? argument.slice("--coverage=".length) : "";
}

async function startCoverage(page, outputPath) {
  if (!outputPath) return;
  await page.coverage.startJSCoverage({ resetOnNavigation: false });
}

async function collectCoverage(page, outputPath) {
  if (!outputPath) return;
  collectedCoverage.push(...await page.coverage.stopJSCoverage());
}

async function checkpointCoverage(page, outputPath) {
  if (!outputPath) return;
  await collectCoverage(page, outputPath);
  await startCoverage(page, outputPath);
}

function writeCoverage(outputPath) {
  if (!outputPath) return;
  fs.writeFileSync(outputPath, JSON.stringify(collectedCoverage));
}

function sourceInventory() {
  return fs.readdirSync(SOURCE_DIR)
    .filter(name => name.endsWith(".js"))
    .sort();
}

function mergeCoverage(coverageFiles) {
  const modules = new Map(
    sourceInventory().map(name => [name, new Map()])
  );

  for (const coverageFile of coverageFiles) {
    const entries = JSON.parse(fs.readFileSync(coverageFile, "utf8"));

    for (const entry of entries) {
      if (!/\/static\/[^/]+\/js\/[^/]+\.js$/.test(entry.url)) continue;

      const name = entry.url.split("/").pop();
      const functions = modules.get(name);
      if (!functions) continue;

      for (const fn of entry.functions) {
        if (!fn.functionName || fn.ranges.length === 0) continue;

        const outer = fn.ranges[0];
        const key = `${fn.functionName}:${outer.startOffset}:${outer.endOffset}`;
        const executed = fn.ranges.some(range => range.count > 0);
        functions.set(key, (functions.get(key) || false) || executed);
      }
    }
  }

  return modules;
}

function printReport(coverageFiles) {
  const modules = mergeCoverage(coverageFiles);
  let totalFunctions = 0;
  let executedFunctions = 0;

  console.log("=== FRONTEND JS NAMED-FUNCTION EXECUTION COVERAGE ===");

  for (const [name, functionMap] of modules) {
    const functions = [...functionMap.values()];
    const executed = functions.filter(Boolean).length;
    const total = functions.length;

    totalFunctions += total;
    executedFunctions += executed;

    if (total === 0) {
      console.log(name.padEnd(28), "NOT LOADED");
      continue;
    }

    const percent = `${(100 * executed / total).toFixed(1)}%`;
    console.log(
      name.padEnd(28),
      `${String(executed).padStart(3)}/${String(total).padEnd(3)}`,
      percent
    );
  }

  console.log("----------------------------------------");
  const percent = totalFunctions === 0
    ? "n/a"
    : `${(100 * executedFunctions / totalFunctions).toFixed(1)}%`;
  console.log(`Named functions: ${executedFunctions}/${totalFunctions} ${percent}`);
  console.log("Informational only: no coverage threshold is enforced.");
}

if (require.main === module) {
  const coverageFiles = process.argv.slice(2);

  if (coverageFiles.length === 0) {
    console.error("Usage: node frontend-coverage.js <coverage.json> [...]");
    process.exitCode = 2;
  } else {
    printReport(coverageFiles);
  }
}

module.exports = {
  coverageOutputPath,
  startCoverage,
  collectCoverage,
  checkpointCoverage,
  writeCoverage
};
