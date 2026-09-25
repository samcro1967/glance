# Contributing to Glance

Thank you for contributing to this Glance fork.

This repository is a maintained, substantially extended distribution of upstream Glance. It preserves upstream configuration compatibility and the familiar Glance user experience while maintaining additional functionality, architectural extensions, reliability improvements, operational hardening, diagnostics, regression protection, and controlled development, release, and deployment tooling.

Contributions should preserve upstream compatibility where practical, reuse the existing architecture and shared contracts, and avoid unnecessary divergence. Internal implementation does not need to remain identical to upstream when a change provides concrete functional, reliability, maintainability, observability, security, or regression-protection value.

For the complete record of fork-specific functionality, architecture, compatibility, and lifecycle design, see [About this fork](docs/fork.md).

## Development model

Development follows this integration and release path:

```text
development branch → dev → main → formal release → explicit production deployment → main-to-dev synchronization
```

`dev` is the protected integration branch for ongoing development. `main` is the protected stable and release-ready branch.

Normal feature, fix, refactor, and documentation work should:

1. start from the appropriate current `dev` state;
2. use a focused development branch;
3. return to `dev` through a pull request;
4. pass the required local and CI validation before merge.

Direct development on `dev` or `main` is intentionally avoided.

Local `dev` may intentionally contain committed work that has not yet been pushed when that work is being parked for inclusion in the next feature pull request. The repository Makefile understands this state and protects against starting work from a behind or diverged `dev` history.

Feature integration, promotion from `dev` to `main`, formal release creation, production deployment, and post-release main-to-dev synchronization are distinct lifecycle stages. Completing one stage does not implicitly authorize the next.

The repository also provides explicit high-level workflows that deliberately compose multiple guarded stages. In particular, `make ship` is the normal end-to-end workflow for runtime and code changes when the complete feature-to-production lifecycle has been intentionally requested. Its invocation explicitly authorizes the documented stages it composes, subject to their validation and safety checks. `make ship-nonruntime` provides the corresponding guarded high-level workflow for qualifying non-runtime changes without creating a formal release or deploying production.

## Use the Makefile first

The repository `Makefile` is the authoritative interface for normal development, validation, repository inspection, pull-request operations, CI monitoring, visual QA, release management, deployment, and lifecycle recovery.

Prefer an existing Makefile target over manually reproducing the same operation with Git, GitHub CLI, Go, Docker, Compose, or other commands.

Run:

```text
make help
```

to see the currently supported workflow and target contracts.

Do not infer what a workflow target does from its name alone when its behavior matters. Review the current Makefile help or target implementation before relying on assumptions about lifecycle boundaries, validation, cleanup, publication, or deployment.

Common development targets include:

```text
make test
make test-race
make test-focused
make fuzz FUZZ=FuzzName FUZZTIME=30s
make fuzz-all FUZZTIME=10s
make build
make lint
make check
make validate
make validate-all
make lighthouse
make coverage
make benchmark
make performance-check
make performance
make performance-runtime PAGE=<page>
make pprof-capture PROFILE=<profile>
make pprof-summary PROFILE=<profile>
make vuln
make status
make staged-diff

make test-instance-start
make test-instance-status
make test-instance-stop

make test-prod-start
make test-prod-status
make test-prod-stop
make test-prod-config-refresh

make frontend-audit
make frontend-check
make frontend-coverage

make visual-check
make visual-screenshots
make visual-final
```

Some test and runtime targets support Makefile variables or aliases for focused validation and isolated runtime configuration. Prefer those supported interfaces over manually reproducing the underlying Go, Docker, or configuration operations.

