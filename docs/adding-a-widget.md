# Adding a widget

[Glance README](../README.md) · [Contributing](../CONTRIBUTING.md) · [Widgets](widgets.md) · [Adding a micro-widget](adding-a-micro-widget.md)

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

Before implementation, name the closest reference widget or widgets and trace each relevant path completely: implementation, tests, source-test configuration, visual registration, documentation screenshots, and public documentation. If the new widget must differ architecturally, identify the concrete requirement that makes the established pattern insufficient.

### Prove one vertical slice before scaling out

When introducing multiple related widgets, fully implement and runtime-validate one representative widget before implementing the remaining family members. Similar presentation or provider concepts are not sufficient reason to build several integrations in parallel before the first one is proven.

For an external-provider widget, the first vertical slice should prove the complete path that users will exercise:

```text
provider request -> parsing -> selection -> normalization -> widget update -> template render -> browser resources -> browser display
```

If real-provider or source-runtime validation fails, stop. Establish the root cause and reconfirm the implementation approach before adding sibling widgets, documentation screenshots, new abstractions, or infrastructure workarounds.

### Create the feature branch only after investigation

Investigation and design review should happen before implementation. Once the approach is understood and agreed, start feature work through the repository workflow:

```text
make branch NEW_BRANCH=feature/widget-name
```

Do not create the branch merely to begin investigation.

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

### Prove browser-visible resources early

When provider content contains images or other resources that the browser will fetch directly, validate representative resource URLs in the same browser/runtime context Glance will use before depending on them in the widget design. A successful server-side HTTP request does not prove that a browser can load or render the resource.

If browser delivery fails because of provider policy, redirects, authentication, content type, CORS/ORB behavior, or another browser-facing restriction, stop and compare the requirement with the closest existing widget and the established resource-proxy architecture before changing shared infrastructure. Do not expand shared proxy or HTTP behavior merely to work around a provider-specific failure without first proving that the architectural change is appropriate.

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

Use the maintained focused-test interface during development. For a normal native widget, scope the run to the Glance package as well as the widget's test names:

```text
make test-focused TEST_RUN='TestExampleWidget' TEST_PACKAGE='./internal/glance'
```

A regular-expression test selector can cover a related group when appropriate:

```text
make test-focused TEST_RUN='Test(ExampleWidget|ExampleWidgetRendering)' TEST_PACKAGE='./internal/glance'
```

`TEST_PACKAGE` defaults to `./...` when omitted, but widget development should normally use the narrowest package that owns the changed behavior. Focused tests do not replace the broader repository validation required before integration.

## Test-instance placement and review URL

Every normal native widget added for development and visual validation must have one intentional canonical location in `test-instance.yml`. Do not place it on an arbitrary page or column merely to make it render.

The hierarchy is:

```text
test-instance.yml
  pages
    -> page (name + slug)
       -> head-widgets, columns, or bottom-widgets
           -> column (size) when using columns
             -> widget
```

A *page* is the browser destination. Its `slug` determines its route. A *column* only controls layout inside that page; column size does not create another URL.

For example:

```yaml
- name: Daily Discovery
  slug: daily-discovery
  columns:
    - size: medium
      widgets:
        - type: word-of-the-day
```

With the maintained source test instance running at `http://127.0.0.1:18080`, the review URL is therefore:

```text
http://127.0.0.1:18080/daily-discovery
```

The URL is derived from the page `slug`, not the page name, column size, widget type, gallery category, or screenshot filename. Before giving a review URL, read the widget's actual containing page in `test-instance.yml` and use that page's `slug`; do not infer or invent a route.

Choose the canonical page by responsibility. Add the widget to the existing page whose purpose best matches it. Add a new page only when no existing canonical page is appropriate. Within that page, choose `small`, `medium`, or `full` based on the widget's intended presentation and the closest comparable widgets; column size does not affect routing.

Keep the visual registries aligned with that canonical placement:

```text
test-instance.yml                           page slug: daily-discovery
testdata/visual/visual-pages.json        page:      daily-discovery
testdata/visual/widget-gallery.json      category:  daily-discovery
testdata/visual/widget-screenshots.json  route:     /daily-discovery
testdata/visual/docs-images.json          route:     /daily-discovery
```

`testdata/visual/screenshots.js` also owns the canonical QA route table. If a genuinely new canonical QA page is added, register its slug/route in `qaPageRoutes`. Adding a widget to an already registered page does not require another route entry.

These values describe different things and should not be conflated:

