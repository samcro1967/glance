# Adding a widget

[Glance README](../README.md) · [Contributing](../CONTRIBUTING.md) · [Widgets](widgets.md)

---

This guide defines the repository-specific workflow for adding a native Glance widget. It supplements [Contributing to Glance](../CONTRIBUTING.md); the general development, testing, security, compatibility, and lifecycle requirements in that document still apply.

The goal is not merely to make a widget render. A new widget should fit the existing Glance architecture, visual language, refresh lifecycle, provider-validation strategy, documentation system, and validation workflow without creating unnecessary parallel infrastructure.

## 1. Investigate before implementing

Before creating files, trace the closest existing widgets end to end. Identify the implementation that most closely matches the new widget's data source, lifecycle, configuration, and presentation.

Review, as applicable:

- widget registration and construction in `internal/glance/widget-capability-map.go`;
- the widget struct, initialization, update, rendering, and request handling patterns;
- shared configuration and normalization helpers;
- shared HTTP clients, timeouts, TLS handling, authentication, cancellation, status/error classification, and resource proxy support;
- existing templates and semantic presentation primitives;
- widget CSS and global semantic theme variables;
- tests for similar widgets and shared helpers;
- source-test configuration in `test-instance.yml` and deterministic provider fixtures in `testdata/visual/fixture-server.js` where required;
- visual gallery, widget screenshot, and documentation image registries;
- public widget documentation and fork documentation.

Prefer extending an established abstraction when the new behavior genuinely belongs there. Do not create a new abstraction merely because the widget itself is new.

## 2. Fit the native widget lifecycle

Use the existing widget lifecycle and ownership model rather than introducing a parallel refresh mechanism.

A typical data-backed widget should use the established widget base, initialize its defaults, validate its configuration, fetch through the shared HTTP infrastructure, honor `context.Context` cancellation, normalize provider data into presentation-facing data, and render through the normal template system.

Where applicable, preserve the established contracts for:

- cache and refresh scheduling;
- stale/degraded data behavior;
- timeout and cancellation;
- HTTP status handling and error visibility;
- reload and live-widget replacement;
- resource cleanup and ownership;
- authentication and authorization;
- resource proxying for remote images or other browser resources.

### Establish refresh behavior in the widget lifecycle

A provider-backed or otherwise refreshable widget must establish a refreshable cache policy during `initialize()` using the existing widget lifecycle helpers, such as `withCacheDuration`, `withCacheCron`, or the appropriate shared equivalent.

Do not rely solely on centralized builtin defaults to make an otherwise infinitely cached widget refreshable. Builtin or user configuration may refine or override the widget's policy, but the widget itself must participate correctly in the native refresh lifecycle after initialization.

Add focused coverage proving a newly initialized refreshable widget is eligible for its initial update. When practical, runtime validation should also confirm that the provider was actually contacted rather than inferring success from configuration or compilation alone.

Provider-specific payloads should remain behind provider-specific parsing or normalization boundaries. Templates should not need to understand raw provider responses.

## 3. Reuse shared configuration and infrastructure

Before adding widget-specific configuration or helpers, check whether the repository already owns the concept.

Reuse shared infrastructure for HTTP clients, timeouts, TLS/insecure handling, authentication, headers, resource proxying, dates and durations, URLs, status/error classification, and other common mechanics when the existing abstraction fits.

Do not duplicate provider credentials into rendered HTML, browser-visible resource URLs, logs, errors, fixtures, screenshots, or documentation. Remote artwork that requires credentials should use the established resource-proxy architecture rather than exposing source credentials to the browser.

New optional configuration should preserve existing behavior when omitted unless the feature explicitly requires otherwise.

## 4. Use the Glance visual language

Glance owns the dashboard visual language. A new widget should look native without inventing its own theme system.

Use global semantic theme variables and existing presentation primitives wherever they represent the same concept. Reuse established list, metadata, status, progress, table, control, spacing, typography, border, and interaction patterns when applicable.

Widget-specific CSS is appropriate for genuinely widget-specific structure, responsive layout, or information presentation. It should not duplicate global theme semantics or hard-code a separate visual system.

In particular:

- do not hard-code colors when an appropriate semantic theme variable exists;
- do not create widget-specific cards, borders, spacing, or controls when an existing primitive already represents the concept;
- do not force specialized content through a generic primitive when doing so harms the information model or usability;
- preserve light/dark and user theme behavior through the normal semantic hierarchy;
- preserve responsive behavior and established narrow-column behavior where applicable.

