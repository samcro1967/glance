package glance

import (
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	themeStyleTemplate         = mustParseTemplate("theme-style.gotmpl")
	themePresetPreviewTemplate = mustParseTemplate("theme-preset-preview.html")
)

func (a *application) handleThemeChangeRequest(w http.ResponseWriter, r *http.Request) {
	themeKey := r.PathValue("key")

	properties, exists := a.Config.Theme.Presets.Get(themeKey)
	if !exists && themeKey != "default" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if themeKey == "default" {
		properties = &a.Config.Theme.themeProperties
	}

	resolved := properties
	if pageSlug := r.URL.Query().Get("page"); pageSlug != "" {
		page, exists := a.slugToPage[pageSlug]
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var err error
		resolved, err = resolveTheme(properties, &page.Theme)
		if err != nil {
			writeInternalServerError(w, "Failed to resolve theme", err)
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "theme",
		Value:    themeKey,
		Path:     a.Config.Server.BaseURL + "/",
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(2 * 365 * 24 * time.Hour),
	})

	w.Header().Set("Content-Type", "text/css")
	w.Header().Set("X-Scheme", ternary(resolved.Light, "light", "dark"))
	w.Write([]byte(resolved.CSS))
}

type themeConfiguredProperties struct {
	configuredFields yamlConfiguredFields `yaml:"-"`
}

func (p *themeConfiguredProperties) captureConfiguredFields(node *yaml.Node) {
	p.configuredFields = yamlMappingFields(node)
}

type themeHeadingProperties struct {
	themeConfiguredProperties `yaml:"-"`
	FontFamily                string         `yaml:"font-family"`
	FontSize                  string         `yaml:"font-size"`
	FontWeight                string         `yaml:"font-weight"`
	TextColor                 *hslColorField `yaml:"text-color"`
}

type themeTypographyProperties struct {
	themeConfiguredProperties `yaml:"-"`
	FontFamily                string                 `yaml:"font-family"`
	FontSize                  string                 `yaml:"font-size"`
	FontWeight                string                 `yaml:"font-weight"`
	TextColor                 *hslColorField         `yaml:"text-color"`
	SecondaryTextColor        *hslColorField         `yaml:"secondary-text-color"`
	MutedTextColor            *hslColorField         `yaml:"muted-text-color"`
	Headings                  themeHeadingProperties `yaml:"headings"`
}

type themePageOverlayProperties struct {
	themeConfiguredProperties `yaml:"-"`
	Color                     *hslColorField `yaml:"color"`
	Opacity                   *float32       `yaml:"opacity"`
}

type themePageProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundImage           string                     `yaml:"background-image"`
	BackgroundPosition        string                     `yaml:"background-position"`
	BackgroundSize            string                     `yaml:"background-size"`
	BackgroundRepeat          string                     `yaml:"background-repeat"`
	BackgroundAttachment      string                     `yaml:"background-attachment"`
	AmbientAccent             string                     `yaml:"ambient-accent"`
	Overlay                   themePageOverlayProperties `yaml:"overlay"`
}

type themeSurfaceProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	BorderColor               *hslColorField `yaml:"border-color"`
	Radius                    string         `yaml:"radius"`
	Shadow                    string         `yaml:"shadow"`
	Blur                      string         `yaml:"blur"`
}

type themeHeaderProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	TextColor                 *hslColorField `yaml:"text-color"`
	BorderColor               *hslColorField `yaml:"border-color"`
	Radius                    string         `yaml:"radius"`
	Shadow                    string         `yaml:"shadow"`
	Blur                      string         `yaml:"blur"`
}

type themeCardProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	BorderColor               *hslColorField `yaml:"border-color"`
	Radius                    string         `yaml:"radius"`
	Shadow                    string         `yaml:"shadow"`
}

type themeNavigationProperties struct {
	themeConfiguredProperties `yaml:"-"`
	TextColor                 *hslColorField `yaml:"text-color"`
	HoverColor                *hslColorField `yaml:"hover-color"`
	ActiveColor               *hslColorField `yaml:"active-color"`
	AccentColor               *hslColorField `yaml:"accent-color"`
	FontSize                  string         `yaml:"font-size"`
	FontWeight                string         `yaml:"font-weight"`
}

type themeWidgetHeaderProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	TextColor                 *hslColorField `yaml:"text-color"`
	AccentColor               *hslColorField `yaml:"accent-color"`
	BorderColor               *hslColorField `yaml:"border-color"`
	FontSize                  string         `yaml:"font-size"`
	FontWeight                string         `yaml:"font-weight"`
}

type themeGroupProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	TextColor                 *hslColorField `yaml:"text-color"`
	HoverColor                *hslColorField `yaml:"hover-color"`
	ActiveColor               *hslColorField `yaml:"active-color"`
	AccentColor               *hslColorField `yaml:"accent-color"`
	BorderColor               *hslColorField `yaml:"border-color"`
}

type themeButtonProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	TextColor                 *hslColorField `yaml:"text-color"`
}

type themeControlProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField        `yaml:"background-color"`
	TextColor                 *hslColorField        `yaml:"text-color"`
	MutedColor                *hslColorField        `yaml:"muted-color"`
	BorderColor               *hslColorField        `yaml:"border-color"`
	FocusColor                *hslColorField        `yaml:"focus-color"`
	Radius                    string                `yaml:"radius"`
	Button                    themeButtonProperties `yaml:"button"`
}

type themeFooterProperties struct {
	themeConfiguredProperties `yaml:"-"`
	BackgroundColor           *hslColorField `yaml:"background-color"`
	TextColor                 *hslColorField `yaml:"text-color"`
	AccentColor               *hslColorField `yaml:"accent-color"`
	BorderColor               *hslColorField `yaml:"border-color"`
	FontSize                  string         `yaml:"font-size"`
	FontWeight                string         `yaml:"font-weight"`
}

type themeElevatedSurfaceProperties struct {
	themeConfiguredProperties `yaml:"-"`
	ElevatedBackgroundColor   *hslColorField `yaml:"elevated-background-color"`
	ElevatedBorderColor       *hslColorField `yaml:"elevated-border-color"`
	SeparatorColor            *hslColorField `yaml:"separator-color"`
}

type themeProperties struct {
	BackgroundColor          *hslColorField `yaml:"background-color"`
	PrimaryColor             *hslColorField `yaml:"primary-color"`
	PositiveColor            *hslColorField `yaml:"positive-color"`
	NegativeColor            *hslColorField `yaml:"negative-color"`
	Light                    bool           `yaml:"light"`
	ContrastMultiplier       float32        `yaml:"contrast-multiplier"`
	TextSaturationMultiplier float32        `yaml:"text-saturation-multiplier"`

	AccentColor  *hslColorField `yaml:"accent-color"`
	WarningColor *hslColorField `yaml:"warning-color"`

	Typography   themeTypographyProperties      `yaml:"typography"`
	Page         themePageProperties            `yaml:"page"`
	Header       themeHeaderProperties          `yaml:"header"`
	Navigation   themeNavigationProperties      `yaml:"navigation"`
	Widgets      themeSurfaceProperties         `yaml:"widgets"`
	WidgetHeader themeWidgetHeaderProperties    `yaml:"widget-header"`
	Cards        themeCardProperties            `yaml:"cards"`
	Groups       themeGroupProperties           `yaml:"groups"`
	Controls     themeControlProperties         `yaml:"controls"`
	Footer       themeFooterProperties          `yaml:"footer"`
	Surfaces     themeElevatedSurfaceProperties `yaml:"surfaces"`

	Key                  string        `yaml:"-"`
	Name                 string        `yaml:"-"`
	CSS                  template.CSS  `yaml:"-"`
	PreviewHTML          template.HTML `yaml:"-"`
	BackgroundColorAsHex string        `yaml:"-"`

	configuredFields yamlConfiguredFields `yaml:"-"`
}

func captureThemeConfiguredFields(config *config, contents []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return err
	}

	if len(document.Content) == 0 {
		return nil
	}

	root := document.Content[0]

	_, themeNode := yamlMappingValue(root, "theme")
	if themeNode != nil {
		config.Theme.captureConfiguredFields(themeNode)

		_, presetsNode := yamlMappingValue(themeNode, "presets")
		if presetsNode != nil && presetsNode.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(presetsNode.Content); i += 2 {
				key := presetsNode.Content[i].Value
				preset, exists := config.Theme.Presets.Get(key)
				if !exists {
					continue
				}
				preset.captureConfiguredFields(presetsNode.Content[i+1])
			}
		}
	}

	_, pagesNode := yamlMappingValue(root, "pages")
	if pagesNode != nil && pagesNode.Kind == yaml.SequenceNode {
		for i, pageNode := range pagesNode.Content {
			if i >= len(config.Pages) {
				break
			}

			_, pageThemeNode := yamlMappingValue(pageNode, "theme")
			if pageThemeNode != nil {
				config.Pages[i].Theme.captureConfiguredFields(pageThemeNode)
			}
		}
	}

	return nil
}

func (t *themeProperties) captureConfiguredFields(node *yaml.Node) {
	t.configuredFields = yamlMappingFields(node)

	t.captureNestedConfiguredFields(node, "typography", &t.Typography.themeConfiguredProperties)
	if _, typographyNode := yamlMappingValue(node, "typography"); typographyNode != nil {
		t.Typography.captureNestedConfiguredFields(typographyNode)
	}

	t.captureNestedConfiguredFields(node, "page", &t.Page.themeConfiguredProperties)
	if _, pageNode := yamlMappingValue(node, "page"); pageNode != nil {
		t.Page.captureNestedConfiguredFields(pageNode)
	}

	t.captureNestedConfiguredFields(node, "header", &t.Header.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "navigation", &t.Navigation.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "widgets", &t.Widgets.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "widget-header", &t.WidgetHeader.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "cards", &t.Cards.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "groups", &t.Groups.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "controls", &t.Controls.themeConfiguredProperties)
	if _, controlsNode := yamlMappingValue(node, "controls"); controlsNode != nil {
		t.Controls.captureNestedConfiguredFields(controlsNode)
	}

	t.captureNestedConfiguredFields(node, "footer", &t.Footer.themeConfiguredProperties)
	t.captureNestedConfiguredFields(node, "surfaces", &t.Surfaces.themeConfiguredProperties)
}

