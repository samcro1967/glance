package glance

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultHTTPClientTransportSettings(t *testing.T) {
	tests := []struct {
		name   string
		client *http.Client
	}{
		{
			name:   "default",
			client: defaultHTTPClient,
		},
		{
			name:   "insecure",
			client: defaultInsecureHTTPClient,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, ok := test.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport type = %T, want *http.Transport", test.client.Transport)
			}

			if transport.MaxIdleConnsPerHost != 10 {
				t.Fatalf(
					"MaxIdleConnsPerHost = %d, want 10",
					transport.MaxIdleConnsPerHost,
				)
			}

			if transport.IdleConnTimeout != 90*time.Second {
				t.Fatalf(
					"IdleConnTimeout = %s, want %s",
					transport.IdleConnTimeout,
					90*time.Second,
				)
			}
		})
	}
}
