package glance

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersonalStateStorePersistsAndIsolatesIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "personal-state.json")
	store, err := newPersonalStateStore(path)
	if err != nil {
		t.Fatalf("newPersonalStateStore() error = %v", err)
	}

	value := []byte(`[{"text":"one","checked":false}]`)
	if err := store.put("user-a", "todo", "home", value); err != nil {
		t.Fatalf("put() error = %v", err)
	}

	got, err := store.get("user-a", "todo", "home")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if string(got) != string(value) {
		t.Fatalf("state = %s, want %s", got, value)
	}
	if _, err := store.get("user-b", "todo", "home"); !errors.Is(err, errPersonalStateNotFound) {
		t.Fatalf("other identity error = %v, want not found", err)
	}
	if _, err := store.get("user-a", "timer", "home"); !errors.Is(err, errPersonalStateNotFound) {
		t.Fatalf("other namespace error = %v, want not found", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if strings.Contains(string(contents), "user-a") {
		t.Fatal("personal state file exposed raw authorization identity")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("state file mode = %o, want 600", gotMode)
	}

	reloaded, err := newPersonalStateStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	got, err = reloaded.get("user-a", "todo", "home")
	if err != nil {
		t.Fatalf("reloaded get() error = %v", err)
	}

	var gotCompact bytes.Buffer
	if err := json.Compact(&gotCompact, got); err != nil {
		t.Fatalf("compact reloaded state: %v", err)
	}
	var wantCompact bytes.Buffer
	if err := json.Compact(&wantCompact, value); err != nil {
		t.Fatalf("compact expected state: %v", err)
	}
	if !bytes.Equal(gotCompact.Bytes(), wantCompact.Bytes()) {
		t.Fatalf("reloaded state = %s, want %s", got, value)
	}
}

func TestPersonalStateStoreRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "personal-state.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newPersonalStateStore(path); err == nil || !strings.Contains(err.Error(), "decoding personal state file") {
		t.Fatalf("error = %v, want corrupt state diagnostic", err)
	}
}

func TestPersonalStateConfigValidation(t *testing.T) {
	secret, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatal(err)
	}
	absolutePath := filepath.Join(t.TempDir(), "state.json")
	pages := "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets: []\n"
	auth := "auth:\n  secret-key: " + secret + "\n  users:\n    test-user:\n      password: test-password\n"

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "requires authentication",
			yaml:    "server:\n  personal-state:\n    enabled: true\n    path: " + absolutePath + "\n" + pages,
			wantErr: "server personal-state requires authentication",
		},
		{
			name:    "requires path",
			yaml:    "server:\n  personal-state:\n    enabled: true\n" + auth + pages,
			wantErr: "server personal-state path is required when enabled",
		},
		{
			name:    "requires absolute path",
			yaml:    "server:\n  personal-state:\n    enabled: true\n    path: relative.json\n" + auth + pages,
			wantErr: "server personal-state path must be absolute",
		},
		{
			name: "valid authenticated state",
			yaml: "server:\n  personal-state:\n    enabled: true\n    path: " + absolutePath + "\n" + auth + pages,
		},
		{
			name: "disabled preserves existing behavior",
			yaml: pages,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newConfigFromYAML([]byte(tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("newConfigFromYAML() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPersonalStateHandlersRequireIdentityAndRoundTrip(t *testing.T) {
	app := newAuthTestApplication(t)
	app.Config.Server.PersonalState.Enabled = true
	app.Config.Server.PersonalState.Path = filepath.Join(t.TempDir(), "state.json")
	store, err := newPersonalStateStore(app.Config.Server.PersonalState.Path)
	if err != nil {
		t.Fatal(err)
	}
	app.personalState = store

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/personal-state/todo/home", nil)
	unauthorized.SetPathValue("namespace", "todo")
	unauthorized.SetPathValue("id", "home")
	unauthorizedRecorder := httptest.NewRecorder()
	app.handlePersonalStateGetRequest(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRecorder.Code, http.StatusUnauthorized)
	}

	token := authTestSessionToken(t, app, "test-user", time.Now())
	postRequest := httptest.NewRequest(http.MethodPost, "/api/personal-state/todo/home", strings.NewReader(`[{"text":"persisted"}]`))
	postRequest.SetPathValue("namespace", "todo")
	postRequest.SetPathValue("id", "home")
	postRequest.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: token})
	postRecorder := httptest.NewRecorder()
	app.handlePersonalStatePostRequest(postRecorder, postRequest)
	if postRecorder.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, body = %s", postRecorder.Code, postRecorder.Body.String())
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/personal-state/todo/home", nil)
	getRequest.SetPathValue("namespace", "todo")
	getRequest.SetPathValue("id", "home")
	getRequest.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: token})
	getRecorder := httptest.NewRecorder()
	app.handlePersonalStateGetRequest(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
	}
	if body := strings.TrimSpace(getRecorder.Body.String()); body != `[{"text":"persisted"}]` {
		t.Fatalf("GET body = %s", body)
	}
}

func TestPersonalStateKeyValidation(t *testing.T) {
	for _, tt := range []struct {
		namespace string
		id        string
		wantErr   bool
	}{
		{"todo", "default", false},
		{"timer", "important-dates", false},
		{"other", "default", true},
		{"todo", "", true},
		{"todo", "nested/id", true},
	} {
		err := validatePersonalStateKey(tt.namespace, tt.id)
		if (err != nil) != tt.wantErr {
			t.Fatalf("validatePersonalStateKey(%q, %q) error = %v", tt.namespace, tt.id, err)
		}
	}
}