func (t *themeProperties) captureNestedConfiguredFields(
	node *yaml.Node,
	key string,
	properties *themeConfiguredProperties,
) {
	_, valueNode := yamlMappingValue(node, key)
	properties.captureConfiguredFields(valueNode)
}

func (t *themeTypographyProperties) captureNestedConfiguredFields(node *yaml.Node) {
	_, headingsNode := yamlMappingValue(node, "headings")
	t.Headings.captureConfiguredFields(headingsNode)
}

func (t *themePageProperties) captureNestedConfiguredFields(node *yaml.Node) {
	_, overlayNode := yamlMappingValue(node, "overlay")
	t.Overlay.captureConfiguredFields(overlayNode)
}

func (t *themeControlProperties) captureNestedConfiguredFields(node *yaml.Node) {
	_, buttonNode := yamlMappingValue(node, "button")
	t.Button.captureConfiguredFields(buttonNode)
}

func mergeThemeTypography(base themeTypographyProperties, override themeTypographyProperties) themeTypographyProperties {
	merged := base

	if override.configuredFields["font-family"] {
		merged.FontFamily = override.FontFamily
	}
	if override.configuredFields["font-size"] {
		merged.FontSize = override.FontSize
	}
	if override.configuredFields["font-weight"] {
		merged.FontWeight = override.FontWeight
	}
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["secondary-text-color"] {
		merged.SecondaryTextColor = override.SecondaryTextColor
	}
	if override.configuredFields["muted-text-color"] {
		merged.MutedTextColor = override.MutedTextColor
	}

	if override.Headings.configuredFields["font-family"] {
		merged.Headings.FontFamily = override.Headings.FontFamily
	}
	if override.Headings.configuredFields["font-size"] {
		merged.Headings.FontSize = override.Headings.FontSize
	}
	if override.Headings.configuredFields["font-weight"] {
		merged.Headings.FontWeight = override.Headings.FontWeight
	}
	if override.Headings.configuredFields["text-color"] {
		merged.Headings.TextColor = override.Headings.TextColor
	}

	return merged
}

func mergeThemePage(base themePageProperties, override themePageProperties) themePageProperties {
	merged := base

	if override.configuredFields["background-image"] {
		merged.BackgroundImage = override.BackgroundImage
	}
	if override.configuredFields["background-position"] {
		merged.BackgroundPosition = override.BackgroundPosition
	}
	if override.configuredFields["background-size"] {
		merged.BackgroundSize = override.BackgroundSize
	}
	if override.configuredFields["background-repeat"] {
		merged.BackgroundRepeat = override.BackgroundRepeat
	}
	if override.configuredFields["background-attachment"] {
		merged.BackgroundAttachment = override.BackgroundAttachment
	}
	if override.configuredFields["ambient-accent"] {
		merged.AmbientAccent = override.AmbientAccent
	}
	if override.Overlay.configuredFields["color"] {
		merged.Overlay.Color = override.Overlay.Color
	}
	if override.Overlay.configuredFields["opacity"] {
		merged.Overlay.Opacity = override.Overlay.Opacity
	}

	return merged
}

func mergeThemeHeader(base themeHeaderProperties, override themeHeaderProperties) themeHeaderProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	if override.configuredFields["radius"] {
		merged.Radius = override.Radius
	}
	if override.configuredFields["shadow"] {
		merged.Shadow = override.Shadow
	}
	if override.configuredFields["blur"] {
		merged.Blur = override.Blur
	}
	return merged
}

func mergeThemeSurface(base themeSurfaceProperties, override themeSurfaceProperties) themeSurfaceProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	if override.configuredFields["radius"] {
		merged.Radius = override.Radius
	}
	if override.configuredFields["shadow"] {
		merged.Shadow = override.Shadow
	}
	if override.configuredFields["blur"] {
		merged.Blur = override.Blur
	}
	return merged
}

func mergeThemeCard(base themeCardProperties, override themeCardProperties) themeCardProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	if override.configuredFields["radius"] {
		merged.Radius = override.Radius
	}
	if override.configuredFields["shadow"] {
		merged.Shadow = override.Shadow
	}
	return merged
}

