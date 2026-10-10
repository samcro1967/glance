"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { reportLighthouse } = require("./report_lighthouse");

function render(report) {
  const lines = [];
  reportLighthouse(report, line => lines.push(line));
  return lines.join("\n");
}

test("reports category scores and performance metrics", () => {
  const output = render({
    categories: {
      performance: { score: 0.74 },
      accessibility: { score: 0.96, auditRefs: [] }
    },
    audits: {
      "first-contentful-paint": { numericValue: 1200, score: 0.9 },
      "largest-contentful-paint": { numericValue: 2500, score: 0.75 },
      "speed-index": { numericValue: 1900, score: 0.85 },
      "total-blocking-time": { numericValue: 150, score: 0.95 },
      "cumulative-layout-shift": { numericValue: 0.207784, score: 0.4 }
    }
  });

  assert.match(output, /performance: 74/);
  assert.match(output, /FCP: 1200 ms/);
  assert.match(output, /LCP: 2500 ms/);
  assert.match(output, /Speed Index: 1900 ms/);
  assert.match(output, /TBT: 150 ms/);
  assert.match(output, /CLS: 0\.2078/);
});

test("handles missing performance metrics", () => {
  const output = render({
    categories: { performance: { score: 1 } },
    audits: {}
  });

  assert.match(output, /CLS: unavailable/);
  assert.match(output, /LCP: unavailable/);
});

test("reports layout shift diagnostics when present", () => {
  const output = render({
    categories: { performance: { score: 0.8 } },
    audits: {
      "layout-shifts": {
        displayValue: "2 layout shifts",
        details: {
          items: [
            {
              node: { nodeLabel: "Mobile navigation" },
              score: 0.15
            }
          ]
        }
      }
    }
  });

  assert.match(output, /LAYOUT SHIFT DIAGNOSTICS/);
  assert.match(output, /Mobile navigation: 0.15/);
});

test("reports accessibility findings", () => {
  const output = render({
    categories: {
      accessibility: {
        score: 0.96,
        auditRefs: [{ id: "target-size" }]
      }
    },
    audits: {
      "target-size": {
        id: "target-size",
        score: 0.5,
        details: { items: [] }
      }
    }
  });

  assert.match(output, /accessibility findings: target-size/);
});

test("rejects malformed reports", () => {
  assert.throws(
    () => render({ categories: {} }),
    /Invalid Lighthouse report/
  );
});
