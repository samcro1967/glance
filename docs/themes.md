# Themes

Glance includes a native theme system for customizing the visual
appearance of the dashboard through YAML configuration. Most dashboard
styling can be changed without writing CSS, while custom CSS remains
available for advanced or highly specialized presentation.

Themes can control:

-   Core colors and light or dark appearance
-   Typography and headings
-   Page backgrounds, overlays, and ambient accents
-   Header appearance
-   Navigation
-   Widgets and widget headers
-   Cards
-   Groups and tabs
-   Form controls and buttons
-   Footer appearance
-   Elevated surfaces and separators

Theme properties can be configured globally and most visual properties
can also be overridden for individual pages.

## Theme resolution and inheritance

Glance resolves appearance in the following order:

``` text
Native Glance defaults
        ↓
Selected base theme
        ↓
Current page theme override
        ↓
custom-css-file
```

The top-level `theme` is the configured default appearance. A page-level
`theme` is a partial override of whichever base theme is active.
Properties omitted from the page override continue to inherit from the
selected base theme.

Custom CSS is loaded after native theme styling and remains the final
styling layer.

This means native themes can handle normal dashboard customization while
CSS can still be used for gradients, complex effects, highly specific
component styling, or anything else intentionally outside the native
theme model.

## Basic configuration

Themes are configured with the top-level `theme` property:

``` yaml
theme:
  light: false
  background-color: 215 33 3
  primary-color: 213 31 67
  positive-color: 130 58 70
  warning-color: 40 85 65
  negative-color: 4 100 74
  accent-color: 213 31 67
  contrast-multiplier: 2
  text-saturation-multiplier: 1.5
```

All properties are optional. Existing Glance configurations using only
the original theme properties remain valid.

## Colors

Theme colors use HSL values written as three space-separated numbers:

``` yaml
accent-color: 213 31 67
```

The values represent `hue saturation lightness`.

  Component    Range
  ------------ --------------
  Hue          `0` to `360`
  Saturation   `0` to `100`
  Lightness    `0` to `100`

Do not include commas, `hsl()`, or percent signs.

## Core properties

  ------------------------------------------------------------------------------
  Property                       Type                    Description
  ------------------------------ ----------------------- -----------------------
  `light`                        boolean                 Uses light-scheme text
                                                         and contrast behavior
                                                         when `true`.

  `background-color`             HSL                     Base dashboard
                                                         background color.

  `primary-color`                HSL                     Primary Glance color
                                                         used by links and
                                                         existing components.

  `positive-color`               HSL                     Positive status color.
                                                         Inherits from
                                                         `primary-color` when
                                                         omitted.

  `warning-color`                HSL                     Warning and caution
                                                         color.

  `negative-color`               HSL                     Negative, failure, or
                                                         error color.

  `accent-color`                 HSL                     General visual accent
                                                         used when a more
                                                         specific accent is not
                                                         configured.

  `contrast-multiplier`          number                  Adjusts derived text
                                                         contrast.

  `text-saturation-multiplier`   number                  Adjusts saturation of
                                                         derived text colors.

  `custom-css-file`              string                  Optional custom CSS
                                                         loaded after native
                                                         theme styling.

  `disable-picker`               boolean                 Disables the browser
                                                         theme picker when
                                                         `true`.

  `presets`                      object                  Defines named user
                                                         themes available from
                                                         the theme picker.
  ------------------------------------------------------------------------------

`custom-css-file`, `disable-picker`, and `presets` are top-level
theme-management settings. They are not page-level visual overrides.

## Typography

``` yaml
theme:
  typography:
    font-family: system
    font-size: medium
    font-weight: normal
    text-color: 216 100 99
    secondary-text-color: 216 24 89
    muted-text-color: 214 18 79
    headings:
      font-family: system
      font-size: large
      font-weight: bold
      text-color: 216 100 99
```

### Typography properties

  -----------------------------------------------------------------------
  Property                            Values
  ----------------------------------- -----------------------------------
  `font-family`                       `default`, `system`, `sans-serif`,
                                      `serif`, `monospace`

  `font-size`                         `small`, `medium`, `large`

  `font-weight`                       `normal`, `medium`, `semibold`,
                                      `bold`

  `text-color`                        HSL

  `secondary-text-color`              HSL

  `muted-text-color`                  HSL
  -----------------------------------------------------------------------

