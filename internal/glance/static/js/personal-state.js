import { frontendDiagnosticError } from "./diagnostics.js";

function localStorageKey(namespace, id) {
    return `${namespace}-${id}`;
}

function readLocalValue(key, fallback, validate, sanitizeErrors = false) {
    const raw = localStorage.getItem(key);
    if (raw === null) return { found: false, value: fallback, raw: null };

    try {
        const value = JSON.parse(raw);
        if (!validate(value)) return { found: false, value: fallback, raw: null };
        return { found: true, value, raw };
    } catch (error) {
        const detail = sanitizeErrors ? new Error("Search history parse failed") : error;
        console.error(`Failed to parse local ${key} state`, detail);
        frontendDiagnosticError("personal_state_local_parse_error", detail);
        return { found: false, value: fallback, raw: null };
    }
}

function personalStateEndpoint(namespace, id) {
    const serverID = id || "default";
    return `${pageData.baseURL}/api/personal-state/${encodeURIComponent(namespace)}/${encodeURIComponent(serverID)}`;
}

async function putPersonalState(endpoint, serialized, keepalive = false) {
    const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: serialized,
        keepalive,
    });

    if (!response.ok) {
        throw new Error(`personal state write failed with HTTP ${response.status}`);
    }
}

export function createPersonalState(namespace, id, options = {}) {
    const key = localStorageKey(namespace, id);
    // Search explicitly chooses a storage mode. Server-only state must never
    // migrate/fall back to a shared browser value on logout or server failure.
    const serverOnly = options.storage === "server";
    const enabled = options.storage !== "browser" && pageData.personalStateEnabled === true;
    const endpoint = personalStateEndpoint(namespace, id);
    let writeQueue = Promise.resolve();

    return {
        async load(fallback, validate = () => true) {
            let local = { found: false, value: fallback, raw: null };
            try {
                if (!serverOnly) local = readLocalValue(key, fallback, validate, namespace === "search");
                if (serverOnly && !enabled) throw new Error("server personal state is not enabled");
            } catch (error) {
                options.onError?.();
                frontendDiagnosticError("personal_state_load_error", new Error("Personal state storage unavailable"));
                return fallback;
            }
            if (!enabled) return local.value;

            try {
                const response = await fetch(endpoint, {
                    headers: { "Accept": "application/json" },
                });

                if (response.ok) {
                    const value = await response.json();
                    if (!validate(value)) throw new Error("personal state response has an invalid shape");
                    return value;
                }
                if (response.status !== 404) {
                    throw new Error(`personal state read failed with HTTP ${response.status}`);
                }

                if (local.found && local.raw !== null) {
                    await putPersonalState(endpoint, local.raw);
                    localStorage.removeItem(key);
                }
                return local.value;
            } catch (error) {
                const detail = namespace === "search" ? new Error("Search history read failed") : error;
                console.error(`Failed to load server-side ${namespace} state`, detail);
                frontendDiagnosticError("personal_state_load_error", detail);
                options.onError?.();
                return local.value;
            }
        },

        save(value) {
            const serialized = JSON.stringify(value);
            if (!enabled) {
                try {
                    if (serverOnly) throw new Error("server personal state is not enabled");
                    localStorage.setItem(key, serialized);
                } catch (error) {
                    options.onError?.();
                    frontendDiagnosticError("personal_state_save_error", new Error("Personal state storage unavailable"));
                }
                return;
            }

            writeQueue = writeQueue
                .then(() => putPersonalState(endpoint, serialized, options.keepalive === true))
                .catch(error => {
                    const detail = namespace === "search" ? new Error("Search history write failed") : error;
                    console.error(`Failed to save server-side ${namespace} state`, detail);
                    frontendDiagnosticError("personal_state_save_error", detail);
                    options.onError?.();
                });
        },
    };
}
