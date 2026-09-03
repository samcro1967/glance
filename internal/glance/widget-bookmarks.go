package glance

import (
	"html/template"

	"gopkg.in/yaml.v3"
)

var bookmarksWidgetTemplate = mustParseTemplate("bookmarks.html", "widget-base.html")

type bookmarkLink struct {
	Title        string          `yaml:"title"`
	URL          string          `yaml:"url"`
	Description  string          `yaml:"description"`
	Icon         customIconField `yaml:"icon"`
	SameTabRaw   *bool           `yaml:"same-tab"`
	SameTab      bool            `yaml:"-"`
	HideArrowRaw *bool           `yaml:"hide-arrow"`
	HideArrow    bool            `yaml:"-"`
	Target       string          `yaml:"target"`
}

type bookmarkGroup struct {
	Title            string               `yaml:"title"`
	Color            *hslColorField       `yaml:"color"`
	SameTab          bool                 `yaml:"same-tab"`
	HideArrow        bool                 `yaml:"hide-arrow"`
	Target           string               `yaml:"target"`
	Links            []bookmarkLink       `yaml:"links"`
	configuredFields yamlConfiguredFields `yaml:"-"`
}

func (group *bookmarkGroup) UnmarshalYAML(node *yaml.Node) error {
	type plain bookmarkGroup
	if err := node.Decode((*plain)(group)); err != nil {
		return err
	}

	group.configuredFields = yamlMappingFields(node)
	return nil
}

type bookmarksWidget struct {
	widgetBase `yaml:",inline"`
	cachedHTML template.HTML   `yaml:"-"`
	Groups     []bookmarkGroup `yaml:"groups"`
}

func (widget *bookmarksWidget) initialize() error {
	widget.withTitle("Bookmarks").withError(nil)

	for g := range widget.Groups {
		group := &widget.Groups[g]
		for l := range group.Links {
			link := &group.Links[l]
			if link.SameTabRaw == nil {
				link.SameTab = group.SameTab
			} else {
				link.SameTab = *link.SameTabRaw
			}

			if link.HideArrowRaw == nil {
				link.HideArrow = group.HideArrow
			} else {
				link.HideArrow = *link.HideArrowRaw
			}

			if link.Target == "" {
				if group.Target != "" {
					link.Target = group.Target
				} else {
					if link.SameTab {
						link.Target = ""
					} else {
						link.Target = "_blank"
					}
				}
			}
		}
	}

	widget.cachedHTML = widget.renderTemplate(widget, bookmarksWidgetTemplate)

	return nil
}

func (widget *bookmarksWidget) Render() template.HTML {
	return widget.cachedHTML
}

func (widget *bookmarksWidget) setDefaultNewTab(value bool) {
	for i := range widget.Groups {
		group := &widget.Groups[i]

		if group.configuredFields["same-tab"] {
			continue
		}

		group.SameTab = !value
	}
}
