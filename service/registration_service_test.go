package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	utils "ofc-user-service/utils"
)

// ---- fakes ---------------------------------------------------------------

type fakeStore struct{ commits, rollbacks int }

func (s *fakeStore) DB() *gorm.DB                                             { return nil }
func (s *fakeStore) BeginTx(ctx c.Context) (c.Context, error)                 { return ctx.WithTx(&gorm.DB{}), nil }
func (s *fakeStore) CommitTx(c.Context) error                                 { s.commits++; return nil }
func (s *fakeStore) Rollback(c.Context) error                                 { s.rollbacks++; return nil }
func (s *fakeStore) TryAdvisoryLock(c.Context, int) (bool, error)             { return true, nil }
func (s *fakeStore) AdvisoryUnlock(c.Context, int) error                      { return nil }
func (s *fakeStore) UpdateSeedRegistry(c.Context, *models.SeedRegistry) error { return nil }
func (s *fakeStore) GetSeedRegistryByFileName(c.Context, string) (*models.SeedRegistry, error) {
	return nil, nil
}

// fakeTenants returns sp only when it is active, like GetActiveByID.
type fakeTenants struct{ sp *models.ServiceProvider }

func (f fakeTenants) GetActiveByID(c.Context, uuid.UUID) (*models.ServiceProvider, error) {
	if f.sp == nil || f.sp.StatusID != spActive {
		return nil, nil
	}
	return f.sp, nil
}

type fakeStatuses map[string]int16

func (f fakeStatuses) IDByName(_ c.Context, name string) (int16, error) {
	if id, ok := f[name]; ok {
		return id, nil
	}
	return 0, errors.New("unknown status " + name)
}

type fakeUsers struct{ taken bool }

func (f fakeUsers) PhoneExists(c.Context, uuid.UUID, string) (bool, error) { return f.taken, nil }

type fakeDevices struct {
	registered []models.Device
	err        error
}

func (f *fakeDevices) RegisterPending(_ c.Context, spID uuid.UUID, info models.Device) (*models.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	info.ID, info.ServiceProviderID = uuid.New(), spID
	f.registered = append(f.registered, info)
	return &info, nil
}

type fakeOTPs struct {
	devices  []uuid.UUID
	issued   []string
	revoked  []string
	issueErr error
}

func (f *fakeOTPs) Issue(_ c.Context, _ string, _ uuid.UUID, recipient string, deviceID uuid.UUID) (*IssuedOTP, error) {
	if f.issueErr != nil {
		return nil, f.issueErr
	}
	f.issued = append(f.issued, recipient)
	f.devices = append(f.devices, deviceID)
	return &IssuedOTP{Code: "123456", ExpiresAt: time.Now().Add(5 * time.Minute), ResendAfter: time.Minute}, nil
}

func (f *fakeOTPs) RevokeQuietly(_ c.Context, _ string, _ uuid.UUID, recipient string, _ uuid.UUID) {
	f.revoked = append(f.revoked, recipient)
}

type fakeAudit struct {
	entries []AuditEvent
	err     error
}

func (f *fakeAudit) RecordEvent(_ c.Context, e AuditEvent) error {
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, e)
	return nil
}

type fakeSMS struct {
	sent []string
	err  error
}

func (f *fakeSMS) Send(_ c.Context, to, _ string) error {
	f.sent = append(f.sent, to)
	return f.err
}

// ---- harness -------------------------------------------------------------

const (
	spActive    int16 = 1
	spSuspended int16 = 2
)

type registrationHarness struct {
	svc     *RegistrationService
	store   *fakeStore
	devices *fakeDevices
	otps    *fakeOTPs
	audit   *fakeAudit
	sms     *fakeSMS
	tenant  uuid.UUID
}

func newRegistrationHarness(devMode bool) *registrationHarness {
	h := &registrationHarness{store: &fakeStore{}, devices: &fakeDevices{}, otps: &fakeOTPs{}, audit: &fakeAudit{}, sms: &fakeSMS{}, tenant: uuid.New()}
	h.svc = &RegistrationService{
		store:            h.store,
		serviceProviders: fakeTenants{sp: &models.ServiceProvider{ID: h.tenant, StatusID: spActive}},
		users:            fakeUsers{},
		devices:          h.devices,
		otps:             h.otps,
		audit:            h.audit,
		sms:              h.sms,
		devMode:          devMode,
	}
	return h
}

func (h *registrationHarness) ctx() c.Context {
	return c.NewContext(context.Background()).WithServiceProviderID(h.tenant).WithSession("test")
}

func wantAppError(t *testing.T, err error, want models.ResponseCode) *models.AppError {
	t.Helper()
	appErr, ok := models.AsAppError(err)
	if !ok || appErr.Code != want.Code {
		t.Fatalf("want code %d (%s), got %v", want.Code, want.Message, err)
	}
	return appErr
}

var testDevice = models.DeviceFilters{DeviceID: "device-abc", DeviceType: "MOBILE_ANDROID", OSName: "Android"}

// ---- tests ---------------------------------------------------------------

func TestRequestOTP_DevelopmentReturnsCodeAndSkipsSMS(t *testing.T) {
	h := newRegistrationHarness(true)

	res, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice, IPAddress: "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Code == nil || *res.Code != "123456" {
		t.Fatalf("development must return the code, got %v", res.Code)
	}
	if len(h.sms.sent) != 0 {
		t.Fatal("development must not send SMS")
	}
	if len(h.otps.issued) != 1 || h.otps.issued[0] != "+923001234567" {
		t.Fatalf("want one OTP for the phone, got %v", h.otps.issued)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != "registration.otp_requested" {
		t.Fatalf("want one audit entry, got %+v", h.audit.entries)
	}
	if p := fmt.Sprint(h.audit.entries[0].Payload); strings.Contains(p, "3001234567") {
		t.Fatalf("audit payload must mask the phone: %s", p)
	}
}