- **page name** — human-readable navigation label;
- **page slug** — browser route and source of the manual review URL;
- **column size** — Layout only;
- **widget type** — widget implementation/configuration identity;
- **gallery category** — Canonical visual grouping;
- **screenshot selector** — stable DOM target used to isolate the widget.

After placing the widget, start the maintained runtime with `make test-instance-start`, then review the exact URL derived from its containing page slug. If the page placement changes, update every visual route that refers to the old page before continuing.

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

`visual-check` validates the visual registries and documentation contract; it does not capture screenshots. The visual contract should report complete gallery and widget screenshot coverage before the feature is considered integrated.

For a new or changed widget, prefer an exact canonical widget capture:

    make visual-screenshots VISUAL_WIDGET=example-widget

`VISUAL_WIDGET` selects the widget by its key in `testdata/visual/widget-screenshots.json` and captures only that mapped widget. The widget must have a valid canonical screenshot recipe there before targeted capture can succeed.

Use page or dashboard scope when a broader visual change intentionally requires multiple canonical captures:

    make visual-screenshots VISUAL_PAGE=appropriate-page
    make visual-screenshots VISUAL_DASHBOARD=appropriate-dashboard

`VISUAL_WIDGET`, `VISUAL_PAGE`, and `VISUAL_DASHBOARD` are mutually exclusive. `VIEWPORT=desktop|mobile` selects the capture viewport for widget, page, or dashboard QA. Desktop widget captures isolate the selected widget; mobile widget captures preserve the full 430x900 viewport after navigating to the appropriate mobile column and bringing the widget into view.

## 9. Validate the source runtime before documentation capture

After focused tests pass, exercise the current source through the maintained deterministic test runtime before generating documentation screenshots:

```text
make test-instance-start
make test-instance-status
```

Open the page containing the widget and validate the actual changed behavior in the browser. For provider-backed widgets, confirm that the expected provider path was exercised and that browser-visible resources, interactions, responsive behavior, and error/degraded states relevant to the change behave correctly.

Do not infer runtime correctness from passing unit tests, configuration validation, generated HTML, a successful HTTP request made outside the browser, compilation, or a screenshot command completing successfully.

A source-runtime failure is a stop condition. Diagnose and resolve the failure before proceeding to documentation screenshots or additional related widgets.

Explicit `test-instance-start` and `test-instance-restart` are persistent interactive workflows. They first clean any mutually exclusive Makefile-managed test runtime, then leave the requested runtime available until it is explicitly stopped or restarted. Use:

```text
make test-instance-restart
```

when current-source or runtime configuration changes require a clean replacement.

When interactive runtime validation is complete, stop the maintained runtime:

```text
make test-instance-stop
```

Browser, frontend, visual, Lighthouse, performance, and similar Makefile orchestration that starts a temporary test runtime owns that runtime for the duration of the command and must stop it on both success and failure. Do not rely on a successful screenshot or validation command leaving a test server running for later inspection; start an explicit interactive runtime when continued inspection is required.

## 10. Register the documentation screenshot

Browser-managed widget documentation images are registered in:

```text
testdata/visual/docs-images.json
```

For a normal desktop widget element capture, use the complete browser recipe:

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

## 11. Capture only the new documentation screenshot during development

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

## 12. Write the widget documentation

Create:

```text
docs/widgets/<widget-name>.md
```

Follow existing widget documentation for structure and terminology. Document the supported services or data sources, required and optional configuration, defaults, examples, security-sensitive configuration, important provider differences, and behavior users need to understand.

Add the widget to `docs/widgets.md`. Update `README.md` and `docs/fork.md` when the new capability changes the high-level supported feature set or fork-specific capability record.

Reference the browser-managed screenshot through the normal documentation path rather than embedding an independently maintained image.

## 13. Validate real changed behavior

Validation should progress from focused evidence to broader evidence.

A typical sequence is:

