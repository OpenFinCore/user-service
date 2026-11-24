package service

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"ofc-user-service/models"
)

// ---- rate limiting -------------------------------------------------------

func newLimited(t *testing.T, limit RateLimit, key RateLimitKey) (http.Handler, *miniredis.Miniredis, *int) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	auth := NewAuthService(client, time.Minute).WithRateLimits(map[string]RateLimit{"test:key": limit})
	reached := 0
	h := auth.RateLimit("test:key", key)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		WriteSuccess(w, http.StatusOK, nil)
	}))
	return h, mr, &reached
}

func hit(h http.Handler, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimit_BlocksOverLimitWithRetryAfter(t *testing.T) {
	h, mr, reached := newLimited(t, RateLimit{Limit: 3, Window: time.Minute}, ByClientIP)

	for i := 0; i < 3; i++ {
		if rec := hit(h, "10.0.0.1:1234"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, rec.Code)
		}
	}
	rec := hit(h, "10.0.0.1:1234")
	if body := envelope(t, rec); rec.Code != http.StatusTooManyRequests || body.Code != models.CodeTooManyRequests.Code {
		t.Fatalf("want 429 code %d, got %d %+v", models.CodeTooManyRequests.Code, rec.Code, body)
	}
	if rec.Header().Get("Retry-After") != "60" || *reached != 3 {
		t.Fatalf("want Retry-After 60 and 3 handled, got %q / %d", rec.Header().Get("Retry-After"), *reached)
	}

	// Keys are independent, and the window resets.
	if rec := hit(h, "10.0.0.2:1234"); rec.Code != http.StatusOK {
		t.Fatalf("other IP must not be limited, got %d", rec.Code)
	}
	mr.FastForward(time.Minute + time.Second)
	if rec := hit(h, "10.0.0.1:1234"); rec.Code != http.StatusOK {
		t.Fatalf("window must reset, got %d", rec.Code)
	}
}

func TestRateLimit_FailsClosedWhenRedisIsDown(t *testing.T) {
	h, mr, reached := newLimited(t, RateLimit{Limit: 3, Window: time.Minute}, ByClientIP)
	mr.Close()
	if rec := hit(h, "10.0.0.1:1234"); rec.Code != http.StatusServiceUnavailable || *reached != 0 {
		t.Fatalf("want 503 and handler not reached, got %d / %d", rec.Code, *reached)
	}
}

func TestRateLimit_MissingKeyIsAnInternalError(t *testing.T) {
	// ByDevice without RequireTenant/RequireDevice in front: a wiring bug
	// must not turn into an unlimited route.
	h, _, reached := newLimited(t, RateLimit{Limit: 3, Window: time.Minute}, ByDevice)
	if rec := hit(h, "10.0.0.1:1234"); rec.Code != http.StatusInternalServerError || *reached != 0 {
		t.Fatalf("want 500 and handler not reached, got %d / %d", rec.Code, *reached)
	}
}

func TestRateLimit_UnknownNamePanicsAtWiring(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for an undefined rate limit")
		}
	}()
	NewAuthService(nil, time.Minute).RateLimit("does-not-exist", ByClientIP)
}

// ---- client IP -----------------------------------------------------------

func TestResolveClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.168.1.5/32")}
	cases := []struct {
		name, remote, xff, want string
	}{
		{"untrusted peer ignores XFF", "203.0.113.9:5000", "1.2.3.4", "203.0.113.9"},
		{"trusted proxy uses last hop", "10.1.2.3:5000", "198.51.100.7", "198.51.100.7"},
		{"spoofed left entries are ignored", "10.1.2.3:5000", "6.6.6.6, 198.51.100.7", "198.51.100.7"},
		{"chain of trusted proxies", "10.1.2.3:5000", "198.51.100.7, 192.168.1.5, 10.9.9.9", "198.51.100.7"},
		{"trusted proxy without XFF", "10.1.2.3:5000", "", "10.1.2.3"},
		{"malformed hop stops the walk", "10.1.2.3:5000", "198.51.100.7, garbage", "10.1.2.3"},
		{"ipv6 peer", "[2001:db8::1]:5000", "", "2001:db8::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remote
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := resolveClientIP(req, trusted); got != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestClientIP_UsesValueResolvedByCommon(t *testing.T) {
	var got string
	h := clientIPMiddleware([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.7" {
		t.Fatalf("got %q", got)
	}
}

// ---- recover -------------------------------------------------------------

func TestRecover_PanicAfterResponseStartedAbortsInsteadOfWritingTwice(t *testing.T) {
	h := recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("boom")
	}))
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Fatalf("want http.ErrAbortHandler, got %v", p)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// ---- tenant cache --------------------------------------------------------

func TestTTLCache_ExpiresAndStaysBounded(t *testing.T) {
	now := time.Unix(0, 0)
	c := newTTLCache[int, string](3)
	c.now = func() time.Time { return now }

	c.Set(1, "a", time.Second)
	if v, ok := c.Get(1); !ok || v != "a" {
		t.Fatal("want hit")
	}
	now = now.Add(2 * time.Second)
	if _, ok := c.Get(1); ok {
		t.Fatal("want expiry")
	}

	for i := 0; i < 100; i++ {
		c.Set(i, "x", time.Hour)
	}
	if len(c.entries) > 3 {
		t.Fatalf("cache grew past its bound: %d", len(c.entries))
	}
}

func TestServiceProviderService_CachesActiveLookups(t *testing.T) {
	s := &ServiceProviderService{active: newTTLCache[uuid.UUID, *models.ServiceProvider](10)}
	id := uuid.New()
	s.active.Set(id, &models.ServiceProvider{ID: id, Name: "cached"}, time.Minute)

	sp, err := s.GetActiveByID(bg(), id) // no store: must be served from cache
	if err != nil || sp == nil || sp.Name != "cached" {
		t.Fatalf("want cached tenant, got %+v %v", sp, err)
	}
	sp.Name = "mutated"
	again, _ := s.GetActiveByID(bg(), id)
	if again.Name != "cached" {
		t.Fatal("callers must get a copy, not the cached value")
	}

	missing := uuid.New()
	s.active.Set(missing, nil, time.Minute)
	if sp, err := s.GetActiveByID(bg(), missing); sp != nil || err != nil {
		t.Fatalf("want cached not-found, got %+v %v", sp, err)
	}
}
