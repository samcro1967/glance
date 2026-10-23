# Glance visual QA fixture

The visual QA configuration mirrors the eight widget categories in `docs/widgets.md`. The separate `Themes` dashboard exercises global inheritance, explicit dark and light page themes, partial overrides, semantic components, and nested composition.

## Contracts

- `test-instance.yml` is the complete local visual QA dashboard.
- `testdata/expanded-theme.yml` owns the expanded global theme fixture.
- `testdata/screenshot-calendar.ics` preserves the original reference calendar.
- `testdata/visual/generate-calendar.py` generates the date-relative `generated-calendar.ics` for Calendar and ICS Events during test-instance startup. The generated file is ignored by Git and removed during cleanup.
- `widget-gallery.json` classifies every registered widget exactly once.
- `visual-pages.json` lists the canonical pages captured by browser QA.
- `docs-images.json` explicitly lists documentation PNGs managed by automation.
- `check-gallery.py` fails when registry, page, or docs-image contracts drift.
- `screenshots.js` captures QA pages and documentation images with project-local Playwright and system Google Chrome.

## Commands

`make visual-check` validates structural contracts without opening a browser.

`make visual-screenshots` captures all canonical QA pages into `testdata/visual/screenshots/`. These files are intentionally ignored by Git. Use `VISUAL_DASHBOARD=<name>` or `VISUAL_PAGE=<slug>` to refresh only one dashboard or canonical page without deleting unrelated captures, for example `make visual-screenshots DASHBOARD=themes` or `make visual-screenshots PAGE=theme-components`.

`make visual-docs` stages every managed browser documentation capture by default. Use `VISUAL_DASHBOARD=<name>` or `VISUAL_PAGE=<slug>` for a targeted refresh. The staging pre-check permits newly registered browser-managed images to be absent from `docs/images` so they can be captured and reviewed before promotion; static-managed images, manifest ownership, Markdown references, and unmanaged files remain strict.

`make visual-docs` also supports `VISUAL_IMAGE=<manifest-relative-path>` to stage exactly one browser-managed documentation image. `DASHBOARD`, `PAGE`, and `IMAGE` are mutually exclusive capture scopes, and the selected image must exist in `docs-images.json` with `kind: browser`. For example: `make visual-docs VISUAL_IMAGE=instructions/footer-micro-widgets.png`.

`make visual-docs-promote` promotes reviewed staged captures. Use `VISUAL_DASHBOARD=<name>` or `VISUAL_PAGE=<slug>` to promote the matching visual scope, or `VISUAL_IMAGE=<manifest-relative-path>` to promote exactly one browser-managed image, for example `make visual-docs-promote VISUAL_IMAGE=instructions/footer-micro-widgets.png`. `DASHBOARD`, `PAGE`, and `IMAGE` are mutually exclusive promotion scopes. An `IMAGE` selection must exist in `docs-images.json` and be browser-managed.

Full promotion retains the exact complete-staging-set contract. `make visual-check` remains the strict final contract and requires every managed documentation image to exist in `docs/images`. Diagrams, instructional screenshots, GIFs, and other static-managed media are preserved rather than browser-captured.

`make visual-all` runs the structural check and both screenshot modes.

The runner defaults to `http://127.0.0.1:18080` and `/usr/bin/google-chrome`. Override them with `GLANCE_VISUAL_URL` and `GLANCE_VISUAL_CHROME` when necessary.

Service-dependent widgets intentionally use a closed localhost endpoint where a public deterministic provider is unavailable. This keeps the fixture public/test-safe and provides explicit error/degraded-state coverage. Docker Containers depends on the runtime Docker socket.

## Widget style variants: desktop and mobile documentation

Keep canonical QA widget screenshots separate from documentation images. The
canonical QA mobile screenshot uses the full 430x900 viewport; do not change
that default to crop documentation variants.

For a distinct style variant, add a deterministic widget fixture with a stable
selector in `test-instance.yml`, declare the style in `widget-variants.json`,
and register both images in `docs-images.json`:

```json
"widgets/example-horizontal.png": {
  "kind": "browser", "capture": "element",
  "route": "/appropriate-page", "selector": ".visual-fixture-example-horizontal"
},
"widgets/mobile/example-horizontal.png": {
  "kind": "browser", "capture": "element",
  "route": "/appropriate-page", "selector": ".visual-fixture-example-horizontal",
  "viewport": { "width": 430, "height": 900 },
  "mobileCapture": "element"
}
```

`mobileCapture: "element"` is opt-in and crops to the widget after selecting its
mobile column and scrolling it into view. Without this option, mobile element
recipes preserve the full viewport, including navigation. The contract permits
only declared mobile style variants in `widgets/mobile/` and requires their
Markdown references. Use the canonical image `widgets/mobile/example.png` for
the standard mobile view.

Stage and review each image before promoting it:

```text
make visual-docs VISUAL_IMAGE=widgets/example-horizontal.png
make visual-docs VISUAL_IMAGE=widgets/mobile/example-horizontal.png
make visual-docs-promote VISUAL_IMAGE=widgets/example-horizontal.png
make visual-docs-promote VISUAL_IMAGE=widgets/mobile/example-horizontal.png
make visual-check
```

Staging is under `testdata/visual/docs-staging/`; promotion copies only the
selected reviewed image to the matching path under `docs/images/`. Reference
both desktop and mobile variants in `docs/widgets/example.md`. Run focused
visual capture first, then the project's normal `make check` and `make validate`
gates before integration. The canonical QA screenshots in
`testdata/visual/screenshots/` are separate and need not duplicate every
style variant.

## Decoration runtime profiles

Decorations are global, so their captures use public alternate configurations
under `testdata/decorations/`, each including the unchanged canonical fixture.
Browser documentation recipes may specify `testConfig` pointing to one of
these committed files. The capture runner switches profiles through
`make test-instance-start TEST_CONFIG=...` when required; the outer managed
runner stops the final runtime on success or failure. Ordinary recipes restore
`test-instance.yml` when needed. This works for scoped and aggregate captures.

The production renderer and real configuration are used; no decoration HTML
is injected for screenshots. Captures require all footer artwork to load.
Use viewport page captures (`fullPage: false`) to show fixed edge lights:

    make visual-docs VISUAL_IMAGE=pages/decorations-halloween.png
    make visual-docs-promote VISUAL_IMAGE=pages/decorations-halloween.png

Inspect staged images before promotion. Every built-in design has a profile
and managed documentation image. Do not add decorations to widget registries.

For interactive review:

    make test-instance-start TEST_CONFIG=testdata/decorations/halloween.yml

Open `http://127.0.0.1:18080/screenshots/docs-overview`. Stop it with
`make test-instance-stop` when finished. `make frontend-check` also exercises
the automatic profile on desktop and mobile, controlling the browser clock to
cross midnight and enter a schedule gap. It uses the normal production date
selection with no test-only runtime configuration switches.

Use `TEST_CONFIG=testdata/decorations/halloween-animated.yml` for interactive
Halloween animation review. The ordinary `halloween.yml` profile deliberately
sets `animation: false` to keep documentation captures stable. Reduced-motion
preferences suppress twinkling in either profile.

Stage all decoration images, without touching other documentation captures:

    make visual-docs VISUAL_DECORATIONS=true

After visually reviewing the staged set, promote it with:

    make visual-docs-promote VISUAL_DECORATIONS=true

`VISUAL_DECORATIONS=true` is mutually exclusive with image, page and dashboard
scope. Promotion requires every decoration image to be staged.

## Widget loading lifecycle

Run `make widget-loading-check` to exercise the delayed local fixture on desktop
and mobile. The check covers cold initial loading, static content, grouped
widgets, footer summaries, status bars, a completed provider failure, reduced
motion, and completion before SSE subscription. It uses the canonical Makefile
test runtime and stops that runtime and its fixture server on exit. No production
configuration or external provider is used.

The widget loading check also holds the calculator module pending while asserting
that the page is visible, group tabs and the converter work, and live results arrive.
One case rejects that module to verify failure isolation.
