<p align="center"><img src="docs/logo.png"></p>
<h1 align="center">Glance</h1>
<p align="center">
  <a href="#installation">Install</a> •
  <a href="docs/configuration.md#configuring-glance">Configuration</a> •
  <a href="https://github.com/sponsors/glanceapp">Sponsor</a>
</p>
<p align="center">
  <a href="https://github.com/glanceapp/community-widgets">Community widgets</a> •
  <a href="docs/preconfigured-pages.md">Preconfigured pages</a> •
  <a href="docs/themes.md">Themes</a>
</p>

<p align="center">A lightweight, highly customizable dashboard that displays<br> your feeds in a beautiful, streamlined interface</p>

![](docs/images/readme-main-image.png)

## About this fork

This repository is a fork of [Glance](https://github.com/glanceapp/glance) maintained by [samcro1967](https://github.com/samcro1967/glance).

It tracks the upstream Glance project while incorporating additional functionality, reliability fixes, operational hardening, expanded regression testing, and deployment tooling not currently available in the upstream release.

The goal of the fork is to remain compatible with upstream Glance where practical while providing additional functionality and addressing reliability or operational issues encountered in production use. Changes incorporated from existing upstream pull requests or other Glance-derived projects are identified below where applicable.

**Fork module identity** — Uses `github.com/samcro1967/glance` as the Go module and build identity while retaining `github.com/glanceapp/glance` as the upstream project for attribution and ongoing upstream synchronization.

### Changes from upstream

- **Stack widget** — Adds the `stack` widget from upstream [PR #765](https://github.com/glanceapp/glance/pull/765), allowing multiple widgets to be stacked vertically and treated as a single widget.
- **Analog clock widget** — Adds an `analog-clock` widget based on upstream [PR #747](https://github.com/glanceapp/glance/pull/747), providing a theme-aware analog clock with optional date and AM/PM indicators, configurable numerical dial markers, and additional clocks for configured timezones.
- **Nested groups** — Allows `group` widgets to contain other `group` widgets, enabling multiple levels of tabbed navigation.
- **Timer widget** — Adds a browser-local `timer` widget for creating countdowns to specific dates and times. Timers can be added, edited, deleted, and reordered directly in the widget, persist in the browser's local storage, and update automatically as their target times approach or pass.
- **ICS Events widget** — Adds a native `ics-events` widget for displaying events from iCalendar (ICS) feeds. The widget fetches and parses calendar data server-side, supports configurable date ranges and event limits, handles recurring events and timezone-aware dates, and participates in the standard Glance caching, refresh, recovery, and live-update lifecycle.
- **Markdown widget** — Adds a native `markdown` widget for rendering configured Markdown content using Glance styling. Markdown is rendered server-side with HTML sanitization and supports standard Markdown formatting without requiring an external service or Custom API endpoint.
- **Named dashboards** — Allows configured pages to be organized into multiple independently addressable dashboards with dashboard-specific navigation. Pages are defined once and can be shared across dashboards while retaining the same underlying widget state, caching, and update lifecycle. When dashboards are configured, the application logo provides a dashboard switcher for quickly moving between dashboards, with the current dashboard indicated in the menu. Named dashboards whose generated slugs conflict with existing page slugs are ignored with a warning, allowing the remaining dashboards and pages to continue operating normally. Existing configurations without `dashboards` continue to use the standard Glance page and logo behavior.
- **Medium page columns** — Adds a `medium` page column size for proportional desktop layouts. Three `medium` columns create an equal three-column layout, while `medium` + `full` creates an approximately one-third/two-thirds layout and can be reversed as `full` + `medium`. Existing `small` and `full` layouts remain unchanged, and medium columns use the standard one-column-at-a-time navigation on mobile.
- **Bottom widgets** — Adds a `bottom-widgets` page section that mirrors `head-widgets`, allowing standard Glance widgets to span the full page width below the configured columns. Bottom widgets participate in the same initialization, provider setup, refresh, caching, and recovery lifecycle as widgets elsewhere on the page. Existing configurations without `bottom-widgets` remain unchanged.
- **Custom API timeouts** — Adds configurable request timeouts to `custom-api` widgets based on upstream [PR #997](https://github.com/glanceapp/glance/pull/997), with independent timeout settings for primary requests and subrequests.
- **Custom API string helpers** — Adds template helpers for common string operations in `custom-api` widgets, including case conversion, substring checks, prefix and suffix checks, case-insensitive comparison, splitting, and joining.
- **Custom API dynamic request methods** — Adds `withMethod` for dynamically created Custom API requests, allowing templates to explicitly select the HTTP request method while preserving the existing GET default and body-implies-POST behavior.
- **Custom API stale fallback** — Preserves the last successfully rendered `custom-api` content when a refresh fails, displays a visible stale indicator with the age of the last successful update, and automatically clears the stale state after a successful refresh.
- **Automatic widget recovery** — Adds server-side background refresh and recovery for updateable widgets. Expired or previously failed widgets are retried automatically without requiring a page to be open or manually refreshed. Refreshes are synchronized per widget to prevent duplicate concurrent updates while preserving parallel updates across different widgets, use bounded concurrency and progressive retry backoff after failures, and are cancelled cleanly during shutdown and configuration reloads. Normal successful refresh timing continues to respect each widget's configured cache duration. The `custom-api` widget additionally preserves its last successfully rendered content while a refresh is failing.
- **Live widget updates** — Addresses upstream [issue #1042](https://github.com/glanceapp/glance/issues/1042) by automatically updating visible widgets in the browser when their server-side refresh completes, without requiring a full page reload. Live updates reuse the existing widget refresh scheduler and cache timing rather than introducing a separate polling cycle. The server sends lightweight widget ID notifications over Server-Sent Events (SSE), and the browser fetches and replaces only the affected widget while preserving surrounding page and container state such as selected group tabs. Nested refreshable widgets are supported, concurrent notifications are coalesced safely, and browser-side behaviors are reinitialized only within replaced widget content. The feature requires no additional configuration and falls back to existing static page behavior when SSE is unavailable.
- **Versioned asset base URL fix** — Corrects versioned asset paths when Glance is configured with a base URL, ensuring assets such as the web manifest resolve beneath the configured base path instead of producing malformed paths or HTTP 404 responses.
- **HTTP server startup failure handling** — Incorporates upstream [PR #1047](https://github.com/glanceapp/glance/pull/1047), causing Glance to exit with a nonzero status when the HTTP server fails to start instead of remaining running in a broken state. This allows container restart policies and external monitoring to correctly detect and respond to startup failures.
- **Mountpoint CLI fix** — Incorporates upstream [PR #1065](https://github.com/glanceapp/glance/pull/1065), fixing the `mountpoint:info <path>` command so it can be invoked as documented instead of being rejected as an unknown command.
- **IPv6 Docker remote sources** — Incorporates upstream [PR #1064](https://github.com/glanceapp/glance/pull/1064), correctly formatting IPv6 addresses used by remote Docker sources while preserving TCP, HTTP, HTTPS, explicit-port, and default-port behavior.
- **YAML comment variable parsing** — Incorporates upstream [PR #965](https://github.com/glanceapp/glance/pull/965), preventing configuration variables inside YAML comments from being expanded while correctly preserving hashes inside quoted values and handling escaped or doubled quotes.
- **Search autofocus fix** — Incorporates upstream [PR #885](https://github.com/glanceapp/glance/pull/885), ensuring search widgets configured with `autofocus` reliably receive focus after dynamic initialization, including in browsers such as Firefox.
- **Structured operational logging and diagnostics** — Adds structured lifecycle, failure, recovery, and configuration diagnostics for improved container and service observability. Glance logs meaningful application, HTTP server, and background refresh scheduler lifecycle events; reports widget failure or degraded transitions with actionable causes without repeatedly logging the same failed state; and records when affected widgets recover. Configuration errors include source-aware file and line information where available, including included YAML files, semantic configuration errors, widget initialization failures, configuration variables, and Custom API template errors. HTTP and provider diagnostics are sanitized to avoid exposing configured URLs, query strings, response bodies, authentication credentials, tokens, cookies, and other potentially sensitive values. Internal template-rendering failures are logged server-side while clients receive a generic HTTP 500 response.
- **Configuration watcher reliability** — Hardens configuration file watching and reload behavior against concurrent shutdown and configuration-change activity. Debounce timer ownership is synchronized to prevent races between pending configuration notifications and watcher cleanup. Watcher shutdown also prevents new debounce work and waits for an already-running configuration callback to finish before returning, preventing configuration callbacks from continuing to mutate application state after watcher shutdown or during application teardown.
- **Runtime lifecycle hardening** — Expands synchronization and regression protection around application startup, shutdown, configuration reloads, background refresh scheduling, and watcher cleanup. Background scheduler cancellation propagates cleanly to active work and shutdown waits for scheduler workers to finish. Configuration watcher cleanup waits for active callbacks, and server lifecycle behavior is protected for cases including shutdown before startup and HTTP bind failures.
- **DNS statistics zero-value handling** — Prevents invalid percentage values when DNS providers return zero queries or zero blocked queries. Graph normalization and blocked-domain percentages safely remain at zero when their denominator is zero, covering AdGuard Home, Pi-hole v5, Pi-hole v6, and Technitium DNS Server.
- **Themed page-not-found response** — Addresses upstream [issue #1062](https://github.com/glanceapp/glance/issues/1062) by replacing the plain-text response for unknown page URLs with a themed Glance 404 page that preserves HTTP 404 semantics and provides links to the configured pages available in the current dashboard. Dashboard paths and configured base URLs are preserved, while API page-content requests continue to return a plain HTTP 404.
- **RSS parsing and rendering hardening** — Improves RSS title and image parsing based on upstream [PR #1044](https://github.com/glanceapp/glance/pull/1044) and addresses upstream [issue #1011](https://github.com/glanceapp/glance/issues/1011), including HTML title sanitization, image discovery from item metadata and feed content, and safe resolution of relative image URLs. Also addresses upstream [issue #919](https://github.com/glanceapp/glance/issues/919) by removing embedded HTML comments from feed descriptions, and [issue #962](https://github.com/glanceapp/glance/issues/962) by validating and resolving RSS item links before rendering so malformed or unsafe URLs cannot produce Go template `ZgotmplZ` output.
- **GitHub release fetching optimization** — Optimizes GitHub release requests for repositories configured with `include-prereleases: true` by requesting only the single release that Glance consumes instead of GitHub's default page of results, reducing response size and processing overhead without changing release selection behavior. Based on [Dynacat PR #97](https://github.com/Panonim/dynacat/pull/97).
- **HTTP connection reuse hardening** — Adds a finite idle connection timeout and consistent connection-pool settings to the shared HTTP transports, preventing idle connections from being retained indefinitely. Adapted from `matt2k7/glance` commit `b13c1f98699da36232933cbb3619003f8922ee5e`.
- **YouTube uploads feed fallback** — Falls back from the Shorts-filtered `UULF` channel uploads feed to the standard `UU` uploads feed when the primary feed fails, while preserving existing cache and error behavior. Adapted from `JacksonMcDonaldDev/glance` commit `b6082e3355c36644a3eb31dadada1ea75d7d78e1`.
- **Remote Server Stats mountpoint configuration** — Applies mountpoint visibility, naming, and ordering settings to system information returned by remote Glance agents. Adapted from `rakkateichou/glance` commit `64d3b1c1`.

### Testing and regression protection

The fork includes substantially expanded automated regression coverage intended both to protect fork-specific behavior and to make future upstream synchronization safer.

Regression coverage includes configuration parsing and diagnostics; configuration watcher concurrency and shutdown; application and HTTP server lifecycle behavior; dashboard routing; widget refresh synchronization, recovery, caching, and cancellation; healthy, failed, degraded, stale, and recovered transitions; Custom API requests, templates, timeouts, stale fallback, and errors; Docker and provider behavior; malformed or incomplete external responses; transport failures; diagnostic sanitization; HTTP error boundaries; DNS zero-value calculations; application initialization and migration; and regression tests for defects discovered while stabilizing the fork.

The test suite contains more than 300 tests and currently exercises approximately 80% of the Go codebase. Coverage is used as a measurement rather than as a goal by itself. Testing is concentrated on behavior where regressions, malformed external data, concurrency, lifecycle transitions, provider failures, and upstream changes present meaningful risk.

Concurrency-sensitive behavior is validated with Go's race detector. During stabilization, important suites were also executed repeatedly with both normal and race-enabled test runs to expose intermittent concurrency or lifecycle failures.

Several production defects were discovered through this process, reproduced with regression tests, and then fixed. Those tests remain in the suite to protect against recurrence.

### Development and CI validation

Changes to the fork are developed on focused branches and merged into the protected `main` branch through pull requests.

Pull requests are automatically validated with the complete Go test suite, the Go race detector, a complete Go build, and whitespace validation.

The repository `Makefile` provides standardized commands for development, validation, repository inspection, GitHub pull request operations, CI monitoring, and production deployment.

Common targets include:

```text
make test
make test-race
make test-count COUNT=10
make test-race-count COUNT=10
make build
make test-instance-start
make test-instance-status
make test-instance-stop
make fmt-check
make diff-check
make staged-check
make check
make coverage
make vuln
make status
make staged-diff
make upstream-status
make verify-main
make pr-view PR=<number>
make pr-runs
make pr-merge PR=<number>
make ci-watch RUN=<id>
make ci-view RUN=<id>
make deploy-status
make deploy
```

`make check` runs the standard local pre-pull-request validation suite. Repeated test targets are available for concurrency-sensitive or high-risk changes where a single successful test execution may not provide sufficient confidence.

For interactive development and browser validation, `make test-instance-start` builds the current source tree and starts an isolated Glance instance using a generated minimal configuration on port `18080`. `make test-instance-status` reports the process and HTTP status, and `make test-instance-stop` stops the instance and removes its generated binary, configuration, PID file, and log. The port can be overridden when needed, for example with `make test-instance-start TEST_PORT=18081`.

### Security and dependency maintenance

The fork includes additional security and dependency maintenance beyond functional changes.

- Go and project dependencies are periodically reviewed and updated.
- GitHub Actions dependencies are maintained alongside application dependencies.
- `govulncheck` is used to identify reachable Go vulnerabilities.
- Container builds upgrade available Alpine packages during the image build so published images receive current package-level security fixes available from the configured Alpine repositories.
- Operational and provider diagnostics are intentionally sanitized to avoid logging authentication credentials, authorization headers, cookies, tokens, configured URLs containing sensitive query parameters, response bodies, or similar potentially sensitive information.
- Error responses exposed to HTTP clients avoid disclosing internal template-rendering details while retaining actionable server-side diagnostics.

Security scanning can still report vulnerabilities for which an upstream package or distribution fix is not yet available. The presence of such a report does not imply that the fork contains a project-level fix for the underlying third-party vulnerability.

### Container images

The `main` branch is automatically built and published to GitHub Container Registry.

Available image tags include:

```text
ghcr.io/samcro1967/glance:latest
ghcr.io/samcro1967/glance:sha-<commit>
```

Published images contain OCI revision metadata identifying the source commit from which the image was built.

### Production deployment safeguards

The repository includes Makefile-based deployment tooling intended to make deployments reproducible and to prevent a stale or unexpected container image from being deployed.

`make deploy-status` performs a read-only comparison of the current source branch and revision, the locally available GHCR image and its source revision, and the currently running production container and its source revision.

`make deploy` performs a guarded production deployment. Before changing the running service it:

1. requires deployment from `main`;
2. requires a clean source working tree;
3. refreshes `origin` and requires local `main` to exactly match `origin/main`;
4. pulls `ghcr.io/samcro1967/glance:latest`;
5. reads the image's OCI source revision;
6. requires the image revision to exactly match the source repository's current `HEAD`; and
7. refuses to recreate the production service if those revisions differ.

After the image passes validation, the deployment recreates only the Glance Compose service and verifies that the running container reports the expected source revision, uses the exact validated image, the local Glance HTTP endpoint becomes ready, and recent container logs are available for immediate inspection.

This revision check intentionally prevents deployment during the interval between merging a change to `main` and completion of the corresponding GitHub Container Registry build.

### Upstream compatibility and maintenance

The fork continues to track the upstream Glance repository.

Upstream changes are reviewed before integration rather than being automatically applied to production. The expanded regression and race-test suites make it easier to evaluate future upstream changes while protecting fork-specific functionality and previously fixed defects.

Where functionality in this fork originates from an existing upstream pull request or another Glance-derived project, the corresponding source is identified in this README. Other changes were developed specifically for this fork based on functionality or reliability requirements encountered while operating it.

When a merged and tested fork implementation directly addresses an existing upstream issue, discussion, or pull request, a brief informational reference to the fork implementation is also left on the relevant upstream thread where appropriate.

The fork intentionally avoids unnecessary divergence from upstream. Production changes are made when they provide required functionality, address an observed defect, improve operational reliability, or provide meaningful regression protection. Areas that are functioning correctly are generally left unchanged rather than modified solely to increase test coverage or introduce speculative abstractions.

### Current stability

The fork has undergone a focused stabilization effort covering configuration handling, application lifecycle behavior, widget refresh and recovery, provider failure handling, concurrency, shutdown and reload behavior, diagnostics, security, regression testing, CI validation, and production deployment.

The stabilization process included targeted race-detector testing and repeated execution of concurrency-sensitive tests. Multiple defects discovered during this work were first reproduced through regression tests and then corrected, leaving those tests in place to protect against recurrence.

The codebase is now treated as a production baseline rather than an active stabilization project. Future changes are expected to focus on required functionality, observed production defects, worthwhile upstream changes, dependency and security maintenance, and regression protection for newly discovered issues.

The intent is to keep the fork maintainable and transparent while minimizing unnecessary divergence from upstream.

See the [configuration documentation](docs/configuration.md#configuring-glance) for details on using the additional widgets and functionality.

## Features
### Various widgets
* RSS feeds
* Subreddit posts
* Hacker News posts
* Weather forecasts
* YouTube channel uploads
* Twitch channels
* Market prices
* Docker containers status
* Server stats
* Custom widgets
* [and many more...](docs/configuration.md#configuring-glance)

### Fast and lightweight
* Low memory usage
* Few dependencies
* Minimal vanilla JS
* Single <20mb binary available for multiple OSs & architectures and just as small Docker container
* Uncached pages usually load within ~1s (depending on internet speed and number of widgets)

### Tons of customizability
* Different layouts
* As many pages/tabs as you need
* Numerous configuration options for each widget
* Multiple styles for some widgets
* Custom CSS

### Optimized for mobile devices
Because you'll want to take it with you on the go.

![](docs/images/mobile-preview.png)

### Themeable
Easily create your own theme by tweaking a few numbers or choose from one of the [already available themes](docs/themes.md).

![](docs/images/themes-example.png)

<br>

## Configuration
Configuration is done through YAML files, to learn more about how the layout works, how to add more pages and how to configure widgets, visit the [configuration documentation](docs/configuration.md#configuring-glance).
<details>
<summary><strong>Preview example configuration file</strong></summary>
<br>

```yaml
pages:
  - name: Home
    columns:
      - size: small
        widgets:
          - type: calendar
            first-day-of-week: monday

          - type: rss
            limit: 10
            collapse-after: 3
            cache: 12h
            feeds:
              - url: https://selfh.st/rss/
                title: selfh.st
                limit: 4
              - url: https://ciechanow.ski/atom.xml
              - url: https://www.joshwcomeau.com/rss.xml
                title: Josh Comeau
              - url: https://samwho.dev/rss.xml
              - url: https://ishadeed.com/feed.xml
                title: Ahmad Shadeed

          - type: twitch-channels
            channels:
              - theprimeagen
              - j_blow
              - piratesoftware
              - cohhcarnage
              - christitustech
              - EJ_SA

      - size: full
        widgets:
          - type: group
            widgets:
              - type: hacker-news
              - type: lobsters

          - type: videos
            channels:
              - UCXuqSBlHAE6Xw-yeJA0Tunw # Linus Tech Tips
              - UCR-DXc1voovS8nhAvccRZhg # Jeff Geerling
              - UCsBjURrPoezykLs9EqgamOA # Fireship
              - UCBJycsmduvYEL83R_U4JriQ # Marques Brownlee
              - UCHnyfMqiRRG1u-2MsSQLbXA # Veritasium

          - type: group
            widgets:
              - type: reddit
                subreddit: technology
                show-thumbnails: true
              - type: reddit
                subreddit: selfhosted
                show-thumbnails: true

      - size: small
        widgets:
          - type: weather
            location: London, United Kingdom
            units: metric
            hour-format: 12h

          - type: markets
            markets:
              - symbol: SPY
                name: S&P 500
              - symbol: BTC-USD
                name: Bitcoin
              - symbol: NVDA
                name: NVIDIA
              - symbol: AAPL
                name: Apple
              - symbol: MSFT
                name: Microsoft

          - type: releases
            cache: 1d
            repositories:
              - glanceapp/glance
              - go-gitea/gitea
              - immich-app/immich
              - syncthing/syncthing
```
</details>

<br>

## Installation

Choose one of the following methods:

<details>
<summary><strong>Docker compose using upstream directory structure</strong></summary>
<br>

> [!NOTE]
> This convenience template is maintained by the upstream Glance project and uses the upstream `glanceapp/glance` container image by default. To use this fork, change the `image` value in the generated `docker-compose.yml` to `ghcr.io/samcro1967/glance:latest` before starting the container.

Create a new directory called `glance` as well as the template files within it by running:

```bash
mkdir glance && cd glance && curl -sL https://github.com/glanceapp/docker-compose-template/archive/refs/heads/main.tar.gz | tar -xzf - --strip-components 2
```

*[click here to view the files that will be created](https://github.com/glanceapp/docker-compose-template/tree/main/root)*

Then, edit the following files as desired:
* `docker-compose.yml` to configure the port, volumes and other containery things
* `config/home.yml` to configure the widgets or layout of the home page
* `config/glance.yml` if you want to change the theme or add more pages

<details>
<summary>Other files you may want to edit</summary>

* `.env` to configure environment variables that will be available inside configuration files
* `assets/user.css` to add custom CSS
</details>

When ready, run:

```bash
docker compose up -d
```

If you encounter any issues, you can check the logs by running:

```bash
docker compose logs
```

<hr>
</details>

<details>
<summary><strong>Docker compose manual (recommended)</strong></summary>
<br>

Create a `docker-compose.yml` file with the following contents:

```yaml
services:
  glance:
    container_name: glance
    image: ghcr.io/samcro1967/glance:latest
    restart: unless-stopped
    volumes:
      - ./config:/app/config
    ports:
      - 8080:8080
```

Then, create a new directory called `config` and download the example starting [`glance.yml`](docs/glance.yml) file from this fork into it by running:

```bash
mkdir config && wget -O config/glance.yml https://raw.githubusercontent.com/samcro1967/glance/refs/heads/main/docs/glance.yml
```

Feel free to edit the `glance.yml` file to your liking, and when ready run:

```bash
docker compose up -d
```

If you encounter any issues, you can check the logs by running:

```bash
docker logs glance
```

<hr>
</details>

<details>
<summary><strong>Manual binary installation</strong></summary>
<br>

This fork is currently distributed as a container image through GitHub Container Registry. Precompiled binary releases are not currently published by this fork.

If you require a standalone binary, you can build this fork from source using the Go toolchain. Alternatively, the [upstream Glance project](https://github.com/glanceapp/glance/releases/latest) publishes precompiled binaries for Linux, Windows and macOS. Be aware that upstream binaries do not include the additional functionality documented in the [About this fork](#about-this-fork) section.

A starting configuration for this fork is available in [`docs/glance.yml`](docs/glance.yml).

<hr>
</details>

<details>
<summary><strong>Other</strong></summary>
<br>

The following third-party installation channels are available for upstream Glance. They may not install this fork or include its additional functionality:

* [Proxmox VE Helper Script](https://community-scripts.org/scripts/glance?id=glance)
* [NixOS package](https://search.nixos.org/packages?channel=unstable&show=glance)
* [Hostinger](https://www.hostinger.com/vps/docker/glance/)
* [Coolify.io](https://coolify.io/docs/services/glance/)

<hr>
</details>

<br>

## Common issues
<details>
<summary><strong>Requests timing out</strong></summary>

The most common cause of this is when using Pi-Hole, AdGuard Home or other ad-blocking DNS services, which by default have a fairly low rate limit. Depending on the number of widgets you have in a single page, this limit can very easily be exceeded. To fix this, increase the rate limit in the settings of your DNS service.

If using Podman, in some rare cases the timeout can be caused by an unknown issue, in which case it may be resolved by adding the following to the bottom of your `docker-compose.yml` file:
```yaml
networks:
  podman:
    external: true
```
</details>

<details>
<summary><strong>Broken layout for markets, bookmarks or other widgets</strong></summary>

This is almost always caused by the browser extension Dark Reader. To fix this, disable dark mode for the domain where Glance is hosted.
</details>

<details>
<summary><strong>cannot unmarshal !!map into []glance.page</strong></summary>

The most common cause of this is having a `pages` key in your `glance.yml` and then also having a `pages` key inside one of your included pages. To fix this, remove the `pages` key from the top of your included pages.

</details>

<br>

## FAQ
<details>
<summary><strong>Does the information on the page update automatically?</strong></summary>

Updateable widgets are refreshed and recovered server-side when their cached data expires, even when no page is open. When a page is open, refreshed widget content is automatically delivered to the browser without requiring a full page reload. Some client-side information also updates dynamically where it makes sense, such as the clock widget and relative times.
</details>

<details>
<summary><strong>How frequently do widgets update?</strong></summary>
Updateable widgets are refreshed server-side after their cached data expires. The normal refresh interval is determined by each widget's cache lifetime and can be configured where supported. Failed refreshes are retried automatically using progressive backoff so temporary dependency or network failures can recover without restarting Glance or opening the page.
</details>

<details>
<summary><strong>Can I create my own widgets?</strong></summary>

Yes, there are multiple ways to create custom widgets:
* `iframe` widget - allows you to embed things from other websites
* `html` widget - allows you to insert your own static HTML
* `extension` widget - fetch HTML from a URL
* `custom-api` widget - fetch JSON from a URL and render it using custom HTML
</details>

<details>
<summary><strong>Can I change the title of a widget?</strong></summary>

Yes, the title of all widgets can be changed by specifying the `title` property in the widget's configuration:

```yaml
- type: rss
  title: My custom title

- type: markets
  title: My custom title

- type: videos
  title: My custom title

# and so on for all widgets...
```
</details>

<br>

## Feature requests

Issues and feature requests related to functionality specific to this fork can be submitted to the [fork's issue tracker](https://github.com/samcro1967/glance/issues).

For requests concerning upstream Glance rather than fork-specific functionality, use the [upstream Glance issue tracker](https://github.com/glanceapp/glance/issues).

## Building from source

Choose one of the following methods:

<details>
<summary><strong>Build binary with Go</strong></summary>
<br>

Requirements: [Go](https://go.dev/dl/) >= v1.23

To build the project for your current OS and architecture, run:

```bash
go build -o build/glance .
```

To build for a specific OS and architecture, run:

```bash
GOOS=linux GOARCH=amd64 go build -o build/glance .
```

[*click here for a full list of GOOS and GOARCH combinations*](https://go.dev/doc/install/source#:~:text=$GOOS%20and%20$GOARCH)

Alternatively, if you just want to run the app without creating a binary, like when you're testing out changes, you can run:

```bash
go run .
```
<hr>
</details>

<details>
<summary><strong>Build project and Docker image with Docker</strong></summary>
<br>

Requirements: [Docker](https://docs.docker.com/engine/install/)

To build the project and image using just Docker, run:

*(replace `owner` with your name or organization)*

```bash
docker build -t owner/glance:latest .
```

If you wish to push the image to a registry (by default Docker Hub), run:

```bash
docker push owner/glance:latest
```

<hr>
</details>

<br>

## Contributing guidelines

* Before working on a new feature it's preferable to submit a feature request first and state that you'd like to implement it yourself
* Please don't submit PRs for feature requests that are either in the roadmap<sup>[1]</sup>, backlog<sup>[2]</sup> or icebox<sup>[3]</sup>
* Use `dev` for the base branch if you're adding new features or fixing bugs, otherwise use `main`
* Avoid introducing new dependencies
* Avoid making backwards-incompatible configuration changes
* Avoid introducing new colors or hard-coding colors, use the standard `primary`, `positive` and `negative`
* For icons, try to use [heroicons](https://heroicons.com/) where applicable
* Provide a screenshot of the changes if UI related where possible
* No `package.json`

<details>
<summary><strong><sup>[1] [2] [3]</sup></strong></summary>

[1] The feature likely already has work put into it that may conflict with your implementation

[2] The demand, implementation or functionality for this feature is not yet clear

[3] No plans to add this feature for the time being

</details>

<br>

## Thank you

To all the people who were generous enough to [sponsor](https://github.com/sponsors/glanceapp) the project and to everyone who has contributed in any way, be it PRs, submitting issues, helping others in the discussions or Discord server, creating guides and tools or just mentioning Glance on social media. Your support is greatly appreciated and helps keep the project going.
