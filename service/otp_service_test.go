package service

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	c "ofc-user-service/context"
	"ofc-user-service/models"
)

func newTestOTPService(t *testing.T) (*OTPService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewOTPService(client, "test-secret", OTPPolicy{
		Length: 6, TTL: 5 * time.Minute, ResendCooldown: time.Minute, MaxRequestsPerHour: 3, MaxVerifyAttempts: 5,
	}), mr
}

func bg() c.Context { return c.NewContext(context.Background()) }

// dev is the device the codes in these tests are held against.
var dev = uuid.New()

func TestOTPService_IssueStoresOnlyHash(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp := uuid.New()

	otp, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(otp.Code) {
		t.Fatalf("bad code %q", otp.Code)
	}

	key := s.keys(OTPPurposeRegistration, sp, "+923001234567", dev)[0]
	stored := mr.HGet(key, "code_hash")
	if stored == "" || stored == otp.Code || strings.Contains(stored, otp.Code) {
		t.Fatalf("code must be stored hashed, got %q", stored)
	}
	if stored != s.hash(OTPPurposeRegistration, sp, "+923001234567", dev, otp.Code) {
		t.Fatal("stored hash does not match the issued code")
	}
	if ttl := mr.TTL(key); ttl != 5*time.Minute {
		t.Fatalf("want 5m TTL, got %v", ttl)
	}
	if mr.HGet(key, "max_attempts") != "5" || mr.HGet(key, "attempts") != "0" {
		t.Fatal("attempt counters not initialised")
	}
}

func TestOTPService_Cooldown(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp := uuid.New()

	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(20 * time.Second)

	_, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev)
	appErr, ok := models.AsAppError(err)
	if !ok || appErr.Code != models.CodeOTPResendCooldown.Code || appErr.RetryAfter != 40*time.Second {
		t.Fatalf("want RATE_LIMITED retry 40s, got %v", err)
	}

	mr.FastForward(41 * time.Second)
	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatalf("cooldown elapsed, want success: %v", err)
	}
}

func TestOTPService_NewCodeReplacesOld(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp := uuid.New()
	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(61 * time.Second)
	second, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev)
	if err != nil {
		t.Fatal(err)
	}
	stored := mr.HGet(s.keys(OTPPurposeRegistration, sp, "+923001234567", dev)[0], "code_hash")
	if stored != s.hash(OTPPurposeRegistration, sp, "+923001234567", dev, second.Code) {
		t.Fatal("only the newest code may be stored")
	}
}

func TestOTPService_HourlyCap(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp := uuid.New()
	for i := 0; i < 3; i++ {
		if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
			t.Fatalf("issue %d: %v", i+1, err)
		}
		mr.FastForward(61 * time.Second)
	}
	_, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev)
	appErr, ok := models.AsAppError(err)
	if !ok || appErr.Code != models.CodeOTPHourlyLimit.Code {
		t.Fatalf("want hourly cap, got %v", err)
	}
	// 3 x 61s elapsed since the window opened.
	if want := time.Hour - 183*time.Second; appErr.RetryAfter != want {
		t.Fatalf("want retry after %v, got %v", want, appErr.RetryAfter)
	}
}

func TestOTPService_ScopedByPurposeTenantAndPhone(t *testing.T) {
	s, _ := newTestOTPService(t)
	sp := uuid.New()
	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234568", dev); err != nil {
		t.Fatal("another phone must not be rate limited:", err)
	}
	if _, err := s.Issue(bg(), OTPPurposeRegistration, uuid.New(), "+923001234567", dev); err != nil {
		t.Fatal("another tenant must not be rate limited:", err)
	}
}

func TestOTPService_RevokeReleasesLimits(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp := uuid.New()
	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal(err)
	}
	keys := s.keys(OTPPurposeRegistration, sp, "+923001234567", dev)
	if mr.Exists(keys[0]) || mr.Exists(keys[1]) {
		t.Fatal("code and cooldown must be removed")
	}
	if got, _ := mr.Get(keys[2]); got != "0" {
		t.Fatalf("hourly counter must be released, got %q", got)
	}
	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal("must be able to retry immediately after a revoke:", err)
	}
}

