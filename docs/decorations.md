# Decorations

[Glance README](../README.md) · [Configuration](configuration.md) · [Icons](configuration.md#icons)

Decorations add a strand of glowing lights around the viewport and artwork on
either side of the footer center. They are global to dashboard pages, separate
from Glance's color themes and widgets. Login, documentation and error pages
remain undecorated. Omit the section, or set `enabled: false`, to disable them.

## Manual preset

```yaml
decorations:
  enabled: true
  theme: halloween
  lights: true
  footer-icons: true
  animation: false
```

| Property | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | Enable dashboard decorations. |
| `theme` | Required when enabled | A built-in preset, `custom`, or `auto`. |
| `lights` | `true` | Draw lights along all four viewport edges. |
| `footer-icons` | `true` | Display configured artwork beside the footer center. |
| `animation` | `false` | Vary each bulb independently with slow brightness changes; reduced-motion preferences take precedence. |
| `schedule` | None | Required only with `auto`. |

| Category | Built-in presets |
| --- | --- |
| Seasons | `spring`, `summer`, `autumn`, `winter` |
| Winter holidays | `new-year`, `martin-luther-king-day`, `presidents-day`, `valentines-day` |
| Spring holidays | `st-patricks-day`, `easter`, `mothers-day`, `memorial-day` |
| Summer holidays | `juneteenth`, `fathers-day`, `independence-day`, `labor-day` |
| Autumn and year-end holidays | `columbus-day`, `indigenous-peoples-day`, `halloween`, `veterans-day`, `thanksgiving`, `christmas` |

The categories organize the catalog; they do not determine activation dates.
Presets define their own colors and artwork; custom fields on a preset are errors.

Lights use one continuous rounded wire. Animated bulbs have deterministic,
independent phases and 3-6 second brightness transitions, staying softly lit
instead of blinking off. Animation affects brightness only, not bulb position.

Lights do not intercept clicks or change widget theme colors. On mobile, the
bottom strand sits above navigation. Footer artwork follows the normal footer:
`branding.hide-footer` hides it, while lights remain enabled. A custom footer
keeps its content and receives artwork on either side. Artwork shrinks on narrow
screens; large custom footer content still needs to fit the available width.

## Custom design

```yaml
decorations:
  enabled: true
  theme: custom
  light-colors: ["#cc5500", "#daa520"]
  footer-left-icon: twemoji:1f341
  footer-right-icon: twemoji:1f342
```

The three custom fields are valid **only with `theme: custom`**, including entries
inside an automatic schedule. `light-colors` accepts one to eight `#RRGGBB`
colors and is required when lights are enabled. At least one footer icon is
required when footer icons are enabled; omit either side to leave it empty.

Icon fields accept the shared `twemoji:`, `mdi:`, `si:`, `di:`, `sh:` and `hla:`
references, or safe local `/assets/` paths served through `server.assets-path`.
Arbitrary remote URLs, credentials and traversal paths are not accepted here.
Existing icon-source inversion behavior is preserved. Local paths also work
under a configured server base URL.

## Automatic annual schedule

```yaml
decorations:
  enabled: true
  theme: auto
  lights: true
  footer-icons: true
  animation: false
  schedule:
    - theme: halloween
      start: "10-01"
      end: "10-31"
    - theme: custom
      start: "11-01"
      end: "11-30"
      light-colors: ["#cc5500", "#daa520"]
      footer-left-icon: twemoji:1f341
      footer-right-icon: twemoji:1f342
    - theme: christmas
      start: "12-01"
      end: "12-30"
    - theme: new-year
      start: "12-31"
      end: "01-05"
```

Dates use quoted `MM-DD`, are inclusive, and repeat each year. Ranges may cross
New Year. February 29 matches only in leap years. Overlaps, invalid dates, empty
schedules, nested `auto`, and unknown fields/presets are configuration errors.
Adjacent ranges must not share an endpoint. `auto` selects from the ranges
you configure; it does not supply a default holiday calendar or calculate
movable dates such as Easter, Thanksgiving, Mother's Day, or Labor Day.
Use a deliberate annual window (for example November for Thanksgiving), or
update the configured date range for a specific year's holiday. To mix seasons
and holidays, split seasonal ranges around holiday windows; overlapping ranges
do not have implicit holiday priority. Outside scheduled ranges nothing is
displayed. The shared switches apply to every entry; individual entries contain
only the design and date range.

The viewer's **browser-local date** controls the schedule, not the container
timezone. Open dashboards update at midnight and recheck after tab visibility
or browser history restoration. A bounded minute check catches device clock or
timezone changes. No backend widget refresh or page reload is required for a
date transition. Configuration edits follow Glance's normal reload behavior;
reload an already open dashboard to obtain the new configuration.

## Artwork and preset maintenance

Twemoji artwork is fetched by the browser from jsDelivr at a pinned version;
no holiday images or Twemoji JavaScript library are bundled. CDN access is
required unless custom local artwork is used. A failed image is hidden and
reported through the existing frontend diagnostics without logging its URL.
Lights and dashboard content continue working.

Browse the [Twemoji gallery](https://jdecked.github.io/twemoji/v/latest/preview-svg.html).
Artwork is from [Twemoji](https://github.com/jdecked/twemoji), originally Twitter
and other contributors, maintained by jdecked and contributors, and licensed
under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). Built-in references
use unmodified artwork. The gallery may show a newer version than Glance's pin.

Maintainers edit `internal/glance/decoration-themes.yml` to add presets. Use a
unique lowercase hyphenated name, one to eight hex colors, and icon references.
`auto` and `custom` are reserved. The embedded catalog is validated by tests and
when decoration configuration is loaded. Rebuilding Glance incorporates catalog
changes; this is not a second runtime configuration file. Add matching public
test profiles and managed screenshot recipes for new presets.

## Visual examples

These images are generated from real source-runtime profiles through the
managed documentation capture workflow.

![Halloween lights and footer artwork](images/pages/decorations-halloween.png)
![Halloween footer detail](images/instructions/decorations-footer.png)
![Mobile decorations](images/pages/decorations-mobile.png)
![Custom decoration design](images/pages/decorations-custom.png)
![Thanksgiving decorations](images/pages/decorations-thanksgiving.png)
![Christmas decorations](images/pages/decorations-christmas.png)
![New Year decorations](images/pages/decorations-new-year.png)
![Valentine's Day decorations](images/pages/decorations-valentines-day.png)
![St. Patrick's Day decorations](images/pages/decorations-st-patricks-day.png)
![Easter decorations](images/pages/decorations-easter.png)
![Independence Day decorations](images/pages/decorations-independence-day.png)
![Autumn decorations](images/pages/decorations-autumn.png)
![Winter decorations](images/pages/decorations-winter.png)

![Spring decorations](images/pages/decorations-spring.png)
![Summer decorations](images/pages/decorations-summer.png)
![Martin Luther King Jr. Day decorations](images/pages/decorations-martin-luther-king-day.png)
![Presidents' Day decorations](images/pages/decorations-presidents-day.png)
![Memorial Day decorations](images/pages/decorations-memorial-day.png)
![Juneteenth decorations](images/pages/decorations-juneteenth.png)
![Labor Day decorations](images/pages/decorations-labor-day.png)
![Columbus Day decorations](images/pages/decorations-columbus-day.png)
![Indigenous Peoples' Day decorations](images/pages/decorations-indigenous-peoples-day.png)
![Veterans Day decorations](images/pages/decorations-veterans-day.png)
![Mother's Day decorations](images/pages/decorations-mothers-day.png)
![Father's Day decorations](images/pages/decorations-fathers-day.png)

---

[Glance README](../README.md) · [Configuration](configuration.md) · [Back to top](#decorations)
