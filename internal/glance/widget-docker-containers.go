package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var dockerContainersWidgetTemplate = mustParseTemplate("docker-containers.html", "widget-base.html")

type dockerContainersWidget struct {
	widgetBase           `yaml:",inline"`
	HideByDefault        bool                         `yaml:"hide-by-default"`
	RunningOnly          bool                         `yaml:"running-only"`
	Category             string                       `yaml:"category"`
	SockPath             string                       `yaml:"sock-path"`
	FormatContainerNames bool                         `yaml:"format-container-names"`
	GroupBy              string                       `yaml:"group-by"`
	Containers           dockerContainerList          `yaml:"-"`
	Groups               []dockerContainerGroup       `yaml:"-"`
	LabelOverrides       map[string]map[string]string `yaml:"containers"`
	DefaultNewTab        *bool                        `yaml:"-"`
}

func (widget *dockerContainersWidget) initialize() error {
	widget.withTitle("Docker Containers").withCacheDuration(1 * time.Minute)

	if widget.SockPath == "" {
		widget.SockPath = "/var/run/docker.sock"
	}

	if widget.GroupBy != "" && widget.GroupBy != dockerContainerGroupByComposeProject {
		return fmt.Errorf("group-by must be one of: %s", dockerContainerGroupByComposeProject)
	}

	return nil
}

func (widget *dockerContainersWidget) update(ctx context.Context) {
	containers, err := fetchDockerContainers(
		ctx,
		widget.SockPath,
		widget.HideByDefault,
		widget.Category,
		widget.RunningOnly,
		widget.FormatContainerNames,
		widget.LabelOverrides,
		widget.DefaultNewTab,
	)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	containers.sortByStateIconThenName()
	widget.Groups = nil
	if widget.GroupBy == dockerContainerGroupByComposeProject {
		widget.Groups = groupDockerContainersByComposeProject(containers)
	}
	widget.Containers = containers
}

func (widget *dockerContainersWidget) Render() template.HTML {
	return widget.renderTemplate(widget, dockerContainersWidgetTemplate)
}

const (
	dockerContainerLabelHide           = "glance.hide"
	dockerContainerLabelName           = "glance.name"
	dockerContainerLabelURL            = "glance.url"
	dockerContainerLabelDescription    = "glance.description"
	dockerContainerLabelSameTab        = "glance.same-tab"
	dockerContainerLabelIcon           = "glance.icon"
	dockerContainerLabelID             = "glance.id"
	dockerContainerLabelParent         = "glance.parent"
	dockerContainerLabelCategory       = "glance.category"
	dockerContainerLabelComposeProject = "com.docker.compose.project"
)

const dockerContainerGroupByComposeProject = "compose-project"

const (
	dockerContainerStateIconOK     = "ok"
	dockerContainerStateIconPaused = "paused"
	dockerContainerStateIconWarn   = "warn"
	dockerContainerStateIconOther  = "other"
)

var dockerContainerStateIconPriorities = map[string]int{
	dockerContainerStateIconWarn:   0,
	dockerContainerStateIconOther:  1,
	dockerContainerStateIconPaused: 2,
	dockerContainerStateIconOK:     3,
}

type dockerContainerJsonResponse struct {
	Names  []string              `json:"Names"`
	Image  string                `json:"Image"`
	State  string                `json:"State"`
	Status string                `json:"Status"`
	Labels dockerContainerLabels `json:"Labels"`
}

type dockerContainerLabels map[string]string

func (l *dockerContainerLabels) getOrDefault(label, def string) string {
	if l == nil {
		return def
	}

	v, ok := (*l)[label]
	if !ok {
		return def
	}

	if v == "" {
		return def
	}

	return v
}

type dockerContainer struct {
	Name           string
	URL            string
	SameTab        bool
	Image          string
	State          string
	StateText      string
	StateIcon      string
	Description    string
	Icon           customIconField
	Children       dockerContainerList
	ComposeProject string
}

type dockerContainerGroup struct {
	Name       string
	Containers dockerContainerList
}

type dockerContainerList []dockerContainer

