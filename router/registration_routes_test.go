package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	c "ofc-user-service/context"
	"ofc-user-service/handlers"
	"ofc-user-service/models"
	"ofc-user-service/service"
)

type fakeRegistration struct {
	called bool
	gotCtx c.Context
	gotIn  service.RequestOTPInput
	res    *service.RequestOTPResult
	err    error
}

func (f *fakeRegistration) RequestOTP(ctx c.Context, in service.RequestOTPInput) (*service.RequestOTPResult, error) {
	f.called, f.gotCtx, f.gotIn = true, ctx, in
	return f.res, f.err
}

// newTestAPI wires the registration routes exactly like New does, with a
// real AuthService backed by an in-process Redis.
func newTestAPI(t *testing.T, reg *fakeRegistration) http.Handler {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return newAPI(service.NewAuthService(client, 5*time.Minute), handlers.NewRegistrationHandler(reg))
}

// post sends a request with fresh, valid guard headers.
func post(t *testing.T, api http.Handler, tenant, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/users/register/request", strings.NewReader(body))
	req.Header.Set(service.HeaderRequestID, uuid.NewString())
	req.Header.Set(service.HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
	if tenant != "" {
		req.Header.Set(service.HeaderServiceProvider, tenant)
	}
	setDeviceHeaders(req)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	return rec
}

func setDeviceHeaders(req *http.Request) {
	req.Header.Set(service.HeaderDeviceID, "device-abc")
	req.Header.Set(service.HeaderDeviceType, "MOBILE_ANDROID")
	req.Header.Set(service.HeaderOSName, "Android")
}

func envelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json %q: %v", rec.Body.String(), err)
	}
	for _, k := range []string{"code", "message", "data"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("envelope missing %q: %v", k, m)
		}
	}
	return m
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, status int, rc models.ResponseCode) {
	t.Helper()
	body := envelope(t, rec)
	if rec.Code != status || int(body["code"].(float64)) != rc.Code || body["message"] != rc.Message {
		t.Fatalf("want %d code %d, got %d %v", status, rc.Code, rec.Code, body)
	}
}

