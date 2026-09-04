package glance

// widgetCapability identifies a reusable user-facing widget configuration
// capability. A capability is shared only when its name and semantics are
// intended to be consistent across every widget that declares support for it.
type widgetCapability string

const (
	widgetCapabilityTitle             widgetCapability = "title"
	widgetCapabilityTitleURL          widgetCapability = "title-url"
	widgetCapabilityHideHeader        widgetCapability = "hide-header"
	widgetCapabilityCSSClass          widgetCapability = "css-class"
	widgetCapabilityCache             widgetCapability = "cache"
	widgetCapabilityNewTab            widgetCapability = "new-tab"
	widgetCapabilityLimit             widgetCapability = "limit"
	widgetCapabilityCollapseAfter     widgetCapability = "collapse-after"
	widgetCapabilityCollapseAfterRows widgetCapability = "collapse-after-rows"
	widgetCapabilityTimeout           widgetCapability = "timeout"
	widgetCapabilityAllowInsecure     widgetCapability = "allow-insecure"
	widgetCapabilityHeaders           widgetCapability = "headers"
	widgetCapabilityBasicAuth         widgetCapability = "basic-auth"
	widgetCapabilityProxy             widgetCapability = "proxy"
)

type widgetCapabilityScope uint8

const (
	widgetCapabilityScopeGlobal widgetCapabilityScope = 1 << iota
	widgetCapabilityScopeType
	widgetCapabilityScopeInstance
	widgetCapabilityScopeChild
)

type widgetCapabilityDefinition struct {
	Capability widgetCapability
	Scopes     widgetCapabilityScope
}

func (d widgetCapabilityDefinition) supports(scope widgetCapabilityScope) bool {
	return d.Scopes&scope != 0
}
