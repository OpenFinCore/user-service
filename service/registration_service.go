package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	c "ofc-user-service/context"
	"ofc-user-service/logger"
	"ofc-user-service/models"
	"ofc-user-service/repository"
	utils "ofc-user-service/utils"
)

type RequestOTPInput struct {
	PhoneNumber string
	// Device is the device the request comes from (X-Device-* headers); it
	// is registered as PENDING_VERIFICATION and the OTP is held against it.
	Device models.DeviceFilters
	// IPAddress and UserAgent describe the caller, for the audit trail.
	IPAddress string
	UserAgent string
}

type RequestOTPResult struct {
	DeviceID    uuid.UUID
	ExpiresAt   time.Time
	ResendAfter time.Duration
	// Code is the plain OTP. It is set ONLY in the development stage, where
	// no SMS is sent; it is always nil otherwise.
	Code *string
}

// The capabilities RegistrationService uses from the services it is linked
// to. The concrete services satisfy them; tests substitute fakes.
type (
	registrationTenants interface {
		GetActiveByID(ctx c.Context, id uuid.UUID) (*models.ServiceProvider, error)
	}
	registrationUsers interface {
		PhoneExists(ctx c.Context, spID uuid.UUID, phone string) (bool, error)
	}
	registrationDevices interface {
		RegisterPending(ctx c.Context, spID uuid.UUID, info models.Device) (*models.Device, error)
	}
	registrationOTPs interface {
		Issue(ctx c.Context, purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID) (*IssuedOTP, error)
		RevokeQuietly(ctx c.Context, purpose string, spID uuid.UUID, recipient string, deviceID uuid.UUID)
	}
	Audit interface {
		RecordEvent(ctx c.Context, ev AuditEvent) error
	}
)

// RegistrationService is the end-user registration domain. It holds no
// table of its own: every step is a method call on the service that owns
// it (tenant checks on ServiceProviderService, audit on AuditLogService,
// codes on OTPService, ...), linked in NewServiceContainer.
type RegistrationService struct {
	store            repository.Store
	serviceProviders registrationTenants
	users            registrationUsers
	devices          registrationDevices
	otps             registrationOTPs
	audit            Audit
	sms              SMSSender
	// devMode skips SMS delivery and returns the code in the result.
	devMode bool
}

func NewRegistrationService(
	store repository.Store,
	serviceProviders *ServiceProviderService,
	users *UserService,
	devices *DeviceService,
	otps *OTPService,
	auditLogs *AuditLogService,
	sms SMSSender,
	devMode bool,
) *RegistrationService {
	return &RegistrationService{
		store:            store,
		serviceProviders: serviceProviders,
		users:            users,
		devices:          devices,
		otps:             otps,
		audit:            auditLogs,
		sms:              sms,
		devMode:          devMode,
	}
}

// RequestOTP starts registration for a phone number: it checks the tenant
// and that the number is free, registers the caller's device as
// PENDING_VERIFICATION, issues a one-time passcode held against that device
// (stored hashed in Redis, rate limited per phone), records an audit entry,
// and delivers the code by SMS - or, in development, returns it in the
// result instead.
func (s *RegistrationService) RequestOTP(ctx c.Context, in RequestOTPInput) (*RequestOTPResult, error) {
	spIDPtr := ctx.ServiceProviderID()
	if spIDPtr == nil {
		return nil, models.NewAppError(models.CodeServiceProviderHeaderRequired)
	}
	spID := *spIDPtr

	// Tenant-scoped reads and writes run in a transaction so row-level
	// security applies to them.
	var device *models.Device
	err := repository.RunInTx(ctx, s.store, func(ctx c.Context) error {
		sp, err := s.serviceProviders.GetActiveByID(ctx, spID)
		if err != nil {
			return fmt.Errorf("registration: loading service provider: %w", err)
		}
		if sp == nil {
			return models.NewAppError(models.CodeServiceProviderNotFound)
		}
		taken, err := s.users.PhoneExists(ctx, spID, in.PhoneNumber)
		if err != nil {
			return fmt.Errorf("registration: checking phone: %w", err)
		}
		if taken {
			return models.NewAppError(models.CodePhoneAlreadyRegistered)
		}
		device, err = s.devices.RegisterPending(ctx, spID, deviceFromInput(in))
		if err != nil {
			return fmt.Errorf("registration: registering device: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	otp, err := s.otps.Issue(ctx, OTPPurposeRegistration, spID, in.PhoneNumber, device.ID)
	if err != nil {
		return nil, err
	}

	if err := s.audit.RecordEvent(ctx, AuditEvent{
		Action:       "registration.otp_requested",
		ResourceType: "device",
		ResourceID:   &device.ID,
		Payload:      map[string]any{"phone_number": utils.MaskPhone(in.PhoneNumber), "channel": "SMS", "device_uuid": device.DeviceUUID},
		IPAddress:    in.IPAddress,
		UserAgent:    in.UserAgent,
	}); err != nil {
		s.otps.RevokeQuietly(ctx, OTPPurposeRegistration, spID, in.PhoneNumber, device.ID)
		return nil, err
	}

	result := &RequestOTPResult{DeviceID: device.ID, ExpiresAt: otp.ExpiresAt, ResendAfter: otp.ResendAfter}

	if s.devMode {
		result.Code = &otp.Code
		logger.LogInfo(fmt.Sprintf("registration otp issued for %s (development: not sent, returned in response)", utils.MaskPhone(in.PhoneNumber)), ctx.Session())
		return result, nil
	}

	if err := s.sms.Send(ctx, in.PhoneNumber, utils.OtpSMSText(otp.Code, otp.ExpiresAt.Sub(time.Now()))); err != nil {
		s.otps.RevokeQuietly(ctx, OTPPurposeRegistration, spID, in.PhoneNumber, device.ID)
		return nil, models.NewAppError(models.CodeOTPDeliveryFailed).WithCause(err)
	}
	logger.LogInfo(fmt.Sprintf("registration otp sent to %s", utils.MaskPhone(in.PhoneNumber)), ctx.Session())
	return result, nil
}

// deviceFromInput maps the request's device headers and caller details to
// a devices row.
func deviceFromInput(in RequestOTPInput) models.Device {
	return models.Device{
		DeviceUUID: in.Device.DeviceID,
		DeviceType: in.Device.DeviceType,
		DeviceName: utils.OptionalString(in.Device.DeviceName),
		OSName:     utils.OptionalString(in.Device.OSName),
		OSVersion:  utils.OptionalString(in.Device.OSVersion),
		AppVersion: utils.OptionalString(in.Device.AppVersion),
		IPAddress:  utils.ParseIP(in.IPAddress),
		UserAgent:  utils.OptionalString(utils.Truncate(strings.TrimSpace(in.UserAgent), 512)),
	}
}
