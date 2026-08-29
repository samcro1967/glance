package glance

import "testing"

func TestDockerContainersRemoteSourceURL(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "IPv4 address",
			source: "tcp://127.0.0.1:2375",
			want:   "http://127.0.0.1:2375",
		},
		{
			name:   "hostname",
			source: "tcp://docker.example.com:2375",
			want:   "http://docker.example.com:2375",
		},
		{
			name:   "IPv6 address",
			source: "tcp://[::1]:2375",
			want:   "http://[::1]:2375",
		},
		{
			name:   "HTTP hostname with explicit port",
			source: "http://docker.example.com:2375",
			want:   "http://docker.example.com:2375",
		},
		{
			name:   "HTTPS hostname with explicit port",
			source: "https://docker.example.com:2376",
			want:   "https://docker.example.com:2376",
		},
		{
			name:   "HTTP default port",
			source: "http://docker.example.com",
			want:   "http://docker.example.com:80",
		},
		{
			name:   "HTTPS default port",
			source: "https://docker.example.com",
			want:   "https://docker.example.com:443",
		},
		{
			name:   "IPv6 HTTPS address",
			source: "https://[2001:db8::1]:2376",
			want:   "https://[2001:db8::1]:2376",
		},
		{
			name:   "IPv6 HTTPS default port",
			source: "https://[2001:db8::1]",
			want:   "https://[2001:db8::1]:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dockerContainersRemoteSourceURL(tt.source)
			if err != nil {
				t.Fatalf("dockerContainersRemoteSourceURL() returned an error: %v", err)
			}

			if got != tt.want {
				t.Errorf("dockerContainersRemoteSourceURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
