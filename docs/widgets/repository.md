# Repository

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Glance README](../../README.md)

Display general information about a repository as well as a list of the latest open pull requests and issues.

Example:
## Quick start


```yaml
- type: repository
  repository: glanceapp/glance
  pull-requests-limit: 5
  issues-limit: 3
  commits-limit: 3
```

Preview:

![Repository widget](../images/repository-preview.png)

## Configuration

This widget also supports the [shared widget properties](../widgets.md#shared-properties).

| Name | Type | Required | Default |
| ---- | ---- | -------- | ------- |
| repository | string | yes |  |
| token | string | no | |
| pull-requests-limit | integer | no | 3 |
| issues-limit | integer | no | 3 |
| commits-limit | integer | no | -1 |

### `repository`
The owner and repository name that will have their information displayed.

### `token`
Without authentication Github allows for up to 60 requests per hour. You can easily exceed this limit and start seeing errors if your cache time is low or you have many instances of this widget. To circumvent this you can [create a read only token from your Github account](https://github.com/settings/personal-access-tokens/new) and provide it here.

### `pull-requests-limit`
The maximum number of latest open pull requests to show. Set to `-1` to not show any.

### `issues-limit`
The maximum number of latest open issues to show. Set to `-1` to not show any.

### `commits-limit`
The maximum number of lastest commits to show from the default branch. Set to `-1` to not show any.


---

[Widgets](../widgets.md) · [Configuration](../configuration.md) · [Back to top](#repository)
