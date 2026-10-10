let chartModulePromise;

export async function setupPresentation(root = document) {
    if (root === null) {
        return [];
    }

    const cleanupCallbacks = [];

    if (root.matches?.("[data-glance-table]") ||
        root.querySelector("[data-glance-table]")) {
        const { setupPresentationTables } = await import("./presentation-table.js");
        cleanupCallbacks.push(...setupPresentationTables(root));
    }

    if (root.matches?.("[data-glance-chart]") ||
        root.querySelector("[data-glance-chart]")) {
        chartModulePromise ??= import("./presentation-chart.js");
        const { setupPresentationCharts } = await chartModulePromise;
        cleanupCallbacks.push(...setupPresentationCharts(root));
    }

    return cleanupCallbacks;
}

export function refreshPresentationTheme() {
    if (chartModulePromise === undefined) {
        return;
    }

    chartModulePromise
        .then(({ refreshPresentationCharts }) => refreshPresentationCharts())
        .catch(error => {
            console.error("Failed to refresh Glance chart theme", error);
        });
}