func TestOTPService_ConcurrentIssueOnlyOneWins(t *testing.T) {
	s, _ := newTestOTPService(t)
	sp := uuid.New()

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("want exactly 1 successful issue, got %d", ok)
	}
}

func TestOTPService_CodeHeldPerDeviceLimitsPerPhone(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp := uuid.New()
	other := uuid.New()

	if _, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", dev); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists(s.keys(OTPPurposeRegistration, sp, "+923001234567", dev)[0]) || mr.Exists(s.keys(OTPPurposeRegistration, sp, "+923001234567", other)[0]) {
		t.Fatal("code must be stored against the requesting device only")
	}
	_, err := s.Issue(bg(), OTPPurposeRegistration, sp, "+923001234567", other)
	if appErr, ok := models.AsAppError(err); !ok || appErr.Code != models.CodeOTPResendCooldown.Code {
		t.Fatalf("another device must not bypass the phone's cooldown, got %v", err)
	}
}

func TestGenerateNumericCode_RejectsBadLength(t *testing.T) {
	if _, err := generateNumericCode(3); err == nil {
		t.Fatal("want error")
	}
}

func TestOTPService_ReserveThenIssueReserved(t *testing.T) {
	s, mr := newTestOTPService(t)
	sp, phone := uuid.New(), "+923001234567"

	if err := s.Reserve(bg(), OTPPurposeRegistration, sp, phone); err != nil {
		t.Fatalf("fresh phone must be allowed: %v", err)
	}
	err := s.Reserve(bg(), OTPPurposeRegistration, sp, phone)
	if appErr, ok := models.AsAppError(err); !ok || appErr.Code != models.CodeOTPResendCooldown.Code || appErr.RetryAfter != time.Minute {
		t.Fatalf("second reserve must hit the cooldown, got %v", err)
	}

	// The reserved code is stored without re-checking the limits.
	otp, err := s.IssueReserved(bg(), OTPPurposeRegistration, sp, phone, dev)
	if err != nil {
		t.Fatal(err)
	}
	key := s.keys(OTPPurposeRegistration, sp, phone, dev)[0]
	if mr.HGet(key, "code_hash") != s.hash(OTPPurposeRegistration, sp, phone, dev, otp.Code) || mr.TTL(key) != 5*time.Minute {
		t.Fatal("reserved code not stored correctly")
	}
	if got, _ := mr.Get(s.keys(OTPPurposeRegistration, sp, phone, dev)[2]); got != "1" {
		t.Fatalf("one reservation = one counted code, hourly counter is %q", got)
	}
}

func TestOTPService_ReserveHourlyCapAndRelease(t *testing.T) {
	s, mr := newTestOTPService(t) // cap: 3 per hour
	sp, phone := uuid.New(), "+923001234567"
	for i := 0; i < 3; i++ {
		if err := s.Reserve(bg(), OTPPurposeRegistration, sp, phone); err != nil {
			t.Fatalf("reserve %d: %v", i+1, err)
		}
		mr.FastForward(time.Minute + time.Second)
	}
	err := s.Reserve(bg(), OTPPurposeRegistration, sp, phone)
	if appErr, ok := models.AsAppError(err); !ok || appErr.Code != models.CodeOTPHourlyLimit.Code {
		t.Fatalf("want hourly cap, got %v", err)
	}

	// Releasing an unused reservation gives the slot back.
	s.RevokeQuietly(bg(), OTPPurposeRegistration, sp, phone, uuid.Nil)
	if err := s.Reserve(bg(), OTPPurposeRegistration, sp, phone); err != nil {
		t.Fatalf("released slot must be reusable: %v", err)
	}
}

func TestOTPService_ConcurrentReserveOnlyOneWins(t *testing.T) {
	s, _ := newTestOTPService(t)
	sp := uuid.New()
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Reserve(bg(), OTPPurposeRegistration, sp, "+923001234567") == nil {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("exactly one concurrent reserve may win, got %d", won)
	}
}
