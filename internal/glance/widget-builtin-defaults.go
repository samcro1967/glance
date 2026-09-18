package glance

import "time"

// builtinWidgetDefaults is the lowest-precedence source for centralized
// user-observable widget defaults. User-configured global defaults, type
// defaults, and explicit widget configuration override these values.
var builtinWidgetDefaults = map[string]widgetDefaultValues{
	"change-detection": {
		Limit:         intDefault(10),
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
		Limit:         intDefault(15),
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(30 * time.Minute),
	},
	"ics-events": {
		Limit:         intDefault(25),
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(30 * time.Minute),
	},
	"lobsters": {
		Limit:         intDefault(15),
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
	"prometheus": {
		Cache: durationDefault(5 * time.Minute),
	},
	"reddit": {
		Limit:         intDefault(15),
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(30 * time.Minute),
	},
	"releases": {
		Limit:         intDefault(10),
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(2 * time.Hour),
	},
	"repository": {
		Cache: durationDefault(time.Hour),
	},
	"rss": {
		Limit:         intDefault(25),
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
		Limit:         intDefault(10),
		CollapseAfter: intDefault(5),
		Cache:         durationDefault(10 * time.Minute),
	},
	"videos": {
		Limit:             intDefault(25),
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
