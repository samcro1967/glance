package glance

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	monitorResourceCacheDuration = 5 * time.Minute
	monitorResourceIdleRetention = 24 * time.Hour
)

var monitorResourceCache = newKeyedResourceCache[[32]byte, siteStatus](
	monitorResourceIdleRetention,
)

func monitorResourceRequestKey(request *SiteStatusRequest) [32]byte {
	url := request.DefaultURL
	if request.CheckURL != "" {
		url = request.CheckURL
	}

	timeout := request.Timeout
	if timeout == 0 {
		timeout = durationField(3 * time.Second)
	}

	var builder strings.Builder

	builder.WriteString(url)
	builder.WriteByte(0)
	builder.WriteString(fmt.Sprintf("%t", request.AllowInsecure))
	builder.WriteByte(0)
	builder.WriteString(fmt.Sprintf("%d", timeout))
	builder.WriteByte(0)
	builder.WriteString(request.BasicAuth.Username)
	builder.WriteByte(0)
	builder.WriteString(request.BasicAuth.Password)
	builder.WriteByte(0)

	keys := make([]string, 0, len(request.Headers))
	for key := range request.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte(0)
		builder.WriteString(request.Headers[key])
		builder.WriteByte(0)
	}

	return sha256.Sum256([]byte(builder.String()))
}

func fetchMonitorSiteResource(ctx context.Context, request *SiteStatusRequest) (siteStatus, error) {
	key := monitorResourceRequestKey(request)

	return monitorResourceCache.Get(
		ctx,
		key,
		func(cached cachedEntry[siteStatus], now time.Time) bool {
			return now.Sub(cached.timestamp) < monitorResourceCacheDuration
		},
		func(ctx context.Context) (siteStatus, error) {
			return fetchMonitorSiteResourceUncached(ctx, request)
		},
	)
}

func fetchMonitorSiteResourceUncached(ctx context.Context, request *SiteStatusRequest) (siteStatus, error) {
	url := request.DefaultURL
	if request.CheckURL != "" {
		url = request.CheckURL
	}

	timeout := time.Duration(request.Timeout)
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return siteStatus{
			Error: err,
		}, nil
	}

	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}

	if request.BasicAuth.Username != "" || request.BasicAuth.Password != "" {
		req.SetBasicAuth(request.BasicAuth.Username, request.BasicAuth.Password)
	}

	client := defaultHTTPClient
	if request.AllowInsecure {
		client = defaultInsecureHTTPClient
	}

	start := time.Now()
	response, err := client.Do(req)
	responseTime := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return siteStatus{
				Code:         0,
				ResponseTime: responseTime,
				TimedOut:     true,
				Error:        err,
			}, nil
		}

		return siteStatus{
			Code:         0,
			ResponseTime: responseTime,
			Error:        err,
		}, nil
	}
	defer response.Body.Close()

	return siteStatus{
		Code:         response.StatusCode,
		ResponseTime: responseTime,
	}, nil
}
