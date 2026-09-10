import "./vendor/chartjs/chart.umd.min.js";
import { frontendDiagnosticError } from "./diagnostics.js";
import { getPresentationConfig } from "./presentation-config.js";

const initializedCharts = new WeakMap();

function cssColor(name) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function chartColors() {
    return {
        primary: cssColor("--color-primary"),
        positive: cssColor("--color-positive"),
        negative: cssColor("--color-negative"),
        text: cssColor("--color-text-base"),
        highlight: cssColor("--color-text-highlight"),
        subdue: cssColor("--color-text-subdue"),
        grid: cssColor("--color-graph-gridlines"),
        background: cssColor("--color-widget-background")
    };
}

function chartPalette(colors) {
    return [colors.primary, colors.positive, colors.negative, colors.highlight, colors.subdue];
}

function chartConfig(element) {
    const name = element.dataset.glanceChart;
    if (!name) throw new Error("Glance chart requires a configuration name");
    const config = getPresentationConfig(element).charts?.[name];
    if (config === undefined) throw new Error(`Unknown Glance chart configuration: ${name}`);
    return config;
}

function parseChartData(element) {
    const script = element.querySelector(":scope > script[type=\"application/json\"]");
    if (script === null) throw new Error("Glance chart requires application/json data");
    return JSON.parse(script.textContent);
}

function createCanvas(element) {
    const canvas = document.createElement("canvas");
    canvas.setAttribute("role", "img");
    canvas.setAttribute("aria-label", element.dataset.glanceChartLabel || "Chart");
    element.appendChild(canvas);
    return canvas;
}

function lineDatasets(data, colors, fill) {
    const palette = chartPalette(colors);
    return (data.series || []).map((series, index) => ({
        label: series.label || `Series ${index + 1}`,
        data: series.values || [],
        borderColor: palette[index % palette.length],
        backgroundColor: palette[index % palette.length],
        borderWidth: 2,
        pointRadius: 2,
        pointHoverRadius: 4,
        tension: 0.25,
        fill
    }));
}

function standardOptions(colors, config) {
    return {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { intersect: false, mode: "index" },
        plugins: {
            legend: {
                display: config.legend !== false,
                labels: { color: colors.text, boxWidth: 10, boxHeight: 10 }
            }
        },
        scales: {
            x: {
                stacked: config.stacked === true,
                ticks: { color: colors.subdue },
                grid: { color: colors.grid }
            },
            y: {
                stacked: config.stacked === true,
                beginAtZero: config.min === undefined,
                min: config.min,
                max: config.max,
                ticks: {
                    color: colors.subdue,
                    callback: config.unit ? (value) => `${value}${config.unit}` : undefined
                },
                grid: { color: colors.grid }
            }
        }
    };
}

function buildStandardChart(type, data, config, colors) {
    const chartType = type === "area" ? "line" : type;
    const fill = type === "area";
    const datasets = chartType === "bar"
        ? lineDatasets(data, colors, false).map((dataset) => ({ ...dataset, tension: 0, pointRadius: 0 }))
        : lineDatasets(data, colors, fill);
    return {
        type: chartType,
        data: { labels: data.labels || [], datasets },
        options: standardOptions(colors, config)
    };
}

