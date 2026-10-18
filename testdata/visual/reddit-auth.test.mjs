import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

const source = readFileSync(
  new URL("../../internal/glance/reddit-browser.go", import.meta.url),
  "utf8"
);

const method = source.match(
  /func redditBrowserAuthenticated\(ctx context\.Context\) \(bool, error\) \{([\s\S]*?)\n\}/
);

assert.ok(method, "Reddit authentication detector must exist");

const expression = method[1].match(
  /redditBrowserEvaluate\(ctx, `([\s\S]*?)`\)/
);

assert.ok(expression, "Authentication expression must be available");

function detect({ account = false, inbox = false, login = false } = {}) {
  const element = textContent => ({ textContent });

  const document = {
    querySelectorAll(selector) {
      if (selector === "button") {
        return account ? [element("Expand user menu")] : [];
      }

      if (selector === 'a[href="/notifications"]') {
        return inbox ? [element("Open inbox")] : [];
      }

      throw new Error(`Unexpected selector: ${selector}`);
    },

    querySelector(selector) {
      if (selector === 'a[href="/login/"], a[href="/login"]') {
        return login ? element("Log In") : null;
      }

      throw new Error(`Unexpected selector: ${selector}`);
    }
  };

  return runInNewContext(expression[1], { document });
}

test("Reddit authenticated account controls", () => {
  assert.equal(detect({ account: true, inbox: true }), "yes");
});

test("Reddit logged-out navigation", () => {
  assert.equal(detect({ login: true }), "no");
});

test("Reddit incomplete account controls remain pending", () => {
  assert.equal(detect({ account: true }), "pending");
  assert.equal(detect({ inbox: true }), "pending");
  assert.equal(detect(), "pending");
});

test("Reddit authenticated controls take precedence over login link", () => {
  assert.equal(
    detect({ account: true, inbox: true, login: true }),
    "yes"
  );
});