1. identify and trace the closest reference widget end to end;
2. inspect the real provider response and establish the provider contract when the widget integrates with an external service;
3. prove representative browser-visible provider resources when the design depends on them;
4. create the feature branch with `make branch NEW_BRANCH=feature/widget-name` only after the implementation approach is agreed;
5. implement one representative vertical slice;
6. run focused unit/provider tests with `make test-focused TEST_RUN='TestExampleWidget' TEST_PACKAGE='./internal/glance'`, using fixtures derived from the observed provider contract where deterministic coverage is useful;
7. validate rendered behavior through `make test-instance-start` / `make test-instance-status` using real provider data by default, including confirmation that the provider was actually contacted and external resources rendered correctly;
8. **stop and reassess** if the real provider or source runtime does not behave as expected; do not scale out the widget family or redesign shared infrastructure while the first vertical slice is unproven;
9. add deterministic fixture validation for stable automated failure, edge-case, or regression coverage where needed;
10. run `make visual-check`, then capture the new or changed widget with `make visual-screenshots VISUAL_WIDGET=example-widget`; use `VISUAL_PAGE=...` or `VISUAL_DASHBOARD=...` instead only when the change intentionally requires broader visual coverage;
11. stage only the new documentation image with `make visual-docs VISUAL_IMAGE='widgets/example.png'`, visually review it, and promote only that image with `make visual-docs-promote VISUAL_IMAGE='widgets/example.png'`;
12. run `make check` for broad development validation;
13. run `make validate` for the release gate;
14. review the complete diff before commit.

Use `make test-instance-*` for source-runtime and visual testing. The maintained source test instance should use real production provider data by default when that is feasible and safe. Use explicitly activated deterministic fixtures when stable provider responses, failure modes, edge cases, or sensitive data require them. Use `make test-prod-*` when the current source must be exercised against production-representative configuration, networks, assets, or providers without replacing the running production container.

A successful build, one passing test, or a successful screenshot capture is not sufficient evidence for a runtime behavior change. Validate the behavior that actually changed.

## 14. Review the complete integration surface

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

## 15. Finish through the repository workflow

Use Makefile targets whenever the repository exposes a maintained workflow operation. Ordinary working-tree Git operations intentionally remain direct Git commands: there are no generic `make add`, `make commit`, `make diff`, or `make restore` targets. Use `git diff` for ordinary diff review, `git add` for staging, and `git commit` for commits. Use `make staged-diff` for the maintained staged-change review. Do not search for or invent generic Make wrappers when the Makefile help explicitly defines the operation as direct Git.

After validation and complete diff review, commit the feature using the repository's normal commit conventions. `make park` is the supported way to return committed feature work to local `dev` without pushing it:

```text
make park
```

If an uncommitted feature experiment is intentionally being discarded instead, use the guarded abandonment workflow:

```text
make abandon
```

`make abandon` is intentionally limited to feature branches with no feature-only commits. It discards tracked and untracked working-tree changes, returns to local `dev`, deletes the abandoned feature branch, and verifies that `dev` is clean. It refuses committed feature work so that committed history cannot be discarded accidentally.

Shipping remains governed by the normal repository workflow and requires explicit authorization; adding a widget does not create a separate PR, release, or deployment path.

## Final checklist

Before considering a new widget ready for integration, confirm that:

- the closest existing implementation was investigated first;
- the named reference widget was traced through implementation, tests, runtime, visual coverage, screenshots, and documentation;
- when adding a widget family, one representative widget completed real runtime validation before sibling implementations began;
- shared architecture and helpers are reused where appropriate;
- configuration is validated and backward-compatible where applicable;
- cancellation, timeout, refresh, stale/degraded, and cleanup behavior are correct where relevant;
- errors remain visible and credentials remain private;
- the widget uses semantic theming and native presentation primitives where appropriate;
- real provider coverage exists when the provider is feasible and safe to exercise directly;
- browser-visible provider resources were proven in the actual browser/runtime context when applicable;
- deterministic fixture coverage exists where stable automated responses, failure modes, edge cases, or sensitive data require it;
- focused tests cover important success, failure, and edge behavior;
- focused development runs use `TEST_PACKAGE` to avoid unnecessarily broad test execution;
- explicit interactive test runtimes are stopped when no longer needed;
- Makefile orchestration that starts a temporary test runtime cleans it up on success and failure;
- `testdata/visual/widget-gallery.json` includes the widget in the canonical visual gallery where applicable;
- `testdata/visual/widget-screenshots.json` defines its canonical QA screenshot recipe and supports targeted capture with `VISUAL_WIDGET=<widget>`;
- `visual-check`, QA capture, documentation staging, and documentation promotion were treated as distinct operations;
- `testdata/visual/docs-images.json` defines its managed documentation screenshot recipe;
- the new documentation screenshot was staged, visually reviewed, and promoted intentionally;
- public widget documentation is complete;
- real provider/runtime behavior was validated when applicable;
- `make check` and `make validate` pass at the appropriate stages;
- the complete diff contains only intended changes.

---

[Contributing](../CONTRIBUTING.md) · [Widgets](widgets.md) · [Back to top](#adding-a-widget)
