package emby

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// IdentityStore holds only server-scoped token digests, never raw credentials.
// Bindings are hints from authenticated upstream responses, not authorization:
// the user policy and item/playback access are rechecked with the caller token.
type IdentityStore interface {
	Lookup(context.Context, string) (string, error)
	Remember(context.Context, string, string) error
	Forget(context.Context, string) error
}

var ErrIdentityUnknown = errors.New("Emby caller identity unavailable; log in through gateway")

const identityTTL = 30 * 24 * time.Hour
const identityPrefix = "mediatool:emby:identity:v1:"

func IdentityKey(upstream *url.URL, token string) string {
	scope := *upstream
	scope.RawQuery, scope.Fragment, scope.RawPath = "", "", ""
	scope.User = nil
	scope.Scheme = strings.ToLower(scope.Scheme)
	scope.Host = strings.ToLower(scope.Host)
	scope.Path = strings.TrimRight(scope.Path, "/")
	digest := sha256.Sum256([]byte(scope.String() + "\x00" + token))
	return identityPrefix + hex.EncodeToString(digest[:])
}

type RedisIdentityStore struct{ Redis *redis.Client }

func (s *RedisIdentityStore) Lookup(ctx context.Context, key string) (string, error) {
	id, err := s.Redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrIdentityUnknown
	}
	return id, err
}
func (s *RedisIdentityStore) Remember(ctx context.Context, key, id string) error {
	if !itemIDPattern.MatchString(id) {
		return ErrIdentityUnknown
	}
	return s.Redis.Set(ctx, key, id, identityTTL).Err()
}
func (s *RedisIdentityStore) Forget(ctx context.Context, key string) error {
	return s.Redis.Del(ctx, key).Err()
}

type identityEntry struct {
	id      string
	expires time.Time
}
type memoryIdentityStore struct {
	mu      sync.Mutex
	entries map[string]identityEntry
}

func NewMemoryIdentityStore() IdentityStore {
	return &memoryIdentityStore{entries: make(map[string]identityEntry)}
}
func (s *memoryIdentityStore) Lookup(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok || !time.Now().Before(entry.expires) {
		delete(s.entries, key)
		return "", ErrIdentityUnknown
	}
	return entry.id, nil
}
func (s *memoryIdentityStore) Remember(_ context.Context, key, id string) error {
	if !itemIDPattern.MatchString(id) {
		return ErrIdentityUnknown
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.entries {
		if !now.Before(e.expires) {
			delete(s.entries, k)
		}
	}
	// Standalone/test usage remains bounded. Eviction only causes safe fallback.
	if len(s.entries) >= 4096 {
		for k := range s.entries {
			delete(s.entries, k)
			break
		}
	}
	s.entries[key] = identityEntry{id: id, expires: now.Add(identityTTL)}
	return nil
}
func (s *memoryIdentityStore) Forget(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}

var authorizationTokenPattern = regexp.MustCompile(`(?i)Token="([^"]+)"`)

// Reject different credentials on one playback request so identity lookup and
// Emby authorization cannot interpret different tokens.
func unambiguousToken(r *http.Request) (string, error) {
	var tokens []string
	for key, values := range r.URL.Query() {
		if strings.EqualFold(key, "api_key") {
			tokens = append(tokens, values...)
		}
	}
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		tokens = append(tokens, r.Header.Values(header)...)
	}
	for _, header := range []string{"Authorization", "X-Emby-Authorization"} {
		for _, value := range r.Header.Values(header) {
			if strings.HasPrefix(value, "Bearer ") {
				tokens = append(tokens, strings.TrimPrefix(value, "Bearer "))
			}
			for _, match := range authorizationTokenPattern.FindAllStringSubmatch(value, -1) {
				tokens = append(tokens, match[1])
			}
		}
	}
	var token string
	for _, candidate := range tokens {
		if candidate == "" {
			continue
		}
		if token != "" && token != candidate {
			return "", ErrIdentityUnknown
		}
		token = candidate
	}
	return token, nil
}
