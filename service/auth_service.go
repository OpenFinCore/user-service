package service

import (
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// AuthService decides how requests are authenticated and scoped. The
// router calls it to put the middleware a route needs in front of that
// route; the middleware logic itself lives in middleware.go.
type AuthService struct {
	redis        *redis.Client
	replayWindow time.Duration
	now          func() time.Time
}

// NewAuthService builds the service. replayWindow is both the allowed clock
// skew of X-Timestamp and how long a (request id, timestamp) pair stays
// claimed in Redis (2 x replayWindow).
func NewAuthService(rdb *redis.Client, replayWindow time.Duration) *AuthService {
	return &AuthService{redis: rdb, replayWindow: replayWindow, now: time.Now}
}

// Common wraps every API: panic recovery, an access-log line, and the
// request guard (mandatory X-Request-ID + X-Timestamp, duplicate rejection).
func (s *AuthService) Common(next http.Handler) http.Handler {
	return recoverMiddleware(accessLogMiddleware(requestGuardMiddleware(s.redis, s.replayWindow, s.now)(next)))
}

// RequireTenant scopes a route to the tenant in X-Service-Provider-ID and
// rejects requests without a valid one.
func (s *AuthService) RequireTenant(next http.Handler) http.Handler {
	return tenantMiddleware(next)
}

// NotFound answers routes that don't exist, in the standard envelope.
func (s *AuthService) NotFound() http.Handler {
	return http.HandlerFunc(routeNotFound)
}
