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