## 5. Register the widget

Add the widget through the normal capability registry in `internal/glance/widget-capability-map.go` and follow the surrounding registration pattern.

Registration, configuration decoding, defaults, and validation should be exercised by tests. A widget is not fully integrated merely because its Go type can be instantiated directly.

## 6. Add provider and fixture coverage

Every registered widget should be representable in the canonical source test instance. Runtime and visual validation should use the real production provider by default so the integration proves the same network, parsing, resource, and rendering path users will exercise.

Use deterministic fixtures when live provider data is not feasible or is sensitive, and for automated tests that specifically require stable, repeatable provider responses, failure modes, or edge cases. Fixtures supplement real-provider validation; they should not silently replace a usable public production provider in the normal source test instance.

Before implementing or updating a provider parser, inspect a representative response from the real provider and identify the exact fields, nesting, ordering, encoding, and fallback behavior the widget depends on.

Deterministic fixtures must be derived from that observed provider contract and preserve the relevant structure. Do not simplify or invent a provider response shape merely to make the fixture or parser easier to implement. A fixture-backed parser test proves compatibility with the fixture, not necessarily compatibility with the provider.

For feeds containing multiple records, explicitly establish how the desired record is selected. Do not assume provider ordering unless that ordering is part of a documented contract; prefer selection by an explicit date, identifier, or other semantic field, with an intentional fallback where appropriate.

When deterministic provider coverage is needed, extend `testdata/visual/fixture-server.js` and activate the fixture explicitly for that test or workflow rather than globally overriding normal test-instance provider behavior. Fixture data should exercise the presentation meaningfully without being unnecessarily large.

Never place real credentials, tokens, personal identities, or production-only URLs in committed fixtures.

## 7. Add automated tests

Add focused tests for the behavior introduced by the widget and for any shared behavior changed to support it.

Depending on the widget, coverage should include:

- initialization and defaults;
- configuration validation and invalid configurations;
- provider request construction and authentication semantics;
- response parsing and normalization;
- HTTP failures and status handling;
- cancellation and timeout behavior;
- empty results and malformed or incomplete provider data;
- zero or boundary values such as zero duration or missing timestamps;
- multiple items/sessions when supported;
- stale/degraded behavior where relevant;
- rendering and resource proxy behavior;
- explicit checks that credentials or sensitive source URLs cannot leak into rendered output.

Use the maintained focused-test interface during development:

```text
make test-focused TEST_RUN='<test expression>'
```

Focused tests do not replace the broader repository validation required before integration.

## 8. Register visual QA coverage

Add the widget to the appropriate category in:

```text
testdata/visual/widget-gallery.json
```

Add its canonical isolated screenshot mapping to:

```text
testdata/visual/widget-screenshots.json
```

Use the page containing the validated widget and a stable selector. Prefer the native widget type selector when it uniquely identifies the capture, for example:

```text
.widget-type-example
```

Use a dedicated fixture selector only when the page contains multiple instances or the native selector is otherwise ambiguous.

Run:

```text
make visual-check
```

The visual contract should report complete gallery and widget screenshot coverage before the feature is considered integrated.

## 9. Register the documentation screenshot

Browser-managed widget documentation images are registered in:

```text
testdata/visual/docs-images.json
```

For a normal widget element capture, use the complete browser recipe:

```json
"widgets/example.png": {
  "kind": "browser",
  "capture": "element",
  "route": "/appropriate-page",
  "selector": ".visual-fixture-example"
}
```

`kind` and `capture` are required parts of the recipe. An element capture must define a selector. A page capture should not define an element selector.

Normal documentation captures should represent the page state without scripted interaction. When the documented state inherently requires a user interaction, a browser-managed recipe may define an optional `click` CSS selector. The screenshot runner waits for that control to become visible and clicks it after the page is ready but before the normal capture. Use `clickNth` only when the selector intentionally matches multiple controls and a specific zero-based match is required.

For example, an expanded widget dialog can be captured from the real widget interaction:

```json
"widgets/example-expanded.png": {
  "kind": "browser",
  "capture": "element",
  "route": "/appropriate-page",
  "click": ".visual-fixture-example [data-widget-expand]",
  "selector": ".widget-expand-dialog"
}
```

Prefer the ordinary interaction-free recipe for the canonical widget image. Add an interactive documentation capture only when it demonstrates a distinct user-visible state that the normal image cannot show.

