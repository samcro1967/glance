package glance

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samcro1967/glance/pkg/sysinfo"

	"gopkg.in/yaml.v3"
)

var serverStatsWidgetTemplate = mustParseTemplate("server-stats.html", "widget-base.html")

type serverStatsWidget struct {
	widgetBase `yaml:",inline"`
	Servers    []serverStatsRequest `yaml:"servers"`
}

func (widget *serverStatsWidget) initialize() error {
	widget.withTitle("Server Stats").withCacheDuration(15 * time.Second)
	widget.widgetBase.WIP = true

	if len(widget.Servers) == 0 {
		widget.Servers = []serverStatsRequest{{Type: "local"}}
	}

	for i := range widget.Servers {
		widget.Servers[i].URL = strings.TrimRight(widget.Servers[i].URL, "/")

		if widget.Servers[i].Timeout == 0 {
			widget.Servers[i].Timeout = durationField(3 * time.Second)
		}
	}

	return nil
}

func (widget *serverStatsWidget) update(ctx context.Context) {
	var wg sync.WaitGroup
	var resultMu sync.Mutex
	succeeded := 0
	failed := 0
	var firstFailure error

	recordSuccess := func() {
		resultMu.Lock()
		succeeded++
		resultMu.Unlock()
	}

	recordFailure := func(err error) {
		resultMu.Lock()
		failed++
		if firstFailure == nil {
			firstFailure = err
		}
		resultMu.Unlock()
	}

	for i := range widget.Servers {
		serv := &widget.Servers[i]

		if serv.Type == "local" {
			info, _ := sysinfo.Collect(serv.SystemInfoRequest)

			serv.Info = info
			serv.IsReachable = true
			recordSuccess()

			continue
		}

		wg.Add(1)
		go func(serv *serverStatsRequest, index int) {
			defer wg.Done()

			info, err := fetchRemoteServerInfo(ctx, serv)
			if err != nil {
				serv.IsReachable = false
				serv.Info = &sysinfo.SystemInfo{
					Hostname: "Unnamed server #" + strconv.Itoa(index+1),
				}
				recordFailure(err)
				return
			}

			serv.IsReachable = true
			serv.Info = info
			recordSuccess()
		}(serv, i)
	}

	wg.Wait()

	if err := ctx.Err(); err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	var err error
	switch {
	case failed == 0:
	case succeeded == 0:
		err = contentFetchError(
			errNoContent,
			failed,
			len(widget.Servers),
			"servers",
			firstFailure,
		)
	default:
		err = contentFetchError(
			errPartialContent,
			failed,
			len(widget.Servers),
			"servers",
			firstFailure,
		)
	}

	widget.canContinueUpdateAfterHandlingErr(err)
}

func (widget *serverStatsWidget) Render() template.HTML {
	return widget.renderTemplate(widget, serverStatsWidgetTemplate)
}

type serverStatsRequest struct {
	*sysinfo.SystemInfoRequest `yaml:",inline"`
	configuredFields           yamlConfiguredFields `yaml:"-"`
	Info                       *sysinfo.SystemInfo  `yaml:"-"`
	IsReachable                bool                 `yaml:"-"`
	StatusText                 string               `yaml:"-"`
	Name                       string               `yaml:"name"`
	HideSwap                   bool                 `yaml:"hide-swap"`
	Type                       string               `yaml:"type"`
	URL                        string               `yaml:"url"`
	Token                      string               `yaml:"token"`
	Timeout                    durationField        `yaml:"timeout"`
	AllowInsecure              bool                 `yaml:"allow-insecure"`
	// Support for other agents
	// Provider                   string              `yaml:"provider"`
}

func (request *serverStatsRequest) UnmarshalYAML(node *yaml.Node) error {
	type plain serverStatsRequest
	if err := node.Decode((*plain)(request)); err != nil {
		return err
	}

	request.configuredFields = yamlMappingFields(node)
	return nil
}

func fetchRemoteServerInfo(ctx context.Context, infoReq *serverStatsRequest) (*sysinfo.SystemInfo, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(infoReq.Timeout))
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodGet,
		infoReq.URL+"/api/sysinfo/all",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating remote system info request: %w", err)
	}

	if infoReq.Token != "" {
		request.Header.Set("Authorization", "Bearer "+infoReq.Token)
	}

	client := ternary(infoReq.AllowInsecure, defaultInsecureHTTPClient, defaultHTTPClient)
	info, err := decodeJsonFromRequest[*sysinfo.SystemInfo](client, request)
	if err != nil {
		return nil, err
	}

	infoReq.SystemInfoRequest.Filter(info)

	return info, nil
}

func (widget *serverStatsWidget) setDefaultTimeout(value durationField) {
	for i := range widget.Servers {
		if !widget.Servers[i].configuredFields["timeout"] {
			widget.Servers[i].Timeout = value
		}
	}
}

func (widget *serverStatsWidget) setDefaultAllowInsecure(value bool) {
	for i := range widget.Servers {
		if !widget.Servers[i].configuredFields["allow-insecure"] {
			widget.Servers[i].AllowInsecure = value
		}
	}
}
