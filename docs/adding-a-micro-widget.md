# Adding a micro-widget

[Glance README](../README.md) · [Contributing](../CONTRIBUTING.md) · [Adding a widget](adding-a-widget.md) · [Configuration](configuration.md)

---

Micro-widgets are compact presentation sources that can be used in either the Status Bar or the footer micro-widget layer. The two containers own placement and layout; a micro-widget owns its configuration, data lifecycle, and compact presentation semantics.

## 1. Use the shared registry

`internal/glance/micro-widget.go` contains `microWidgetRegistry`, the authoritative registry of micro-widget types. Register a type exactly once. A registered type is eligible in both the Status Bar and footer; do not add container-specific allow-lists or type switches.

Every registered micro-widget implements the shared `microWidget` contract. Footer placement uses `GetPosition`, while both containers consume the shared `MicroItems` presentation contract. Footer rendering uses the normal widget render path.

## 2. Reuse canonical widget ownership

When a micro-widget corresponds to a native widget, embed or delegate to that native widget rather than duplicating its provider, configuration, update, cache, stale/error, or recovery logic. The micro type should contain only compact-presentation or container-placement options that do not belong to the canonical widget.

For example, Weather, Markets, Monitor, Docker, RSS, and Custom API micro-widgets reuse their native widget implementations. Their registry descriptors also identify the canonical widget type used for widget-default and capability resolution; this is especially important when the micro name differs from the native widget name, such as `docker` and `docker-containers`. A micro implementation must not introduce a second provider fetch/update path merely to render compactly.

Lightweight micro-only sources such as Bookmark, Link, and Clock may remain self-contained when there is no provider-backed lifecycle to share.

## 3. Keep containers presentation-only

Status Bar and Footer must not inspect concrete micro-widget Go types. They iterate the shared micro contract. Adding a new registered micro-widget must not require adding the type to either container.

Container-specific behavior remains container-owned. Examples include Status Bar ticker/wrap behavior and link policy, and Footer left/right placement, position limits, responsive hiding, and footer visibility.

## 4. Preserve configuration compatibility

Existing Status Bar child configuration and `footer-micro-widgets` YAML are public contracts. Refactors must preserve existing valid configuration unless an intentional compatibility change is separately approved.

Footer micro-widgets require `position`; Status Bar children retain configuration order and do not require a position.

## 5. Preserve lifecycle semantics

Provider-backed micro-widgets participate in the normal widget lifecycle: defaults, initialization, providers, refresh scheduling, stale/degraded state, error recovery, reload-state reuse, diagnostics, and resource proxying. Do not create a parallel scheduler or reload mechanism for micro-widgets.

Static micro-widgets should use infinite/no-refresh behavior and must not create unnecessary scheduled work.

## 6. Rendering

Micro-widget data/configuration is shared, but Status Bar and Footer presentation may differ. Do not force both containers into identical markup or density. Reuse semantic CSS primitives only when they are genuinely shared.

A new micro-widget must provide useful compact presentation in both containers before it is registered.

## 7. Tests

Add focused coverage for:

- registry construction and the invariant that every registered type is accepted by both containers;
- YAML decoding and configuration validation;
- canonical widget data/update reuse for provider-backed types;
- Footer ordering and capacity behavior;
- Status Bar ordering and ticker/wrap behavior;
- error, stale/degraded, and recovery presentation;
- reload-state reuse and refresh registration where applicable;
- resource proxy and link behavior where applicable;
- browser rendering in both containers, including mobile/overflow checks.

Run focused validation first, then `make check` and `make validate` at the appropriate stages. UI changes require rendered browser validation.

## 8. Documentation

Update the Footer and Status Bar documentation when the new type adds user-facing configuration. Keep this guide current when the shared micro-widget contract or registry changes.

---

[Adding a widget](adding-a-widget.md) · [Configuration](configuration.md) · [Back to top](#adding-a-micro-widget)