func mergeThemeNavigation(base themeNavigationProperties, override themeNavigationProperties) themeNavigationProperties {
	merged := base
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["hover-color"] {
		merged.HoverColor = override.HoverColor
	}
	if override.configuredFields["active-color"] {
		merged.ActiveColor = override.ActiveColor
	}
	if override.configuredFields["accent-color"] {
		merged.AccentColor = override.AccentColor
	}
	if override.configuredFields["font-size"] {
		merged.FontSize = override.FontSize
	}
	if override.configuredFields["font-weight"] {
		merged.FontWeight = override.FontWeight
	}
	return merged
}

func mergeThemeWidgetHeader(base themeWidgetHeaderProperties, override themeWidgetHeaderProperties) themeWidgetHeaderProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["accent-color"] {
		merged.AccentColor = override.AccentColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	if override.configuredFields["font-size"] {
		merged.FontSize = override.FontSize
	}
	if override.configuredFields["font-weight"] {
		merged.FontWeight = override.FontWeight
	}
	return merged
}

func mergeThemeGroup(base themeGroupProperties, override themeGroupProperties) themeGroupProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["hover-color"] {
		merged.HoverColor = override.HoverColor
	}
	if override.configuredFields["active-color"] {
		merged.ActiveColor = override.ActiveColor
	}
	if override.configuredFields["accent-color"] {
		merged.AccentColor = override.AccentColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	return merged
}

func mergeThemeControl(base themeControlProperties, override themeControlProperties) themeControlProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["muted-color"] {
		merged.MutedColor = override.MutedColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	if override.configuredFields["focus-color"] {
		merged.FocusColor = override.FocusColor
	}
	if override.configuredFields["radius"] {
		merged.Radius = override.Radius
	}
	if override.Button.configuredFields["background-color"] {
		merged.Button.BackgroundColor = override.Button.BackgroundColor
	}
	if override.Button.configuredFields["text-color"] {
		merged.Button.TextColor = override.Button.TextColor
	}
	return merged
}

func mergeThemeFooter(base themeFooterProperties, override themeFooterProperties) themeFooterProperties {
	merged := base
	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["text-color"] {
		merged.TextColor = override.TextColor
	}
	if override.configuredFields["accent-color"] {
		merged.AccentColor = override.AccentColor
	}
	if override.configuredFields["border-color"] {
		merged.BorderColor = override.BorderColor
	}
	if override.configuredFields["font-size"] {
		merged.FontSize = override.FontSize
	}
	if override.configuredFields["font-weight"] {
		merged.FontWeight = override.FontWeight
	}
	return merged
}

func mergeThemeElevatedSurface(base themeElevatedSurfaceProperties, override themeElevatedSurfaceProperties) themeElevatedSurfaceProperties {
	merged := base
	if override.configuredFields["elevated-background-color"] {
		merged.ElevatedBackgroundColor = override.ElevatedBackgroundColor
	}
	if override.configuredFields["elevated-border-color"] {
		merged.ElevatedBorderColor = override.ElevatedBorderColor
	}
	if override.configuredFields["separator-color"] {
		merged.SeparatorColor = override.SeparatorColor
	}
	return merged
}

func mergeThemeProperties(base themeProperties, override themeProperties) themeProperties {
	merged := base
	merged.Typography = mergeThemeTypography(base.Typography, override.Typography)
	merged.Page = mergeThemePage(base.Page, override.Page)
	merged.Header = mergeThemeHeader(base.Header, override.Header)
	merged.Navigation = mergeThemeNavigation(base.Navigation, override.Navigation)
	merged.Widgets = mergeThemeSurface(base.Widgets, override.Widgets)
	merged.WidgetHeader = mergeThemeWidgetHeader(base.WidgetHeader, override.WidgetHeader)
	merged.Cards = mergeThemeCard(base.Cards, override.Cards)
	merged.Groups = mergeThemeGroup(base.Groups, override.Groups)
	merged.Controls = mergeThemeControl(base.Controls, override.Controls)
	merged.Footer = mergeThemeFooter(base.Footer, override.Footer)
	merged.Surfaces = mergeThemeElevatedSurface(base.Surfaces, override.Surfaces)

	if override.configuredFields["background-color"] {
		merged.BackgroundColor = override.BackgroundColor
	}
	if override.configuredFields["primary-color"] {
		merged.PrimaryColor = override.PrimaryColor
	}
	if override.configuredFields["positive-color"] {
		merged.PositiveColor = override.PositiveColor
	}
	if override.configuredFields["negative-color"] {
		merged.NegativeColor = override.NegativeColor
	}
	if override.configuredFields["light"] {
		merged.Light = override.Light
	}
	if override.configuredFields["contrast-multiplier"] {
		merged.ContrastMultiplier = override.ContrastMultiplier
	}
	if override.configuredFields["text-saturation-multiplier"] {
		merged.TextSaturationMultiplier = override.TextSaturationMultiplier
	}
	if override.configuredFields["accent-color"] {
		merged.AccentColor = override.AccentColor
	}
	if override.configuredFields["warning-color"] {
		merged.WarningColor = override.WarningColor
	}

	return merged
}

