package glance

type widgetDescriptor struct {
	constructor  func() widget
	capabilities []widgetCapabilityDefinition
}

// widgetRegistry is the authoritative registry of public widget types.
// Each public type explicitly defines how it is constructed and which
// reusable capabilities beyond widgetBase it supports.
var widgetRegistry = map[string]widgetDescriptor{
	"calendar": {
		constructor: func() widget { return &calendarWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"calendar-legacy": {
		constructor: func() widget { return &oldCalendarWidget{} },
	},
	"ics-events": {
		constructor: func() widget { return &icsEventsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"clock": {
		constructor: func() widget { return &clockWidget{} },
	},
	"analog-clock": {
		constructor: func() widget { return &analogClockWidget{} },
	},
	"weather": {
		constructor: func() widget { return &weatherWidget{} },
	},
	"bookmarks": {
		constructor: func() widget { return &bookmarksWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"iframe": {
		constructor: func() widget { return &iframeWidget{} },
	},
	"markdown": {
		constructor: func() widget { return &markdownWidget{} },
	},
	"html": {
		constructor: func() widget { return &htmlWidget{} },
	},
	"hacker-news": {
		constructor: func() widget { return &hackerNewsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"releases": {
		constructor: func() widget { return &releasesWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"videos": {
		constructor: func() widget { return &videosWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfterRows, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"markets": {
		constructor: func() widget { return &marketsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"stocks": {
		constructor: func() widget { return &marketsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"reddit": {
		constructor: func() widget { return &redditWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityProxy, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"rss": {
		constructor: func() widget { return &rssWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"monitor": {
		constructor: func() widget { return &monitorWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"twitch-top-games": {
		constructor: func() widget { return &twitchGamesWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"twitch-channels": {
		constructor: func() widget { return &twitchChannelsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"lobsters": {
		constructor: func() widget { return &lobstersWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"change-detection": {
		constructor: func() widget { return &changeDetectionWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityLimit, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityCollapseAfter, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"repository": {
		constructor: func() widget { return &repositoryWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"search": {
		constructor: func() widget { return &searchWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"extension": {
		constructor: func() widget { return &extensionWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"group": {
		constructor: func() widget { return &groupWidget{} },
	},
	"dns-stats": {
		constructor: func() widget { return &dnsStatsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
		},
	},
	"split-column": {
		constructor: func() widget { return &splitColumnWidget{} },
	},
	"custom-api": {
		constructor: func() widget { return &customAPIWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
			{widgetCapabilityHeaders, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
			{widgetCapabilityBasicAuth, widgetCapabilityScopeType | widgetCapabilityScopeInstance | widgetCapabilityScopeChild},
		},
	},
	"docker-containers": {
		constructor: func() widget { return &dockerContainersWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"server-stats": {
		constructor: func() widget { return &serverStatsWidget{} },
		capabilities: []widgetCapabilityDefinition{
			{widgetCapabilityTimeout, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
			{widgetCapabilityAllowInsecure, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeChild},
		},
	},
	"timer": {
		constructor: func() widget { return &timerWidget{} },
	},
	"to-do": {
		constructor: func() widget { return &todoWidget{} },
	},
	"unit-converter": {
		constructor: func() widget { return &unitConverterWidget{} },
	},
	"calculator": {
		constructor: func() widget { return &calculatorWidget{} },
	},
	"stack": {
		constructor: func() widget { return &stackWidget{} },
	},
	"status-bar": {
		constructor: func() widget { return &statusBarWidget{} },
	},
}

var commonWidgetCapabilities = []widgetCapabilityDefinition{
	{widgetCapabilityTitle, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityTitleURL, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityHideHeader, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityCSSClass, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityCache, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
	{widgetCapabilityNewTab, widgetCapabilityScopeGlobal | widgetCapabilityScopeType | widgetCapabilityScopeInstance},
}

func widgetSupportsCapability(widgetType string, capability widgetCapability, scope widgetCapabilityScope) bool {
	descriptor, ok := widgetRegistry[widgetType]
	if !ok {
		return false
	}

	for _, definition := range commonWidgetCapabilities {
		if definition.Capability == capability && definition.Scopes&scope != 0 {
			return true
		}
	}

	for _, definition := range descriptor.capabilities {
		if definition.Capability == capability && definition.Scopes&scope != 0 {
			return true
		}
	}

	return false
}
