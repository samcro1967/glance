import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import test from "node:test";

const modulePath = path.resolve("internal/glance/static/js/presentation.js");

async function loadPresentation(chartSource, events) {
    let source = await fs.readFile(modulePath, "utf8");

    const diagnosticsSource = `
        export function frontendDiagnosticError(event, error) {
            globalThis.__presentationDiagnosticEvents.push({
                event,
                detail: error instanceof Error ? error.message : String(error),
            });
        }
    `;

    const moduleURL = text =>
        `data:text/javascript;base64,${Buffer.from(text).toString("base64")}`;

    source = source.replace(
        "./diagnostics.js",
        moduleURL(diagnosticsSource),
    );

    source = source.replace(
        `import("./presentation-chart.js")`,
        `import("${moduleURL(chartSource)}")`,
    );

    assert.ok(source.includes("data:text/javascript;base64,"));

    globalThis.__presentationDiagnosticEvents = events;

    return import(moduleURL(source));
}

async function withPresentation(chartSource, callback) {
    const previousEvents = globalThis.__presentationDiagnosticEvents;
    const previousConsoleError = console.error;
    const events = [];
    const errors = [];

    console.error = (...args) => errors.push(args);

    try {
        const presentation = await loadPresentation(chartSource, events);
        await callback(presentation, events, errors);
    } finally {
        console.error = previousConsoleError;
        globalThis.__presentationDiagnosticEvents = previousEvents;
    }
}

const chartRoot = {
    matches: () => false,
    querySelector: selector =>
        selector === "[data-glance-chart]" ? {} : null,
};

async function flushPromises() {
    await new Promise(resolve => setImmediate(resolve));
}

test("theme refresh without initialized charts remains a no-op", async () => {
    await withPresentation("", async (presentation, events, errors) => {
        presentation.refreshPresentationTheme();
        await flushPromises();
        assert.deepEqual(events, []);
        assert.deepEqual(errors, []);
    });
});

test("successful chart theme refresh emits no failure diagnostic", async () => {
    const chartSource = `
        export function setupPresentationCharts() { return []; }
        export function refreshPresentationCharts() {
            globalThis.__presentationRefreshCount =
                (globalThis.__presentationRefreshCount ?? 0) + 1;
        }
    `;

    const previousCount = globalThis.__presentationRefreshCount;
    globalThis.__presentationRefreshCount = 0;

    try {
        await withPresentation(chartSource, async (presentation, events, errors) => {
            await presentation.setupPresentation(chartRoot);
            presentation.refreshPresentationTheme();
            await flushPromises();

            assert.equal(globalThis.__presentationRefreshCount, 1);
            assert.deepEqual(events, []);
            assert.deepEqual(errors, []);
        });
    } finally {
        globalThis.__presentationRefreshCount = previousCount;
    }
});

test("rejected chart theme refresh reports shared diagnostics", async () => {
    const chartSource = `
        export function setupPresentationCharts() { return []; }
        export function refreshPresentationCharts() {
            throw new Error("theme refresh failed");
        }
    `;

    await withPresentation(chartSource, async (presentation, events, errors) => {
        await presentation.setupPresentation(chartRoot);
        presentation.refreshPresentationTheme();
        await flushPromises();

        assert.deepEqual(events, [{
            event: "presentation_chart_theme_refresh_error",
            detail: "theme refresh failed",
        }]);
        assert.equal(errors.length, 1);
        assert.equal(errors[0][0], "Failed to refresh Glance chart theme");
    });
});
