package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"ofc-user-service/conf"
	"ofc-user-service/logger"
	"ofc-user-service/repository"
	pgstore "ofc-user-service/repository/postgres"

	"go.uber.org/zap"
)

// Container wires the store to every entity-specific service and runs the
// lookup-table seeders once at startup. It has no HTTP/API surface -
// consumers (future controllers, jobs, tests) reach into it directly.
type Container struct {
	Store repository.Store
	Log   *zap.Logger

	ServiceProviderStatuses    *ServiceProviderStatusService
	UserTypeStatuses           *UserTypeStatusService
	TwoFactorStatuses          *TwoFactorStatusService
	UserStatuses               *UserStatusService
	DeviceStatuses             *DeviceStatusService
	SessionStatuses            *SessionStatusService
	RefreshTokenStatuses       *RefreshTokenStatusService
	PasswordResetTokenStatuses *PasswordResetTokenStatusService

	ServiceProviders *ServiceProviderService
	UserTypes        *UserTypeService
	Permissions      *PermissionService
	Roles            *RoleService
	RolePermissions  *RolePermissionService
	Countries        *CountryService
	Users            *UserService

	Devices             *DeviceService
	UserSessions        *UserSessionService
	RefreshTokens       *RefreshTokenService
	PasswordResetTokens *PasswordResetTokenService
	AuditLogs           *AuditLogService

	// Domain services: they own no table, they combine the ones above.
	Auth         *AuthService
	OTPs         *OTPService
	Registration *RegistrationService

	Seeds SeedsService
}

func NewServiceContainer() *Container {
	config := conf.GetConfig()
	log := logger.LoggerInit(config.Logger)
	store := pgstore.SharedStore()

	serviceProviderStatuses := NewServiceProviderStatusService(store)
	deviceStatuses := NewDeviceStatusService(store)

	container := &Container{
		Store: store,
		Log:   log,

		ServiceProviderStatuses:    serviceProviderStatuses,
		UserTypeStatuses:           NewUserTypeStatusService(store),
		TwoFactorStatuses:          NewTwoFactorStatusService(store),
		UserStatuses:               NewUserStatusService(store),
		DeviceStatuses:             deviceStatuses,
		SessionStatuses:            NewSessionStatusService(store),
		RefreshTokenStatuses:       NewRefreshTokenStatusService(store),
		PasswordResetTokenStatuses: NewPasswordResetTokenStatusService(store),

		ServiceProviders: NewServiceProviderService(store, serviceProviderStatuses),
		UserTypes:        NewUserTypeService(store),
		Permissions:      NewPermissionService(store),
		Roles:            NewRoleService(store),
		RolePermissions:  NewRolePermissionService(store),
		Countries:        NewCountryService(store),
		Users:            NewUserService(store),

		Devices:             NewDeviceService(store, deviceStatuses),
		UserSessions:        NewUserSessionService(store),
		RefreshTokens:       NewRefreshTokenService(store),
		PasswordResetTokens: NewPasswordResetTokenService(store),
		AuditLogs:           NewAuditLogService(store),
	}

	container.Seeds = NewSeedsService(store, config)
	if err := container.Seeds.SeedAll(); err != nil {
		panic(fmt.Errorf("seeding failed: %w", err))
	}

	if config.IsDevelopment() {
		devTenantID, err := uuid.Parse(config.DevTenant.ID)
		if err != nil {
			panic(fmt.Errorf("devTenant.id must be a UUID: %w", err))
		}
		ensureDevTenant(NewDevTenantService(store, container.ServiceProviders, devTenantID, config.DevTenant.Name))
	}

	container.Auth = NewAuthService(repository.SharedRedis(), time.Duration(config.Request.ReplayWindowSeconds)*time.Second)
	container.OTPs = NewOTPService(repository.SharedRedis(), config.OTP.HashSecret, OTPPolicy{
		Length:             config.OTP.Length,
		TTL:                time.Duration(config.OTP.TTLSeconds) * time.Second,
		ResendCooldown:     time.Duration(config.OTP.ResendCooldownSeconds) * time.Second,
		MaxRequestsPerHour: config.OTP.MaxRequestsPerHour,
		MaxVerifyAttempts:  config.OTP.MaxVerifyAttempts,
	})
	container.Registration = NewRegistrationService(
		store,
		container.ServiceProviders,
		container.Users,
		container.Devices,
		container.OTPs,
		container.AuditLogs,
		UnconfiguredSMSSender{},
		config.IsDevelopment(),
	)

	return container
}
