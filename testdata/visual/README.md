# Glance visual QA fixture

The visual QA configuration mirrors the eight widget categories in `docs/widgets.md`. The separate `Themes` dashboard exercises global inheritance, explicit dark and light page themes, partial overrides, semantic components, and nested composition.

## Contracts

- `glance-test.yml` is the complete local visual QA dashboard.
- `testdata/expanded-theme.yml` owns the expanded global theme fixture.
- `testdata/screenshot-calendar.ics` provides deterministic calendar content.
- `widget-gallery.json` classifies every registered widget exactly once.
- `visual-pages.json` lists the canonical pages captured by browser QA.
- `docs-images.json` explicitly lists documentation PNGs managed by automation.
- `check-gallery.py` fails when registry, page, or docs-image contracts drift.
- `screenshots.js` captures QA pages and documentation images with project-local Playwright and system Google Chrome.

## Commands

`make visual-check` validates structural contracts without opening a browser.

`make visual-screenshots` captures all canonical QA pages into `testdata/visual/screenshots/`. These files are intentionally ignored by Git. Use `DASHBOARD=<name>` or `PAGE=<slug>` to refresh only one dashboard or canonical page without deleting unrelated captures, for example `make visual-screenshots DASHBOARD=themes` or `make visual-screenshots PAGE=theme-components`.

`make visual-docs` stages every managed browser documentation capture by default. Use `DASHBOARD=<name>` or `PAGE=<slug>` for a targeted refresh. The staging pre-check permits newly registered browser-managed images to be absent from `docs/images` so they can be captured and reviewed before promotion; static-managed images, manifest ownership, Markdown references, and unmanaged files remain strict.

`make visual-docs-promote` promotes reviewed staged captures. Use `DASHBOARD=<name>` or `PAGE=<slug>` to promote the matching visual scope, or `IMAGE=<manifest-relative-path>` to promote exactly one browser-managed image, for example `make visual-docs-promote IMAGE=instructions/footer-micro-widgets.png`. `DASHBOARD`, `PAGE`, and `IMAGE` are mutually exclusive promotion scopes. An `IMAGE` selection must exist in `docs-images.json` and be browser-managed.

Full promotion retains the exact complete-staging-set contract. `make visual-check` remains the strict final contract and requires every managed documentation image to exist in `docs/images`. Diagrams, instructional screenshots, GIFs, and other static-managed media are preserved rather than browser-captured.

`make visual-all` runs the structural check and both screenshot modes.

The runner defaults to `http://127.0.0.1:18080` and `/usr/bin/google-chrome`. Override them with `GLANCE_VISUAL_URL` and `GLANCE_VISUAL_CHROME` when necessary.

Service-dependent widgets intentionally use a closed localhost endpoint where a public deterministic provider is unavailable. This keeps the fixture public/test-safe and provides explicit error/degraded-state coverage. Docker Containers depends on the runtime Docker socket.