function buildCircularChart(type, data, config, colors) {
    const palette = chartPalette(colors);
    return {
        type,
        data: {
            labels: data.labels || [],
            datasets: [{
                data: data.values || [],
                backgroundColor: (data.values || []).map((_, index) => palette[index % palette.length]),
                borderColor: colors.background,
                borderWidth: 2
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            plugins: {
                legend: {
                    display: config.legend !== false,
                    labels: { color: colors.text }
                }
            }
        }
    };
}

function gaugeValue(data, config) {
    const min = Number(config.min ?? 0);
    const max = Number(config.max ?? 100);
    return Math.min(max, Math.max(min, Number(data.value ?? min)));
}

function buildGauge(data, config, colors) {
    const min = Number(config.min ?? 0);
    const max = Number(config.max ?? 100);
    const value = gaugeValue(data, config);
    return {
        type: "doughnut",
        data: {
            datasets: [{
                data: [value - min, Math.max(0, max - value)],
                backgroundColor: [colors.primary, colors.grid],
                borderWidth: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            rotation: -90,
            circumference: 180,
            cutout: "72%",
            plugins: { legend: { display: false }, tooltip: { enabled: false } }
        }
    };
}

function buildSparkline(data, colors) {
    return {
        type: "line",
        data: {
            labels: (data.values || []).map((_, index) => index),
            datasets: [{
                data: data.values || [],
                borderColor: colors.primary,
                backgroundColor: colors.primary,
                borderWidth: 2,
                pointRadius: 0,
                tension: 0.3,
                fill: false
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            plugins: { legend: { display: false }, tooltip: { enabled: false } },
            scales: { x: { display: false }, y: { display: false } },
            elements: { line: { borderJoinStyle: "round" } }
        }
    };
}

function buildChart(data, config, colors) {
    const type = config.type;
    if (type === "line" || type === "area" || type === "bar") {
        return buildStandardChart(type, data, config, colors);
    }
    if (type === "pie" || type === "doughnut") {
        return buildCircularChart(type, data, config, colors);
    }
    if (type === "gauge") return buildGauge(data, config, colors);
    if (type === "sparkline") return buildSparkline(data, colors);
    throw new Error(`Unsupported Glance chart type: ${type}`);
}

function clearChartArtifacts(element) {
    element.querySelector("canvas")?.remove();
    element.querySelector(".glance-chart-gauge-value")?.remove();
    element.querySelector(".glance-chart-gauge-label")?.remove();
}

function clearChartError(element) {
    element.classList.remove("glance-chart-failed");
    element.querySelector(":scope > .glance-state-error")?.remove();
}

function showChartError(element, error) {
    clearChartArtifacts(element);
    clearChartError(element);
    element.classList.add("glance-chart-failed");
    const message = document.createElement("div");
    message.className = "glance-state glance-state-error";
    message.textContent = "Unable to render chart.";
    element.appendChild(message);
    frontendDiagnosticError("presentation_chart_initialize_error", error);
    console.error("Failed to initialize Glance chart", error);
}

function setupChart(element) {
    if (initializedCharts.has(element)) return null;

    clearChartError(element);
    const config = chartConfig(element);
    const data = parseChartData(element);
    element.style.height = `${config.height || 180}px`;

    let canvas = null;
    let instance = null;
    try {
        if (config.type === "gauge") {
            const value = document.createElement("div");
            value.className = "glance-chart-gauge-value";
            value.textContent = `${gaugeValue(data, config)}${config.unit ?? ""}`;
            const label = document.createElement("div");
            label.className = "glance-chart-gauge-label";
            label.textContent = data.label ?? "";
            element.appendChild(value);
            element.appendChild(label);
        }

        canvas = createCanvas(element);
        instance = new Chart(canvas, buildChart(data, config, chartColors()));
        initializedCharts.set(element, { instance, config });
    } catch (error) {
        instance?.destroy();
        clearChartArtifacts(element);
        throw error;
    }

    return () => {
        const current = initializedCharts.get(element);
        if (current === undefined) return;
        initializedCharts.delete(element);
        current.instance.destroy();
        clearChartArtifacts(element);
        clearChartError(element);
        element.style.removeProperty("height");
    };
}

export function setupPresentationCharts(root = document) {
    const cleanupCallbacks = [];
    const elements = [];
    if (root.matches?.("[data-glance-chart]")) elements.push(root);
    elements.push(...root.querySelectorAll("[data-glance-chart]"));

    for (const element of elements) {
        try {
            const cleanup = setupChart(element);
            if (cleanup !== null) cleanupCallbacks.push(cleanup);
        } catch (error) {
            showChartError(element, error);
        }
    }
    return cleanupCallbacks;
}

export function refreshPresentationCharts() {
    for (const element of document.querySelectorAll("[data-glance-chart]")) {
        const current = initializedCharts.get(element);
        if (current === undefined) continue;
        try {
            const data = parseChartData(element);
            const config = chartConfig(element);
            const next = buildChart(data, config, chartColors());
            current.instance.data = next.data;
            current.instance.options = next.options;
            current.instance.update("none");
        } catch (error) {
            frontendDiagnosticError("presentation_chart_theme_refresh_error", error);
            console.error("Failed to refresh Glance chart theme", error);
        }
    }
}
