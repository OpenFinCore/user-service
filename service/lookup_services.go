package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

// Named wrappers around the generic engine for the eight status/lookup
// tables. Each is discoverable by name (ServiceProviderStatusService, ...)
// and free to grow entity-specific business methods later without
// disturbing the generic CRUD it wraps.

type ServiceProviderStatusService struct {
	*CRUDService[models.ServiceProviderStatus]
}

func NewServiceProviderStatusService(store repository.Store) *ServiceProviderStatusService {
	return &ServiceProviderStatusService{NewCRUDService[models.ServiceProviderStatus](store)}
}

type UserTypeStatusService struct {
	*CRUDService[models.UserTypeStatus]
}

func NewUserTypeStatusService(store repository.Store) *UserTypeStatusService {
	return &UserTypeStatusService{NewCRUDService[models.UserTypeStatus](store)}
}

type TwoFactorStatusService struct {
	*CRUDService[models.TwoFactorStatus]
}

func NewTwoFactorStatusService(store repository.Store) *TwoFactorStatusService {
	return &TwoFactorStatusService{NewCRUDService[models.TwoFactorStatus](store)}
}

type UserStatusService struct {
	*CRUDService[models.UserStatus]
}

func NewUserStatusService(store repository.Store) *UserStatusService {
	return &UserStatusService{NewCRUDService[models.UserStatus](store)}
}

type DeviceStatusService struct {
	*CRUDService[models.DeviceStatus]
}

func NewDeviceStatusService(store repository.Store) *DeviceStatusService {
	return &DeviceStatusService{NewCRUDService[models.DeviceStatus](store)}
}

type SessionStatusService struct {
	*CRUDService[models.SessionStatus]
}

func NewSessionStatusService(store repository.Store) *SessionStatusService {
	return &SessionStatusService{NewCRUDService[models.SessionStatus](store)}
}

type RefreshTokenStatusService struct {
	*CRUDService[models.RefreshTokenStatus]
}

func NewRefreshTokenStatusService(store repository.Store) *RefreshTokenStatusService {
	return &RefreshTokenStatusService{NewCRUDService[models.RefreshTokenStatus](store)}
}

type PasswordResetTokenStatusService struct {
	*CRUDService[models.PasswordResetTokenStatus]
}

func NewPasswordResetTokenStatusService(store repository.Store) *PasswordResetTokenStatusService {
	return &PasswordResetTokenStatusService{NewCRUDService[models.PasswordResetTokenStatus](store)}
}