Branch, pull-request, CI, release, deployment, synchronization, and recovery operations are also guarded by Makefile targets. See [Development and CI validation](docs/fork.md#development-and-ci-validation) for the complete lifecycle.

## Implementation expectations

Investigate the existing implementation before changing it.

Prefer the smallest clean change that fits the existing Glance architecture. Reuse existing primitives, helpers, lifecycle contracts, presentation components, CSS, JavaScript infrastructure, configuration patterns, HTTP infrastructure, authentication/session infrastructure, and provider/resource abstractions before creating new ones.

Changes should:

- preserve existing behavior unless changing it is an explicit part of the work;
- preserve backward-compatible configuration wherever practical;
- avoid unnecessary divergence from upstream;
- avoid parallel implementations when an existing shared abstraction can be extended cleanly;
- keep errors and degraded states visible rather than silently swallowing failures;
- consider refresh behavior, cancellation, reloads, stale data, recovery, concurrency, live replacement, and lifecycle ownership where relevant;
- preserve established authentication, authorization, session, proxy-trust, and security boundaries when relevant;
- avoid unrelated cleanup unless it is necessary for the implementation;
- avoid new dependencies unless they provide a clear benefit that cannot reasonably be achieved with the existing stack.

For regressions, establish the expected behavior, current behavior, and likely cause or regression point before implementing a fix.

For broader architectural work, investigate sufficiently to understand existing ownership boundaries and similar implementations before introducing or extending an abstraction. Prefer evidence of duplication, inconsistency, failure risk, or maintainability cost over speculative centralization.

### Adding a new widget

New widgets must follow the repository-specific implementation, presentation, fixture, testing, documentation, and visual-QA contracts described in [Adding a widget](docs/adding-a-widget.md). Use that guide as the implementation checklist while retaining the general contribution requirements in this document.

## Testing

Behavior changes and defect fixes should include regression coverage when applicable.

Start with focused validation for the code being changed, then run the broader repository validation required before a pull request.

The standard local pre-pull-request suite is:

```text
make check
```

This includes the Go test suite, race testing, build validation, formatting and whitespace checks, documentation validation, correctness-oriented Go static analysis, frontend architecture auditing, and other maintained fast validation contracts.

For comprehensive release-gate validation, use:

```text
make validate
```

`make validate` is the authoritative release gate. It extends `make check` with deterministic browser regression testing, visual QA contract validation, and Go vulnerability analysis.

For deeper informational engineering analysis, use:

```text
make validate-all
```

`make validate-all` extends the authoritative release gate with Go coverage, frontend execution coverage, benchmarks, and Lighthouse analysis. Those measurements and Lighthouse scores are engineering evidence; they are not release thresholds unless a threshold is explicitly established elsewhere.

`make lighthouse` can also be run independently against the deterministic test instance. It reports category scores and accessibility findings while cleaning up its temporary report and test runtime.

Use:

```text
make test
```

for the normal Go test suite and:

```text
make test-race
```

when concurrency-sensitive behavior is involved.

For a focused Go test selection, use the maintained focused-test interface rather than manually reconstructing the package invocation when it fits the task:

```text
make test-focused TEST_RUN='<test expression>'
```

Focused testing is useful while developing or diagnosing a specific subsystem, including authentication and OIDC behavior, but it does not replace the broader validation required before integration.

Different regression tools protect different failure classes and should be used where they fit the changed behavior. The race detector identifies unsafe concurrent memory access; repeated and soak-style tests help expose intermittent or workload-driven failures; `go.uber.org/goleak` detects goroutines that survive beyond their intended ownership boundary; and native Go fuzzing exercises parsers, codecs, normalization, policy boundaries, and other input-sensitive logic with malformed or unexpected inputs. These mechanisms complement deterministic regression tests rather than replacing them.

For focused native fuzzing, use:

```text
make fuzz FUZZ=FuzzName FUZZTIME=30s
```

Use `FUZZ_PACKAGE=<package>` when the maintained target is outside the default `./internal/glance` package. To exercise all maintained fuzz targets sequentially with a bounded per-target duration, use:

```text
make fuzz-all FUZZTIME=10s
```

Active time-budgeted fuzzing is intentionally not part of `make check` or `make validate`. Ordinary Go test execution still runs committed fuzz seed corpora as deterministic regression coverage. When fuzzing discovers a production defect, add a deterministic regression test for the minimized failure rather than relying on a local fuzz cache for permanent protection.

Changes that introduce or alter goroutine ownership, cancellation, schedulers, shared background work, server or runtime lifecycle, live updates, or similar asynchronous behavior should include or update goleak coverage when that provides a meaningful ownership-boundary assertion.

Repeated test targets are available where a single execution is insufficient evidence for concurrency-sensitive or intermittent behavior.

A successful compile, static-analysis run, or one passing test is not sufficient evidence for a runtime behavior change. Validate the behavior that actually changed.

## Authentication and authorization changes

Authentication is a security boundary and requires validation beyond successful login.

Glance supports its local username/password authentication mechanism and fork-specific OpenID Connect (OIDC) authentication. These mechanisms can coexist and share the established Glance session model.

Changes affecting authentication, authorization, sessions, OIDC, login/logout behavior, trusted proxies, secure-cookie decisions, or authenticated presentation should preserve the existing shared authentication architecture rather than creating independent session or authorization mechanisms.

When changing authentication behavior, validate the relevant contract end to end, including where applicable:

- local authentication compatibility;
- OIDC provider discovery and initialization;
- authorization callback validation;
- state, nonce, and PKCE protections;
- ID-token verification;
- configured identity restrictions such as `allowed-users`;
- session creation, validation, expiration, and invalidation;
- login and logout behavior;
- coexistence of local and OIDC authentication;
- current-user presentation;
- trusted-proxy and HTTPS behavior;
- configuration reload behavior;
- behavior of existing sessions across configuration changes;
- denied and malformed authentication attempts;
- logging and diagnostic behavior on authentication failures.

Authentication tests and diagnostics must use synthetic or intentionally non-sensitive fixtures. Do not commit real client secrets, authorization codes, tokens, cookies, session material, personal identities, or generated local authentication configuration.

OIDC diagnostics should identify the stage and bounded reason for a failure without logging authorization codes, tokens, session material, OIDC subjects, email addresses, claims payloads, PKCE verifiers, client secrets, or other sensitive provider data.

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

When changing live or dynamically initialized frontend behavior, consider initial rendering, navigation, refreshes, repeated live replacements, reconnects, cleanup, cancellation, resource ownership, and browser errors where relevant.

Live-update and Server-Sent Events behavior must also account for idle connections and intermediary timeouts. Long-lived streams should retain explicit liveness behavior, and client recovery must remain bounded so that a silently dead connection cannot leave the page indefinitely disconnected from live updates. Validate initial connection, idle periods, reconnects, page navigation, repeated updates, cleanup, and recovery where relevant.

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

The maintained `test-instance.yml` fixture exercises all registered widget types, hierarchical defaults and overrides, themes, semantic presentation components, layout composition, and deterministic content used by browser and visual regression testing.

The Makefile owns the fixture server and test-instance lifecycle. Do not manually reproduce that environment when the maintained targets provide the required test.

## Production-runtime validation

Some changes benefit from validation against the real production configuration, assets, networks, environment, and data while still testing the current local source.

For those cases, use the isolated production-runtime workflow:

```text
make test-prod-start
make test-prod-status
make test-prod-stop
```

This builds the current source into an isolated test image and uses the production container as its runtime reference. It does not replace the running production container.

For performance investigations against that production-representative environment, use `make performance-runtime PAGE=<page>`. The maintained workflow correlates the browser performance snapshot with backend rendering, refresh synchronization, widget refresh activity, and outbound HTTP diagnostics from the same isolated runtime. Prefer this correlated evidence over manually constructing a runtime or inferring an application bottleneck from a single timing source.

The production-runtime workflow also supports maintained configuration and environment overrides for cases where a feature must be exercised without modifying the real production configuration. Use the Makefile-supported override interfaces rather than manually constructing an alternate Docker runtime.

Relevant interfaces include the production-test configuration refresh target and supported test-runtime configuration/environment variables. These allow features such as OIDC authentication to be exercised against the isolated production-like runtime while keeping test-only credentials and configuration outside the committed repository.

When changing an override file while the isolated runtime is active, use:

```text
make test-prod-config-refresh
```

where appropriate so the generated test configuration is refreshed through the maintained workflow rather than manually editing generated runtime files.

Authentication runtime fixtures, environment files, generated test configuration, provider credentials, and other local test secrets must remain untracked and must not be committed.

The canonical source test instance and production-runtime test use the same isolated test port and therefore must be used mutually exclusively.

After a change has integrated into `dev`, the published `dev` image can be validated against the real runtime through the corresponding `test-container-*` Makefile workflow.

## Documentation

Keep documentation synchronized with user-visible configuration, behavior, architecture, and development contracts.

When applicable:

- update widget documentation for widget configuration or behavior changes;
- update [Configuration](docs/configuration.md) for shared or top-level configuration changes, including authentication and OIDC configuration;
- update [Themes](docs/themes.md) for public theme behavior;
- update [About this fork](docs/fork.md) when fork-specific capabilities, architecture, compatibility contracts, authentication behavior, or lifecycle behavior materially change;
- update this contributing guide when supported development, testing, security, or Makefile workflow contracts materially change;
- update the main [README](README.md) when the high-level identity, supported capabilities, installation model, or user-facing project overview materially changes;
- update generated documentation screenshots through the visual workflow when presentation changes affect them.

Do not duplicate detailed reference material across documents unnecessarily. Prefer linking to the authoritative document for a subject.

Avoid embedding volatile counts, percentages, benchmark values, or other measurements in general documentation unless the value itself represents a maintained contract. Prefer describing the underlying guarantee or measurement process when exact values are expected to evolve.

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

Before committing authentication, security, or runtime-fixture changes, also verify that local environment files, credentials, generated configuration, tokens, cookies, provider identities, and other sensitive or machine-specific artifacts have not entered the staged change.

Keep commits and pull requests focused on the agreed problem.

## Pull requests and CI

Normal development changes return to `dev` through a pull request.

Use the Makefile-managed branch, push, pull-request, CI, merge, cleanup, promotion, release, deployment, synchronization, and recovery workflows rather than manually reproducing them when the corresponding targets are available.

Pull requests targeting protected branches are validated through the repository's Makefile validation contract. Required CI must succeed before merge.

For incremental work, a feature merge into `dev` completes the normal feature-integration stage. Promotion to `main`, formal release creation, production deployment, and main-to-dev synchronization remain separate explicit lifecycle stages and should not be performed merely because the preceding stage succeeded.

When the complete lifecycle has explicitly been requested, `make ship` is the established high-level workflow and intentionally proceeds through its documented feature integration, promotion, release, production deployment, synchronization, and final-verification stages unless a validation, safety check, or other failure stops it. Do not artificially split an explicitly requested `make ship` workflow into manual lifecycle stages.

For qualifying non-runtime changes, use the documented `make ship-nonruntime` workflow rather than creating an unnecessary formal runtime release or production deployment.

If a high-level workflow stops, do not blindly rerun it. Use:

```text
make workflow-status
```

to establish the current repository, pull-request, CI, image, release, deployment, and synchronization state before choosing the appropriate Makefile recovery target.

See [Development and CI validation](docs/fork.md#development-and-ci-validation) for the complete branch, pull-request, release, deployment, synchronization, and recovery lifecycle.

## Security and dependencies

Avoid exposing credentials, authentication headers, cookies, tokens, authorization codes, OIDC claims or identities, configured sensitive URLs, response bodies, or other secrets through logs, diagnostics, errors, tests, fixtures, generated configuration, or committed environment files.

Reuse the repository's existing sanitization, bounded diagnostic classification, authentication/session infrastructure, and error-boundary behavior when adding diagnostics or provider integrations.

Security-sensitive failures should remain diagnosable without exposing sensitive values. Prefer bounded stage, reason, provider, operation, or failure classifications over raw errors when a raw error may contain credentials, tokens, identities, claims, request parameters, or other sensitive material.

Authentication changes should preserve the established encrypted session model, authorization boundaries, trusted-proxy handling, and secure-cookie behavior unless changing one of those contracts is explicitly part of the work.

New dependencies should be justified by a clear architectural or functional need and should not duplicate functionality already available through the existing stack.

Security and dependency validation available through the repository Makefile should be used when relevant to the change.

## Upstream compatibility

This fork intentionally continues to track upstream Glance while preserving upstream configuration compatibility as an explicit project goal.

Avoid unnecessary divergence. Where practical, changes should preserve existing configuration and user-facing behavior and fit established Glance concepts so future upstream synchronization remains manageable. Internal architecture may evolve beyond upstream where doing so provides concrete functional, reliability, maintainability, observability, security, or regression-protection value.

When functionality is derived from an upstream pull request, issue, or another Glance-derived project, preserve appropriate provenance in the fork documentation.

The goal is not to change functioning areas solely for abstraction, cleanup, or coverage. Changes should address required functionality, observed defects, maintainability needs with concrete benefit, worthwhile upstream work, security or dependency maintenance, or meaningful regression protection.
