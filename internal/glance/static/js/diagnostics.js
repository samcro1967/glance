const frontendDiagnosticsEnabled = pageData.frontendDiagnostics === true;
const frontendDiagnosticsBuffer = [];
const frontendDiagnosticsSession = frontendDiagnosticsEnabled
    ? (
        typeof crypto !== "undefined" &&
        typeof crypto.randomUUID === "function"
            ? crypto.randomUUID()
            : `${Date.now()}-${Math.random().toString(36).slice(2)}`
    ).slice(0, 64)
    : "";
let frontendDiagnosticsSequence = 0;
let frontendDiagnosticsFlushTimer = null;
let frontendDiagnosticsFlushInProgress = false;
let frontendDiagnosticsImmediateFlushPending = false;

export function frontendDiagnostic(event, fields = {}, flush = false) {
    if (!frontendDiagnosticsEnabled) {
        return;
    }

    const diagnostic = {
        event,
        page: pageData.slug || "",
        session: frontendDiagnosticsSession,
        sequence: ++frontendDiagnosticsSequence,
    };

    if (fields.widget !== undefined) {
        diagnostic.widget = String(fields.widget);
    }

    if (fields.detail !== undefined && fields.detail !== "") {
        diagnostic.detail = String(fields.detail).slice(0, 256);
    }

    if (fields.elapsedMS !== undefined) {
        diagnostic.elapsed_ms = Math.max(0, fields.elapsedMS);
    }

    if (fields.status !== undefined) {
        diagnostic.status = fields.status;
    }

    if (fields.length !== undefined) {
        diagnostic.length = Math.max(0, fields.length);
    }

    if (fields.state !== undefined) {
        diagnostic.state = fields.state;
    }

    if (fields.metrics !== undefined) {
        diagnostic.metrics = fields.metrics;
    }

    frontendDiagnosticsBuffer.push(diagnostic);

    if (flush) {
        if (frontendDiagnosticsFlushInProgress) {
            frontendDiagnosticsImmediateFlushPending = true;
        } else {
            flushFrontendDiagnostics();
        }
        return;
    }

    if (frontendDiagnosticsFlushTimer === null) {
        frontendDiagnosticsFlushTimer = setTimeout(
            flushFrontendDiagnostics,
            1000
        );
    }
}

function flushFrontendDiagnostics() {
    if (
        !frontendDiagnosticsEnabled ||
        frontendDiagnosticsFlushInProgress ||
        frontendDiagnosticsBuffer.length === 0
    ) {
        return;
    }

    if (frontendDiagnosticsFlushTimer !== null) {
        clearTimeout(frontendDiagnosticsFlushTimer);
        frontendDiagnosticsFlushTimer = null;
    }

    const events = frontendDiagnosticsBuffer.splice(0, 50);
    frontendDiagnosticsFlushInProgress = true;

    fetch(`${pageData.baseURL}/api/frontend-diagnostics`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({ events }),
        keepalive: true,
    })
        .catch(() => {
            // Diagnostics must never interfere with normal page behavior.
        })
        .finally(() => {
            frontendDiagnosticsFlushInProgress = false;

            if (frontendDiagnosticsBuffer.length === 0) {
                frontendDiagnosticsImmediateFlushPending = false;
                return;
            }

            if (frontendDiagnosticsImmediateFlushPending) {
                frontendDiagnosticsImmediateFlushPending = false;
                flushFrontendDiagnostics();
                return;
            }

            frontendDiagnosticsFlushTimer = setTimeout(
                flushFrontendDiagnostics,
                1000
            );
        });
}

export function frontendDiagnosticErrorDetail(value) {
    if (value instanceof Error) {
        return `${value.name}: ${value.message}`.slice(0, 256);
    }

    return String(value).slice(0, 256);
}

export function frontendDiagnosticError(event, error, fields = {}, flush = true) {
    frontendDiagnostic(event, {
        ...fields,
        detail: frontendDiagnosticErrorDetail(error),
    }, flush);
}

function frontendDiagnosticLongTaskCapture(durationMS = 30000) {
    if (!frontendDiagnosticsEnabled) {
        return;
    }

    if (
        typeof PerformanceObserver === "undefined" ||
        !PerformanceObserver.supportedEntryTypes?.includes("longtask")
    ) {
        frontendDiagnostic("long_task_capture_unsupported");
        return;
    }

    let count = 0;
    let totalDuration = 0;
    let maxDuration = 0;
    const startedAt = performance.now();

    const observer = new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
            count++;
            totalDuration += entry.duration;
            maxDuration = Math.max(maxDuration, entry.duration);
        }
    });

    try {
        observer.observe({ type: "longtask", buffered: true });
    } catch (error) {
        frontendDiagnosticError(
            "long_task_capture_error",
            error
        );
        return;
    }

    frontendDiagnostic("long_task_capture_start", {
        metrics: {
            duration_ms: durationMS,
        },
    });

    setTimeout(() => {
        observer.disconnect();

        frontendDiagnostic("long_task_capture_complete", {
            elapsedMS: performance.now() - startedAt,
            metrics: {
                count,
                total_duration_ms: totalDuration,
                max_duration_ms: maxDuration,
            },
        }, true);
    }, durationMS);
}

