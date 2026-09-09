package glance

import (
	"errors"
	"fmt"
	"time"
)

type microClock struct {
	Position   int    `yaml:"position"`
	HourFormat string `yaml:"hour-format"`
	Timezone   string `yaml:"timezone"`
	Label      string `yaml:"label"`
}

func (m *microClock) GetPosition() int {
	return m.Position
}

func (m *microClock) UnmarshalYAML(unmarshal func(any) error) error {
	type microClockPlain microClock

	if err := unmarshal((*microClockPlain)(m)); err != nil {
		return err
	}

	if m.HourFormat == "" {
		m.HourFormat = "24h"
	}

	if m.HourFormat != "12h" && m.HourFormat != "24h" {
		return errors.New("clock micro-widget hour-format must be either 12h or 24h")
	}

	if m.Timezone != "" {
		if _, err := time.LoadLocation(m.Timezone); err != nil {
			return fmt.Errorf("invalid timezone %q: %v", m.Timezone, err)
		}
	}

	return nil
}

func (m *microClock) GetType() string {
	return "clock"
}