func (containers dockerContainerList) sortByStateIconThenName() {
	p := &dockerContainerStateIconPriorities

	sort.SliceStable(containers, func(a, b int) bool {
		if containers[a].StateIcon != containers[b].StateIcon {
			return (*p)[containers[a].StateIcon] < (*p)[containers[b].StateIcon]
		}

		return strings.ToLower(containers[a].Name) < strings.ToLower(containers[b].Name)
	})
}

func groupDockerContainersByComposeProject(containers dockerContainerList) []dockerContainerGroup {
	grouped := make(map[string]dockerContainerList)
	ungrouped := make(dockerContainerList, 0)

	for i := range containers {
		container := containers[i]
		if container.ComposeProject == "" {
			ungrouped = append(ungrouped, container)
			continue
		}
		grouped[container.ComposeProject] = append(grouped[container.ComposeProject], container)
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := strings.ToLower(names[i])
		right := strings.ToLower(names[j])
		if left == right {
			return names[i] < names[j]
		}
		return left < right
	})

	groups := make([]dockerContainerGroup, 0, len(names)+1)
	for _, name := range names {
		groups = append(groups, dockerContainerGroup{Name: name, Containers: grouped[name]})
	}
	if len(ungrouped) > 0 {
		groups = append(groups, dockerContainerGroup{Containers: ungrouped})
	}
	return groups
}

func dockerContainerStateToStateIcon(container *dockerContainerJsonResponse) string {
	if strings.Contains(strings.ToLower(container.Status), "(unhealthy)") {
		return dockerContainerStateIconWarn
	}

	switch strings.ToLower(container.State) {
	case "running":
		return dockerContainerStateIconOK
	case "paused":
		return dockerContainerStateIconPaused
	case "exited", "dead":
		return dockerContainerStateIconWarn
	default:
		return dockerContainerStateIconOther
	}
}

func fetchDockerContainers(
	ctx context.Context,
	socketPath string,
	hideByDefault bool,
	category string,
	runningOnly bool,
	formatNames bool,
	labelOverrides map[string]map[string]string,
	defaultNewTab *bool,
) (dockerContainerList, error) {
	containers, err := fetchDockerContainersFromSource(
		ctx,
		socketPath,
		runningOnly,
		labelOverrides,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching containers: %w", err)
	}

	containers, children := groupDockerContainerChildren(containers, hideByDefault, category)
	dockerContainers := make(dockerContainerList, 0, len(containers))

	for i := range containers {
		container := &containers[i]

		sameTab := false
		if defaultNewTab != nil {
			sameTab = !*defaultNewTab
		}
		if value, ok := container.Labels[dockerContainerLabelSameTab]; ok && value != "" {
			sameTab = stringToBool(value)
		}

		dc := dockerContainer{
			Name:           deriveDockerContainerName(container, formatNames),
			URL:            container.Labels.getOrDefault(dockerContainerLabelURL, ""),
			Description:    container.Labels.getOrDefault(dockerContainerLabelDescription, ""),
			SameTab:        sameTab,
			Image:          container.Image,
			State:          strings.ToLower(container.State),
			StateText:      strings.ToLower(container.Status),
			Icon:           newCustomIconField(container.Labels.getOrDefault(dockerContainerLabelIcon, "si:docker")),
			ComposeProject: container.Labels.getOrDefault(dockerContainerLabelComposeProject, ""),
		}

		if idValue := container.Labels.getOrDefault(dockerContainerLabelID, ""); idValue != "" {
			if children, ok := children[idValue]; ok {
				for i := range children {
					child := &children[i]
					dc.Children = append(dc.Children, dockerContainer{
						Name:      deriveDockerContainerName(child, formatNames),
						StateText: child.Status,
						StateIcon: dockerContainerStateToStateIcon(child),
					})
				}
			}
		}

		dc.Children.sortByStateIconThenName()

		stateIconSupersededByChild := false
		for i := range dc.Children {
			if dc.Children[i].StateIcon == dockerContainerStateIconWarn {
				dc.StateIcon = dockerContainerStateIconWarn
				stateIconSupersededByChild = true
				break
			}
		}
		if !stateIconSupersededByChild {
			dc.StateIcon = dockerContainerStateToStateIcon(container)
		}

		dockerContainers = append(dockerContainers, dc)
	}

	return dockerContainers, nil
}

