# Contributing to Glance

Thank you for contributing to this Glance fork.

This repository tracks upstream Glance while maintaining additional functionality, reliability improvements, operational hardening, regression protection, and release/deployment tooling. Contributions should preserve upstream compatibility where practical, reuse the existing architecture, and avoid unnecessary divergence.

For the complete record of fork-specific functionality and lifecycle architecture, see [About this fork](docs/fork.md).

## Development model

Development follows this integration and release path:

```text
development branch → dev → main → formal release → explicit production deployment
```

`dev` is the protected integration branch for ongoing development. `main` is the protected stable and release-ready branch.

Normal feature, fix, refactor, and documentation work should:

1. start from the appropriate current `dev` state;
2. use a focused development branch;
3. return to `dev` through a pull request;
4. pass the required local and CI validation before merge.

Direct development on `dev` or `main` is intentionally avoided.

Local `dev` may intentionally contain committed work that has not yet been pushed when that work is being parked for inclusion in the next feature pull request. The repository Makefile understands this state and protects against starting work from a behind or diverged `dev` history.

Promotion from `dev` to `main`, formal release creation, and production deployment are separate lifecycle stages. Merging a feature does not imply promotion, release, or deployment.

## Use the Makefile first

The repository `Makefile` is the authoritative interface for normal development, validation, repository inspection, pull-request operations, CI monitoring, visual QA, release management, and deployment.

Prefer an existing Makefile target over manually reproducing the same operation with Git, GitHub CLI, Go, Docker, Compose, or other commands.

Run:

```text
make help
```

to see the currently supported workflow.

Common development targets include:

```text
make test
make test-race
make build
make check
make coverage
make vuln
make status
make staged-diff

make test-instance-start
make test-instance-status
make test-instance-stop

make frontend-audit
make frontend-check
make frontend-coverage

make visual-check
make visual-screenshots
make visual-final
```

