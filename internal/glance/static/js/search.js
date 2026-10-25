import { frontendDiagnosticError } from "./diagnostics.js";
import { createPersonalState } from "./personal-state.js";
import { rankSearchResults, pruneSearchHistory, searchTemplateURL, safeSearchURL } from "./search-model.js";

const SEARCH_DOMAIN_PATTERN = /^(https?:\/\/\S+|[a-z0-9-]+(\.[a-z0-9-]+)+(:\d+)?([/?#]\S*)?)$/i;
const searchControllers = new Set();
const initializedSearchBoxes = new WeakSet();

// A single page shortcut chooses the first visible matching box. Root cleanup
// owns each box's input listeners, timers, and outstanding requests.
function focusSearchShortcut(event) {
    if (event.defaultPrevented || event.isComposing || event.repeat || event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) return;
    const active = document.activeElement;
    if (active?.isContentEditable || active?.closest("input, textarea, select, [role='textbox']")) return;
    const ordered = Array.from(searchControllers).sort((a, b) => a.widget.compareDocumentPosition(b.widget) & 2 ? 1 : -1);
    for (const controller of ordered) {
        if (controller.widget.getClientRects().length === 0) continue;
        const key = controller.widget.dataset.focusKey;
        if (key === "none") continue;
        const matches = controller.widget.dataset.useLayout === "true"
            ? event.key.toLowerCase() === key : event.code === `Key${key.toUpperCase()}`;
        if (!matches) continue;
        controller.input.focus();
        event.preventDefault();
        break;
    }
}

function initializeSearchBox(widget) {
    const input = widget.querySelector(".search-input");
    const bangBadge = widget.querySelector(".search-bang");
    const dropdown = widget.querySelector(".search-dropdown");
    const list = widget.querySelector(".search-results");
    const status = widget.querySelector(".search-status");
    const actions = widget.querySelector(".search-history-actions");
    const remove = widget.querySelector(".search-remove-history");
    const clear = widget.querySelector(".search-clear-history");
    const limit = Number(widget.dataset.resultsLimit);
    const newTab = widget.dataset.newTab === "true";
    const target = widget.dataset.target || "_blank";
    const openDomains = widget.dataset.openDomains === "true";
    const bangs = Array.from(widget.querySelectorAll(".search-bangs input"), element => ({
        kind: "bang", title: element.dataset.title || element.dataset.shortcut,
        shortcut: element.dataset.shortcut, url: element.dataset.url,
        raw: element.dataset.rawQuery === "true", alias: element.dataset.shortcut,
    }));
    const shortcuts = Array.from(widget.querySelectorAll(".search-shortcuts input"), element => ({
        kind: "shortcut", title: element.dataset.title, url: element.dataset.url,
        alias: element.dataset.alias.trim().toLowerCase(),
    }));
    const historyOptions = { limit: Number(widget.dataset.historyLimit), retention: Number(widget.dataset.historyRetention) };
    const storage = widget.dataset.historyStorage;
    // Never migrate anonymous/browser history into an authenticated account.
    const historyID = storage === "browser"
        ? `${pageData.baseURL}:${pageData.searchHistoryScope}:${widget.dataset.historyId}` : widget.dataset.historyId;
    let history = [];
    let historyUnavailable = false;
    let disposed = false;
    let results = [];
    let local = [];
    let suggestions = [];
    let selected = -1;
    let currentBang = null;
    let lastQuery = "";
    let timer = null;
    let request = null;
    let revision = 0;
    let networkMessage = "";
    const listeners = new AbortController();
    const on = (element, event, callback) => element?.addEventListener(event, callback, { signal: listeners.signal });
    const reportFailure = event => frontendDiagnosticError(event, new Error("Search enhancement unavailable"));
    const historyState = storage === "off" ? null : createPersonalState("search", historyID, {
        storage, keepalive: true,
        onError: () => { historyUnavailable = true; if (!disposed) render(); },
    });
    const historyReady = historyState ? historyState.load([], Array.isArray).then(value => {
        if (disposed) return;
        history = pruneSearchHistory(value, historyOptions);
        if (JSON.stringify(value) !== JSON.stringify(history)) historyState.save(history);
        if (document.activeElement === input) render();
    }) : Promise.resolve();

    function stopRequests() {
        revision++;
        clearTimeout(timer);
        request?.abort();
        request = null;
    }

    function close() {
        stopRequests();
        dropdown.hidden = true;
        input.setAttribute("aria-expanded", "false");
        input.removeAttribute("aria-activedescendant");
        selected = -1;
    }

    function updateBang() {
        const value = input.value.trim();
        currentBang = bangs.find(bang => value === bang.shortcut || value.startsWith(`${bang.shortcut} `)) || null;
        bangBadge.textContent = currentBang?.title || "";
    }

    function render() {
        if (disposed || document.activeElement !== input || input.isComposing) return;
        const query = input.value.trim();
        const candidates = currentBang ? [] : [
            ...shortcuts, ...history.map(item => ({ kind: "history", title: item.query })), ...bangs,
            ...local.map(item => ({ ...item, source: item.kind, kind: "local" })),
            ...suggestions.map(title => ({ kind: "suggestion", title })),
        ];
        const previous = results[selected];
        results = rankSearchResults(query, candidates, limit);
        selected = previous ? results.findIndex(item => item.kind === previous.kind && item.title === previous.title && item.url === previous.url) : -1;
        list.replaceChildren();
        for (const [index, result] of results.entries()) {
            const row = document.createElement("div");
            row.id = `${list.id}-${index}`;
            row.className = "search-result";
            row.setAttribute("role", "option");
            row.setAttribute("aria-selected", "false");
            row.dataset.index = String(index);
            const title = document.createElement("span");
            title.className = "search-result-title";
            title.textContent = result.title;
            const detail = document.createElement("span");
            detail.className = "search-result-detail";
            detail.textContent = [result.alias, result.source || result.kind, result.page].filter(Boolean).join(" · ");
            row.append(title, detail);
            list.append(row);
        }
        actions.hidden = historyState === null || history.length === 0;
        remove.hidden = true;
        const messages = [];
        if (results.length) messages.push(`${results.length} results. Use arrow keys to select.`);
        if (networkMessage) messages.push(networkMessage);
        if (historyUnavailable) messages.push("History storage unavailable; searches still work.");
        status.textContent = messages.join(" ");
        dropdown.hidden = !results.length && actions.hidden && !networkMessage && !historyUnavailable;
        input.setAttribute("aria-expanded", String(!dropdown.hidden));
        input.removeAttribute("aria-activedescendant");
        showSelection();
    }

    function showSelection() {
        Array.from(list.children).forEach((row, index) => row.setAttribute("aria-selected", String(index === selected)));
        if (selected < 0) input.removeAttribute("aria-activedescendant");
        else {
            const row = list.children[selected];
            input.setAttribute("aria-activedescendant", row.id);
            row.scrollIntoView({ block: "nearest" });
        }
        remove.hidden = results[selected]?.kind !== "history";
    }

    async function changeHistory(transform) {
        await historyReady;
        if (disposed || !historyState) return;
        history = pruneSearchHistory(transform(history), historyOptions);
        historyState.save(history);
        window.dispatchEvent(new CustomEvent("glance-search-history", { detail: { id: historyID, storage, history } }));
        render();
    }

    function remember(query) {
        if (!historyState || !query || new TextEncoder().encode(query).length > 512) return;
        void changeHistory(items => [{ query, at: Date.now() }, ...items.filter(item => item.query !== query)]);
    }

    function navigate(url, event, resultTarget = null) {
        const modified = event.ctrlKey || event.metaKey;
        const useNewTab = resultTarget === null ? newTab !== modified : (resultTarget !== "_self") !== modified;
        if (useNewTab) {
            const openedWindow = window.open(url, resultTarget && resultTarget !== "_self" ? resultTarget : target);
            if (openedWindow != null) openedWindow.focus();
        } else window.location.href = url;
        lastQuery = input.value.trim();
        input.value = "";
        updateBang();
        close();
    }

    function submit(event, result = null) {
        if (result?.kind === "history") {
            input.value = result.title;
            result = null;
        }
        updateBang();
        if (result?.kind === "bang") {
            input.value = `${result.shortcut} `;
            updateBang(); close(); input.focus(); return;
        }
        // An unselected fuzzy match must never hijack ordinary web search.
        if (!currentBang && result === null) {
            const value = input.value.trim().toLowerCase();
            result = shortcuts.find(item => item.alias && item.alias === value) || null;
        }
        if (result?.kind === "shortcut" || result?.kind === "local") {
            if (safeSearchURL(result.url)) navigate(result.url, event, result.target || null);
            return;
        }
        let query = result?.title || input.value.trim();
        if (currentBang) query = input.value.trim().slice(currentBang.shortcut.length).trim();
        if (!query && !currentBang) return;
        const template = currentBang?.url || widget.dataset.defaultSearchUrl;
        let url;
        if (openDomains && currentBang == null && SEARCH_DOMAIN_PATTERN.test(query)) {
            url = query.includes("://") ? query : "https://" + query;
        } else url = searchTemplateURL(template, query, currentBang?.raw === true);
        remember(currentBang ? input.value.trim() : query);
        navigate(url, event);
    }

    function update() {
        stopRequests(); updateBang();
        selected = -1;
        local = []; suggestions = []; networkMessage = "";
        render();
        const query = input.value.trim();
        if (!query || currentBang || input.isComposing || query.length > 512) return;
        const sequence = revision;
        timer = setTimeout(() => {
            request = new AbortController();
            const signal = request.signal;
            const endpoint = `${pageData.baseURL}/api/search/${encodeURIComponent(widget.dataset.widget)}`;
            const fetchResults = async (path, apply, failureMessage) => {
                try {
                    const response = await fetch(`${endpoint}/${path}`, { signal, headers: { Accept: "application/json" } });
                    if (!response.ok) throw new Error("Search request unavailable");
                    const data = await response.json();
                    if (!Array.isArray(data)) throw new Error("Invalid search results");
                    if (disposed || signal.aborted || sequence !== revision || document.activeElement !== input) return;
                    apply(data); render();
                } catch {
                    if (signal.aborted || disposed || sequence !== revision) return;
                    networkMessage = failureMessage;
                    reportFailure("search_request_error"); render();
                }
            };
            if (widget.dataset.localSearch === "true") {
                const parameters = new URLSearchParams({ q: query, page: pageData.slug, dashboard: pageData.dashboard || "" });
                void fetchResults(`targets?${parameters}`, data => { local = data.filter(item => typeof item.title === "string" && safeSearchURL(item.url)); }, "Local results unavailable.");
            }
            if (widget.dataset.suggestions === "true" && query.length >= 2) {
                void fetchResults(`suggestions?q=${encodeURIComponent(query)}`, data => { suggestions = data.filter(item => typeof item === "string"); }, "Web suggestions unavailable.");
            }
        }, 200);
    }

    on(input, "input", event => { if (!event.isComposing) update(); });
    on(input, "compositionstart", () => { input.isComposing = true; close(); });
    on(input, "compositionend", () => { input.isComposing = false; update(); });
    on(input, "focus", update);
    on(widget, "focusout", event => { if (!widget.contains(event.relatedTarget)) close(); });
    on(input, "keydown", event => {
        if (event.isComposing || input.isComposing || event.keyCode === 229) return;
        if (event.key === "Escape") {
            event.preventDefault();
            if (!dropdown.hidden) close(); else input.blur();
        } else if (event.key === "Enter") {
            event.preventDefault(); submit(event, selected < 0 ? null : results[selected]);
        } else if ((event.key === "ArrowDown" || event.key === "ArrowUp") && results.length && !dropdown.hidden) {
            event.preventDefault();
            const position = selected + 1;
            selected = (position + (event.key === "ArrowDown" ? 1 : -1) + results.length + 1) % (results.length + 1) - 1;
            showSelection();
        } else if (event.key === "ArrowUp" && lastQuery) {
            event.preventDefault(); input.value = lastQuery; update();
        }
    });
    on(list, "pointerdown", event => event.preventDefault());
    on(list, "click", event => {
        const row = event.target.closest("[data-index]");
        if (row) submit(event, results[Number(row.dataset.index)]);
    });
    on(clear, "click", () => { void changeHistory(() => []); input.focus(); });
    on(remove, "click", () => {
        const query = results[selected]?.title;
        if (query) void changeHistory(items => items.filter(item => item.query !== query));
        input.focus();
    });
    on(widget.querySelector("kbd"), "mousedown", event => { event.preventDefault(); input.focus(); });
    on(window, "storage", event => {
        if (storage !== "browser" || event.key !== `search-${historyID}`) return;
        try { history = pruneSearchHistory(JSON.parse(event.newValue || "[]"), historyOptions); render(); }
        catch { reportFailure("search_history_parse_error"); }
    });
    on(window, "glance-search-history", event => {
        if (event.detail.id !== historyID || event.detail.storage !== storage) return;
        history = pruneSearchHistory(event.detail.history, historyOptions);
        render();
    });
    const controller = { widget, input };
    if (searchControllers.size === 0) document.addEventListener("keydown", focusSearchShortcut);
    searchControllers.add(controller);
    let autofocus = null;
    if (input.hasAttribute("autofocus")) autofocus = requestAnimationFrame(() => { if (!disposed && widget.getClientRects().length) input.focus(); });
    return () => {
        if (disposed) return;
        disposed = true; stopRequests();
        if (autofocus !== null) cancelAnimationFrame(autofocus);
        listeners.abort(); searchControllers.delete(controller); initializedSearchBoxes.delete(widget);
        if (!searchControllers.size) document.removeEventListener("keydown", focusSearchShortcut);
    };
}

function setupSearchBoxes(root = document, registerCleanup = null) {
    const elements = Array.from(root.querySelectorAll(".search"));
    if (root.matches?.(".search")) elements.unshift(root);
    const callbacks = [];
    for (const widget of elements) {
        if (initializedSearchBoxes.has(widget)) continue;
        initializedSearchBoxes.add(widget);
        try {
            const cleanup = initializeSearchBox(widget);
            callbacks.push(cleanup);
            registerCleanup?.(widget.closest("[data-widget-id]"), cleanup);
        }
        catch {
            initializedSearchBoxes.delete(widget);
            frontendDiagnosticError("search_initialize_error", new Error("Search initialization failed"));
        }
    }
    return callbacks;
}

export { setupSearchBoxes };
