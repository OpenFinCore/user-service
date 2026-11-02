package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	c "ofc-user-service/context"
	"ofc-user-service/logger"
	"ofc-user-service/models"
)

// OTP purposes. A purpose keeps codes for different flows (registration,
// login, password reset, ...) apart for the same phone number.
const (
	OTPPurposeRegistration = "REGISTRATION"
)

// OTPPolicy bounds how codes are issued.
type OTPPolicy struct {
	Length             int
	TTL                time.Duration
	ResendCooldown     time.Duration
	MaxRequestsPerHour int
	MaxVerifyAttempts  int
}

// IssuedOTP is a freshly issued code. Code is the plain value and must
// only ever be delivered to the recipient (or returned in development).
type IssuedOTP struct {
	Code        string
	ExpiresAt   time.Time
	ResendAfter time.Duration
}

// OTPService issues one-time passcodes and keeps them in Redis. A code is
// held against the device it was requested from (devices.id), while the
// resend cooldown and hourly cap apply per recipient, so switching devices
// can't be used to flood a phone. Only an HMAC of the code is stored.
// Issuing is a single atomic Lua script, so the limits hold across
// concurrent requests and multiple service instances.
//
// Keys per (purpose, tenant, recipient [, device]), sharing one hash tag so
// they live on the same Redis Cluster slot:
//
//	otp:{P:T:R}:device:D:code  hash {code_hash, attempts, max_attempts, created_at}, TTL = policy.TTL
//	otp:{P:T:R}:cooldown       set while a resend is not yet allowed
//	otp:{P:T:R}:hourly         number of codes issued in the current hour window
type OTPService struct {
	redis  *redis.Client
	secret []byte
	policy OTPPolicy
	now    func() time.Time
}

func NewOTPService(client *redis.Client, hashSecret string, policy OTPPolicy) *OTPService {
	return &OTPService{redis: client, secret: []byte(hashSecret), policy: policy, now: time.Now}
}

// issueScript atomically: rejects while the cooldown key exists, rejects
// once the hourly counter reached the cap, otherwise replaces the code,
// starts the cooldown and bumps the counter. Returns {status, wait_ms}
// with status 0 = issued, 1 = cooldown, 2 = hourly cap.
var issueScript = redis.NewScript(`
local wait = redis.call('PTTL', KEYS[2])
if wait > 0 then
  return {1, wait}
end
local issued = tonumber(redis.call('GET', KEYS[3]) or '0')
if issued >= tonumber(ARGV[4]) then
  return {2, redis.call('PTTL', KEYS[3])}
end
redis.call('DEL', KEYS[1])
redis.call('HSET', KEYS[1], 'code_hash', ARGV[1], 'attempts', 0, 'max_attempts', ARGV[5], 'created_at', ARGV[6])
redis.call('PEXPIRE', KEYS[1], ARGV[2])
redis.call('SET', KEYS[2], 1, 'PX', ARGV[3])
if redis.call('INCR', KEYS[3]) == 1 then
  redis.call('PEXPIRE', KEYS[3], 3600000)
end
return {0, 0}
`)

// revokeScript undoes an issue whose delivery failed: the code is dropped
// and it no longer counts against the cooldown or the hourly cap.
var revokeScript = redis.NewScript(`
redis.call('DEL', KEYS[1], KEYS[2])
if tonumber(redis.call('GET', KEYS[3]) or '0') > 0 then
  redis.call('DECR', KEYS[3])
end
return 1
`)

// Issue generates a code for recipient, stores its hash against deviceID
// and enforces the resend cooldown and hourly cap (returned as
// RATE_LIMITED AppErrors).
func (s *OTPService) Issue(ctx c.Context, purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID) (*IssuedOTP, error) {
	code, err := generateNumericCode(s.policy.Length)
	if err != nil {
		return nil, fmt.Errorf("otp: generating code: %w", err)
	}

	now := s.now().UTC()
	res, err := issueScript.Run(stdContext(ctx), s.redis, s.keys(purpose, spID, recipient, deviceID),
		s.hash(purpose, spID, recipient, deviceID, code),
		s.policy.TTL.Milliseconds(),
		s.policy.ResendCooldown.Milliseconds(),
		s.policy.MaxRequestsPerHour,
		s.policy.MaxVerifyAttempts,
		now.Unix(),
	).Int64Slice()
	if err != nil {
		return nil, fmt.Errorf("otp: storing code: %w", err)
	}

	switch status, wait := res[0], time.Duration(res[1])*time.Millisecond; status {
	case 0:
		return &IssuedOTP{Code: code, ExpiresAt: now.Add(s.policy.TTL), ResendAfter: s.policy.ResendCooldown}, nil
	case 1:
		return nil, models.NewAppError(models.CodeOTPResendCooldown).WithRetryAfter(roundUpToSecond(wait))
	case 2:
		return nil, models.NewAppError(models.CodeOTPHourlyLimit).WithRetryAfter(roundUpToSecond(wait))
	default:
		return nil, fmt.Errorf("otp: unexpected issue status %d", status)
	}
}

// Revoke drops the code held against deviceID and releases the
// recipient's cooldown and hourly-cap slot. Used when the code could not be
// delivered.
func (s *OTPService) Revoke(ctx c.Context, purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID) error {
	if err := revokeScript.Run(stdContext(ctx), s.redis, s.keys(purpose, spID, recipient, deviceID)).Err(); err != nil {
		return fmt.Errorf("otp: revoking code: %w", err)
	}
	return nil
}

// RevokeQuietly is Revoke for failure paths: the request has already
// failed and the caller returns that error, so a revoke error is only
// logged (the code then simply expires).
func (s *OTPService) RevokeQuietly(ctx c.Context, purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID) {
	if err := s.Revoke(ctx, purpose, spID, recipient, deviceID); err != nil {
		logger.LogError(ctx.Session(), err)
	}
}

func (s *OTPService) keys(purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID) []string {
	tag := "otp:{" + purpose + ":" + spID.String() + ":" + recipient + "}"
	return []string{tag + ":device:" + deviceID.String() + ":code", tag + ":cooldown", tag + ":hourly"}
}

// hash is an HMAC-SHA256 under a server-side secret, bound to the purpose,
// tenant, recipient and device: a plain hash of a 6-digit code would be
// brute forced instantly from a Redis dump, and binding stops cross-use.
func (s *OTPService) hash(purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID, code string) string {
	mac := hmac.New(sha256.New, s.secret)
	for _, part := range []string{purpose, spID.String(), recipient, deviceID.String(), code} {
		mac.Write([]byte(part))
		mac.Write([]byte{0})
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// generateNumericCode draws each digit uniformly from crypto/rand.
func generateNumericCode(length int) (string, error) {
	if length < 4 || length > 10 {
		return "", errors.New("otp length must be between 4 and 10")
	}
	var b strings.Builder
	b.Grow(length)
	ten := big.NewInt(10)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, ten)
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

// stdContext returns ctx's standard context (for cancellation/deadlines),
// falling back to Background for a bare c.Context.
func stdContext(ctx c.Context) context.Context {
	if base := ctx.Context(); base != nil {
		return base
	}
	return context.Background()
}

func roundUpToSecond(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	return (d + time.Second - 1).Truncate(time.Second)
}