### Heading properties

`typography.headings` supports:

  -----------------------------------------------------------------------
  Property                            Values
  ----------------------------------- -----------------------------------
  `font-family`                       `default`, `system`, `sans-serif`,
                                      `serif`, `monospace`

  `font-size`                         `small`, `medium`, `large`

  `font-weight`                       `normal`, `medium`, `semibold`,
                                      `bold`

  `text-color`                        HSL
  -----------------------------------------------------------------------

Arbitrary font declarations remain available through custom CSS.

## Page appearance

``` yaml
theme:
  page:
    background-image: /assets/backgrounds/dashboard.jpg
    background-position: center
    background-size: cover
    background-repeat: no-repeat
    background-attachment: fixed
    ambient-accent: subtle
    overlay:
      color: 215 33 3
      opacity: 0.35
```

  -----------------------------------------------------------------------
  Property                            Values
  ----------------------------------- -----------------------------------
  `background-image`                  Local `/assets/...` path

  `background-position`               `center`, `top`, `bottom`, `left`,
                                      `right`, `top-left`, `top-right`,
                                      `bottom-left`, `bottom-right`

  `background-size`                   `auto`, `cover`, `contain`

  `background-repeat`                 `no-repeat`, `repeat`, `repeat-x`,
                                      `repeat-y`

  `background-attachment`             `scroll`, `fixed`

  `ambient-accent`                    `none`, `subtle`, `medium`,
                                      `strong`

  `overlay.color`                     HSL

  `overlay.opacity`                   Number from `0` through `1`
  -----------------------------------------------------------------------

### Background image security

Native page background images must be local assets beneath `/assets/`.

``` yaml
background-image: /assets/backgrounds/home.jpg
```

Remote URLs, protocol-relative URLs, data URLs, file URLs, path
traversal, CSS `url()` expressions, and other unsafe values are
rejected. Use `custom-css-file` for remote images or more complex CSS
backgrounds.

## Header

``` yaml
theme:
  header:
    background-color: 215 20 10
    text-color: 216 100 99
    border-color: 216 20 18
    radius: large
    shadow: medium
    blur: medium
```

  Property             Values
  -------------------- --------------------------------------
  `background-color`   HSL
  `text-color`         HSL
  `border-color`       HSL
  `radius`             `none`, `small`, `medium`, `large`
  `shadow`             `none`, `subtle`, `medium`, `strong`
  `blur`               `none`, `subtle`, `medium`, `strong`

## Navigation

``` yaml
theme:
  navigation:
    text-color: 216 24 89
    hover-color: 216 100 99
    active-color: 216 100 99
    accent-color: 213 31 67
    font-size: large
    font-weight: bold
```

  Property         Values
  ---------------- ----------------------------------------
  `text-color`     HSL
  `hover-color`    HSL
  `active-color`   HSL
  `accent-color`   HSL
  `font-size`      `small`, `medium`, `large`
  `font-weight`    `normal`, `medium`, `semibold`, `bold`

When a component-specific accent is omitted, native styling can fall
back to the general `accent-color`.

## Widgets

``` yaml
theme:
  widgets:
    background-color: 215 18 9
    border-color: 216 20 18
    radius: large
    shadow: medium
    blur: medium
```

  Property             Values
  -------------------- --------------------------------------
  `background-color`   HSL
  `border-color`       HSL
  `radius`             `none`, `small`, `medium`, `large`
  `shadow`             `none`, `subtle`, `medium`, `strong`
  `blur`               `none`, `subtle`, `medium`, `strong`

## Widget headers

``` yaml
theme:
  widget-header:
    background-color: 215 20 10
    text-color: 216 100 99
    accent-color: 213 31 67
    border-color: 216 20 18
    font-size: large
    font-weight: bold
```

  Property             Values
  -------------------- ----------------------------------------
  `background-color`   HSL
  `text-color`         HSL
  `accent-color`       HSL
  `border-color`       HSL
  `font-size`          `small`, `medium`, `large`
  `font-weight`        `normal`, `medium`, `semibold`, `bold`

## Cards

``` yaml
theme:
  cards:
    background-color: 215 16 11
    border-color: 216 18 18
    radius: medium
    shadow: subtle
```

  Property             Values
  -------------------- --------------------------------------
  `background-color`   HSL
  `border-color`       HSL
  `radius`             `none`, `small`, `medium`, `large`
  `shadow`             `none`, `subtle`, `medium`, `strong`

