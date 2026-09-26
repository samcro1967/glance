# Releases

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display a list of latest releases for specific repositories on GitHub, GitLab, Codeberg, Docker Hub or GitHub Container Registry (GHCR).

Example:
## Quick start


```yaml
- type: releases
  show-source-icon: true
  repositories:
    - go-gitea/gitea
    - jellyfin/jellyfin
    - samcro1967/glance
    - codeberg:redict/redict
    - gitlab:fdroid/fdroidclient
    - dockerhub:gotify/server
    - ghcr:samcro1967/glance:latest
```

Preview:

![Releases widget](../images/widgets/releases.png)

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| repositories | array | yes |  |
| show-source-icon | boolean | no | false |  |
| token | string | no | |
| gitlab-token | string | no | |
| limit | integer | no | 10 |
| collapse-after | integer | no | 5 |

### `repositories`
A list of repositories to fetch the latest release for. Only the name/repo is required, not the full URL. A prefix can be specified for repositories hosted elsewhere such as GitLab, Codeberg, Docker Hub and GHCR. Example:

```yaml
repositories:
  - gitlab:inkscape/inkscape
  - dockerhub:grafana/grafana
  - ghcr:samcro1967/glance
  - codeberg:redict/redict
```

GitLab and Codeberg repositories can use a custom `base-url` when the service is hosted somewhere other than the public provider. The value must be an absolute `http` or `https` origin without credentials, a path, query, or fragment:

```yaml
repositories:
  - repository: gitlab:group/project
    base-url: https://gitlab.example.com
  - repository: codeberg:owner/project
    base-url: https://forge.example.com
```

When `base-url` is omitted, GitLab uses `https://gitlab.com` and Codeberg uses `https://codeberg.org`. Custom base URLs are not supported for GitHub or Docker Hub repositories.

Official images on Docker Hub can be specified by omitting the owner:

```yaml
repositories:
  - dockerhub:nginx
  - dockerhub:node
  - dockerhub:alpine
```

You can also specify exact tags for Docker Hub images:

```yaml
repositories:
  - dockerhub:nginx:latest
  - dockerhub:nginx:stable-alpine
```

GHCR images use the `ghcr:` prefix and require an owner plus image name. Without a tag, Glance selects the newest tagged package version returned by GitHub Packages. You can also track an exact tag:

```yaml
repositories:
  - ghcr:samcro1967/glance
  - ghcr:samcro1967/glance:latest
  - ghcr:samcro1967/glance:dev
```

GHCR package requests require GitHub authentication. Configure `token` with a GitHub token that can read packages; a classic personal access token requires the `read:packages` scope.

To include prereleases you can specify the repository as an object and use the `include-prereleases` property:

**Note: This feature is currently only available for GitHub repositories.**

```yaml
repositories:
  - gitlab:inkscape/inkscape
  - repository: samcro1967/glance
    include-prereleases: true
  - codeberg:redict/redict
```

### `show-source-icon`
Shows an icon of the source (GitHub/GitLab/Codeberg/Docker Hub/GHCR) next to the repository name when set to `true`. GHCR uses the GitHub source icon.

### `token`
Without authentication GitHub allows for up to 60 requests per hour. You can easily exceed this limit and start seeing errors if you're tracking lots of repositories or your cache time is low. The same token is also used for GHCR package requests. To circumvent GitHub API rate limits you can [create a read only token from your GitHub account](https://github.com/settings/personal-access-tokens/new) and provide it here. For private GHCR packages, the token must be able to read the package; classic personal access tokens require the `read:packages` scope.

You can also specify the value for this token through an ENV variable using the syntax `${GITHUB_TOKEN}` where `GITHUB_TOKEN` is the name of the variable that holds the token. If you've installed Glance through docker you can specify the token in your docker-compose:

```yaml
services:
  glance:
    image: ghcr.io/samcro1967/glance:latest
    environment:
      - GITHUB_TOKEN=<your token>
```

and then use it in your `glance.yml` like this:

```yaml
- type: releases
  token: ${GITHUB_TOKEN}
  repositories: ...
```

This way you can safely check your `glance.yml` in version control without exposing the token.

### `gitlab-token`
Same as the above but used when fetching GitLab releases.

### `limit`
The maximum number of releases to show.

## `collapse-after`
How many releases are visible before the "SHOW MORE" button appears. Set to `-1` to never collapse.


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#releases)
