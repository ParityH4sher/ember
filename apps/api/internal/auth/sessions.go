package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const (
	maxSessions = 10000
	maxPending  = 1000
	pendingTTL  = 10 * time.Minute
)

// User is the signed-in person, taken from validated ID token claims.
type User struct {
	Subject string   `json:"subject"`
	Name    string   `json:"name,omitempty"`
	Email   string   `json:"email,omitempty"`
	Groups  []string `json:"groups,omitempty"`
}

type session struct {
	user    User
	role    Role   // decided at sign-in from the ID token's groups
	idToken string // kept only as id_token_hint for provider sign-out
	expires time.Time
}

// pending is one sign-in attempt between the redirect to the provider and the callback.
type pending struct {
	nonce    string
	verifier string
	returnTo string
	expires  time.Time
}

// errTooManySessions means the in-memory store is full.
var errTooManySessions = errors.New("too many active sessions")

// sessionStore keeps sessions and pending sign-in attempts. Any other error
// than errTooManySessions means the store could not be reached or answered
// unexpectedly; callers must then neither sign anyone in nor out (fail closed).
// Lookups of unknown or expired entries return ok == false and a nil error.
type sessionStore interface {
	addSession(ctx context.Context, id string, v session) error
	session(ctx context.Context, id string) (session, bool, error)
	// removeSession deletes a session and returns it if it was still valid.
	removeSession(ctx context.Context, id string) (session, bool, error)
	addPending(ctx context.Context, state string, v pending) error
	// takePending returns and removes a sign-in attempt; each state is single-use.
	takePending(ctx context.Context, state string) (pending, bool, error)
}

// memoryStore keeps sessions in process memory, the default. Restarting the
// API signs everyone out, which matches the prototype's other process-local
// state. Its methods never fail except when it is full.
type memoryStore struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[string]session
	pending  map[string]pending
}

func newMemoryStore(now func() time.Time) *memoryStore {
	return &memoryStore{now: now, sessions: map[string]session{}, pending: map[string]pending{}}
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *memoryStore) sweep() {
	now := s.now()
	for id, v := range s.sessions {
		if !now.Before(v.expires) {
			delete(s.sessions, id)
		}
	}
	for id, v := range s.pending {
		if !now.Before(v.expires) {
			delete(s.pending, id)
		}
	}
}

func (s *memoryStore) addSession(_ context.Context, id string, v session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= maxSessions {
		s.sweep()
		if len(s.sessions) >= maxSessions {
			return errTooManySessions
		}
	}
	s.sessions[id] = v
	return nil
}

func (s *memoryStore) session(_ context.Context, id string) (session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if ok && !s.now().Before(v.expires) {
		delete(s.sessions, id)
		return session{}, false, nil
	}
	return v, ok, nil
}

func (s *memoryStore) removeSession(_ context.Context, id string) (session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	delete(s.sessions, id)
	return v, ok && s.now().Before(v.expires), nil
}

// addPending records a sign-in attempt. When full it evicts the oldest
// attempt, so unauthenticated requests cannot block new sign-ins.
func (s *memoryStore) addPending(_ context.Context, state string, v pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) >= maxPending {
		s.sweep()
	}
	for len(s.pending) >= maxPending {
		oldest := ""
		for id, p := range s.pending {
			if oldest == "" || p.expires.Before(s.pending[oldest].expires) {
				oldest = id
			}
		}
		delete(s.pending, oldest)
	}
	s.pending[state] = v
	return nil
}

func (s *memoryStore) takePending(_ context.Context, state string) (pending, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.pending[state]
	delete(s.pending, state)
	return v, ok && s.now().Before(v.expires), nil
}