Branch, pull-request, CI, release, and deployment operations are also guarded by Makefile targets. See [Development and CI validation](docs/fork.md#development-and-ci-validation) for the complete lifecycle.

## Implementation expectations

Investigate the existing implementation before changing it.

Prefer the smallest clean change that fits the existing Glance architecture. Reuse existing primitives, helpers, lifecycle contracts, presentation components, CSS, JavaScript infrastructure, configuration patterns, and provider/resource abstractions before creating new ones.

Changes should:

- preserve existing behavior unless changing it is an explicit part of the work;
- preserve backward-compatible configuration wherever practical;
- avoid unnecessary divergence from upstream;
- avoid parallel implementations when an existing shared abstraction can be extended cleanly;
- keep errors and degraded states visible rather than silently swallowing failures;
- consider refresh behavior, cancellation, reloads, stale data, recovery, and concurrency where relevant;
- avoid unrelated cleanup unless it is necessary for the implementation;
- avoid new dependencies unless they provide a clear benefit that cannot reasonably be achieved with the existing stack.

For regressions, establish the expected behavior, current behavior, and likely cause or regression point before implementing a fix.

## Testing

Behavior changes and defect fixes should include regression coverage when applicable.

Start with focused validation for the code being changed, then run the broader repository validation required before a pull request.

The standard local pre-pull-request suite is:

```text
make check
```

This includes the Go test suite, race testing, build validation, whitespace checks, documentation validation, and maintained architecture audits.

Use:

```text
make test
```

for the normal Go test suite and:

```text
make test-race
```

when concurrency-sensitive behavior is involved.

A successful compile or one passing test is not sufficient evidence for a runtime behavior change. Validate the behavior that actually changed.

## Frontend changes

Frontend changes require validation beyond Go tests.

Use:

```text
make frontend-check
```

for deterministic browser regression testing. The maintained browser scenarios cover desktop and mobile behavior, navigation, theme interaction, page recovery, live widget replacement, disclosures, contained frontend failures, authentication recovery, initialization, and browser error detection.

Use:

```text
make frontend-audit
```

to validate frontend architecture contracts, including semantic theme ownership and frontend diagnostic conventions.

`make frontend-coverage` runs the maintained browser scenarios with Chromium/V8 execution coverage for Glance-owned JavaScript. This coverage is informational and is intended to identify meaningful unexercised frontend code; it is not a percentage threshold for accepting changes.

When changing live or dynamically initialized frontend behavior, consider initial rendering, navigation, refreshes, repeated live replacements, reconnects, cleanup, and browser errors where relevant.

Do not introduce a JavaScript package-management or build pipeline such as `package.json` unless the project architecture is deliberately changed to require one.

## Themes and presentation

Glance owns the dashboard visual language.

Use the native semantic theme hierarchy and shared presentation primitives for concepts that are genuinely common across widgets. Avoid introducing widget-specific hard-coded colors, borders, spacing, controls, cards, tables, status presentation, or interaction behavior when an established Glance semantic primitive already represents the same concept.

Different widgets may present different information without inventing unrelated visual systems.

Specialized structures should remain specialized where forcing them through a generic primitive would harm their information model or usability. Examples include calendars, weather visualization, charts, clocks, calculator geometry, media layouts, thumbnails, and other widget-specific responsive structures.

Theme and presentation changes should preserve compatibility for configurations that omit new optional properties.

See [Themes](docs/themes.md) for the public theme contract.

## Visual QA

UI and presentation changes should use the repository's canonical visual-QA system rather than unrelated manual screenshots.

The visual registry covers every registered widget and maintains canonical page and isolated-widget screenshots.

Use:

```text
make visual-check
```

to validate the visual registry, screenshot mappings, visual pages, and documentation-image ownership.

During focused development, the visual tooling supports scoped page or dashboard capture where appropriate. The unqualified workflow remains the complete validation path.

For final visual validation of a presentation change, use:

```text
make visual-final
```

`make visual-final` performs the aggregate canonical visual capture, stages browser-managed documentation screenshots, promotes the generated documentation images, and verifies the final visual contract.

Review the resulting image changes as part of the code review. Generated screenshots should not be accepted merely because the capture command succeeded.

## Documentation screenshots

Browser-managed documentation screenshots are generated from the deterministic Glance test fixture. Do not manually maintain replacements for images owned by that workflow.

Managed browser screenshots live under:

```text
docs/images/widgets
docs/images/pages
docs/images/themes
```

The visual workflow also owns configured browser-generated example and instruction images where applicable.

Instruction images that cannot be generated from the Glance fixture remain statically managed under:

```text
docs/images/instructions
```

`make visual-docs` stages browser-managed documentation images without modifying `docs/images`. `make visual-docs-promote` promotes the staged set. For normal finalization of UI work, prefer `make visual-final`, which captures, promotes, and verifies the complete managed set in one workflow.

## Test instances

For deterministic development and browser validation, use the canonical source test instance:

```text
make test-instance-start
make test-instance-status
make test-instance-stop
```

The maintained `glance-test.yml` fixture exercises all registered widget types, hierarchical defaults and overrides, themes, semantic presentation components, layout composition, and deterministic content used by browser and visual regression testing.

The Makefile owns the fixture server and test-instance lifecycle. Do not manually reproduce that environment when the maintained targets provide the required test.

## Production-runtime validation

Some changes benefit from validation against the real production configuration, assets, networks, environment, and data while still testing the current local source.

For those cases, use the isolated production-runtime workflow:

```text
make test-prod-start TEST_RUNTIME_CONTAINER=<container>
make test-prod-status
make test-prod-stop
```

This builds the current source into an isolated test image and uses the named production container as its runtime reference. It does not replace the running production container.

The canonical source test instance and production-runtime test use the same isolated test port and therefore must be used mutually exclusively.

After a change has integrated into `dev`, the published `dev` image can be validated against the real runtime through the corresponding `test-container-*` Makefile workflow.

## Documentation

Keep documentation synchronized with user-visible configuration, behavior, and development contracts.

When applicable:

- update widget documentation for widget configuration or behavior changes;
- update [Configuration](docs/configuration.md) for shared or top-level configuration changes;
- update [Themes](docs/themes.md) for public theme behavior;
- update [About this fork](docs/fork.md) when fork-specific capabilities or architecture materially change;
- update generated documentation screenshots through the visual workflow when presentation changes affect them.

Do not duplicate detailed reference material across documents unnecessarily. Prefer linking to the authoritative document for a subject.

Documentation is validated as part of the repository's standard validation workflow.

## Review before commit

Before committing, inspect the complete intended change.

Use the repository validation and diff targets where applicable, including:

```text
make diff-check
make staged-check
make staged-diff
```

Do not treat generated files, screenshots, formatting changes, or broad mechanical edits as correct solely because they were produced automatically. Review them as part of the change.

Keep commits and pull requests focused on the agreed problem.

## Pull requests and CI

Normal development changes return to `dev` through a pull request.

Use the Makefile-managed branch, push, pull-request, CI, merge, and cleanup workflows rather than manually reproducing them when the corresponding targets are available.

Pull requests targeting protected branches are validated by CI. Required CI must succeed before merge.

A feature merge into `dev` is the end of the normal development integration stage. Promotion to `main`, formal release creation, and production deployment remain separate explicit stages.

See [Development and CI validation](docs/fork.md#development-and-ci-validation) for the complete branch and pull-request lifecycle.

## Security and dependencies

Avoid exposing credentials, authentication headers, cookies, tokens, configured sensitive URLs, response bodies, or other secrets through logs, diagnostics, errors, tests, or fixtures.

Reuse the repository's existing sanitization and error-boundary behavior when adding diagnostics or provider integrations.

New dependencies should be justified by a clear architectural or functional need and should not duplicate functionality already available through the existing stack.

Security and dependency validation available through the repository Makefile should be used when relevant to the change.

## Upstream compatibility

This fork intentionally continues to track upstream Glance.

Avoid unnecessary divergence. Where practical, changes should fit existing upstream architecture and preserve compatibility so future upstream synchronization remains manageable.

When functionality is derived from an upstream pull request, issue, or another Glance-derived project, preserve appropriate provenance in the fork documentation.

The goal is not to change functioning areas solely for abstraction, cleanup, or coverage. Changes should address required functionality, observed defects, maintainability needs with concrete benefit, worthwhile upstream work, security/dependency maintenance, or meaningful regression protection.
