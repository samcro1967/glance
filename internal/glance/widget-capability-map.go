package glance

var registeredWidgetTypes = map[string]struct{}{
	"calendar": {}, "calendar-legacy": {}, "ics-events": {},
	"clock": {}, "analog-clock": {}, "weather": {},
	"bookmarks": {}, "iframe": {}, "markdown": {}, "html": {},
	"hacker-news": {}, "releases": {}, "videos": {},
	"markets": {}, "stocks": {}, "reddit": {}, "rss": {},
	"monitor": {}, "twitch-top-games": {}, "twitch-channels": {},
	"lobsters": {}, "change-detection": {}, "repository": {},
	"search": {}, "extension": {}, "group": {}, "dns-stats": {},
	"split-column": {}, "custom-api": {}, "docker-containers": {},
	"server-stats": {}, "timer": {}, "to-do": {}, "unit-converter": {}, "stack": {},
	"status-bar": {},
}

var commonWidgetCapabilities = []widgetCapabilityDefinition{
	{widgetCapabilityTitle, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityTitleURL, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityHideHeader, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityCSSClass, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityCache, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
}

// widgetTypeCapabilities contains capabilities beyond widgetBase. Entries are
// explicit so a new widget must deliberately opt in to reusable behavior.
var widgetTypeCapabilities = map[string][]widgetCapabilityDefinition{
	"change-detection": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"custom-api": {
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
		{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
		{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
	},
	"dns-stats": {
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"extension": {
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"hacker-news": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"ics-events": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
	},
	"lobsters": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"bookmarks": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
	},
	"docker-containers": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
	},
	"monitor": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
	},
	"reddit": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityProxy, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"releases": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"rss": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
	},
	"markets": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"stocks": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"repository": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"search": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"server-stats": {
		{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
	},
	"twitch-channels": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"twitch-top-games": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
	"videos": {
		{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		{widgetCapabilityCollapseAfterRows, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	},
}

func widgetSupportsCapability(widgetType string, capability widgetCapability, scope widgetCapabilityScope) bool {
	if _, ok := registeredWidgetTypes[widgetType]; !ok {
		return false
	}

	for _, definition := range commonWidgetCapabilities {
		if definition.Capability == capability && definition.Scopes&scope != 0 {
			return true
		}
	}

	for _, definition := range widgetTypeCapabilities[widgetType] {
		if definition.Capability == capability && definition.Scopes&scope != 0 {
			return true
		}
	}

	return false
}