func TestRequestOTP_ProductionSendsSMSAndHidesCode(t *testing.T) {
	h := newRegistrationHarness(false)
	res, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != nil {
		t.Fatal("code must never be returned outside development")
	}
	if len(h.sms.sent) != 1 || h.sms.sent[0] != "+923001234567" {
		t.Fatalf("want one SMS, got %v", h.sms.sent)
	}
}

func TestRequestOTP_SMSFailureRevokesAndReturnsUnavailable(t *testing.T) {
	h := newRegistrationHarness(false)
	h.sms.err = errors.New("provider down")
	_, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	wantAppError(t, err, models.CodeOTPDeliveryFailed)
	if len(h.otps.revoked) != 1 {
		t.Fatal("undelivered code must be revoked")
	}
}

func TestRequestOTP_AuditFailureRevokes(t *testing.T) {
	h := newRegistrationHarness(true)
	h.audit.err = errors.New("db down")
	_, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	if err == nil || len(h.otps.revoked) != 1 {
		t.Fatalf("want error and revoke, got err=%v revoked=%v", err, h.otps.revoked)
	}
	if _, ok := models.AsAppError(err); ok {
		t.Fatal("infrastructure errors must not be exposed as models.AppError")
	}
}

func TestRequestOTP_MissingTenant(t *testing.T) {
	h := newRegistrationHarness(true)
	_, err := h.svc.RequestOTP(c.NewContext(context.Background()), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	wantAppError(t, err, models.CodeServiceProviderHeaderRequired)
}

func TestRequestOTP_UnknownOrInactiveTenant(t *testing.T) {
	for name, sp := range map[string]*models.ServiceProvider{"unknown": nil, "suspended": {StatusID: spSuspended}} {
		t.Run(name, func(t *testing.T) {
			h := newRegistrationHarness(true)
			h.svc.serviceProviders = fakeTenants{sp: sp}
			_, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
			wantAppError(t, err, models.CodeServiceProviderNotFound)
			if len(h.otps.issued) != 0 {
				t.Fatal("no OTP may be issued for an inactive tenant")
			}
		})
	}
}

func TestRequestOTP_PhoneAlreadyRegistered(t *testing.T) {
	h := newRegistrationHarness(true)
	h.svc.users = fakeUsers{taken: true}
	_, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	wantAppError(t, err, models.CodePhoneAlreadyRegistered)
	if len(h.otps.issued) != 0 {
		t.Fatal("no OTP may be issued for a registered phone")
	}
}

func TestRequestOTP_RateLimitPassesThrough(t *testing.T) {
	h := newRegistrationHarness(true)
	h.otps.issueErr = models.NewAppError(models.CodeOTPResendCooldown).WithRetryAfter(40 * time.Second)
	_, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	if wantAppError(t, err, models.CodeOTPResendCooldown).RetryAfter != 40*time.Second {
		t.Fatal("retry-after must be preserved")
	}
	if len(h.audit.entries) != 0 {
		t.Fatal("rate-limited requests must not be audited as issued")
	}
}

func TestRequestOTP_RegistersPendingDeviceAndHoldsOTPAgainstIt(t *testing.T) {
	h := newRegistrationHarness(true)
	res, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice, IPAddress: "10.0.0.1", UserAgent: "app/1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.devices.registered) != 1 {
		t.Fatalf("want one device registered, got %d", len(h.devices.registered))
	}
	d := h.devices.registered[0]
	if d.DeviceUUID != "device-abc" || d.DeviceType != "MOBILE_ANDROID" || d.OSName == nil || *d.OSName != "Android" ||
		d.DeviceName != nil || d.IPAddress == nil || d.UserAgent == nil || *d.UserAgent != "app/1.0" || d.ServiceProviderID != h.tenant {
		t.Fatalf("unexpected device %+v", d)
	}
	if res.DeviceID != d.ID || len(h.otps.devices) != 1 || h.otps.devices[0] != d.ID {
		t.Fatal("the OTP must be held against the registered device and its id returned")
	}
	if e := h.audit.entries[0]; e.ResourceType != "device" || e.ResourceID == nil || *e.ResourceID != d.ID {
		t.Fatalf("audit must reference the device, got %+v", e)
	}
}

func TestRequestOTP_DeviceFailureIssuesNoOTP(t *testing.T) {
	h := newRegistrationHarness(true)
	h.devices.err = errors.New("db down")
	if _, err := h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice}); err == nil {
		t.Fatal("want error")
	}
	if len(h.otps.issued) != 0 || h.store.rollbacks != 1 {
		t.Fatalf("want rollback and no OTP, got issued=%v rollbacks=%d", h.otps.issued, h.store.rollbacks)
	}
}

func TestRequestOTP_RejectedRequestsRegisterNoDevice(t *testing.T) {
	h := newRegistrationHarness(true)
	h.svc.users = fakeUsers{taken: true}
	_, _ = h.svc.RequestOTP(h.ctx(), RequestOTPInput{PhoneNumber: "+923001234567", Device: testDevice})
	if len(h.devices.registered) != 0 {
		t.Fatal("no device may be registered for an already registered phone")
	}
}

func TestMaskPhone(t *testing.T) {
	if got := utils.MaskPhone("+923001234567"); got != "+92******4567" {
		t.Fatalf("got %q", got)
	}
}