## Groups and tabs

``` yaml
theme:
  groups:
    background-color: 215 18 9
    text-color: 215 15 74
    hover-color: 215 20 90
    active-color: 216 100 99
    accent-color: 213 31 67
    border-color: 216 18 17
```

  Property             Values
  -------------------- --------
  `background-color`   HSL
  `text-color`         HSL
  `hover-color`        HSL
  `active-color`       HSL
  `accent-color`       HSL
  `border-color`       HSL

## Controls

``` yaml
theme:
  controls:
    background-color: 215 16 11
    text-color: 216 24 89
    muted-color: 214 18 79
    border-color: 216 20 18
    focus-color: 213 31 67
    radius: medium
    button:
      background-color: 215 18 12
      text-color: 216 24 89
```

  Property                    Values
  --------------------------- ------------------------------------
  `background-color`          HSL
  `text-color`                HSL
  `muted-color`               HSL
  `border-color`              HSL
  `focus-color`               HSL
  `radius`                    `none`, `small`, `medium`, `large`
  `button.background-color`   HSL
  `button.text-color`         HSL

When `focus-color` is omitted, native controls can fall back to the
general theme accent.

## Footer

``` yaml
theme:
  footer:
    background-color: 215 20 7
    text-color: 214 18 79
    accent-color: 213 31 67
    border-color: 216 18 16
    font-size: small
    font-weight: medium
```

  Property             Values
  -------------------- ----------------------------------------
  `background-color`   HSL
  `text-color`         HSL
  `accent-color`       HSL
  `border-color`       HSL
  `font-size`          `small`, `medium`, `large`
  `font-weight`        `normal`, `medium`, `semibold`, `bold`

## Elevated surfaces and separators

``` yaml
theme:
  surfaces:
    elevated-background-color: 215 18 10
    elevated-border-color: 216 20 19
    separator-color: 216 18 16
```

  Property                      Values
  ----------------------------- --------
  `elevated-background-color`   HSL
  `elevated-border-color`       HSL
  `separator-color`             HSL

## Page-specific themes

Any page can override visual theme properties with its own `theme`
block:

``` yaml
pages:
  - name: Home
    theme:
      accent-color: 204 100 72
      navigation:
        active-color: 204 100 72
      widget-header:
        accent-color: 204 100 72
    columns:
      # ...

  - name: News
    theme:
      accent-color: 40 74 63
    columns:
      # ...
```

Page themes are partial overrides. You do not need to repeat the global
theme.

``` yaml
theme:
  widgets:
    background-color: 215 18 9
    border-color: 216 20 18
    radius: large

pages:
  - name: Home
    theme:
      widgets:
        border-color: 204 100 72
```

On Home, only the widget border changes. The widget background and
radius continue to inherit from the selected base theme.

Inheritance applies recursively to nested theme groups. Explicit values
such as `false` and `0` are preserved rather than being treated as
omitted.

## Theme picker

When enabled, the theme picker contains:

1.  **Glance Dark**
2.  **Glance Light**
3.  Any explicitly named user themes defined under `presets`

The top-level `theme` configuration defines the configured default
appearance. It does **not** become an additional picker entry.

To disable theme switching:

``` yaml
theme:
  disable-picker: true
```

## Named user themes

Additional selectable themes are defined beneath `presets`:

``` yaml
theme:
  disable-picker: false

  presets:
    midnight:
      light: false
      background-color: 255 28 5
      primary-color: 275 75 70
      positive-color: 145 55 62
      warning-color: 40 85 65
      negative-color: 4 85 65
      accent-color: 285 80 72
      contrast-multiplier: 1.5
      text-saturation-multiplier: 1.2
      widgets:
        background-color: 255 20 11
        border-color: 285 30 28
        radius: medium
        shadow: strong
```

A named theme is a complete alternative base theme, not an overlay on
the top-level theme.

When a named theme is selected, the current page theme override is
applied after that selected theme:

``` text
Midnight
   ↓
Home page override
   ↓
custom-css-file
```

The same page override therefore works with Glance Dark, Glance Light,
and every named user theme.

## Complete example

