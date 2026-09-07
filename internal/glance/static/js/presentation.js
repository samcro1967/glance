import { setupPresentationTables } from "./presentation-table.js";
import { setupPresentationCharts, refreshPresentationCharts } from "./presentation-chart.js";

export function setupPresentation(root = document) {
    if (root === null) {
        return [];
    }

    return [
        ...setupPresentationTables(root),
        ...setupPresentationCharts(root)
    ];
}

export function refreshPresentationTheme() {
    refreshPresentationCharts();
}
