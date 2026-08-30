<p align="center"><img src="docs/logo.png"></p>
<h1 align="center">Glance</h1>
<p align="center">
  <a href="#installation">Install</a> •
  <a href="docs/configuration.md#configuring-glance">Configuration</a> •
  <a href="https://discord.com/invite/7KQ7Xa9kJd">Discord</a> •
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

It tracks the upstream Glance project while incorporating additional functionality not currently available in the upstream release.

### Changes from upstream

- **Stack widget** — Adds the `stack` widget from upstream [PR #765](https://github.com/glanceapp/glance/pull/765), allowing multiple widgets to be stacked vertically and treated as a single widget.
- **Nested groups** — Allows `group` widgets to contain other `group` widgets, enabling multiple levels of tabbed navigation.
- **Custom API timeouts** — Adds configurable request timeouts to `custom-api` widgets based on upstream [PR #997](https://github.com/glanceapp/glance/pull/997), with independent timeout settings for primary requests and subrequests.
- **Custom API stale fallback** — Preserves the last successfully rendered `custom-api` content when a refresh fails, displays a visible stale indicator with the age of the last successful update, and automatically clears the stale state after a successful refresh.
- **HTTP server startup failure handling** — Incorporates upstream [PR #1047](https://github.com/glanceapp/glance/pull/1047), causing Glance to exit with a nonzero status when the HTTP server fails to start instead of remaining running in a broken state. This allows container restart policies and external monitoring to correctly detect and respond to startup failures.
- **Mountpoint CLI fix** — Incorporates upstream [PR #1065](https://github.com/glanceapp/glance/pull/1065), fixing the `mountpoint:info <path>` command so it can be invoked as documented instead of being rejected as an unknown command.
- **IPv6 Docker remote sources** — Incorporates upstream [PR #1064](https://github.com/glanceapp/glance/pull/1064), correctly formatting IPv6 addresses used by remote Docker sources while preserving TCP, HTTP, HTTPS, explicit-port, and default-port behavior.
- **YAML comment variable parsing** — Incorporates upstream [PR #965](https://github.com/glanceapp/glance/pull/965), preventing configuration variables inside YAML comments from being expanded while correctly preserving hashes inside quoted values and handling escaped or doubled quotes.
- **Search autofocus fix** — Incorporates upstream [PR #885](https://github.com/glanceapp/glance/pull/885), ensuring search widgets configured with `autofocus` reliably receive focus after dynamic initialization, including in browsers such as Firefox.
- **Automatic widget recovery** — Adds server-side background refresh and recovery for updateable widgets. Expired or previously failed widgets are retried automatically without requiring a page to be open or manually refreshed. Refreshes are synchronized per widget to prevent duplicate concurrent updates while preserving parallel updates across different widgets, use bounded concurrency and progressive retry backoff after failures, and are cancelled cleanly during shutdown and configuration reloads. Normal successful refresh timing continues to respect each widget's configured cache duration. The `custom-api` widget additionally preserves its last successfully rendered content while a refresh is failing.
- **Structured operational logging and diagnostics** — Adds structured lifecycle, failure, recovery, and configuration diagnostics for improved container and service observability. Glance logs meaningful application, HTTP server, and background refresh scheduler lifecycle events; reports widget failure or degraded transitions with actionable causes without repeatedly logging the same failed state; and records when affected widgets recover. Configuration errors include source-aware file and line information where available, including included YAML files, semantic configuration errors, widget initialization failures, configuration variables, and Custom API template errors. HTTP and provider diagnostics are sanitized to avoid exposing configured URLs, query strings, response bodies, authentication credentials, tokens, cookies, and other potentially sensitive values. Internal template-rendering failures are logged server-side while clients receive a generic HTTP 500 response.
- **Automated testing and validation** — Substantially expands automated regression coverage for fork functionality, including configuration diagnostics, dashboard routing, widget refresh synchronization and recovery, failure/degraded/recovery transitions, provider error handling, transport-error sanitization, Custom API behavior, Docker and provider diagnostics, HTTP error boundaries, and application startup and migration handling. Pull requests are automatically validated with the Go test suite, race detector, and build checks.
- **Named dashboards** — Allows configured pages to be organized into multiple independently addressable dashboards with dashboard-specific navigation. Pages are defined once and can be shared across dashboards while retaining the same underlying widget state, caching, and update lifecycle. When dashboards are configured, the application logo provides a dashboard switcher for quickly moving between dashboards, with the current dashboard indicated in the menu. Named dashboards whose generated slugs conflict with existing page slugs are ignored with a warning, allowing the remaining dashboards and pages to continue operating normally. Existing configurations without `dashboards` continue to use the standard Glance page and logo behavior.
- **Container image** — Automatically builds and publishes this fork from the `main` branch to GitHub Container Registry:
  - `ghcr.io/samcro1967/glance:latest`
  - `ghcr.io/samcro1967/glance:sha-<commit>`

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
<summary><strong>Docker compose using provided directory structure (recommended)</strong></summary>
<br>

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
<summary><strong>Docker compose manual</strong></summary>
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

Then, create a new directory called `config` and download the example starting [`glance.yml`](https://github.com/glanceapp/glance/blob/main/docs/glance.yml) file into it by running:

```bash
mkdir config && wget -O config/glance.yml https://raw.githubusercontent.com/glanceapp/glance/refs/heads/main/docs/glance.yml
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

Precompiled binaries are available for Linux, Windows and macOS (x86, x86_64, ARM and ARM64 architectures).

### Linux

Visit the [latest release page](https://github.com/glanceapp/glance/releases/latest) for available binaries. You can place the binary in `/opt/glance/` and have it start with your server via a [systemd service](https://linuxhandbook.com/create-systemd-services/). By default, when running the binary, it will look for a `glance.yml` file in the directory it's placed in. To specify a different path for the config file, use the `--config` option:

```bash
/opt/glance/glance --config /etc/glance.yml
```

To grab a starting template for the config file, run:

```bash
wget https://raw.githubusercontent.com/glanceapp/glance/refs/heads/main/docs/glance.yml
```

### Windows

Download and extract the executable from the [latest release](https://github.com/glanceapp/glance/releases/latest) (most likely the file called `glance-windows-amd64.zip` if you're on a 64-bit system) and place it in a folder of your choice. Then, create a new text file called `glance.yml` in the same folder and paste the content from [here](https://raw.githubusercontent.com/glanceapp/glance/refs/heads/main/docs/glance.yml) in it. You should then be able to run the executable and access the dashboard by visiting `http://localhost:8080` in your browser.



<hr>
</details>

<details>
<summary><strong>Other</strong></summary>
<br>

Glance can also be installed through the following 3rd party channels:
* [Proxmox VE Helper Script](https://community-scripts.org/scripts/glance?id=glance)
* [NixOS package](https://search.nixos.org/packages?channel=unstable&show=glance)
* [Hostinger](https://www.hostinger.com/vps/docker/glance)
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
Updateable widgets are refreshed and recovered server-side when their cached data expires, even when no page is open. A browser page refresh may still be required to display newly fetched server-side content. Some client-side information also updates dynamically where it makes sense, such as the clock widget and relative times.
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

New feature suggestions are always welcome and will be considered, though please keep in mind that some of them may be out of scope for what the project is trying to achieve (or is reasonably capable of). If you have an idea for a new feature and would like to share it, you can do so [here](https://github.com/glanceapp/glance/issues/new?template=feature_request.yml).

Feature requests are tagged with one of the following:

* [Roadmap](https://github.com/glanceapp/glance/labels/roadmap) - will be implemented in a future release
* [Backlog](https://github.com/glanceapp/glance/labels/backlog) - may be implemented in the future but needs further feedback or interest from the community
* [Icebox](https://github.com/glanceapp/glance/labels/icebox) - no plans to implement as it doesn't currently align with the project's goals or capabilities, may be revised at a later date

<br>

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
