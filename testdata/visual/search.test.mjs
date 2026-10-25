// Executable pure-policy tests complement the real browser Search scenario.
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import test from "node:test";

const source = await fs.readFile("internal/glance/static/js/search-model.js", "utf8");
const { searchScore, rankSearchResults, pruneSearchHistory, searchTemplateURL, safeSearchURL } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

test("ranking prioritizes exact aliases, then prefixes, substrings and abbreviations", () => {
    const items = [
        { kind: "shortcut", title: "GitHub", alias: "gh", url: "/github" },
        { kind: "shortcut", title: "GH documentation", url: "/docs" },
        { kind: "shortcut", title: "My gh site", url: "/site" },
    ];
    assert.equal(searchScore("gh", "GitHub"), 1);
    assert.deepEqual(rankSearchResults("gh", items, 2).map(item => item.url), ["/github", "/docs"]);
    assert.equal(rankSearchResults("unmatched", items, 8).length, 0);
});

test("result merging bounds duplicates and shows only history for an empty query", () => {
    const items = [{ kind: "history", title: "query" }, { kind: "history", title: "query" }, { kind: "shortcut", title: "site", url: "/site" }];
    assert.equal(rankSearchResults("", items, 8).length, 1);
    assert.equal(rankSearchResults("query", [...items, { kind: "suggestion", title: "query example" }], 1).length, 1);
});

test("history prunes malformed, expired, future and duplicate entries with a hard cap", () => {
    const now = 2000000000000;
    const entries = [null, { query: "expired", at: now - 31 * 86400000 }, { query: "future", at: now + 120000 },
        { query: "new", at: now }, { query: "new", at: now - 1 }, { query: "old", at: now - 100 }, { query: "x".repeat(513), at: now }];
    assert.deepEqual(pruneSearchHistory(entries, { limit: 1, retention: 30 }, now), [{ query: "new", at: now }]);
    assert.deepEqual(pruneSearchHistory({}, { limit: 50, retention: 30 }, now), []);
});

test("every placeholder is encoded by default and raw queries are explicit", () => {
    assert.equal(searchTemplateURL("https://example/?a=!QUERY!&b=!QUERY!", "a/b &c"), "https://example/?a=a%2Fb%20%26c&b=a%2Fb%20%26c");
    assert.equal(searchTemplateURL("https://github.com/!QUERY!", "owner/repo", true), "https://github.com/owner/repo");
});

test("local navigation rejects executable URLs, credentials, backslashes and protocol-relative URLs", () => {
    for (const value of ["javascript:alert(1)", "//evil.example", "/\\evil.example", "https://user:pass@example.com", " https://example.com", "file:///tmp/test"]) assert.equal(safeSearchURL(value), false, value);
    for (const value of ["/dashboard/page", "https://example.com/path", "http://localhost:8080"]) assert.equal(safeSearchURL(value), true, value);
});
