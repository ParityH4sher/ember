package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

// RedisConfig selects the Redis session store. Load it with LoadRedisConfig.
type RedisConfig struct {
	Addr     string
	password string
}

// LoadRedisConfig reads EMBER_REDIS_ADDR (host:port) and the optional
// EMBER_REDIS_PASSWORD_FILE. Without an address it returns nil, and sessions
// stay in memory.
func LoadRedisConfig(addr, passwordFile string) (*RedisConfig, error) {
	if addr == "" {
		if passwordFile != "" {
			return nil, errors.New("EMBER_REDIS_PASSWORD_FILE is set but EMBER_REDIS_ADDR is not")
		}
		return nil, nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" || strings.ContainsAny(host, "/ ") {
		return nil, errors.New("EMBER_REDIS_ADDR must be host:port, for example redis:6379")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil, errors.New("EMBER_REDIS_ADDR must name a port from 1 through 65535")
	}
	c := &RedisConfig{Addr: addr}
	if passwordFile != "" {
		data, err := readSmallFile(passwordFile, maxSecretBytes)
		if err != nil {
			return nil, fmt.Errorf("read Redis password: %w", err)
		}
		if c.password = strings.TrimSpace(string(data)); c.password == "" {
			return nil, errors.New("Redis password file is empty")
		}
	}
	return c, nil
}

const (
	redisSessionPrefix = "ember:session:"
	redisPendingPrefix = "ember:pending:"
)

// redisStore keeps sessions and sign-in attempts in Redis, so they survive an
// API restart and can be shared by several API replicas. Keys hold a SHA-256
// of the cookie value or state, so a copy of the data cannot be replayed as a
// cookie. Redis expires each entry at the end of its lifetime.
type redisStore struct {
	client *redisClient
	now    func() time.Time
	logger *slog.Logger
}

type redisSession struct {
	User    User      `json:"user"`
	Role    string    `json:"role"`
	IDToken string    `json:"idToken"`
	Expires time.Time `json:"expires"`
}

type redisPending struct {
	Nonce    string    `json:"nonce"`
	Verifier string    `json:"verifier"`
	ReturnTo string    `json:"returnTo"`
	Expires  time.Time `json:"expires"`
}

func redisKey(prefix, secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return prefix + hex.EncodeToString(sum[:])
}

func (s *redisStore) put(ctx context.Context, key string, value any, expires time.Time) error {
	ttl := expires.Sub(s.now())
	if ttl <= 0 {
		return nil // already expired; nothing to keep
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.client.set(ctx, key, string(raw), ttl)
}

func (s *redisStore) addSession(ctx context.Context, id string, v session) error {
	return s.put(ctx, redisKey(redisSessionPrefix, id), redisSession{
		User: v.user, Role: v.role.String(), IDToken: v.idToken, Expires: v.expires,
	}, v.expires)
}

func (s *redisStore) session(ctx context.Context, id string) (session, bool, error) {
	raw, ok, err := s.client.get(ctx, redisKey(redisSessionPrefix, id))
	if err != nil || !ok {
		return session{}, false, err
	}
	return s.decodeSession(raw)
}

func (s *redisStore) removeSession(ctx context.Context, id string) (session, bool, error) {
	raw, ok, err := s.client.getDel(ctx, redisKey(redisSessionPrefix, id))
	if err != nil || !ok {
		return session{}, false, err
	}
	return s.decodeSession(raw)
}

// decodeSession treats a record it cannot read as no session: it never signs
// anyone in on data it does not understand.
func (s *redisStore) decodeSession(raw string) (session, bool, error) {
	var v redisSession
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		s.logger.Warn("ignoring unreadable session record in Redis", "error", err)
		return session{}, false, nil
	}
	var role Role
	switch v.Role {
	case RoleUser.String():
		role = RoleUser
	case RoleAdmin.String():
		role = RoleAdmin
	default:
		s.logger.Warn("ignoring session record with an unknown role in Redis")
		return session{}, false, nil
	}
	if v.User.Subject == "" || !s.now().Before(v.Expires) {
		return session{}, false, nil
	}
	return session{user: v.User, role: role, idToken: v.IDToken, expires: v.Expires}, true, nil
}

func (s *redisStore) addPending(ctx context.Context, state string, v pending) error {
	return s.put(ctx, redisKey(redisPendingPrefix, state), redisPending{
		Nonce: v.nonce, Verifier: v.verifier, ReturnTo: v.returnTo, Expires: v.expires,
	}, v.expires)
}

func (s *redisStore) takePending(ctx context.Context, state string) (pending, bool, error) {
	raw, ok, err := s.client.getDel(ctx, redisKey(redisPendingPrefix, state))
	if err != nil || !ok {
		return pending{}, false, err
	}
	var v redisPending
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		s.logger.Warn("ignoring unreadable sign-in attempt in Redis", "error", err)
		return pending{}, false, nil
	}
	if !s.now().Before(v.Expires) {
		return pending{}, false, nil
	}
	return pending{nonce: v.Nonce, verifier: v.Verifier, returnTo: safeReturnTo(v.ReturnTo), expires: v.Expires}, true, nil
}
