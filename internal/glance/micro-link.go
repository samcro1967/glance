package glance

import "errors"

type microLink struct {
	Position int    `yaml:"position"`
	Title    string `yaml:"title"`
	URL      string `yaml:"url"`
	SameTab  bool   `yaml:"same-tab"`
}

func (m *microLink) GetPosition() int {
	return m.Position
}

func (m *microLink) GetType() string {
	return "link"
}

func (m *microLink) UnmarshalYAML(unmarshal func(any) error) error {
	type plain microLink

	if err := unmarshal((*plain)(m)); err != nil {
		return err
	}

	if m.Title == "" {
		return errors.New("link micro-widget title is required")
	}

	if m.URL == "" {
		return errors.New("link micro-widget url is required")
	}

	return nil
}
