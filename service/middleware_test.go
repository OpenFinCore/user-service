package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"ofc-user-service/models"
)

var guardNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// newGuardedHandler returns AuthService.Common around a handler that
// records whether it was reached and the session id it saw.
func newGuardedHandler(t *testing.T) (http.Handler, *miniredis.Miniredis, *bool, *string) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	auth := NewAuthService(client, 5*time.Minute)
	auth.now = func() time.Time { return guardNow }

	reached, session := false, ""
	h := auth.Common(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached, session = true, AppContext(r).Session()
		WriteSuccess(w, http.StatusOK, nil)
	}))
	return h, mr, &reached, &session
}

func send(h http.Handler, requestID, timestamp string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/anything", nil)
	if requestID != "" {
		req.Header.Set(HeaderRequestID, requestID)
	}
	if timestamp != "" {
		req.Header.Set(HeaderTimestamp, timestamp)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func envelope(t *testing.T, rec *httptest.ResponseRecorder) Response {
	t.Helper()
	var body Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad envelope %q: %v", rec.Body.String(), err)
	}
	return body
}

func ts(d time.Duration) string { return strconv.FormatInt(guardNow.Add(d).Unix(), 10) }

func TestRequestGuard_AcceptsValidRequest(t *testing.T) {
	h, mr, reached, session := newGuardedHandler(t)

	rec := send(h, "req-1", ts(0))

	if rec.Code != http.StatusOK || !*reached {
		t.Fatalf("want request to pass, got %d %s", rec.Code, rec.Body)
	}
	if *session != "req-1" || rec.Header().Get(HeaderRequestID) != "req-1" {
		t.Fatal("request id must be echoed and used as the session id")
	}
	key := "reqguard:{req-1}:" + ts(0)
	if !mr.Exists(key) || mr.TTL(key) != 10*time.Minute {
		t.Fatalf("want pair claimed for 2 x window, ttl=%v", mr.TTL(key))
	}
}

func TestRequestGuard_RejectsMissingOrInvalidHeaders(t *testing.T) {
	cases := []struct {
		name, id, ts string
		want         models.ResponseCode
	}{
		{"missing request id", "", ts(0), models.CodeRequestIDHeaderRequired},
		{"invalid request id", "has space", ts(0), models.CodeRequestIDHeaderInvalid},
		{"too long request id", string(make([]byte, 65)), ts(0), models.CodeRequestIDHeaderInvalid},
		{"missing timestamp", "req-1", "", models.CodeTimestampHeaderRequired},
		{"non-numeric timestamp", "req-1", "yesterday", models.CodeTimestampHeaderInvalid},
		{"negative timestamp", "req-1", "-5", models.CodeTimestampHeaderInvalid},
		{"too old", "req-1", ts(-6 * time.Minute), models.CodeTimestampOutOfWindow},
		{"too far in future", "req-1", ts(6 * time.Minute), models.CodeTimestampOutOfWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, reached, _ := newGuardedHandler(t)
			rec := send(h, tc.id, tc.ts)
			body := envelope(t, rec)
			if rec.Code != http.StatusBadRequest || body.Code != tc.want.Code || body.Message != tc.want.Message || body.Data != nil {
				t.Fatalf("want 400 code %d, got %d %+v", tc.want.Code, rec.Code, body)
			}
			if *reached {
				t.Fatal("handler must not run")
			}
		})
	}
}

func TestRequestGuard_RejectsDuplicateWithinWindow(t *testing.T) {
	h, mr, _, _ := newGuardedHandler(t)

	if rec := send(h, "req-1", ts(0)); rec.Code != http.StatusOK {
		t.Fatalf("first request: %d", rec.Code)
	}
	mr.FastForward(4 * time.Minute)

	rec := send(h, "req-1", ts(0))
	body := envelope(t, rec)
	if rec.Code != http.StatusConflict || body.Code != models.CodeDuplicateRequest.Code {
		t.Fatalf("want 409 code %d, got %d %+v", models.CodeDuplicateRequest.Code, rec.Code, body)
	}
	if rec.Header().Get(HeaderRequestID) != "req-1" {
		t.Fatal("rejections must still echo the request id")
	}
}

func TestRequestGuard_SameIDDifferentTimestampIsNotDuplicate(t *testing.T) {
	h, _, _, _ := newGuardedHandler(t)
	send(h, "req-1", ts(0))
	if rec := send(h, "req-1", ts(time.Second)); rec.Code != http.StatusOK {
		t.Fatalf("different timestamp must pass, got %d", rec.Code)
	}
	if rec := send(h, "req-2", ts(0)); rec.Code != http.StatusOK {
		t.Fatalf("different request id must pass, got %d", rec.Code)
	}
}

func TestRequestGuard_FailsClosedWhenRedisIsDown(t *testing.T) {
	h, mr, reached, _ := newGuardedHandler(t)
	mr.Close()

	rec := send(h, "req-1", ts(0))
	body := envelope(t, rec)
	if rec.Code != http.StatusServiceUnavailable || body.Code != models.CodeServiceUnavailable.Code || *reached {
		t.Fatalf("want 503 code %d, got %d %+v", models.CodeServiceUnavailable.Code, rec.Code, body)
	}
}

func TestRecover_PanicBecomesInternalEnvelope(t *testing.T) {
	mr := miniredis.RunT(t)
	auth := NewAuthService(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 5*time.Minute)
	auth.now = func() time.Time { return guardNow }
	h := auth.Common(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	rec := send(h, "req-1", ts(0))
	if body := envelope(t, rec); rec.Code != http.StatusInternalServerError || body.Code != models.CodeInternal.Code {
		t.Fatalf("want 500 code %d, got %d %+v", models.CodeInternal.Code, rec.Code, body)
	}
}
