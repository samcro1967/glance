# Docker Containers

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)


Display the status of your Docker containers along with an icon and an optional short description.

![Docker Containers widget](../images/widgets/docker-containers.png)
## Quick start


```yaml
- type: docker-containers
  hide-by-default: false
```

> [!NOTE]
>
> The widget requires access to `docker.sock`. If you're running Glance inside a container, this can be done by mounting the socket as a volume:
>
> ```yaml
> services:
>   glance:
>     image: glanceapp/glance
>     volumes:
>       - /var/run/docker.sock:/var/run/docker.sock
> ```

Configuration of the containers is done via labels applied to each container:

```yaml
  jellyfin:
    image: jellyfin/jellyfin:latest
    labels:
      glance.name: Jellyfin
      glance.icon: si:jellyfin
      glance.url: https://jellyfin.domain.com
      glance.description: Movies & shows
```

Alternatively, you can also define the values within your `glance.yml` via the `containers` property, where the key is the container name and each value is the same as the labels but without the "glance." prefix:

```yaml
- type: docker-containers
  containers:
    container_name_1:
      name: Container Name
      description: Description of the container
      url: https://container.domain.com
      icon: si:container-icon
      hide: false
```

For services with multiple containers you can specify a `glance.id` on the "main" container and `glance.parent` on each "child" container:

<details>
<summary>View <code>docker-compose.yml</code></summary>
<br>

```yaml
services:
  immich-server:
    image: ghcr.io/immich-app/immich-server
    labels:
      glance.name: Immich
      glance.icon: si:immich
      glance.url: https://immich.domain.com
      glance.description: Image & video management
      glance.id: immich

  redis:
    image: docker.io/redis:6.2-alpine
    labels:
      glance.parent: immich
      glance.name: Redis

  database:
    image: docker.io/tensorchord/pgvecto-rs:pg14-v0.2.0
    labels:
      glance.parent: immich
      glance.name: DB

  proxy:
    image: nginx:stable
    labels:
      glance.parent: immich
      glance.name: Proxy
```
</details>
<br>

This will place all child containers under the `Immich` container when hovering over its icon:


If any of the child containers are down, their status will propagate up to the parent container:


## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| hide-by-default | boolean | no | false |
| format-container-names | boolean | no | false |
| sock-path | string | no | /var/run/docker.sock |
| category | string | no | |
| running-only | boolean | no | false |

### `hide-by-default`
Whether to hide the containers by default. If set to `true` you'll have to manually add a `glance.hide: false` label to each container you want to display. By default all containers will be shown and if you want to hide a specific container you can add a `glance.hide: true` label.

### `format-container-names`
When set to `true`, automatically converts container names such as `container_name_1` into `Container Name 1`.

### `sock-path`
The path to the Docker socket. This can also be a [remote socket](https://docs.docker.com/engine/daemon/remote-access/) or proxied socket using something like [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy).

If the socket path starts with `tcp://` or `http://`, it will be treated as a remote socket. Anything else will be treated as a path to a Unix socket.

### `category`
Filter to only the containers which have this category specified via the `glance.category` label. Useful if you want to have multiple containers widgets, each showing a different set of containers.

<details>
<summary>View example</summary>
<br>


```yaml
services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    labels:
      glance.name: Jellyfin
      glance.icon: si:jellyfin
      glance.url: https://jellyfin.domain.com
      glance.category: media

  gitea:
    image: gitea/gitea:latest
    labels:
      glance.name: Gitea
      glance.icon: si:gitea
      glance.url: https://gitea.domain.com
      glance.category: dev-tools

  vaultwarden:
    image: vaultwarden/server:latest
    labels:
      glance.name: Vaultwarden
      glance.icon: si:vaultwarden
      glance.url: https://vaultwarden.domain.com
      glance.category: dev-tools
```

Then you can use the `category` property to filter the containers:

```yaml
- type: docker-containers
  title: Dev tool containers
  category: dev-tools

- type: docker-containers
  title: Media containers
  category: media
```

</details>

### `running-only`
Whether to only show running containers. If set to `true` only containers that are currently running will be displayed. If set to `false` all containers will be displayed regardless of their state.

## Labels
| Name | Description |
| ---- | ----------- |
| glance.name | The name displayed in the UI. If not specified, the name of the container will be used. |
| glance.icon | See [Icons](../configuration.md#icons) for more information on how to specify icons |
| glance.url | The URL that the user will be redirected to when clicking on the container. |
| glance.same-tab | Whether to open the link in the same or a new tab. Default is `false`. |
| glance.description | A short description displayed in the UI. Default is empty. |
| glance.hide | Whether to hide the container. If set to `true` the container will not be displayed. Defaults to `false`. |
| glance.id | The custom ID of the container. Used to group containers under a single parent. |
| glance.parent | The ID of the parent container. Used to group containers under a single parent. |
| glance.category | The category of the container. Used to filter containers by category. |


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#docker-containers)