function frontendDiagnosticPerformanceSnapshot(reason) {
    if (!frontendDiagnosticsEnabled) {
        return;
    }

    const resources = performance.getEntriesByType("resource");
    const navigation = performance.getEntriesByType("navigation")[0];

    frontendDiagnostic("performance_snapshot", {
        detail: `reason=${reason}`,
        metrics: {
            elements: document.getElementsByTagName("*").length,
            widgets: document.querySelectorAll("[data-widget-id]").length,
            images: document.images.length,
            tables: document.getElementsByTagName("table").length,
            resources: resources.length,
        },
    }, true);

    if (navigation) {
        frontendDiagnostic("navigation_snapshot", {
            detail: `type=${navigation.type}`,
            elapsedMS: navigation.duration,
            metrics: {
                dom_interactive_ms: navigation.domInteractive,
                dom_content_loaded_ms: navigation.domContentLoadedEventEnd,
                load_ms: navigation.loadEventEnd,
                transfer_bytes: navigation.transferSize,
                encoded_bytes: navigation.encodedBodySize,
                decoded_bytes: navigation.decodedBodySize,
            },
        });
    }

    if (resources.length > 0) {
        let transferBytes = 0;
        let encodedBytes = 0;
        let decodedBytes = 0;
        let totalDuration = 0;
        let slowest = null;

        for (const resource of resources) {
            transferBytes += resource.transferSize || 0;
            encodedBytes += resource.encodedBodySize || 0;
            decodedBytes += resource.decodedBodySize || 0;
            totalDuration += resource.duration || 0;

            if (slowest === null || resource.duration > slowest.duration) {
                slowest = resource;
            }
        }

        frontendDiagnostic("resource_snapshot", {
            detail: `slowest=${slowest?.name?.slice(0, 240) ?? ""}`,
            metrics: {
                count: resources.length,
                transfer_bytes: transferBytes,
                encoded_bytes: encodedBytes,
                decoded_bytes: decodedBytes,
                total_duration_ms: totalDuration,
                slowest_ms: slowest?.duration ?? 0,
            },
        });
    }

    if (
        performance.memory &&
        Number.isFinite(performance.memory.usedJSHeapSize)
    ) {
        frontendDiagnostic("memory_snapshot", {
            metrics: {
                used_js_heap_bytes: performance.memory.usedJSHeapSize,
                total_js_heap_bytes: performance.memory.totalJSHeapSize,
                js_heap_limit_bytes: performance.memory.jsHeapSizeLimit,
            },
        });
    }
}
export function captureFrontendPerformanceSnapshot(reason = "manual") {
    frontendDiagnosticPerformanceSnapshot(reason);
}

function setupFrontendDiagnosticsLifecycle() {
    if (!frontendDiagnosticsEnabled) {
        return;
    }

    window.addEventListener("error", (event) => {
        frontendDiagnostic("window_error", {
            detail: frontendDiagnosticErrorDetail(
                event.error ?? event.message ?? "unknown error"
            ),
        }, true);
    });

    window.addEventListener("unhandledrejection", (event) => {
        frontendDiagnostic("unhandled_rejection", {
            detail: frontendDiagnosticErrorDetail(event.reason),
        }, true);
    });

    window.addEventListener("pageshow", (event) => {
        frontendDiagnostic("page_show", {
            detail: `persisted=${event.persisted} visibility=${document.visibilityState} online=${navigator.onLine}`,
        }, true);
    });

    document.addEventListener("visibilitychange", () => {
        frontendDiagnostic("visibility_change", {
            detail: `visibility=${document.visibilityState}`,
        });
    });

    window.addEventListener("online", () => {
        frontendDiagnostic("network_online");
    });

    window.addEventListener("offline", () => {
        frontendDiagnostic("network_offline");
    });

    window.addEventListener("load", () => {
        setTimeout(() => {
            frontendDiagnosticPerformanceSnapshot("window_load");
            frontendDiagnosticLongTaskCapture();
        }, 0);
    }, { once: true });
}

setupFrontendDiagnosticsLifecycle();

export function runFrontendDiagnosticStage(name, callback) {
    if (!frontendDiagnosticsEnabled) {
        return callback();
    }

    const started = performance.now();

    frontendDiagnostic("page_initialize_start", {
        detail: name,
    }, true);

    try {
        const result = callback();

        frontendDiagnostic("page_initialize_complete", {
            detail: name,
            elapsedMS: performance.now() - started,
        });

        return result;
    } catch (error) {
        frontendDiagnostic("page_initialize_error", {
            detail: `${name}: ${frontendDiagnosticErrorDetail(error)}`,
            elapsedMS: performance.now() - started,
        }, true);

        throw error;
    }
}

export async function runFrontendDiagnosticAsyncStage(name, callback) {
    if (!frontendDiagnosticsEnabled) {
        return callback();
    }

    const started = performance.now();

    frontendDiagnostic("page_initialize_start", {
        detail: name,
    }, true);

    try {
        const result = await callback();

        frontendDiagnostic("page_initialize_complete", {
            detail: name,
            elapsedMS: performance.now() - started,
        });

        return result;
    } catch (error) {
        frontendDiagnostic("page_initialize_error", {
            detail: `${name}: ${frontendDiagnosticErrorDetail(error)}`,
            elapsedMS: performance.now() - started,
        }, true);

        throw error;
    }
}