func validateThemeEnum(path string, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}

	return fmt.Errorf("%s must be one of: %s; got %q", path, strings.Join(allowed, ", "), value)
}

func (t *themeProperties) validateTypography(path string) error {
	fontFamilies := []string{"default", "system", "sans-serif", "serif", "monospace"}
	fontSizes := []string{"small", "medium", "large"}
	fontWeights := []string{"normal", "medium", "semibold", "bold"}

	if t.Typography.configuredFields["font-family"] {
		if err := validateThemeEnum(path+".typography.font-family", t.Typography.FontFamily, fontFamilies...); err != nil {
			return err
		}
	}
	if t.Typography.configuredFields["font-size"] {
		if err := validateThemeEnum(path+".typography.font-size", t.Typography.FontSize, fontSizes...); err != nil {
			return err
		}
	}
	if t.Typography.configuredFields["font-weight"] {
		if err := validateThemeEnum(path+".typography.font-weight", t.Typography.FontWeight, fontWeights...); err != nil {
			return err
		}
	}
	if t.Typography.Headings.configuredFields["font-family"] {
		if err := validateThemeEnum(path+".typography.headings.font-family", t.Typography.Headings.FontFamily, fontFamilies...); err != nil {
			return err
		}
	}
	if t.Typography.Headings.configuredFields["font-size"] {
		if err := validateThemeEnum(path+".typography.headings.font-size", t.Typography.Headings.FontSize, fontSizes...); err != nil {
			return err
		}
	}
	if t.Typography.Headings.configuredFields["font-weight"] {
		if err := validateThemeEnum(path+".typography.headings.font-weight", t.Typography.Headings.FontWeight, fontWeights...); err != nil {
			return err
		}
	}

	return nil
}

func (t *themeProperties) validateSurfaceTreatments(path string) error {
	radii := []string{"none", "small", "medium", "large"}
	effects := []string{"none", "subtle", "medium", "strong"}

	properties := []struct {
		configured bool
		path       string
		value      string
		allowed    []string
	}{
		{t.Header.configuredFields["radius"], path + ".header.radius", t.Header.Radius, radii},
		{t.Widgets.configuredFields["radius"], path + ".widgets.radius", t.Widgets.Radius, radii},
		{t.Cards.configuredFields["radius"], path + ".cards.radius", t.Cards.Radius, radii},
		{t.Controls.configuredFields["radius"], path + ".controls.radius", t.Controls.Radius, radii},
		{t.Header.configuredFields["shadow"], path + ".header.shadow", t.Header.Shadow, effects},
		{t.Widgets.configuredFields["shadow"], path + ".widgets.shadow", t.Widgets.Shadow, effects},
		{t.Cards.configuredFields["shadow"], path + ".cards.shadow", t.Cards.Shadow, effects},
		{t.Header.configuredFields["blur"], path + ".header.blur", t.Header.Blur, effects},
		{t.Widgets.configuredFields["blur"], path + ".widgets.blur", t.Widgets.Blur, effects},
	}

	for _, property := range properties {
		if !property.configured {
			continue
		}
		if err := validateThemeEnum(property.path, property.value, property.allowed...); err != nil {
			return err
		}
	}

	return nil
}

var themeBackgroundImagePattern = regexp.MustCompile(`^/assets/[A-Za-z0-9_./-]+$`)

func validateThemeBackgroundImage(path string, value string) error {
	if !themeBackgroundImagePattern.MatchString(value) || value == "/assets/" || strings.Contains(value, "..") || strings.Contains(value, "//") {
		return fmt.Errorf("%s must be a safe local path under /assets/; got %q", path, value)
	}

	return nil
}

func (t *themeProperties) validatePage(path string) error {
	if t.Page.configuredFields["background-image"] {
		if err := validateThemeBackgroundImage(path+".page.background-image", t.Page.BackgroundImage); err != nil {
			return err
		}
	}

	properties := []struct {
		configured bool
		path       string
		value      string
		allowed    []string
	}{
		{t.Page.configuredFields["background-position"], path + ".page.background-position", t.Page.BackgroundPosition, []string{"center", "top", "bottom", "left", "right", "top-left", "top-right", "bottom-left", "bottom-right"}},
		{t.Page.configuredFields["background-size"], path + ".page.background-size", t.Page.BackgroundSize, []string{"auto", "cover", "contain"}},
		{t.Page.configuredFields["background-repeat"], path + ".page.background-repeat", t.Page.BackgroundRepeat, []string{"no-repeat", "repeat", "repeat-x", "repeat-y"}},
		{t.Page.configuredFields["background-attachment"], path + ".page.background-attachment", t.Page.BackgroundAttachment, []string{"scroll", "fixed"}},
		{t.Page.configuredFields["ambient-accent"], path + ".page.ambient-accent", t.Page.AmbientAccent, []string{"none", "subtle", "medium", "strong"}},
	}

	for _, property := range properties {
		if !property.configured {
			continue
		}
		if err := validateThemeEnum(property.path, property.value, property.allowed...); err != nil {
			return err
		}
	}

	if t.Page.Overlay.configuredFields["opacity"] {
		if t.Page.Overlay.Opacity == nil || *t.Page.Overlay.Opacity < 0 || *t.Page.Overlay.Opacity > 1 {
			return fmt.Errorf("%s.page.overlay.opacity must be between 0 and 1", path)
		}
	}

	return nil
}

