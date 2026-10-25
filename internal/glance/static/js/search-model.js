// Pure Search policies shared by the DOM controller and executable unit tests.
// Network, storage, and navigation belong to their existing boundary modules.
export function searchScore(query, text) {
    query = query.trim().toLowerCase();
    text = text.toLowerCase();
    if (!query) return 0;
    if (query === text) return 4;
    if (text.startsWith(query)) return 3;
    if (text.includes(query)) return 2;
    const characters = Array.from(query);
    let index = 0;
    for (const character of text) if (character === characters[index]) index++;
    return index === characters.length ? 1 : 0;
}

export function rankSearchResults(query, items, limit) {
    const seen = new Set();
    return items.map((item, order) => ({
        ...item, order,
        score: Math.max(searchScore(query, item.title), searchScore(query, item.alias || "")),
    })).filter(item => {
        const key = `${item.kind}\0${item.url || item.title}`;
        if (seen.has(key)) return false;
        seen.add(key);
        // History on focus is intentional; other sources require typed input.
        return item.score > 0 || (!query && item.kind === "history") || item.kind === "suggestion" || item.kind === "local";
    }).sort((a, b) => b.score - a.score || a.order - b.order).slice(0, limit);
}

export function pruneSearchHistory(value, { limit, retention }, now = Date.now()) {
    if (!Array.isArray(value)) return [];
    const seen = new Set();
    return value.filter(item => {
        if (!item || typeof item.query !== "string" || !item.query.trim() ||
            new TextEncoder().encode(item.query).length > 512 ||
            !Number.isFinite(item.at) || item.at <= now - retention * 86400000 || item.at > now + 60000 || seen.has(item.query)) return false;
        seen.add(item.query);
        return true;
    }).sort((a, b) => b.at - a.at).slice(0, limit).map(({ query, at }) => ({ query, at }));
}

export function searchTemplateURL(template, query, raw = false) {
    return template.replaceAll("!QUERY!", raw ? query : encodeURIComponent(query));
}

export function safeSearchURL(value) {
    if (typeof value !== "string" || value.trim() !== value || /[\\\r\n\t]/.test(value)) return false;
    if (value.startsWith("/") && !value.startsWith("//")) return true;
    try {
        const url = new URL(value);
        return ["http:", "https:"].includes(url.protocol) && !url.username && !url.password;
    } catch { return false; }
}
