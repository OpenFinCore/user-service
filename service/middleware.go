package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	c "ofc-user-service/context"
	"ofc-user-service/logger"
	"ofc-user-service/models"
)

// HTTP middleware logic. Routes don't use these directly: AuthService
// exposes them, and the router asks AuthService for the middleware each
// route needs.

const (
	HeaderRequestID       = "X-Request-ID"
	HeaderTimestamp       = "X-Timestamp"
	HeaderServiceProvider = "X-Service-Provider-ID"
	HeaderDeviceID        = "X-Device-ID"
	HeaderDeviceName      = "X-Device-Name"
	HeaderDeviceType      = "X-Device-Type"
	HeaderOSName          = "X-OS-Name"
	HeaderOSVersion       = "X-OS-Version"
	HeaderAppVersion      = "X-App-Version"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyAppCtx
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestGuardMiddleware makes X-Request-ID and X-Timestamp mandatory on
// every API and rejects replays:
//
//   - X-Request-ID: 1-64 chars of [A-Za-z0-9._-]; it is echoed back and
//     becomes the c.Context session id, so logs can be traced per request.
//   - X-Timestamp: Unix time in seconds, within +/- window of server time.
//   - The pair (request id, timestamp) is claimed in Redis with SET NX for
//     2 x window. A second request with the same pair inside that time is
//     rejected as a duplicate. Requests outside the window are rejected on
//     their timestamp, so a pair can never be replayed after its key expires.
//
// If Redis is unreachable the request is rejected (fail closed): replay
// protection must not silently switch off.
func requestGuardMiddleware(rdb *redis.Client, window time.Duration, now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			switch {
			case id == "":
				WriteError(w, r, models.NewAppError(models.CodeRequestIDHeaderRequired))
				return
			case !validRequestID.MatchString(id):
				WriteError(w, r, models.NewAppError(models.CodeRequestIDHeaderInvalid))
				return
			}

			// The id is valid from here on: echo it and use it for tracing,
			// including on the rejections below.
			w.Header().Set(HeaderRequestID, id)
			ctx := context.WithValue(r.Context(), keyRequestID, id)
			ctx = context.WithValue(ctx, keyAppCtx, c.NewContext(r.Context()).WithSession(id))
			r = r.WithContext(ctx)

			rawTS := r.Header.Get(HeaderTimestamp)
			if rawTS == "" {
				WriteError(w, r, models.NewAppError(models.CodeTimestampHeaderRequired))
				return
			}
			ts, err := strconv.ParseInt(rawTS, 10, 64)
			if err != nil || ts <= 0 {
				WriteError(w, r, models.NewAppError(models.CodeTimestampHeaderInvalid))
				return
			}
			if skew := now().Sub(time.Unix(ts, 0)); skew > window || skew < -window {
				WriteError(w, r, models.NewAppError(models.CodeTimestampOutOfWindow))
				return
			}

			key := "reqguard:{" + id + "}:" + strconv.FormatInt(ts, 10)
			claimed, err := rdb.SetNX(r.Context(), key, now().Unix(), 2*window).Result()
			if err != nil {
				WriteError(w, r, models.NewAppError(models.CodeServiceUnavailable).WithCause(fmt.Errorf("request guard: %w", err)))
				return
			}
			if !claimed {
				WriteError(w, r, models.NewAppError(models.CodeDuplicateRequest))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// recoverMiddleware turns a panic into a logged 500.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				logger.LogError(RequestIDFrom(r), fmt.Sprintf("panic: %v\n%s", p, debug.Stack()))
				WriteError(w, r, fmt.Errorf("panic: %v", p))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// accessLogMiddleware logs one line per request (never bodies: they may
// hold secrets).
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// The request id is only trusted once the guard validated it, which
		// happens further in; log the header only when it is well formed.
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = "-"
		}
		logger.LogInfo(fmt.Sprintf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond)), id)
	})
}

// routeNotFound answers unknown routes in the standard envelope.
func routeNotFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, models.NewAppError(models.CodeRouteNotFound))
}

// tenantMiddleware requires a valid X-Service-Provider-ID and scopes the
// request's c.Context to it. Whether the tenant exists / is active is
// checked by the domain service handling the request.
func tenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get(HeaderServiceProvider)
		if raw == "" {
			WriteError(w, r, models.NewAppError(models.CodeServiceProviderHeaderRequired))
			return
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			WriteError(w, r, models.NewAppError(models.CodeServiceProviderHeaderInvalid))
			return
		}
		ctx := context.WithValue(r.Context(), keyAppCtx, AppContext(r).WithServiceProviderID(id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AppContext returns the c.Context the middleware built for this request.
func AppContext(r *http.Request) c.Context {
	if appCtx, ok := r.Context().Value(keyAppCtx).(c.Context); ok {
		return appCtx
	}
	return c.NewContext(r.Context())
}

// RequestIDFrom returns the request id assigned by the middleware.
func RequestIDFrom(r *http.Request) string {
	id, _ := r.Context().Value(keyRequestID).(string)
	return id
}

// ClientIP is the TCP peer address. X-Forwarded-For is deliberately not
// trusted: it is client-controlled unless a known proxy sets it.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// DeviceHeaders reads the X-Device-* headers describing the caller's
// device (trimmed; validate with DeviceFilters.ValidateDevice).
func DeviceHeaders(r *http.Request) models.DeviceFilters {
	get := func(name string) string { return strings.TrimSpace(r.Header.Get(name)) }
	return models.DeviceFilters{
		DeviceID:   get(HeaderDeviceID),
		DeviceName: get(HeaderDeviceName),
		DeviceType: get(HeaderDeviceType),
		OSName:     get(HeaderOSName),
		OSVersion:  get(HeaderOSVersion),
		AppVersion: get(HeaderAppVersion),
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
