package service

import (
	"fmt"

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

	Seeds SeedsService
}

func NewServiceContainer() *Container {
	config := conf.GetConfig()
	log := logger.LoggerInit(config.Logger)
	store := pgstore.SharedStore()

	container := &Container{
		Store: store,
		Log:   log,

		ServiceProviderStatuses:    NewServiceProviderStatusService(store),
		UserTypeStatuses:           NewUserTypeStatusService(store),
		TwoFactorStatuses:          NewTwoFactorStatusService(store),
		UserStatuses:               NewUserStatusService(store),
		DeviceStatuses:             NewDeviceStatusService(store),
		SessionStatuses:            NewSessionStatusService(store),
		RefreshTokenStatuses:       NewRefreshTokenStatusService(store),
		PasswordResetTokenStatuses: NewPasswordResetTokenStatusService(store),

		ServiceProviders: NewServiceProviderService(store),
		UserTypes:        NewUserTypeService(store),
		Permissions:      NewPermissionService(store),
		Roles:            NewRoleService(store),
		RolePermissions:  NewRolePermissionService(store),
		Countries:        NewCountryService(store),
		Users:            NewUserService(store),

		Devices:             NewDeviceService(store),
		UserSessions:        NewUserSessionService(store),
		RefreshTokens:       NewRefreshTokenService(store),
		PasswordResetTokens: NewPasswordResetTokenService(store),
		AuditLogs:           NewAuditLogService(store),
	}

	container.Seeds = NewSeedsService(store, config)
	if err := container.Seeds.SeedAll(); err != nil {
		panic(fmt.Errorf("seeding failed: %w", err))
	}

	return container
}
