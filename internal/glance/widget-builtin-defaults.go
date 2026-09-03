package glance

import "time"

// builtinWidgetDefaults catalogs user-observable built-in defaults that are
// candidates for centralized resolution. Existing initialize methods remain
// the runtime authority until each default is migrated here with regression
// coverage.
var builtinWidgetDefaults = map[string]widgetDefaultValues{
	"change-detection": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(time.Hour),
	},
	"custom-api": {
		Cache: durationDefault(time.Hour),
	},
	"dns-stats": {
		Cache: durationDefault(10 * time.Minute),
	},
	"docker-containers": {
		Cache: durationDefault(time.Minute),
	},
	"extension": {
		Cache: durationDefault(30 * time.Minute),
	},
	"hacker-news": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(30 * time.Minute),
	},
	"ics-events": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(30 * time.Minute),
	},
	"lobsters": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(time.Hour),
	},
	"markets": {
		Cache: durationDefault(time.Hour),
	},
	"monitor": {
		Cache:   durationDefault(5 * time.Minute),
		Timeout: durationDefault(3 * time.Second),
	},
	"reddit": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(30 * time.Minute),
	},
	"releases": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(2 * time.Hour),
	},
	"repository": {
		Cache: durationDefault(time.Hour),
	},
	"rss": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(2 * time.Hour),
	},
	"server-stats": {
		Cache:   durationDefault(15 * time.Second),
		Timeout: durationDefault(3 * time.Second),
	},
	"twitch-channels": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(10 * time.Minute),
	},
	"twitch-top-games": {
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(10 * time.Minute),
	},
	"videos": {
		CollapseAfter:     intDefault(7),
		CollapseAfterRows: intDefault(4),
		Cache:             durationDefault(time.Hour),
	},
}

func intDefault(value int) *int {
	return &value
}

func durationDefault(value time.Duration) *durationField {
	field := durationField(value)
	return &field
}
