function localStorageKey(namespace, id) {
    return `${namespace}-${id}`;
}

function readLocalValue(key, fallback, validate) {
    const raw = localStorage.getItem(key);
    if (raw === null) return { found: false, value: fallback, raw: null };

    try {
        const value = JSON.parse(raw);
        if (!validate(value)) return { found: false, value: fallback, raw: null };
        return { found: true, value, raw };
    } catch (error) {
        console.error(`Failed to parse local ${key} state`, error);
        return { found: false, value: fallback, raw: null };
    }
}

function personalStateEndpoint(namespace, id) {
    const serverID = id || "default";
    return `${pageData.baseURL}/api/personal-state/${encodeURIComponent(namespace)}/${encodeURIComponent(serverID)}`;
}

async function putPersonalState(endpoint, serialized) {
    const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: serialized,
    });

    if (!response.ok) {
        throw new Error(`personal state write failed with HTTP ${response.status}`);
    }
}

export function createPersonalState(namespace, id) {
    const key = localStorageKey(namespace, id);
    const enabled = pageData.personalStateEnabled === true;
    const endpoint = personalStateEndpoint(namespace, id);
    let writeQueue = Promise.resolve();

    return {
        async load(fallback, validate = () => true) {
            const local = readLocalValue(key, fallback, validate);
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
                console.error(`Failed to load server-side ${namespace} state`, error);
                return local.value;
            }
        },

        save(value) {
            const serialized = JSON.stringify(value);
            if (!enabled) {
                localStorage.setItem(key, serialized);
                return;
            }

            writeQueue = writeQueue
                .then(() => putPersonalState(endpoint, serialized))
                .catch(error => {
                    console.error(`Failed to save server-side ${namespace} state`, error);
                });
        },
    };
}
