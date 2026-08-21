package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type DeviceService struct{ *CRUDService[models.Device] }

func NewDeviceService(store repository.Store) *DeviceService {
	return &DeviceService{NewCRUDService[models.Device](store)}
}

type UserSessionService struct {
	*CRUDService[models.UserSession]
}

func NewUserSessionService(store repository.Store) *UserSessionService {
	return &UserSessionService{NewCRUDService[models.UserSession](store)}
}

type RefreshTokenService struct {
	*CRUDService[models.RefreshToken]
}

func NewRefreshTokenService(store repository.Store) *RefreshTokenService {
	return &RefreshTokenService{NewCRUDService[models.RefreshToken](store)}
}

type PasswordResetTokenService struct {
	*CRUDService[models.PasswordResetToken]
}

func NewPasswordResetTokenService(store repository.Store) *PasswordResetTokenService {
	return &PasswordResetTokenService{NewCRUDService[models.PasswordResetToken](store)}
}

type AuditLogService struct{ *CRUDService[models.AuditLog] }

func NewAuditLogService(store repository.Store) *AuditLogService {
	return &AuditLogService{NewCRUDService[models.AuditLog](store)}
}
