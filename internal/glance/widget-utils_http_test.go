package glance

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDefaultHTTPClientTransportSettings(t *testing.T) {
	tests := []struct {
		name          string
		client        *http.Client
		wantInsecure  bool
		wantTransport *http.Transport
	}{
		{
			name:          "default",
			client:        defaultHTTPClient,
			wantInsecure:  false,
			wantTransport: defaultHTTPTransport,
		},
		{
			name:          "insecure",
			client:        defaultInsecureHTTPClient,
			wantInsecure:  true,
			wantTransport: defaultInsecureHTTPTransport,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, ok := test.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport type = %T, want *http.Transport", test.client.Transport)
			}

			if transport != test.wantTransport {
				t.Fatal("client does not use expected canonical transport")
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

			if transport.Proxy == nil {
				t.Fatal("Proxy is nil, want environment proxy support")
			}

			gotInsecure := transport.TLSClientConfig != nil &&
				transport.TLSClientConfig.InsecureSkipVerify
			if gotInsecure != test.wantInsecure {
				t.Fatalf(
					"InsecureSkipVerify = %v, want %v",
					gotInsecure,
					test.wantInsecure,
				)
			}

			if test.client.Timeout != defaultClientTimeout {
				t.Fatalf(
					"Timeout = %s, want %s",
					test.client.Timeout,
					defaultClientTimeout,
				)
			}
		})
	}
}

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name          string
		timeout       durationField
		allowInsecure bool
		wantTimeout   time.Duration
		wantTransport *http.Transport
	}{
		{
			name:          "default",
			wantTimeout:   defaultClientTimeout,
			wantTransport: defaultHTTPTransport,
		},
		{
			name:          "custom timeout",
			timeout:       durationField(9 * time.Second),
			wantTimeout:   9 * time.Second,
			wantTransport: defaultHTTPTransport,
		},
		{
			name:          "insecure",
			allowInsecure: true,
			wantTimeout:   defaultClientTimeout,
			wantTransport: defaultInsecureHTTPTransport,
		},
		{
			name:          "insecure custom timeout",
			timeout:       durationField(11 * time.Second),
			allowInsecure: true,
			wantTimeout:   11 * time.Second,
			wantTransport: defaultInsecureHTTPTransport,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newHTTPClient(test.timeout, test.allowInsecure)

			if client.Timeout != test.wantTimeout {
				t.Fatalf(
					"Timeout = %s, want %s",
					client.Timeout,
					test.wantTimeout,
				)
			}

			if client.Transport != test.wantTransport {
				t.Fatal("client does not use expected transport")
			}
		})
	}
}

func TestNewHTTPClientDoesNotMutateSharedClient(t *testing.T) {
	client := newHTTPClient(durationField(17*time.Second), false)

	if client == defaultHTTPClient {
		t.Fatal("newHTTPClient returned shared default client")
	}

	if client.Timeout != 17*time.Second {
		t.Fatalf("derived Timeout = %s, want 17s", client.Timeout)
	}

	if defaultHTTPClient.Timeout != defaultClientTimeout {
		t.Fatalf(
			"default client Timeout = %s, want %s",
			defaultHTTPClient.Timeout,
			defaultClientTimeout,
		)
	}

	client.Timeout = 23 * time.Second

	if defaultHTTPClient.Timeout != defaultClientTimeout {
		t.Fatalf(
			"mutating derived client changed default Timeout to %s",
			defaultHTTPClient.Timeout,
		)
	}
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (body *trackingReadCloser) Close() error {
	body.closed = true
	return nil
}

func TestFetchHTTPResponseBodyClosesResponseBody(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		status     string
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			status:     "200 OK",
		},
		{
			name:       "HTTP status failure",
			statusCode: http.StatusBadGateway,
			status:     "502 Bad Gateway",
			wantErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackingReadCloser{
				Reader: strings.NewReader(`{"ok":true}`),
			}

			client := testRequestDoer(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.statusCode,
					Status:     test.status,
					Body:       body,
				}, nil
			})

			request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}

			_, err = fetchHTTPResponseBody(client, request)

			if test.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !body.closed {
				t.Fatal("response body was not closed")
			}
		})
	}
}
