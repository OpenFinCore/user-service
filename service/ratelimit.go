package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"ofc-user-service/models"
)

// RateLimit allows Limit requests per Window for one key.
type RateLimit struct {
	Limit  int
	Window time.Duration
}

// DefaultRateLimits are the named limits routes can use. Each can be
// overridden in config.json ("rateLimits": {"<name>": {"limit": .., "windowSeconds": ..}}).
// Names are "<route>:<key>", e.g. "register-request:ip".
//
// Per-IP limits must stay generous: mobile carriers put many users behind
// one address (CGNAT). The per-device and per-phone limits are the tight
// ones.
var DefaultRateLimits = map[string]RateLimit{
	"register-request:ip":     {Limit: 60, Window: time.Hour},
	"register-request:device": {Limit: 10, Window: time.Hour},
}

// RateLimitKey extracts what a limit counts by from a request. An empty key
// means the middleware it depends on hasn't run - a wiring bug, answered
// with an internal error rather than an unlimited request.
type RateLimitKey func(r *http.Request) string

// ByClientIP counts per client IP (see ClientIP).
func ByClientIP(r *http.Request) string { return ClientIP(r) }

// ByDevice counts per tenant + device id. The route must use RequireTenant
// and RequireDevice before this limit.
func ByDevice(r *http.Request) string {
	ctx := AppContext(r)
	sp, device := ctx.ServiceProviderID(), ctx.DeviceMetaData()
	if sp == nil || device == nil {
		return ""
	}
	return sp.String() + ":" + device.DeviceID
}

// rateLimitScript is a fixed-window counter: INCR, and start the window's
// expiry on its first hit. Returns {count, ttl_ms}.
var rateLimitScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {n, ttl}
`)

// rateLimitMiddleware rejects requests over limit with 429 + Retry-After.
// Like the request guard it fails closed: if Redis is unreachable the
// request is refused rather than let through unlimited.
func rateLimitMiddleware(rdb *redis.Client, name string, limit RateLimit, key RateLimitKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := key(r)
			if k == "" {
				WriteError(w, r, fmt.Errorf("rate limit %q: no key for request (middleware order?)", name))
				return
			}
			res, err := rateLimitScript.Run(r.Context(), rdb, []string{"rl:{" + name + "}:" + k}, limit.Window.Milliseconds()).Int64Slice()
			if err != nil {
				WriteError(w, r, models.NewAppError(models.CodeServiceUnavailable).WithCause(fmt.Errorf("rate limit %q: %w", name, err)))
				return
			}
			if res[0] > int64(limit.Limit) {
				WriteError(w, r, models.NewAppError(models.CodeTooManyRequests).WithRetryAfter(roundUpToSecond(time.Duration(res[1])*time.Millisecond)))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
