#!/usr/bin/env node
"use strict";

const fs = require("node:fs");

function reportLighthouse(result, output = console.log) {
  if (!result || typeof result !== "object" || !result.categories || !result.audits) {
    throw new Error("Invalid Lighthouse report: missing categories or audits");
  }

  for (const [id, category] of Object.entries(result.categories)) {
    const score = typeof category.score === "number"
      ? Math.round(category.score * 100)
      : "N/A";
    output(`${id}: ${score}`);
  }

  output("");
  output("=== LIGHTHOUSE PERFORMANCE METRICS ===");

  const metrics = [
    ["first-contentful-paint", "FCP"],
    ["largest-contentful-paint", "LCP"],
    ["speed-index", "Speed Index"],
    ["total-blocking-time", "TBT"],
    ["cumulative-layout-shift", "CLS"]
  ];

  for (const [id, label] of metrics) {
    const audit = result.audits[id];
    if (!audit || typeof audit.numericValue !== "number") {
      output(`${label}: unavailable`);
      continue;
    }

    const value = id === "cumulative-layout-shift"
      ? audit.numericValue.toFixed(4)
      : `${Math.round(audit.numericValue)} ms`;

    const score = typeof audit.score === "number"
      ? ` (score ${Math.round(audit.score * 100)})`
      : "";

    output(`${label}: ${value}${score}`);
  }

  const layoutShifts = result.audits["layout-shifts"];
  if (layoutShifts) {
    output("");
    output("=== LAYOUT SHIFT DIAGNOSTICS ===");

    if (layoutShifts.displayValue) {
      output(layoutShifts.displayValue);
    }

    const items = layoutShifts.details?.items || [];
    if (items.length === 0) {
      output("No detailed layout-shift items available.");
    }

    for (const item of items.slice(0, 10)) {
      const node = item.node;
      const description = node?.nodeLabel || node?.snippet || node?.selector || "unknown element";
      const score = item.score ?? item.value ?? "N/A";
      output(`- ${description}: ${score}`);
    }
  }

  const accessibility = result.categories.accessibility;
  if (accessibility?.auditRefs) {
    const failed = Object.values(result.audits)
      .filter(a => a.score !== null && a.score < 1)
      .filter(a => a.details || ["button-name", "color-contrast", "target-size"].includes(a.id))
      .filter(a => accessibility.auditRefs.some(ref => ref.id === a.id));

    if (failed.length) {
      output("");
      output(`accessibility findings: ${failed.map(a => a.id).join(", ")}`);
    }
  }
}

if (require.main === module) {
  try {
    const filename = process.argv[2];
    if (!filename) throw new Error("Usage: node scripts/report_lighthouse.js <report.json>");
    reportLighthouse(JSON.parse(fs.readFileSync(filename, "utf8")));
  } catch (error) {
    console.error(`Lighthouse reporting failed: ${error.message}`);
    process.exitCode = 1;
  }
}

module.exports = { reportLighthouse };
