package service

import (
	"fmt"
	"maps"
	"net/http"
	"net/netip"
	"time"

	"github.com/redis/go-redis/v9"
)

// AuthService decides how requests are authenticated and scoped. The
// router calls it to put the middleware a route needs in front of that
// route; the middleware logic itself lives in middleware.go.
type AuthService struct {
	redis          *redis.Client
	replayWindow   time.Duration
	now            func() time.Time
	trustedProxies []netip.Prefix
	rateLimits     map[string]RateLimit
}

// NewAuthService builds the service. replayWindow is both the allowed clock
// skew of X-Timestamp and how long a (request id, timestamp) pair stays
// claimed in Redis (2 x replayWindow). Rate limits start from
// DefaultRateLimits.
func NewAuthService(rdb *redis.Client, replayWindow time.Duration) *AuthService {
	return &AuthService{redis: rdb, replayWindow: replayWindow, now: time.Now, rateLimits: maps.Clone(DefaultRateLimits)}
}

// WithTrustedProxies sets the load balancers whose X-Forwarded-For header
// is honoured when resolving the client IP.
func (s *AuthService) WithTrustedProxies(proxies []netip.Prefix) *AuthService {
	s.trustedProxies = proxies
	return s
}

// WithRateLimits overrides (or adds) named rate limits.
func (s *AuthService) WithRateLimits(limits map[string]RateLimit) *AuthService {
	maps.Copy(s.rateLimits, limits)
	return s
}

// Common wraps every API: panic recovery, an access-log line, client IP
// resolution and the request guard (mandatory X-Request-ID + X-Timestamp,
// duplicate rejection).
func (s *AuthService) Common(next http.Handler) http.Handler {
	return recoverMiddleware(accessLogMiddleware(clientIPMiddleware(s.trustedProxies)(requestGuardMiddleware(s.redis, s.replayWindow, s.now)(next))))
}

// RequireTenant scopes a route to the tenant in X-Service-Provider-ID and
// rejects requests without a valid one.
func (s *AuthService) RequireTenant(next http.Handler) http.Handler {
	return tenantMiddleware(next)
}

// RequireDevice rejects requests without valid X-Device-* headers and puts
// the device on the request's c.Context (DeviceMetaData).
func (s *AuthService) RequireDevice(next http.Handler) http.Handler {
	return deviceMiddleware(next)
}

// RateLimit returns route middleware enforcing the named limit (see
// DefaultRateLimits), counted per key. It panics for an unknown name: that
// is a route wiring bug, caught when the router is built at startup.
func (s *AuthService) RateLimit(name string, key RateLimitKey) func(http.Handler) http.Handler {
	limit, ok := s.rateLimits[name]
	if !ok {
		panic(fmt.Sprintf("rate limit %q is not defined (see service.DefaultRateLimits)", name))
	}
	return rateLimitMiddleware(s.redis, name, limit, key)
}

// NotFound answers routes that don't exist, in the standard envelope.
func (s *AuthService) NotFound() http.Handler {
	return http.HandlerFunc(routeNotFound)
}
