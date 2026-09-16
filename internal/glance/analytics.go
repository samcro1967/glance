package glance

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const analyticsProviderGoatCounter = "goatcounter"

type analyticsConfig struct {
	Provider string `yaml:"provider"`
	Endpoint string `yaml:"endpoint"`
}

func (config analyticsConfig) configured() bool {
	return strings.TrimSpace(config.Provider) != "" || strings.TrimSpace(config.Endpoint) != ""
}

func normalizeAnalyticsEndpoint(configuredEndpoint string) (string, error) {
	endpoint := strings.TrimSpace(configuredEndpoint)
	if endpoint == "" {
		return "", fmt.Errorf("is empty")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("is invalid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("must contain a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("must not contain user information")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("must not contain a path")
	}
	if parsed.RawQuery != "" {
		return "", fmt.Errorf("must not contain a query")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("must not contain a fragment")
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("must contain a host")
	}

	host := hostname
	if port := parsed.Port(); port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}

	return strings.ToLower(parsed.Scheme) + "://" + host, nil
}

func (config analyticsConfig) CountURL() string {
	if config.Provider != analyticsProviderGoatCounter || config.Endpoint == "" {
		return ""
	}
	return config.Endpoint + "/count"
}

func (config analyticsConfig) ScriptURL() string {
	if config.Provider != analyticsProviderGoatCounter || config.Endpoint == "" {
		return ""
	}
	return config.Endpoint + "/count.js"
}