Do not manually create or copy a browser-managed documentation screenshot into `docs/images`.

## 10. Capture only the new documentation screenshot during development

When adding one widget, prefer the scoped documentation-image workflow rather than regenerating or promoting every managed documentation image.

Stage only the new image:

```text
make visual-docs VISUAL_IMAGE='widgets/example.png'
```

This writes the generated image to `testdata/visual/docs-staging` and does not modify `docs/images`.

Review the staged image visually. Successful capture proves that the browser automation completed; it does not prove that the widget looks correct.

After the image is approved, promote only that image:

```text
make visual-docs-promote VISUAL_IMAGE='widgets/example.png'
```

Then run `make status` and verify that only the intended new or deliberately changed documentation images appear in the worktree.

Use the broader visual workflows when broad presentation changes actually require them. Do not use a full managed-image promotion merely to obtain one new widget screenshot.

## 11. Write the widget documentation

Create:

```text
docs/widgets/<widget-name>.md
```

Follow existing widget documentation for structure and terminology. Document the supported services or data sources, required and optional configuration, defaults, examples, security-sensitive configuration, important provider differences, and behavior users need to understand.

Add the widget to `docs/widgets.md`. Update `README.md` and `docs/fork.md` when the new capability changes the high-level supported feature set or fork-specific capability record.

Reference the browser-managed screenshot through the normal documentation path rather than embedding an independently maintained image.

## 12. Validate real changed behavior

Validation should progress from focused evidence to broader evidence.

A typical sequence is:

1. inspect the real provider response and establish the provider contract when the widget integrates with an external service;
2. focused unit/provider tests with `make test-focused`, using fixtures derived from the observed provider contract where deterministic coverage is useful;
3. rendered behavior through the source test instance using real provider data by default, including confirmation that the provider was actually contacted and external resources rendered correctly;
4. deterministic fixture validation for stable automated failure, edge-case, or regression coverage where needed;
5. visual review and documentation screenshot capture/promotion;
6. `make visual-check`;
7. `make check` for broad development validation;
8. `make validate` for the release gate;
9. complete diff review before commit.

Use `make test-instance-*` for source-runtime and visual testing. The maintained source test instance should use real production provider data by default when that is feasible and safe. Use explicitly activated deterministic fixtures when stable provider responses, failure modes, edge cases, or sensitive data require them. Use `make test-prod-*` when the current source must be exercised against production-representative configuration, networks, assets, or providers without replacing the running production container.

A successful build, one passing test, or a successful screenshot capture is not sufficient evidence for a runtime behavior change. Validate the behavior that actually changed.

## 13. Review the complete integration surface

The exact files vary by widget, but a new native widget commonly affects some combination of:

```text
internal/glance/widget-<name>.go
internal/glance/widget-<name>_test.go
internal/glance/widget-capability-map.go
internal/glance/templates/<name>.html
internal/glance/static/css/widget-<name>.css
internal/glance/static/css/widgets.css

test-instance.yml
testdata/visual/fixture-server.js
testdata/visual/widget-gallery.json
testdata/visual/widget-screenshots.json
testdata/visual/docs-images.json

docs/widgets/<name>.md
docs/widgets.md
docs/images/widgets/<name>.png
README.md
docs/fork.md
```

This is an audit list, not a requirement to modify every file. Reuse shared files only when the widget actually needs them, and avoid unrelated cleanup.

## Final checklist

Before considering a new widget ready for integration, confirm that:

- the closest existing implementation was investigated first;
- shared architecture and helpers are reused where appropriate;
- configuration is validated and backward-compatible where applicable;
- cancellation, timeout, refresh, stale/degraded, and cleanup behavior are correct where relevant;
- errors remain visible and credentials remain private;
- the widget uses semantic theming and native presentation primitives where appropriate;
- real provider coverage exists when the provider is feasible and safe to exercise directly;
- deterministic fixture coverage exists where stable automated responses, failure modes, edge cases, or sensitive data require it;
- focused tests cover important success, failure, and edge behavior;
- gallery and widget screenshot mappings are complete;
- the documentation image recipe is complete;
- the new documentation screenshot was staged, visually reviewed, and promoted intentionally;
- public widget documentation is complete;
- real provider/runtime behavior was validated when applicable;
- `make check` and `make validate` pass at the appropriate stages;
- the complete diff contains only intended changes.

---

[Contributing](../CONTRIBUTING.md) · [Widgets](widgets.md) · [Back to top](#adding-a-widget)
