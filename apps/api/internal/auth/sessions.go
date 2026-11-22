package auth

import (
	"crypto/rand"
	"encoding/base64"
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

// store keeps sessions in process memory. Restarting the API signs everyone
// out, which matches the prototype's other process-local state.
type store struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[string]session
	pending  map[string]pending
}

func newStore(now func() time.Time) *store {
	return &store{now: now, sessions: map[string]session{}, pending: map[string]pending{}}
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *store) sweep() {
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

func (s *store) addSession(id string, v session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= maxSessions {
		s.sweep()
		if len(s.sessions) >= maxSessions {
			return false
		}
	}
	s.sessions[id] = v
	return true
}

func (s *store) session(id string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if ok && !s.now().Before(v.expires) {
		delete(s.sessions, id)
		return session{}, false
	}
	return v, ok
}

func (s *store) removeSession(id string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	delete(s.sessions, id)
	return v, ok && s.now().Before(v.expires)
}

// addPending records a sign-in attempt. When full it evicts the oldest
// attempt, so unauthenticated requests cannot block new sign-ins.
func (s *store) addPending(state string, v pending) {
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
}

// takePending returns and removes a sign-in attempt; each state is single-use.
func (s *store) takePending(state string) (pending, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.pending[state]
	delete(s.pending, state)
	return v, ok && s.now().Before(v.expires)
}
