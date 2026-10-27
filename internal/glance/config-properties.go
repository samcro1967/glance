package glance

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Property diagnostics are separate from decode/initialization errors: ignoring
// an unknown setting must not disable its widget or activate startup recovery.
type configPropertyDiagnostic struct {
	configDiagnostic
	Path        string
	Property    string
	WidgetType  string
	Replacement string
}

func (d configPropertyDiagnostic) deprecated() bool { return d.Replacement != "" }

func configValidationIssues(c *config) []error {
	issues := append([]error(nil), c.recoverableErrors...)
	for _, diagnostic := range c.propertyDiagnostics {
		if !diagnostic.deprecated() {
			issue := diagnostic.configDiagnostic
			issues = append(issues, &issue)
		}
	}
	return issues
}

func logConfigPropertyDiagnostics(diagnostics []configPropertyDiagnostic) {
	for _, d := range diagnostics {
		status, action := "unknown", "ignored"
		if d.deprecated() {
			status, action = "deprecated", "supported"
		}
		attrs := []any{"file", d.File, "line", d.Line, "path", d.Path,
			"property", d.Property, "status", status, "action", action}
		if d.WidgetType != "" {
			attrs = append(attrs, "widget_type", d.WidgetType)
		}
		if d.Replacement != "" {
			attrs = append(attrs, "replacement", d.Replacement)
		}
		// Never include property values, source snippets, or expanded secrets.
		slog.Warn("Configuration property diagnostic", attrs...)
	}
}

// orderedYAMLMap stores its entries privately. Expose only the value type for
// schema inspection; user-defined entry names are not configuration properties.
type configPropertyMap interface {
	configPropertyValueType() reflect.Type
}

func (*orderedYAMLMap[K, V]) configPropertyValueType() reflect.Type {
	return reflect.TypeFor[V]()
}

type configPropertyField struct {
	typeOf      reflect.Type
	replacement string
}

type configPropertyScanner struct {
	parsed      *parsedYAMLConfig
	diagnostics []configPropertyDiagnostic
	fields      map[reflect.Type]map[string]configPropertyField
	active      map[*yaml.Node]bool
	seen        map[string]bool
}

func inspectConfigProperties(contents []byte, parsed *parsedYAMLConfig) ([]configPropertyDiagnostic, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return nil, configDiagnosticFromYAMLError(parsed, err)
	}
	scanner := configPropertyScanner{
		parsed: parsed, fields: make(map[reflect.Type]map[string]configPropertyField),
		active: make(map[*yaml.Node]bool), seen: make(map[string]bool),
	}
	if len(document.Content) > 0 {
		scanner.walk(document.Content[0], reflect.TypeFor[config](), "", "")
	}
	return scanner.diagnostics, nil
}

func (s *configPropertyScanner) structFields(t reflect.Type) map[string]configPropertyField {
	if fields, ok := s.fields[t]; ok {
		return fields
	}
	fields := make(map[string]configPropertyField)
	s.fields[t] = fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")
		if tag[0] == "-" {
			continue
		}
		inline := false
		for _, option := range tag[1:] {
			inline = inline || option == "inline"
		}
		if inline {
			child := field.Type
			for child.Kind() == reflect.Pointer {
				child = child.Elem()
			}
			if child.Kind() == reflect.Struct {
				for name, inherited := range s.structFields(child) {
					fields[name] = inherited
				}
			}
			continue
		}
		name := tag[0]
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		property := configPropertyField{typeOf: field.Type}
		// This compatibility alias is explicitly retained by markets.initialize.
		if t == reflect.TypeFor[marketsWidget]() && name == "stocks" {
			property.replacement = "markets"
		}
		fields[name] = property
	}
	return fields
}

type configYAMLProperty struct{ key, value *yaml.Node }

// Match YAML merge precedence: explicit properties override merged mappings;
// earlier mappings in a merge sequence win. Keep the original key node so a
// warning points at the defining file/line rather than the alias use site.
func configYAMLProperties(node *yaml.Node, active map[*yaml.Node]bool) []configYAMLProperty {
	if node == nil || active[node] {
		return nil
	}
	active[node] = true
	defer delete(active, node)
	if node.Kind == yaml.AliasNode {
		return configYAMLProperties(node.Alias, active)
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var properties []configYAMLProperty
	seen := make(map[string]bool)
	var merges []*yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag == "!!merge" {
			merges = append(merges, value)
			continue
		}
		properties = append(properties, configYAMLProperty{key, value})
		seen[key.Value] = true
	}
	for _, merge := range merges {
		nodes := []*yaml.Node{merge}
		if merge.Kind == yaml.SequenceNode {
			nodes = merge.Content
		} else if merge.Kind == yaml.AliasNode && merge.Alias != nil && merge.Alias.Kind == yaml.SequenceNode {
			nodes = merge.Alias.Content
		}
		for _, merged := range nodes {
			for _, property := range configYAMLProperties(merged, active) {
				if !seen[property.key.Value] {
					properties = append(properties, property)
					seen[property.key.Value] = true
				}
			}
		}
	}
	return properties
}

