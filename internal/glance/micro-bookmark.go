package glance

import "errors"

type microBookmark struct {
	Position int             `yaml:"position"`
	Title    string          `yaml:"title"`
	URL      string          `yaml:"url"`
	Icon     customIconField `yaml:"icon"`
	SameTab  bool            `yaml:"same-tab"`
}

func (m *microBookmark) GetPosition() int {
	return m.Position
}

func (m *microBookmark) UnmarshalYAML(unmarshal func(any) error) error {
	type microBookmarkPlain microBookmark

	if err := unmarshal((*microBookmarkPlain)(m)); err != nil {
		return err
	}

	if m.Title == "" {
		return errors.New("bookmark micro-widget title is required")
	}
	if m.URL == "" {
		return errors.New("bookmark micro-widget url is required")
	}

	return nil
}

func (m *microBookmark) GetType() string {
	return "bookmark"
}