func (t *themeProperties) validateComponentTypography(path string) error {
	fontSizes := []string{"small", "medium", "large"}
	fontWeights := []string{"normal", "medium", "semibold", "bold"}

	properties := []struct {
		configured bool
		path       string
		value      string
		allowed    []string
	}{
		{t.Navigation.configuredFields["font-size"], path + ".navigation.font-size", t.Navigation.FontSize, fontSizes},
		{t.Navigation.configuredFields["font-weight"], path + ".navigation.font-weight", t.Navigation.FontWeight, fontWeights},
		{t.WidgetHeader.configuredFields["font-size"], path + ".widget-header.font-size", t.WidgetHeader.FontSize, fontSizes},
		{t.WidgetHeader.configuredFields["font-weight"], path + ".widget-header.font-weight", t.WidgetHeader.FontWeight, fontWeights},
		{t.Footer.configuredFields["font-size"], path + ".footer.font-size", t.Footer.FontSize, fontSizes},
		{t.Footer.configuredFields["font-weight"], path + ".footer.font-weight", t.Footer.FontWeight, fontWeights},
	}

	for _, property := range properties {
		if !property.configured {
			continue
		}
		if err := validateThemeEnum(property.path, property.value, property.allowed...); err != nil {
			return err
		}
	}

	return nil
}

func (t *themeProperties) validate(path string) error {
	if err := t.validateTypography(path); err != nil {
		return err
	}
	if err := t.validateSurfaceTreatments(path); err != nil {
		return err
	}
	if err := t.validatePage(path); err != nil {
		return err
	}
	if err := t.validateComponentTypography(path); err != nil {
		return err
	}

	return nil
}

func validateConfiguredThemes(config *config) error {
	if err := config.Theme.themeProperties.validate("theme"); err != nil {
		return err
	}

	for key, properties := range config.Theme.Presets.Items() {
		if err := properties.validate("theme.presets." + key); err != nil {
			return err
		}
	}

	for i := range config.Pages {
		if err := config.Pages[i].Theme.validate(fmt.Sprintf("pages[%d].theme", i)); err != nil {
			return err
		}
	}

	return nil
}

func resolveTheme(base *themeProperties, pageOverride *themeProperties) (*themeProperties, error) {
	if base == nil {
		return nil, fmt.Errorf("base theme is nil")
	}

	resolved := *base
	if pageOverride != nil {
		resolved = mergeThemeProperties(resolved, *pageOverride)
	}

	resolved.Key = base.Key
	resolved.CSS = ""
	resolved.PreviewHTML = ""
	resolved.BackgroundColorAsHex = ""

	if err := resolved.init(); err != nil {
		return nil, fmt.Errorf("initializing resolved theme: %v", err)
	}

	return &resolved, nil
}

func themeFontFamilyCSS(value string) string {
	switch value {
	case "default":
		return "inherit"
	case "system":
		return `system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif`
	case "sans-serif":
		return "sans-serif"
	case "serif":
		return "serif"
	case "monospace":
		return "monospace"
	default:
		return ""
	}
}

func themeFontSizeCSS(value string) string {
	switch value {
	case "small":
		return "1.2rem"
	case "medium":
		return "1.3rem"
	case "large":
		return "1.5rem"
	default:
		return ""
	}
}

func themeFontWeightCSS(value string) string {
	switch value {
	case "normal":
		return "400"
	case "medium":
		return "500"
	case "semibold":
		return "600"
	case "bold":
		return "700"
	default:
		return ""
	}
}

func themeRadiusCSS(value string) string {
	switch value {
	case "none":
		return "0"
	case "small":
		return "0.35rem"
	case "medium":
		return "5px"
	case "large":
		return "0.85rem"
	default:
		return ""
	}
}

func themeShadowCSS(value string) string {
	switch value {
	case "none":
		return "none"
	case "subtle":
		return "0 6px 18px rgba(0, 0, 0, 0.18)"
	case "medium":
		return "inset 0 1px 0 rgba(255, 255, 255, 0.04), 0 5px 18px rgba(0, 0, 0, 0.24)"
	case "strong":
		return "inset 0 1px 0 rgba(255, 255, 255, 0.055), inset 0 -1px 0 rgba(0, 0, 0, 0.14), 0 8px 24px rgba(0, 0, 0, 0.30), 0 2px 6px rgba(0, 0, 0, 0.24)"
	default:
		return ""
	}
}