func deriveDockerContainerName(container *dockerContainerJsonResponse, formatNames bool) string {
	if v := container.Labels.getOrDefault(dockerContainerLabelName, ""); v != "" {
		return v
	}

	if len(container.Names) == 0 || container.Names[0] == "" {
		return "n/a"
	}

	name := strings.TrimLeft(container.Names[0], "/")

	if formatNames {
		name = strings.ReplaceAll(name, "_", " ")
		name = strings.ReplaceAll(name, "-", " ")

		words := strings.Split(name, " ")
		for i := range words {
			if len(words[i]) > 0 {
				words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
			}
		}
		name = strings.Join(words, " ")
	}

	return name
}

func groupDockerContainerChildren(
	containers []dockerContainerJsonResponse,
	hideByDefault bool,
	category string,
) (
	[]dockerContainerJsonResponse,
	map[string][]dockerContainerJsonResponse,
) {
	parents := make([]dockerContainerJsonResponse, 0, len(containers))
	children := make(map[string][]dockerContainerJsonResponse)

	for i := range containers {
		container := &containers[i]

		if isDockerContainerHidden(container, hideByDefault) {
			continue
		}

		isParent := container.Labels.getOrDefault(dockerContainerLabelID, "") != ""
		parent := container.Labels.getOrDefault(dockerContainerLabelParent, "")

		if !isParent && parent != "" {
			children[parent] = append(children[parent], *container)
			continue
		}

		// Category filtering applies to top-level containers. Children remain
		// associated with their parent regardless of their own category label.
		if category != "" && container.Labels.getOrDefault(dockerContainerLabelCategory, "") != category {
			continue
		}

		parents = append(parents, *container)
	}

	return parents, children
}

func isDockerContainerHidden(container *dockerContainerJsonResponse, hideByDefault bool) bool {
	if v := container.Labels.getOrDefault(dockerContainerLabelHide, ""); v != "" {
		return stringToBool(v)
	}

	return hideByDefault
}

func fetchDockerContainersFromSource(
	ctx context.Context,
	source string,
	runningOnly bool,
	labelOverrides map[string]map[string]string,
) ([]dockerContainerJsonResponse, error) {
	var requestBaseURL string

	var client *http.Client
	if strings.HasPrefix(source, "tcp://") || strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client = newHTTPClient(0, false)
		var err error
		requestBaseURL, err = dockerContainersRemoteSourceURL(source)
		if err != nil {
			return nil, err
		}
	} else {
		requestBaseURL = "http://docker"
		transport := defaultHTTPTransport.Clone()
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", source)
		}
		client = &http.Client{
			Transport: transport,
			Timeout:   defaultClientTimeout,
		}
	}

	fetchAll := ternary(runningOnly, "false", "true")
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestCtx,
		"GET",
		requestBaseURL+"/containers/json?all="+fetchAll,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating Docker request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("sending Docker request: %w", safeHTTPTransportError(err))
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Docker API request: %w", unexpectedHTTPStatusError(response))
	}

	body, err := readDefaultHTTPResponseBody(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Docker response: %w", err)
	}

	var containers []dockerContainerJsonResponse
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("decoding Docker response: %w", err)
	}

	for i := range containers {
		container := &containers[i]
		name := strings.TrimLeft(itemAtIndexOrDefault(container.Names, 0, ""), "/")

		if name == "" {
			continue
		}

		overrides, ok := labelOverrides[name]
		if !ok {
			continue
		}

		if container.Labels == nil {
			container.Labels = make(dockerContainerLabels)
		}

		for label, value := range overrides {
			container.Labels["glance."+label] = value
		}
	}

	return containers, nil
}

func dockerContainersRemoteSourceURL(source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("parsing URL: %w", err)
	}

	scheme := parsed.Scheme
	if scheme == "tcp" {
		scheme = "http"
	}

	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return scheme + "://" + net.JoinHostPort(parsed.Hostname(), port), nil
}

func (widget *dockerContainersWidget) setDefaultNewTab(value bool) {
	widget.DefaultNewTab = new(bool)
	*widget.DefaultNewTab = value
}
