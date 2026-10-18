# Playwright browser integration

[Glance README](../README.md) · [Configuration](configuration.md) · [Reddit widget](widgets/reddit.md)

Glance can optionally connect to a shared Chromium browser through the Chrome DevTools Protocol (CDP). Currently, Reddit widgets can opt in with `use-playwright: true`; other widgets continue using their existing request paths unless they explicitly support browser mode.

The browser runs separately from Glance and retains its own profile, so an authenticated session can survive restarts. Glance does not launch Chromium.

## Prerequisites

- Docker Compose and a Glance instance able to reach the browser container over a trusted Docker network.
- A writable, persistent Chromium profile directory. In the example below Chromium runs as UID/GID 1000; the mounted directory must be writable by that user.
- Sufficient shared memory for Chromium. The example allocates 1 GiB.

## Docker Compose example

The following example is adapted from a working deployment using `mcr.microsoft.com/playwright:v1.64.0-noble`. It starts a graphical Chromium session under Xvfb and forwards CDP from Chromium's loopback listener to port 9223 inside the container. It uses a normal Compose bridge network so the browser can reach Reddit. **Glance must also join `glance-browser`**; if Glance is defined in another Compose project, use an existing shared external Docker network instead.

```yaml
services:
  playwright:
    image: mcr.microsoft.com/playwright:v1.64.0-noble
    container_name: playwright
    hostname: playwright
    user: "0:0"
    restart: unless-stopped
    stop_grace_period: 30s
    shm_size: 1gb
    init: true
    environment:
      HOME: /tmp
      DISPLAY: ":99"
    ports:
      - "127.0.0.1:9222:9223"
    volumes:
      - ./playwright-profile:/data/chromium
    networks:
      - glance-browser
    command:
      - /bin/bash
      - -c
      - |
        set -e
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends socat
        rm -rf /var/lib/apt/lists/*

        setpriv --reuid=1000 --regid=1000 --clear-groups -- \
          Xvfb :99 -screen 0 1440x900x24 -nolisten tcp &
        xvfb_pid=$$!
        sleep 2

        setpriv --reuid=1000 --regid=1000 --clear-groups -- \
          /ms-playwright/chromium-1248/chrome-linux64/chrome \
          --no-sandbox \
          --disable-dev-shm-usage \
          --no-first-run \
          --no-default-browser-check \
          --user-data-dir=/data/chromium \
          --remote-debugging-address=127.0.0.1 \
          --remote-debugging-port=9222 \
          --remote-allow-origins=* \
          --window-size=1400,850 \
          https://www.reddit.com/ &
        browser_pid=$$!

        setpriv --reuid=1000 --regid=1000 --clear-groups -- \
          socat TCP-LISTEN:9223,bind=0.0.0.0,reuseaddr,fork TCP:127.0.0.1:9222 &
        relay_pid=$$!

        shutdown() {
          trap - TERM INT
          kill "$$browser_pid" "$$relay_pid" "$$xvfb_pid" 2>/dev/null || true
          wait "$$browser_pid" "$$relay_pid" "$$xvfb_pid" 2>/dev/null || true
        }
        trap shutdown TERM INT
        wait -n "$$browser_pid" "$$relay_pid"
        result=$$?
        shutdown
        exit "$$result"

networks:
  glance-browser:
    driver: bridge
```

**Important limitations:** This reproduces a tested setup, not a hardened production image. It installs `socat` on startup, starts the container as root before dropping privileges for the browser processes, and disables Chromium's sandbox. Prefer a purpose-built image with dependencies preinstalled and a reviewed sandbox/security configuration for long-term production use. The Chromium executable path is tied to this pinned Playwright image and must be rechecked when upgrading it. The example does not include a browser login UI.

The example publishes port 9222 **only on the Docker host's loopback interface** for local diagnostics. Port 9223 is still accessible to containers on the shared Docker network. Do not publish CDP on a public interface or attach untrusted containers to that network.

Create the profile directory with ownership and permissions appropriate for UID 1000 before starting the service. Protect the directory and its backups because it can contain active Reddit session cookies.

## Configure Glance

In Glance's top-level configuration:

```yaml
playwright:
  endpoint: http://playwright:9223
```

The endpoint must be reachable **from the Glance container**. `127.0.0.1:9222` inside Glance would refer to Glance's own network namespace, not the separate Playwright service.

Enable browser requests per Reddit widget:

```yaml
- type: reddit
  subreddit: selfhosted
  sort-by: new
  use-playwright: true
```

See the [Reddit widget reference](widgets/reddit.md) for the supported options and restrictions.

## Reddit sessions and credentials

You can reuse an already authenticated persistent Chromium profile **without configuring Reddit credentials in Glance**. If that session expires, sign in manually using a trusted method of accessing the browser profile. This Compose example does not provide an interactive browser interface; arrange a secure management method separately if manual login is required.

Alternatively, configure both credentials for automated login and session recovery:

```yaml
playwright:
  endpoint: http://playwright:9223
  reddit:
    username: ${REDDIT_USERNAME}
    password: ${secret:reddit_password}
```

Both `username` and `password` must be set together, or both omitted. Use Glance's existing environment-variable or secret interpolation rather than embedding passwords directly in configuration files. Glance checks the existing session before attempting login; CAPTCHA, MFA, and other interactive challenges still require human action.

## Verification and troubleshooting

From the Docker host, check that the local CDP endpoint responds:

```bash
curl -fsS http://127.0.0.1:9222/json/version
```

A response identifying Chromium and a WebSocket debugger endpoint confirms CDP connectivity **only**. It does not prove that Reddit is authenticated or that a widget refresh succeeds.

- **Endpoint unavailable:** Check container status, Chromium startup logs, the relay, DNS resolution, and shared Docker network membership.
- **Not authenticated:** Sign in to the persistent profile or provide both Reddit credentials for recovery. Human challenges cannot be bypassed.
- **Session lost after restart:** Confirm that `/data/chromium` is mounted persistently and writable by UID 1000; ensure no competing Chromium instance uses the same profile.
- **HTTP 403 without Playwright:** Reddit may be restricting API access. See [Reddit authentication options](widgets/reddit.md#application-authentication) and [browser-backed requests](widgets/reddit.md#browser-backed-reddit-requests).
- **Browser mode fails:** Review the widget error and Chromium logs. An accessible CDP endpoint does not guarantee successful Reddit requests.

## Security considerations

CDP access can control the browser and inspect authenticated sessions. Treat it as privileged administrative access. Use a trusted private Docker network, restrict host access, avoid public port mappings, and protect the persistent profile and its backups. `--remote-allow-origins=*` and `--no-sandbox` in the example increase exposure; they are included to describe the tested configuration, not as general security recommendations.

---

[Glance README](../README.md) · [Configuration](configuration.md) · [Reddit widget](widgets/reddit.md)