func themeBlurCSS(value string) string {
	switch value {
	case "none":
		return "none"
	case "subtle":
		return "blur(4px)"
	case "medium":
		return "blur(8px) saturate(105%)"
	case "strong":
		return "blur(14px) saturate(110%)"
	default:
		return ""
	}
}

func themeAmbientAccentCSS(value string) string {
	switch value {
	case "none":
		return "0"
	case "subtle":
		return "0.08"
	case "medium":
		return "0.16"
	case "strong":
		return "0.24"
	default:
		return ""
	}
}

func (t *themeProperties) PageAmbientAccentCSS() string {
	return themeAmbientAccentCSS(t.Page.AmbientAccent)
}

func (t *themeProperties) TypographyFontFamilyCSS() string {
	return themeFontFamilyCSS(t.Typography.FontFamily)
}

func (t *themeProperties) TypographyFontSizeCSS() string {
	return themeFontSizeCSS(t.Typography.FontSize)
}

func (t *themeProperties) TypographyFontWeightCSS() string {
	return themeFontWeightCSS(t.Typography.FontWeight)
}

func (t *themeProperties) HeadingFontFamilyCSS() string {
	return themeFontFamilyCSS(t.Typography.Headings.FontFamily)
}

func (t *themeProperties) HeadingFontSizeCSS() string {
	return themeFontSizeCSS(t.Typography.Headings.FontSize)
}

func (t *themeProperties) HeadingFontWeightCSS() string {
	return themeFontWeightCSS(t.Typography.Headings.FontWeight)
}

func (t *themeProperties) NavigationFontSizeCSS() string {
	return themeFontSizeCSS(t.Navigation.FontSize)
}

func (t *themeProperties) NavigationFontWeightCSS() string {
	return themeFontWeightCSS(t.Navigation.FontWeight)
}

func (t *themeProperties) WidgetHeaderFontSizeCSS() string {
	return themeFontSizeCSS(t.WidgetHeader.FontSize)
}

func (t *themeProperties) WidgetHeaderFontWeightCSS() string {
	return themeFontWeightCSS(t.WidgetHeader.FontWeight)
}

func (t *themeProperties) FooterFontSizeCSS() string {
	return themeFontSizeCSS(t.Footer.FontSize)
}

func (t *themeProperties) FooterFontWeightCSS() string {
	return themeFontWeightCSS(t.Footer.FontWeight)
}

func (t *themeProperties) HeaderRadiusCSS() string {
	return themeRadiusCSS(t.Header.Radius)
}

func (t *themeProperties) WidgetRadiusCSS() string {
	return themeRadiusCSS(t.Widgets.Radius)
}

func (t *themeProperties) CardRadiusCSS() string {
	return themeRadiusCSS(t.Cards.Radius)
}

func (t *themeProperties) ControlRadiusCSS() string {
	return themeRadiusCSS(t.Controls.Radius)
}

func (t *themeProperties) HeaderShadowCSS() string {
	return themeShadowCSS(t.Header.Shadow)
}

func (t *themeProperties) WidgetShadowCSS() string {
	return themeShadowCSS(t.Widgets.Shadow)
}

func (t *themeProperties) CardShadowCSS() string {
	return themeShadowCSS(t.Cards.Shadow)
}

func (t *themeProperties) HeaderBlurCSS() string {
	return themeBlurCSS(t.Header.Blur)
}

func (t *themeProperties) WidgetBlurCSS() string {
	return themeBlurCSS(t.Widgets.Blur)
}

func (t *themeProperties) init() error {
	css, err := executeTemplateToString(themeStyleTemplate, t)
	if err != nil {
		return fmt.Errorf("compiling theme style: %v", err)
	}
	t.CSS = template.CSS(whitespaceAtBeginningOfLinePattern.ReplaceAllString(css, ""))

	previewHTML, err := executeTemplateToString(themePresetPreviewTemplate, t)
	if err != nil {
		return fmt.Errorf("compiling theme preview: %v", err)
	}
	t.PreviewHTML = template.HTML(previewHTML)

	if t.BackgroundColor != nil {
		t.BackgroundColorAsHex = t.BackgroundColor.ToHex()
	} else {
		t.BackgroundColorAsHex = "#151519"
	}

	return nil
}

func sameThemeHeadings(a, b themeHeadingProperties) bool {
	return a.FontFamily == b.FontFamily &&
		a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight &&
		a.TextColor.SameAs(b.TextColor)
}

func sameThemeTypography(a, b themeTypographyProperties) bool {
	return a.FontFamily == b.FontFamily &&
		a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight &&
		a.TextColor.SameAs(b.TextColor) &&
		a.SecondaryTextColor.SameAs(b.SecondaryTextColor) &&
		a.MutedTextColor.SameAs(b.MutedTextColor) &&
		sameThemeHeadings(a.Headings, b.Headings)
}

