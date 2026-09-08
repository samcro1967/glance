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

`make visual-docs` stages every managed documentation capture by default. Use the same `DASHBOARD=<name>` or `PAGE=<slug>` selectors for a targeted refresh. `make visual-docs-promote` accepts the same selectors and promotes only matching staged images during a selective run. Full promotion retains the exact complete-staging-set contract. Diagrams, instructional screenshots, GIFs, and other unmapped media remain manually maintained.

`make visual-all` runs the structural check and both screenshot modes.

The runner defaults to `http://127.0.0.1:18080` and `/usr/bin/google-chrome`. Override them with `GLANCE_VISUAL_URL` and `GLANCE_VISUAL_CHROME` when necessary.

Service-dependent widgets intentionally use a closed localhost endpoint where a public deterministic provider is unavailable. This keeps the fixture public/test-safe and provides explicit error/degraded-state coverage. Docker Containers depends on the runtime Docker socket.
