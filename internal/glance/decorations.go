package glance

import (
	"bytes"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Decorations belong to dashboard presentation, not the widget scheduler.
// Go validates annual calendar ranges and icon inputs; the browser owns date
// selection so a dashboard left open changes at the viewer's local midnight.
// The catalog contains references only: no external artwork is bundled.
//
//go:embed decoration-themes.yml
var decorationThemesYAML []byte

type decorationDesign struct {
	Theme           string   `yaml:"theme"`
	LightColors     []string `yaml:"light-colors"`
	FooterLeftIcon  string   `yaml:"footer-left-icon"`
	FooterRightIcon string   `yaml:"footer-right-icon"`
}

type decorationScheduleEntry struct {
	decorationDesign `yaml:",inline"`
	Start            string `yaml:"start"`
	End              string `yaml:"end"`
}

type decorationsConfig struct {
	decorationDesign `yaml:",inline"`
	Enabled          bool                      `yaml:"enabled"`
	Lights           bool                      `yaml:"lights"`
	FooterIcons      bool                      `yaml:"footer-icons"`
	Animation        bool                      `yaml:"animation"`
	Schedule         []decorationScheduleEntry `yaml:"schedule"`
	node             *yaml.Node
	catalog          map[string]decorationDesign
}

func (c *decorationsConfig) UnmarshalYAML(node *yaml.Node) error {
	type plain decorationsConfig
	value := plain{Lights: true, FooterIcons: true}
	if err := node.Decode(&value); err != nil {
		return err
	}
	*c = decorationsConfig(value)
	c.node = node
	return nil
}

var decorationColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var decorationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var decorationLibraryNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+(?:\.(?:svg|png|webp))?$`)
var decorationTwemojiPattern = regexp.MustCompile(`^twemoji:[0-9a-f]+(?:-[0-9a-f]+)*$`)

const twemojiVersion = "17.0.3"

// YAML aliases are valid configuration. Resolve them before inspecting source
// nodes, just as yaml.v3 does when decoding the corresponding Go values.
func decorationYAMLValue(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// Annual ranges are represented on a leap-year calendar so February 29
// participates in overlap validation without shifting March in ordinary years.
func decorationDay(value string) (int, error) {
	date, err := time.Parse("2006-01-02", "2000-"+value)
	if err != nil || date.Format("01-02") != value {
		return 0, fmt.Errorf("must be a valid MM-DD date")
	}
	return date.YearDay(), nil
}

func decorationRangeContains(day, start, end int) bool {
	if start <= end {
		return day >= start && day <= end
	}
	return day >= start || day <= end
}

func validateDecorationIcon(value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "/assets/") {
		return validateThemeBackgroundImage("icon", value)
	}
	if decorationTwemojiPattern.MatchString(value) {
		return nil
	}
	// Reuse existing libraries, but reject arbitrary URLs and malformed names.
	prefix, name, ok := strings.Cut(value, ":")
	if ok && (prefix == "mdi" || prefix == "si" || prefix == "di" || prefix == "sh" || prefix == "hla") {
		if decorationLibraryNamePattern.MatchString(name) {
			return nil
		}
	}
	return fmt.Errorf("must be a supported icon-library reference or safe /assets/ path")
}

func validateDecorationDesign(d decorationDesign, lights, icons bool) error {
	if lights && len(d.LightColors) == 0 {
		return fmt.Errorf("light-colors is required when lights are enabled")
	}
	if len(d.LightColors) > 8 {
		return fmt.Errorf("light-colors accepts at most 8 colors")
	}
	for _, color := range d.LightColors {
		if !decorationColorPattern.MatchString(color) {
			return fmt.Errorf("light-colors must contain #RRGGBB values")
		}
	}
	if icons && d.FooterLeftIcon == "" && d.FooterRightIcon == "" {
		return fmt.Errorf("at least one footer icon is required when footer-icons is enabled")
	}
	for _, icon := range []string{d.FooterLeftIcon, d.FooterRightIcon} {
		if err := validateDecorationIcon(icon); err != nil {
			return err
		}
	}
	return nil
}

func loadDecorationThemes(contents []byte) (map[string]decorationDesign, error) {
	catalog := make(map[string]decorationDesign)
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&catalog); err != nil {
		return nil, err
	}
	if len(catalog) == 0 {
		return nil, fmt.Errorf("decoration catalog is empty")
	}
	for name, design := range catalog {
		if !decorationNamePattern.MatchString(name) || name == "auto" || name == "custom" || design.Theme != "" {
			return nil, fmt.Errorf("invalid decoration preset %q", name)
		}
		if err := validateDecorationDesign(design, true, true); err != nil {
			return nil, fmt.Errorf("preset %s: %w", name, err)
		}
	}
	return catalog, nil
}

// Configuration errors use the existing include-aware source mapping. Presence
// checks intentionally distinguish an omitted field from an explicit empty one.
func (c *decorationsConfig) validate(parsed *parsedYAMLConfig) error {
	if c.node == nil {
		return nil
	}
	catalog, err := loadDecorationThemes(decorationThemesYAML)
	if err != nil {
		return fmt.Errorf("built-in decoration catalog: %w", err)
	}
	c.catalog = catalog
	fail := func(node *yaml.Node, message string) error {
		return semanticConfigDiagnostic(parsed, node.Line, fmt.Errorf("decorations: %s", message))
	}
	check := func(node *yaml.Node, allowed map[string]bool) error {
		node = decorationYAMLValue(node)
		for i := 0; i+1 < len(node.Content); i += 2 {
			if !allowed[node.Content[i].Value] {
				return fail(node.Content[i], "unknown field "+node.Content[i].Value)
			}
		}
		return nil
	}
	if err := check(c.node, map[string]bool{"enabled": true, "theme": true, "lights": true, "footer-icons": true, "animation": true, "light-colors": true, "footer-left-icon": true, "footer-right-icon": true, "schedule": true}); err != nil {
		return err
	}
	validateDesign := func(d decorationDesign, node *yaml.Node) error {
		node = decorationYAMLValue(node)
		if d.Theme != "custom" {
			for _, field := range []string{"light-colors", "footer-left-icon", "footer-right-icon"} {
				if key, _ := yamlMappingValue(node, field); key != nil {
					return fail(key, field+" is allowed only with theme: custom")
				}
			}
			if _, exists := catalog[d.Theme]; !exists {
				return fail(node, "unknown theme "+d.Theme)
			}
			return nil
		}
		if err := validateDecorationDesign(d, c.Lights, c.FooterIcons); err != nil {
			return fail(node, err.Error())
		}
		return nil
	}
	_, scheduleNode := yamlMappingValue(c.node, "schedule")
	scheduleNode = decorationYAMLValue(scheduleNode)
	if c.Theme != "auto" {
		if scheduleNode != nil {
			return fail(scheduleNode, "schedule requires theme: auto")
		}
		// An explicitly disabled empty section is valid; supplied designs still
		// undergo validation so enabling them cannot reveal hidden bad fields.
		if !c.Enabled && c.Theme == "" {
			hasDesign := false
			for _, field := range []string{"light-colors", "footer-left-icon", "footer-right-icon"} {
				if key, _ := yamlMappingValue(c.node, field); key != nil {
					hasDesign = true
				}
			}
			if !hasDesign {
				return nil
			}
		}
		return validateDesign(c.decorationDesign, c.node)
	}
	for _, field := range []string{"light-colors", "footer-left-icon", "footer-right-icon"} {
		if key, _ := yamlMappingValue(c.node, field); key != nil {
			return fail(key, field+" belongs inside a custom schedule entry")
		}
	}
	if len(c.Schedule) == 0 {
		return fail(c.node, "theme: auto requires a nonempty schedule")
	}
	occupied := make([]bool, 367)
	for i, entry := range c.Schedule {
		node := scheduleNode.Content[i]
		if err := check(node, map[string]bool{"theme": true, "start": true, "end": true, "light-colors": true, "footer-left-icon": true, "footer-right-icon": true}); err != nil {
			return err
		}
		if err := validateDesign(entry.decorationDesign, node); err != nil {
			return err
		}
		start, err := decorationDay(entry.Start)
		if err != nil {
			return fail(node, fmt.Sprintf("schedule[%d].start %s", i, err))
		}
		end, err := decorationDay(entry.End)
		if err != nil {
			return fail(node, fmt.Sprintf("schedule[%d].end %s", i, err))
		}
		for day := 1; day <= 366; day++ {
			if !decorationRangeContains(day, start, end) {
				continue
			}
			if occupied[day] {
				return fail(node, fmt.Sprintf("schedule[%d] overlaps another entry", i))
			}
			occupied[day] = true
		}
	}
	return nil
}

type decorationBrowserDesign struct {
	Colors      []string `json:"colors"`
	Left        string   `json:"left"`
	Right       string   `json:"right"`
	LeftInvert  bool     `json:"leftInvert"`
	RightInvert bool     `json:"rightInvert"`
}
type decorationBrowserEntry struct {
	Start  string                  `json:"start"`
	End    string                  `json:"end"`
	Design decorationBrowserDesign `json:"design"`
}
type decorationBrowserConfig struct {
	Lights      bool                     `json:"lights"`
	FooterIcons bool                     `json:"footerIcons"`
	Animation   bool                     `json:"animation"`
	Auto        bool                     `json:"auto"`
	Design      decorationBrowserDesign  `json:"design"`
	Schedule    []decorationBrowserEntry `json:"schedule"`
}

// DecorationConfig exposes only validated, resolved presentation data. It uses
// the shared icon resolver and proxy policy rather than sending source strings
// to JavaScript or treating user input as trusted HTML/CSS.
func (a *application) DecorationConfig() *decorationBrowserConfig {
	c := a.Config.Decorations
	if !c.Enabled {
		return nil
	}
	resolve := func(d decorationDesign) decorationBrowserDesign {
		if d.Theme != "custom" {
			d = c.catalog[d.Theme]
		}
		left, right := newCustomIconField(d.FooterLeftIcon), newCustomIconField(d.FooterRightIcon)
		left.resolveResourceProxy(a.resolveResourceProxyURL)
		right.resolveResourceProxy(a.resolveResourceProxyURL)
		return decorationBrowserDesign{Colors: d.LightColors, Left: a.resolveUserDefinedAssetPath(string(left.RenderURL())), Right: a.resolveUserDefinedAssetPath(string(right.RenderURL())), LeftInvert: left.AutoInvert, RightInvert: right.AutoInvert}
	}
	result := &decorationBrowserConfig{Lights: c.Lights, FooterIcons: c.FooterIcons, Animation: c.Animation, Auto: c.Theme == "auto"}
	if result.Auto {
		for _, entry := range c.Schedule {
			result.Schedule = append(result.Schedule, decorationBrowserEntry{Start: entry.Start, End: entry.End, Design: resolve(entry.decorationDesign)})
		}
	} else {
		result.Design = resolve(c.decorationDesign)
	}
	return result
}