func TestRegisterRequest_Created(t *testing.T) {
	code := "123456"
	tenant := uuid.New()
	deviceID := uuid.New()
	reg := &fakeRegistration{res: &service.RequestOTPResult{DeviceID: deviceID, ExpiresAt: time.Now(), ResendAfter: time.Minute, Code: &code}}

	rec := post(t, newTestAPI(t, reg), tenant.String(), `{"phone_number":"+923001234567"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := envelope(t, rec)
	data := body["data"].(map[string]any)
	if int(body["code"].(float64)) != models.CodeSuccess.Code || body["message"] != models.CodeSuccess.Message || data["otp"] != "123456" || data["device_id"] != deviceID.String() || data["resend_after_seconds"].(float64) != 60 {
		t.Fatalf("unexpected body %v", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing cache-control header")
	}
	if sp := reg.gotCtx.ServiceProviderID(); sp == nil || *sp != tenant {
		t.Fatal("tenant not passed to the service")
	}
	if reg.gotCtx.Session() != rec.Header().Get(service.HeaderRequestID) {
		t.Fatal("request id not passed as the session id")
	}
	if reg.gotIn.PhoneNumber != "+923001234567" || reg.gotIn.IPAddress == "" ||
		reg.gotIn.Device != (models.DeviceFilters{DeviceID: "device-abc", DeviceType: "MOBILE_ANDROID", OSName: "Android"}) {
		t.Fatalf("unexpected input %+v", reg.gotIn)
	}
}

func TestRegisterRequest_OmitsOTPWhenAbsent(t *testing.T) {
	reg := &fakeRegistration{res: &service.RequestOTPResult{}}
	data := envelope(t, post(t, newTestAPI(t, reg), uuid.NewString(), `{"phone_number":"+923001234567"}`))["data"].(map[string]any)
	if _, present := data["otp"]; present {
		t.Fatal("otp must be omitted outside development")
	}
}

func TestRegisterRequest_TenantHeader(t *testing.T) {
	reg := &fakeRegistration{}
	api := newTestAPI(t, reg)
	wantCode(t, post(t, api, "", `{"phone_number":"+923001234567"}`), http.StatusBadRequest, models.CodeServiceProviderHeaderRequired)
	wantCode(t, post(t, api, "not-a-uuid", `{"phone_number":"+923001234567"}`), http.StatusBadRequest, models.CodeServiceProviderHeaderInvalid)
	if reg.called {
		t.Fatal("service must not be called without a valid tenant")
	}
}

func TestRegisterRequest_GuardHeadersRequired(t *testing.T) {
	reg := &fakeRegistration{}
	req := httptest.NewRequest(http.MethodPost, "/v1/users/register/request", strings.NewReader(`{"phone_number":"+923001234567"}`))
	req.Header.Set(service.HeaderServiceProvider, uuid.NewString())
	rec := httptest.NewRecorder()
	newTestAPI(t, reg).ServeHTTP(rec, req)
	wantCode(t, rec, http.StatusBadRequest, models.CodeRequestIDHeaderRequired)
	if reg.called {
		t.Fatal("service must not be called without guard headers")
	}
}

func TestRegisterRequest_DuplicateRejected(t *testing.T) {
	reg := &fakeRegistration{res: &service.RequestOTPResult{}}
	api := newTestAPI(t, reg)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/users/register/request", strings.NewReader(`{"phone_number":"+923001234567"}`))
		req.Header.Set(service.HeaderRequestID, "fixed-id")
		req.Header.Set(service.HeaderTimestamp, timestamp)
		req.Header.Set(service.HeaderServiceProvider, uuid.NewString())
		setDeviceHeaders(req)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	if rec := send(); rec.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", rec.Code, rec.Body)
	}
	reg.called = false

	wantCode(t, send(), http.StatusConflict, models.CodeDuplicateRequest)
	if reg.called {
		t.Fatal("duplicate must not reach the service")
	}
}

func TestRegisterRequest_BadBodies(t *testing.T) {
	api := newTestAPI(t, &fakeRegistration{})
	for _, body := range []string{``, `not json`, `{"phone":"+923001234567"}`, `{"phone_number":"+92"}{}`} {
		wantCode(t, post(t, api, uuid.NewString(), body), http.StatusBadRequest, models.CodeInvalidRequestBody)
	}
}

// Phone numbers are normalized and validated by the handler, before the
// registration service is called.
func TestRegisterRequest_NormalizesPhone(t *testing.T) {
	for _, phone := range []string{"+92 300-123 4567", "0092 (300) 1234567", "+92.300.123.4567"} {
		reg := &fakeRegistration{res: &service.RequestOTPResult{ExpiresAt: time.Now(), ResendAfter: time.Minute}}
		rec := post(t, newTestAPI(t, reg), uuid.NewString(), `{"phone_number":"`+phone+`"}`)
		if rec.Code != http.StatusCreated || reg.gotIn.PhoneNumber != "+923001234567" {
			t.Fatalf("%q: status %d, service got %q", phone, rec.Code, reg.gotIn.PhoneNumber)
		}
	}
}

func TestRegisterRequest_InvalidPhones(t *testing.T) {
	cases := map[string]models.ResponseCode{
		`{"phone_number":""}`:            models.CodePhoneNumberRequired,
		`{"phone_number":"   "}`:         models.CodePhoneNumberRequired,
		`{}`:                             models.CodePhoneNumberRequired,
		`{"phone_number":"03001234567"}`: models.CodePhoneNumberInvalid,
		`{"phone_number":"+0123456789"}`: models.CodePhoneNumberInvalid,
		`{"phone_number":"+92abc"}`:      models.CodePhoneNumberInvalid,
		`{"phone_number":"+1234567"}`:    models.CodePhoneNumberInvalid,
	}
	for body, want := range cases {
		reg := &fakeRegistration{}
		rec := post(t, newTestAPI(t, reg), uuid.NewString(), body)
		wantCode(t, rec, http.StatusBadRequest, want)
		if reg.called {
			t.Fatalf("%s: invalid phone must not reach the service", body)
		}
	}
}

func TestRegisterRequest_InvalidDeviceHeaders(t *testing.T) {
	cases := []struct {
		header, value string
		want          models.ResponseCode
	}{
		{service.HeaderDeviceID, "", models.CodeDeviceIDHeaderRequired},
		{service.HeaderDeviceID, "   ", models.CodeDeviceIDHeaderRequired},
		{service.HeaderDeviceID, strings.Repeat("a", 129), models.CodeDeviceIDHeaderInvalid},
		{service.HeaderDeviceType, "", models.CodeDeviceTypeHeaderRequired},
		{service.HeaderDeviceType, "TOASTER", models.CodeDeviceTypeHeaderInvalid},
		{service.HeaderOSVersion, strings.Repeat("1", 33), models.CodeDeviceHeaderTooLong},
	}
	for _, tc := range cases {
		reg := &fakeRegistration{}
		req := httptest.NewRequest(http.MethodPost, "/v1/users/register/request", strings.NewReader(`{"phone_number":"+923001234567"}`))
		req.Header.Set(service.HeaderRequestID, uuid.NewString())
		req.Header.Set(service.HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
		req.Header.Set(service.HeaderServiceProvider, uuid.NewString())
		setDeviceHeaders(req)
		req.Header.Set(tc.header, tc.value)
		rec := httptest.NewRecorder()
		newTestAPI(t, reg).ServeHTTP(rec, req)
		wantCode(t, rec, http.StatusBadRequest, tc.want)
		if reg.called {
			t.Fatalf("%s=%q: invalid device must not reach the service", tc.header, tc.value)
		}
	}
}

func TestRegisterRequest_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		rc     models.ResponseCode
	}{
		{models.NewAppError(models.CodeServiceProviderNotFound), http.StatusNotFound, models.CodeServiceProviderNotFound},
		{models.NewAppError(models.CodePhoneAlreadyRegistered), http.StatusConflict, models.CodePhoneAlreadyRegistered},
		{models.NewAppError(models.CodeOTPResendCooldown).WithRetryAfter(40 * time.Second), http.StatusTooManyRequests, models.CodeOTPResendCooldown},
		{models.NewAppError(models.CodeOTPDeliveryFailed).WithCause(errors.New("provider down")), http.StatusServiceUnavailable, models.CodeOTPDeliveryFailed},
		{errors.New("db password=secret"), http.StatusInternalServerError, models.CodeInternal},
	}
	for _, tc := range cases {
		rec := post(t, newTestAPI(t, &fakeRegistration{err: tc.err}), uuid.NewString(), `{"phone_number":"+923001234567"}`)
		wantCode(t, rec, tc.status, tc.rc)
		if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "provider down") {
			t.Fatal("internal error details leaked")
		}
		if tc.status == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "40" {
			t.Fatalf("want Retry-After 40, got %q", rec.Header().Get("Retry-After"))
		}
	}
}

func TestUnknownRoute_ReturnsEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/nope", nil)
	req.Header.Set(service.HeaderRequestID, uuid.NewString())
	req.Header.Set(service.HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
	rec := httptest.NewRecorder()
	newTestAPI(t, &fakeRegistration{}).ServeHTTP(rec, req)
	wantCode(t, rec, http.StatusNotFound, models.CodeRouteNotFound)
}
