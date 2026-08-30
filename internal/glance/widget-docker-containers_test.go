package glance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDockerContainersWidgetInitializeDefaults(t *testing.T) {
	widget := &dockerContainersWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Title != "Docker Containers" {
		t.Fatalf("title = %q, want %q", widget.Title, "Docker Containers")
	}

	if widget.cacheDuration != time.Minute {
		t.Fatalf(
			"cache duration = %s, want %s",
			widget.cacheDuration,
			time.Minute,
		)
	}

	if widget.SockPath != "/var/run/docker.sock" {
		t.Fatalf(
			"socket path = %q, want %q",
			widget.SockPath,
			"/var/run/docker.sock",
		)
	}
}

func TestDockerContainersWidgetInitializePreservesSocketPath(t *testing.T) {
	widget := &dockerContainersWidget{
		SockPath: "tcp://docker.example.com:2375",
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.SockPath != "tcp://docker.example.com:2375" {
		t.Fatalf(
			"socket path = %q, want configured path",
			widget.SockPath,
		)
	}
}

func TestDockerContainerLabelsGetOrDefault(t *testing.T) {
	t.Run("nil labels", func(t *testing.T) {
		var labels dockerContainerLabels

		if got := labels.getOrDefault("missing", "default"); got != "default" {
			t.Fatalf("value = %q, want %q", got, "default")
		}
	})

	t.Run("missing label", func(t *testing.T) {
		labels := dockerContainerLabels{
			"existing": "value",
		}

		if got := labels.getOrDefault("missing", "default"); got != "default" {
			t.Fatalf("value = %q, want %q", got, "default")
		}
	})

	t.Run("empty label", func(t *testing.T) {
		labels := dockerContainerLabels{
			"example": "",
		}

		if got := labels.getOrDefault("example", "default"); got != "default" {
			t.Fatalf("value = %q, want %q", got, "default")
		}
	})

	t.Run("existing label", func(t *testing.T) {
		labels := dockerContainerLabels{
			"example": "configured",
		}

		if got := labels.getOrDefault("example", "default"); got != "configured" {
			t.Fatalf("value = %q, want %q", got, "configured")
		}
	})
}

func TestDockerContainerStateToStateIcon(t *testing.T) {
	tests := []struct {
		name   string
		state  string
		status string
		want   string
	}{
		{
			name:   "running",
			state:  "running",
			status: "Up 5 minutes",
			want:   dockerContainerStateIconOK,
		},
		{
			name:   "running case insensitive",
			state:  "RUNNING",
			status: "Up 5 minutes",
			want:   dockerContainerStateIconOK,
		},
		{
			name:   "unhealthy running container",
			state:  "running",
			status: "Up 5 minutes (unhealthy)",
			want:   dockerContainerStateIconWarn,
		},
		{
			name:   "unhealthy case insensitive",
			state:  "running",
			status: "Up 5 minutes (UNHEALTHY)",
			want:   dockerContainerStateIconWarn,
		},
		{
			name:   "paused",
			state:  "paused",
			status: "Up 5 minutes (Paused)",
			want:   dockerContainerStateIconPaused,
		},
		{
			name:   "exited",
			state:  "exited",
			status: "Exited (0)",
			want:   dockerContainerStateIconWarn,
		},
		{
			name:   "dead",
			state:  "dead",
			status: "Dead",
			want:   dockerContainerStateIconWarn,
		},
		{
			name:   "unknown",
			state:  "created",
			status: "Created",
			want:   dockerContainerStateIconOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &dockerContainerJsonResponse{
				State:  tt.state,
				Status: tt.status,
			}

			got := dockerContainerStateToStateIcon(container)
			if got != tt.want {
				t.Fatalf("state icon = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsDockerContainerHidden(t *testing.T) {
	tests := []struct {
		name          string
		hideByDefault bool
		labels        dockerContainerLabels
		want          bool
	}{
		{
			name:          "visible by default",
			hideByDefault: false,
			want:          false,
		},
		{
			name:          "hidden by default",
			hideByDefault: true,
			want:          true,
		},
		{
			name:          "explicit hide",
			hideByDefault: false,
			labels: dockerContainerLabels{
				dockerContainerLabelHide: "true",
			},
			want: true,
		},
		{
			name:          "explicit show overrides hide by default",
			hideByDefault: true,
			labels: dockerContainerLabels{
				dockerContainerLabelHide: "false",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &dockerContainerJsonResponse{
				Labels: tt.labels,
			}

			got := isDockerContainerHidden(container, tt.hideByDefault)
			if got != tt.want {
				t.Fatalf("hidden = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeriveDockerContainerName(t *testing.T) {
	tests := []struct {
		name        string
		names       []string
		labels      dockerContainerLabels
		formatNames bool
		want        string
	}{
		{
			name:  "container name",
			names: []string{"/example"},
			want:  "example",
		},
		{
			name:  "label override",
			names: []string{"/example"},
			labels: dockerContainerLabels{
				dockerContainerLabelName: "Custom Name",
			},
			want: "Custom Name",
		},
		{
			name: "missing names",
			want: "n/a",
		},
		{
			name:  "empty first name",
			names: []string{""},
			want:  "n/a",
		},
		{
			name:        "formatted hyphenated name",
			names:       []string{"/example-container"},
			formatNames: true,
			want:        "Example Container",
		},
		{
			name:        "formatted underscored name",
			names:       []string{"/example_container"},
			formatNames: true,
			want:        "Example Container",
		},
		{
			name:        "formatted mixed name",
			names:       []string{"/example-container_name"},
			formatNames: true,
			want:        "Example Container Name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &dockerContainerJsonResponse{
				Names:  tt.names,
				Labels: tt.labels,
			}

			got := deriveDockerContainerName(container, tt.formatNames)
			if got != tt.want {
				t.Fatalf("name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupDockerContainerChildren(t *testing.T) {
	containers := []dockerContainerJsonResponse{
		{
			Names: []string{"/parent"},
			Labels: dockerContainerLabels{
				dockerContainerLabelID: "group",
			},
		},
		{
			Names: []string{"/child"},
			Labels: dockerContainerLabels{
				dockerContainerLabelParent: "group",
			},
		},
		{
			Names: []string{"/standalone"},
		},
		{
			Names: []string{"/hidden"},
			Labels: dockerContainerLabels{
				dockerContainerLabelHide: "true",
			},
		},
	}

	parents, children := groupDockerContainerChildren(containers, false)

	if len(parents) != 2 {
		t.Fatalf("parent count = %d, want 2", len(parents))
	}

	if parents[0].Names[0] != "/parent" {
		t.Fatalf("first parent = %q, want %q", parents[0].Names[0], "/parent")
	}

	if parents[1].Names[0] != "/standalone" {
		t.Fatalf(
			"second parent = %q, want %q",
			parents[1].Names[0],
			"/standalone",
		)
	}

	groupChildren := children["group"]
	if len(groupChildren) != 1 {
		t.Fatalf("group child count = %d, want 1", len(groupChildren))
	}

	if groupChildren[0].Names[0] != "/child" {
		t.Fatalf(
			"group child = %q, want %q",
			groupChildren[0].Names[0],
			"/child",
		)
	}
}

func TestDockerContainerListSortByStateIconThenName(t *testing.T) {
	containers := dockerContainerList{
		{Name: "Zulu", StateIcon: dockerContainerStateIconOK},
		{Name: "Bravo", StateIcon: dockerContainerStateIconWarn},
		{Name: "alpha", StateIcon: dockerContainerStateIconWarn},
		{Name: "Delta", StateIcon: dockerContainerStateIconPaused},
		{Name: "Charlie", StateIcon: dockerContainerStateIconOther},
	}

	containers.sortByStateIconThenName()

	wantNames := []string{
		"alpha",
		"Bravo",
		"Charlie",
		"Delta",
		"Zulu",
	}

	for i, want := range wantNames {
		if containers[i].Name != want {
			t.Fatalf(
				"container %d name = %q, want %q",
				i,
				containers[i].Name,
				want,
			)
		}
	}
}

func TestFetchDockerContainersFromSourceRunningOnlyQuery(t *testing.T) {
	tests := []struct {
		name        string
		runningOnly bool
		wantAll     string
	}{
		{
			name:        "all containers",
			runningOnly: false,
			wantAll:     "true",
		},
		{
			name:        "running only",
			runningOnly: true,
			wantAll:     "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Path; got != "/containers/json" {
					t.Errorf("request path = %q, want %q", got, "/containers/json")
				}

				if got := r.URL.Query().Get("all"); got != tt.wantAll {
					t.Errorf("all query = %q, want %q", got, tt.wantAll)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			containers, err := fetchDockerContainersFromSource(
				context.Background(),
				server.URL,
				"",
				tt.runningOnly,
				nil,
			)
			if err != nil {
				t.Fatalf("fetching containers: %v", err)
			}

			if len(containers) != 0 {
				t.Fatalf("container count = %d, want 0", len(containers))
			}
		})
	}
}

func TestFetchDockerContainersFromSourceAppliesLabelOverridesBeforeCategoryFilter(t *testing.T) {
	response := []dockerContainerJsonResponse{
		{
			Names: []string{"/included"},
			Image: "example/included",
			State: "running",
			Labels: dockerContainerLabels{
				dockerContainerLabelCategory: "original",
			},
		},
		{
			Names: []string{"/excluded"},
			Image: "example/excluded",
			State: "running",
			Labels: dockerContainerLabels{
				dockerContainerLabelCategory: "other",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encoding Docker response: %v", err)
		}
	}))
	defer server.Close()

	containers, err := fetchDockerContainersFromSource(
		context.Background(),
		server.URL,
		"configured",
		false,
		map[string]map[string]string{
			"included": {
				"category":    "configured",
				"name":        "Configured Name",
				"description": "Configured Description",
			},
		},
	)
	if err != nil {
		t.Fatalf("fetching containers: %v", err)
	}

	if len(containers) != 1 {
		t.Fatalf("container count = %d, want 1", len(containers))
	}

	container := containers[0]

	if got := container.Labels[dockerContainerLabelCategory]; got != "configured" {
		t.Fatalf("category label = %q, want %q", got, "configured")
	}

	if got := container.Labels[dockerContainerLabelName]; got != "Configured Name" {
		t.Fatalf("name label = %q, want %q", got, "Configured Name")
	}

	if got := container.Labels[dockerContainerLabelDescription]; got != "Configured Description" {
		t.Fatalf(
			"description label = %q, want %q",
			got,
			"Configured Description",
		)
	}
}

func TestFetchDockerContainersFromSourceCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, err := fetchDockerContainersFromSource(
			ctx,
			server.URL,
			"",
			false,
			nil,
		)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("Docker request did not start")
	}

	cancel()

	select {
	case err := <-result:
		if err == nil {
			close(release)
			t.Fatal("expected cancellation error")
		}

		if !errors.Is(err, context.Canceled) {
			close(release)
			t.Fatalf("error does not preserve cancellation: %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("Docker request did not return after cancellation")
	}

	close(release)
}

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
				t.Errorf(
					"dockerContainersRemoteSourceURL() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}