func sameThemePage(a, b themePageProperties) bool {
	return a.BackgroundImage == b.BackgroundImage &&
		a.BackgroundPosition == b.BackgroundPosition &&
		a.BackgroundSize == b.BackgroundSize &&
		a.BackgroundRepeat == b.BackgroundRepeat &&
		a.BackgroundAttachment == b.BackgroundAttachment &&
		a.AmbientAccent == b.AmbientAccent &&
		a.Overlay.Color.SameAs(b.Overlay.Color) &&
		sameOptionalFloat32(a.Overlay.Opacity, b.Overlay.Opacity)
}

func sameOptionalFloat32(a, b *float32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func sameThemeSurface(a, b themeSurfaceProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.BorderColor.SameAs(b.BorderColor) &&
		a.Radius == b.Radius &&
		a.Shadow == b.Shadow &&
		a.Blur == b.Blur
}

func sameThemeHeader(a, b themeHeaderProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.TextColor.SameAs(b.TextColor) &&
		a.BorderColor.SameAs(b.BorderColor) &&
		a.Radius == b.Radius &&
		a.Shadow == b.Shadow &&
		a.Blur == b.Blur
}

func sameThemeCard(a, b themeCardProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.BorderColor.SameAs(b.BorderColor) &&
		a.Radius == b.Radius &&
		a.Shadow == b.Shadow
}

func sameThemeNavigation(a, b themeNavigationProperties) bool {
	return a.TextColor.SameAs(b.TextColor) &&
		a.HoverColor.SameAs(b.HoverColor) &&
		a.ActiveColor.SameAs(b.ActiveColor) &&
		a.AccentColor.SameAs(b.AccentColor) &&
		a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight
}

func sameThemeWidgetHeader(a, b themeWidgetHeaderProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.TextColor.SameAs(b.TextColor) &&
		a.AccentColor.SameAs(b.AccentColor) &&
		a.BorderColor.SameAs(b.BorderColor) &&
		a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight
}

func sameThemeGroup(a, b themeGroupProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.TextColor.SameAs(b.TextColor) &&
		a.HoverColor.SameAs(b.HoverColor) &&
		a.ActiveColor.SameAs(b.ActiveColor) &&
		a.AccentColor.SameAs(b.AccentColor) &&
		a.BorderColor.SameAs(b.BorderColor)
}

func sameThemeControl(a, b themeControlProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.TextColor.SameAs(b.TextColor) &&
		a.MutedColor.SameAs(b.MutedColor) &&
		a.BorderColor.SameAs(b.BorderColor) &&
		a.FocusColor.SameAs(b.FocusColor) &&
		a.Radius == b.Radius &&
		a.Button.BackgroundColor.SameAs(b.Button.BackgroundColor) &&
		a.Button.TextColor.SameAs(b.Button.TextColor)
}

func sameThemeFooter(a, b themeFooterProperties) bool {
	return a.BackgroundColor.SameAs(b.BackgroundColor) &&
		a.TextColor.SameAs(b.TextColor) &&
		a.AccentColor.SameAs(b.AccentColor) &&
		a.BorderColor.SameAs(b.BorderColor) &&
		a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight
}

func sameThemeElevatedSurface(a, b themeElevatedSurfaceProperties) bool {
	return a.ElevatedBackgroundColor.SameAs(b.ElevatedBackgroundColor) &&
		a.ElevatedBorderColor.SameAs(b.ElevatedBorderColor) &&
		a.SeparatorColor.SameAs(b.SeparatorColor)
}

func (t1 *themeProperties) SameAs(t2 *themeProperties) bool {
	if t1 == nil && t2 == nil {
		return true
	}
	if t1 == nil || t2 == nil {
		return false
	}

	return t1.Light == t2.Light &&
		t1.ContrastMultiplier == t2.ContrastMultiplier &&
		t1.TextSaturationMultiplier == t2.TextSaturationMultiplier &&
		t1.BackgroundColor.SameAs(t2.BackgroundColor) &&
		t1.PrimaryColor.SameAs(t2.PrimaryColor) &&
		t1.PositiveColor.SameAs(t2.PositiveColor) &&
		t1.NegativeColor.SameAs(t2.NegativeColor) &&
		t1.AccentColor.SameAs(t2.AccentColor) &&
		t1.WarningColor.SameAs(t2.WarningColor) &&
		sameThemeTypography(t1.Typography, t2.Typography) &&
		sameThemePage(t1.Page, t2.Page) &&
		sameThemeHeader(t1.Header, t2.Header) &&
		sameThemeNavigation(t1.Navigation, t2.Navigation) &&
		sameThemeSurface(t1.Widgets, t2.Widgets) &&
		sameThemeWidgetHeader(t1.WidgetHeader, t2.WidgetHeader) &&
		sameThemeCard(t1.Cards, t2.Cards) &&
		sameThemeGroup(t1.Groups, t2.Groups) &&
		sameThemeControl(t1.Controls, t2.Controls) &&
		sameThemeFooter(t1.Footer, t2.Footer) &&
		sameThemeElevatedSurface(t1.Surfaces, t2.Surfaces)
}
