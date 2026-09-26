package glance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	personalStateDocumentVersion = 1
	personalStateMaxPayloadBytes = 256 * 1024
	personalStateMaxIDLength     = 128
)

var errPersonalStateNotFound = errors.New("personal state not found")

type personalStateDocument struct {
	Version int                                              `json:"version"`
	Users   map[string]map[string]map[string]json.RawMessage `json:"users"`
}

type personalStateStore struct {
	mu   sync.RWMutex
	path string
	data personalStateDocument
}

func newPersonalStateStore(path string) (*personalStateStore, error) {
	store := &personalStateStore{
		path: path,
		data: personalStateDocument{
			Version: personalStateDocumentVersion,
			Users:   make(map[string]map[string]map[string]json.RawMessage),
		},
	}

	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading personal state file: %w", err)
	}
	if len(contents) == 0 {
		return nil, errors.New("personal state file is empty")
	}
	if err := json.Unmarshal(contents, &store.data); err != nil {
		return nil, fmt.Errorf("decoding personal state file: %w", err)
	}
	if store.data.Version != personalStateDocumentVersion {
		return nil, fmt.Errorf("unsupported personal state version %d", store.data.Version)
	}
	if store.data.Users == nil {
		store.data.Users = make(map[string]map[string]map[string]json.RawMessage)
	}
	return store, nil
}

func personalStateIdentityKey(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func clonePersonalStateDocument(source personalStateDocument) (personalStateDocument, error) {
	encoded, err := json.Marshal(source)
	if err != nil {
		return personalStateDocument{}, err
	}
	var cloned personalStateDocument
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return personalStateDocument{}, err
	}
	return cloned, nil
}

func (s *personalStateStore) get(identity, namespace, id string) (json.RawMessage, error) {
	if s == nil {
		return nil, errPersonalStateNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	userState := s.data.Users[personalStateIdentityKey(identity)]
	if userState == nil || userState[namespace] == nil {
		return nil, errPersonalStateNotFound
	}
	value, exists := userState[namespace][id]
	if !exists {
		return nil, errPersonalStateNotFound
	}
	return append(json.RawMessage(nil), value...), nil
}

func (s *personalStateStore) put(identity, namespace, id string, value json.RawMessage) error {
	if s == nil {
		return errors.New("personal state store is unavailable")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next, err := clonePersonalStateDocument(s.data)
	if err != nil {
		return fmt.Errorf("cloning personal state: %w", err)
	}
	identityKey := personalStateIdentityKey(identity)
	if next.Users[identityKey] == nil {
		next.Users[identityKey] = make(map[string]map[string]json.RawMessage)
	}
	if next.Users[identityKey][namespace] == nil {
		next.Users[identityKey][namespace] = make(map[string]json.RawMessage)
	}
	next.Users[identityKey][namespace][id] = append(json.RawMessage(nil), value...)

	if err := writePersonalStateDocumentAtomic(s.path, next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func writePersonalStateDocumentAtomic(path string, document personalStateDocument) error {
	contents, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding personal state: %w", err)
	}
	contents = append(contents, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating personal state directory: %w", err)
	}

	temporary, err := os.CreateTemp(dir, ".personal-state-*")
	if err != nil {
		return fmt.Errorf("creating personal state temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}

	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("setting personal state permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		cleanup()
		return fmt.Errorf("writing personal state temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("syncing personal state temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("closing personal state temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("replacing personal state file: %w", err)
	}
	return nil
}

func validatePersonalStateKey(namespace, id string) error {
	if namespace != "todo" && namespace != "timer" {
		return errors.New("unsupported personal state namespace")
	}
	if id == "" || len(id) > personalStateMaxIDLength {
		return errors.New("invalid personal state id")
	}
	if strings.ContainsAny(id, "/\\") {
		return errors.New("invalid personal state id")
	}
	return nil
}

func (a *application) personalStateSession(w http.ResponseWriter, r *http.Request) (authenticatedSession, bool) {
	if a.personalState == nil || !a.Config.Server.PersonalState.Enabled {
		w.WriteHeader(http.StatusNotFound)
		return authenticatedSession{}, false
	}

	session, authorized := a.authorizeSession(w, r)
	if !authorized || session.AuthorizationIdentity == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
		return authenticatedSession{}, false
	}
	return session, true
}

func (a *application) handlePersonalStateGetRequest(w http.ResponseWriter, r *http.Request) {
	session, ok := a.personalStateSession(w, r)
	if !ok {
		return
	}

	namespace := r.PathValue("namespace")
	id := r.PathValue("id")
	if err := validatePersonalStateKey(namespace, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	value, err := a.personalState.get(session.AuthorizationIdentity, namespace, id)
	if errors.Is(err, errPersonalStateNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternalServerError(w, "Failed to read personal state", err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(value)
}

func (a *application) handlePersonalStatePostRequest(w http.ResponseWriter, r *http.Request) {
	session, ok := a.personalStateSession(w, r)
	if !ok {
		return
	}

	namespace := r.PathValue("namespace")
	id := r.PathValue("id")
	if err := validatePersonalStateKey(namespace, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, personalStateMaxPayloadBytes))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "personal state payload is too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "could not read personal state payload", http.StatusBadRequest)
		}
		return
	}
	if len(body) == 0 || !json.Valid(body) {
		http.Error(w, "personal state payload must be valid JSON", http.StatusBadRequest)
		return
	}

	if err := a.personalState.put(session.AuthorizationIdentity, namespace, id, json.RawMessage(body)); err != nil {
		slog.Error("Personal state write failed", "namespace", namespace, "id", id, "error", err)
		writeInternalServerError(w, "Failed to persist personal state", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