func (s *configPropertyScanner) report(key *yaml.Node, path, widgetType, replacement string) {
	location := configSourceLocation{Line: key.Line}
	if original, ok := s.parsed.sourceLocation(key.Line); ok {
		location = original
	}
	// An anchor can be reused many times. Warn once per source key and widget
	// type in this load, while still checking each use against its own schema.
	identity := fmt.Sprintf("%s:%d:%d:%s:%s:%s", location.File, location.Line, key.Column, key.Value, widgetType, replacement)
	if s.seen[identity] {
		return
	}
	s.seen[identity] = true
	message := fmt.Sprintf("unknown configuration property %q at %s; ignored by this version", key.Value, path)
	if replacement != "" {
		message = fmt.Sprintf("deprecated configuration property %q at %s; use %q instead", key.Value, path, replacement)
	}
	s.diagnostics = append(s.diagnostics, configPropertyDiagnostic{
		configDiagnostic: configDiagnostic{File: location.File, Line: location.Line, Message: message},
		Path:             path, Property: key.Value, WidgetType: widgetType, Replacement: replacement,
	})
}

func (s *configPropertyScanner) walk(node *yaml.Node, t reflect.Type, path, widgetType string) {
	if node == nil || s.active[node] {
		return
	}
	s.active[node] = true
	defer delete(s.active, node)
	if node.Kind == yaml.AliasNode {
		s.walk(node.Alias, t, path, widgetType)
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[widgets]() || t == reflect.TypeFor[microWidgets]() {
		if node.Kind == yaml.SequenceNode {
			for index, item := range node.Content {
				var meta struct {
					Type string `yaml:"type"`
				}
				if item.Decode(&meta) != nil {
					continue
				}
				var candidate widget
				if t == reflect.TypeFor[widgets]() {
					if descriptor, ok := widgetRegistry[meta.Type]; ok {
						candidate = descriptor.constructor()
					}
				} else if descriptor, ok := microWidgetRegistry[meta.Type]; ok {
					candidate = descriptor.constructor()
				}
				if candidate != nil {
					s.walk(item, reflect.TypeOf(candidate), fmt.Sprintf("%s[%d]", path, index), meta.Type)
				}
				// Unknown types already have a recoverable widget diagnostic.
			}
		}
		return
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		if node.Kind == yaml.SequenceNode {
			for index, item := range node.Content {
				s.walk(item, t.Elem(), fmt.Sprintf("%s[%d]", path, index), widgetType)
			}
		}
		return
	}
	if node.Kind != yaml.MappingNode {
		return // Scalar shorthand is owned and validated by custom decoders.
	}
	properties := configYAMLProperties(node, make(map[*yaml.Node]bool))
	if t.Kind() == reflect.Map {
		for _, property := range properties {
			s.walk(property.value, t.Elem(), configPropertyPath(path, property.key.Value), widgetType)
		}
		return
	}
	if t.Kind() != reflect.Struct {
		return // Arbitrary interface values, including Custom API options.
	}
	if ordered, ok := reflect.New(t).Interface().(configPropertyMap); ok {
		for _, property := range properties {
			s.walk(property.value, ordered.configPropertyValueType(), configPropertyPath(path, property.key.Value), widgetType)
		}
		return
	}
	fields := s.structFields(t)
	for _, property := range properties {
		name := property.key.Value
		childPath := configPropertyPath(path, name)
		field, known := fields[name]
		if !known {
			s.report(property.key, childPath, widgetType, "")
			continue // One warning for an ignored subtree; do not inspect its values.
		}
		if field.replacement != "" {
			s.report(property.key, childPath, widgetType, field.replacement)
		}
		s.walk(property.value, field.typeOf, childPath, widgetType)
	}
}

func configPropertyPath(parent, property string) string {
	if parent == "" {
		return property
	}
	return parent + "." + property
}
