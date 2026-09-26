import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import test from "node:test";

const modulePath = path.resolve(
    "internal/glance/static/js/personal-state.js",
);

class MemoryStorage {
    constructor(initial = {}) {
        this.values = new Map(Object.entries(initial));
    }

    getItem(key) {
        return this.values.has(key) ? this.values.get(key) : null;
    }

    setItem(key, value) {
        this.values.set(key, String(value));
    }

    removeItem(key) {
        this.values.delete(key);
    }
}

function response(status, value) {
    return {
        ok: status >= 200 && status < 300,
        status,
        async json() {
            return value;
        },
    };
}

async function loadModule() {
    const source = await fs.readFile(modulePath, "utf8");
    const encoded = Buffer.from(source).toString("base64");
    return import(`data:text/javascript;base64,${encoded}`);
}

async function withEnvironment(
    { enabled = true, baseURL = "", storage = {}, fetchImpl },
    callback,
) {
    const previousPageData = globalThis.pageData;
    const previousLocalStorage = globalThis.localStorage;
    const previousFetch = globalThis.fetch;
    const previousConsoleError = console.error;
    const errors = [];

    globalThis.pageData = {
        baseURL,
        personalStateEnabled: enabled,
    };
    globalThis.localStorage = new MemoryStorage(storage);
    globalThis.fetch = fetchImpl ?? (async () => {
        throw new Error("unexpected fetch");
    });
    console.error = (...args) => errors.push(args);

    try {
        const module = await loadModule();
        await callback({
            createPersonalState: module.createPersonalState,
            localStorage: globalThis.localStorage,
            errors,
        });
    } finally {
        globalThis.pageData = previousPageData;
        globalThis.localStorage = previousLocalStorage;
        globalThis.fetch = previousFetch;
        console.error = previousConsoleError;
    }
}

async function flushWrites() {
    await new Promise(resolve => setTimeout(resolve, 0));
}

test("disabled mode keeps existing localStorage behavior", async () => {
    await withEnvironment(
        {
            enabled: false,
            storage: {
                "timer-important-dates": JSON.stringify([{ title: "Christmas" }]),
            },
        },
        async ({ createPersonalState, localStorage }) => {
            const state = createPersonalState("timer", "important-dates");
            const loaded = await state.load([], Array.isArray);

            assert.deepEqual(loaded, [{ title: "Christmas" }]);

            state.save([{ title: "New Year" }]);
            assert.equal(
                localStorage.getItem("timer-important-dates"),
                JSON.stringify([{ title: "New Year" }]),
            );
        },
    );
});

test("server state is authoritative when GET succeeds", async () => {
    const requests = [];

    await withEnvironment(
        {
            storage: {
                "todo-home": JSON.stringify([{ text: "local" }]),
            },
            fetchImpl: async (url, options = {}) => {
                requests.push({ url, options });
                return response(200, [{ text: "server" }]);
            },
        },
        async ({ createPersonalState, localStorage }) => {
            const state = createPersonalState("todo", "home");
            const loaded = await state.load([], Array.isArray);

            assert.deepEqual(loaded, [{ text: "server" }]);
            assert.equal(
                localStorage.getItem("todo-home"),
                JSON.stringify([{ text: "local" }]),
            );
            assert.equal(requests.length, 1);
            assert.equal(requests[0].options.method, undefined);
        },
    );
});

test("404 migrates valid local state and removes it only after successful POST", async () => {
    const requests = [];
    const localValue = JSON.stringify([{ title: "Christmas" }]);

    await withEnvironment(
        {
            baseURL: "/glance",
            storage: {
                "timer-important dates": localValue,
            },
            fetchImpl: async (url, options = {}) => {
                requests.push({ url, options });
                if (!options.method) return response(404);
                return response(204);
            },
        },
        async ({ createPersonalState, localStorage }) => {
            const state = createPersonalState("timer", "important dates");
            const loaded = await state.load([], Array.isArray);

            assert.deepEqual(loaded, [{ title: "Christmas" }]);
            assert.equal(localStorage.getItem("timer-important dates"), null);
            assert.equal(requests.length, 2);
            assert.equal(
                requests[0].url,
                "/glance/api/personal-state/timer/important%20dates",
            );
            assert.equal(requests[1].options.method, "POST");
            assert.equal(requests[1].options.body, localValue);
        },
    );
});

test("failed migration retains local state", async () => {
    const localValue = JSON.stringify([{ text: "keep me" }]);

    await withEnvironment(
        {
            storage: { "todo-home": localValue },
            fetchImpl: async (_url, options = {}) => {
                if (!options.method) return response(404);
                return response(500);
            },
        },
        async ({ createPersonalState, localStorage, errors }) => {
            const state = createPersonalState("todo", "home");
            const loaded = await state.load([], Array.isArray);

            assert.deepEqual(loaded, [{ text: "keep me" }]);
            assert.equal(localStorage.getItem("todo-home"), localValue);
            assert.equal(errors.length, 1);
        },
    );
});

test("non-404 server read failure falls back without attempting migration", async () => {
    let requestCount = 0;

    await withEnvironment(
        {
            storage: {
                "todo-home": JSON.stringify([{ text: "local" }]),
            },
            fetchImpl: async () => {
                requestCount++;
                return response(401);
            },
        },
        async ({ createPersonalState, errors }) => {
            const state = createPersonalState("todo", "home");
            const loaded = await state.load([], Array.isArray);

            assert.deepEqual(loaded, [{ text: "local" }]);
            assert.equal(requestCount, 1);
            assert.equal(errors.length, 1);
        },
    );
});

test("invalid server state shape falls back to local state", async () => {
    await withEnvironment(
        {
            storage: {
                "timer-important-dates": JSON.stringify([{ title: "local" }]),
            },
            fetchImpl: async () => response(200, { not: "an array" }),
        },
        async ({ createPersonalState, errors }) => {
            const state = createPersonalState("timer", "important-dates");
            const loaded = await state.load([], Array.isArray);

            assert.deepEqual(loaded, [{ title: "local" }]);
            assert.equal(errors.length, 1);
        },
    );
});

test("server writes are serialized in invocation order", async () => {
    const bodies = [];
    let releaseFirst;

    await withEnvironment(
        {
            fetchImpl: async (_url, options = {}) => {
                bodies.push(options.body);
                if (bodies.length === 1) {
                    await new Promise(resolve => {
                        releaseFirst = resolve;
                    });
                }
                return response(204);
            },
        },
        async ({ createPersonalState }) => {
            const state = createPersonalState("timer", "queue");

            state.save([{ title: "first" }]);
            state.save([{ title: "second" }]);

            await flushWrites();
            assert.deepEqual(bodies, [JSON.stringify([{ title: "first" }])]);

            releaseFirst();
            await flushWrites();
            await flushWrites();

            assert.deepEqual(bodies, [
                JSON.stringify([{ title: "first" }]),
                JSON.stringify([{ title: "second" }]),
            ]);
        },
    );
});

test("failed queued save does not prevent a later save", async () => {
    const bodies = [];

    await withEnvironment(
        {
            fetchImpl: async (_url, options = {}) => {
                bodies.push(options.body);
                return response(bodies.length === 1 ? 500 : 204);
            },
        },
        async ({ createPersonalState, errors }) => {
            const state = createPersonalState("todo", "queue");

            state.save([{ text: "first" }]);
            state.save([{ text: "second" }]);

            await flushWrites();
            await flushWrites();

            assert.deepEqual(bodies, [
                JSON.stringify([{ text: "first" }]),
                JSON.stringify([{ text: "second" }]),
            ]);
            assert.equal(errors.length, 1);
        },
    );
});