``` yaml
theme:
  light: false
  background-color: 215 33 3
  primary-color: 213 31 67
  positive-color: 130 58 70
  warning-color: 40 85 65
  negative-color: 4 100 74
  accent-color: 213 31 67
  contrast-multiplier: 2
  text-saturation-multiplier: 1.5
  disable-picker: false

  typography:
    font-family: system
    font-size: medium
    font-weight: normal
    text-color: 216 100 99
    secondary-text-color: 216 24 89
    muted-text-color: 214 18 79
    headings:
      font-family: system
      font-size: large
      font-weight: bold
      text-color: 216 100 99

  page:
    ambient-accent: subtle
    background-attachment: fixed

  header:
    background-color: 215 20 10
    text-color: 216 100 99
    border-color: 216 20 18
    radius: large
    shadow: medium
    blur: medium

  navigation:
    text-color: 216 24 89
    hover-color: 216 100 99
    active-color: 216 100 99
    font-size: large
    font-weight: bold

  widgets:
    background-color: 215 18 9
    border-color: 216 20 18
    radius: large
    shadow: medium
    blur: medium

  widget-header:
    background-color: 215 20 10
    text-color: 216 100 99
    border-color: 216 20 18
    font-size: large
    font-weight: bold

  cards:
    background-color: 215 16 11
    border-color: 216 18 18
    radius: medium
    shadow: subtle

  groups:
    background-color: 215 18 9
    text-color: 215 15 74
    hover-color: 215 20 90
    active-color: 216 100 99
    border-color: 216 18 17

  controls:
    background-color: 215 16 11
    text-color: 216 24 89
    muted-color: 214 18 79
    border-color: 216 20 18
    radius: medium
    button:
      background-color: 215 18 12
      text-color: 216 24 89

  footer:
    background-color: 215 20 7
    text-color: 214 18 79
    border-color: 216 18 16
    font-size: small
    font-weight: medium

  surfaces:
    elevated-background-color: 215 18 10
    elevated-border-color: 216 20 19
    separator-color: 216 18 16

  presets:
    midnight:
      light: false
      background-color: 255 28 5
      primary-color: 275 75 70
      positive-color: 145 55 62
      warning-color: 40 85 65
      negative-color: 4 85 65
      accent-color: 285 80 72
      contrast-multiplier: 1.5
      text-saturation-multiplier: 1.2
      widgets:
        background-color: 255 20 11
        border-color: 285 30 28
        radius: medium
        shadow: strong

pages:
  - name: Home
    theme:
      accent-color: 204 100 72
    columns:
      # ...

  - name: News
    theme:
      accent-color: 40 74 63
    columns:
      # ...
```

## Custom CSS

Native themes deliberately do not expose every CSS property. For
advanced styling:

``` yaml
theme:
  custom-css-file: /assets/my-style.css
```

Custom CSS is applied after the resolved native theme and can override
native styling.

Use custom CSS for things such as:

-   Complex gradients
-   Detailed hover effects
-   Custom transforms and animations
-   Specialized scrollbar styling
-   Arbitrary fonts
-   Highly specific widget presentation
-   Presentation that cannot be expressed by the safe native theme
    options

Widgets expose `widget-type-{name}` classes and support `css-class` for
dedicated selectors.

``` css
.widget-type-rss a {
    font-size: 1.5rem;
}
```

## Validation

Native theme values are validated when configuration is loaded.

Invalid HSL ranges, unsupported enum values, invalid opacity values,
unsafe background-image paths, and other invalid native theme values
cause a configuration error rather than being inserted into generated
CSS.

This keeps native configuration bounded and predictable while leaving
unrestricted advanced presentation to `custom-css-file`.

## Simple starter themes

A theme does not need to use the complete schema. Compact Glance themes
remain fully supported.

### Gruvbox Dark

``` yaml
theme:
  background-color: 0 0 16
  primary-color: 43 59 81
  positive-color: 61 66 44
  negative-color: 6 96 59
```

### Catppuccin Mocha

``` yaml
theme:
  background-color: 240 21 15
  primary-color: 217 92 76
  positive-color: 115 54 76
  negative-color: 343 81 75
```

### Catppuccin Latte

``` yaml
theme:
  light: true
  background-color: 220 23 95
  primary-color: 220 91 54
  positive-color: 109 58 40
  negative-color: 347 87 44
```

These compact themes can be expanded with any of the native semantic
properties documented above.
