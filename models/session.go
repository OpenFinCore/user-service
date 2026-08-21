package models

import (
	"net"
	"time"

	"github.com/google/uuid"
)

// UserSession (table: user_sessions).
//
// (service_provider_id, id) carries a composite unique index
// (uq_sessions_tenant_id) so `refresh_tokens` can FK against it per-tenant.
type UserSession struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid();uniqueIndex:uq_sessions_tenant_id,priority:2" json:"id"`
	ServiceProviderID uuid.UUID        `gorm:"column:service_provider_id;type:uuid;not null;index:idx_sessions_tenant;uniqueIndex:uq_sessions_tenant_id,priority:1" json:"service_provider_id"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	UserID uuid.UUID `gorm:"column:user_id;type:uuid;not null;index:idx_sessions_user" json:"user_id"`
	User   *User     `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE" json:"user,omitempty"`

	DeviceID *uuid.UUID `gorm:"column:device_id;type:uuid;index:idx_sessions_device" json:"device_id,omitempty"`
	Device   *Device    `gorm:"foreignKey:DeviceID;references:ID;constraint:OnDelete:SET NULL" json:"device,omitempty"`

	StatusID int16          `gorm:"column:status_id;not null;index:idx_sessions_status" json:"status_id"`
	Status   *SessionStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	IPAddress *net.IP `gorm:"column:ip_address;type:inet" json:"ip_address,omitempty"`
	UserAgent *string `gorm:"column:user_agent" json:"user_agent,omitempty"`

	IssuedAt  time.Time  `gorm:"column:issued_at;not null;default:now()" json:"issued_at"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null;check:chk_session_expiry,expires_at > issued_at" json:"expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at;check:chk_session_revoked,revoked_at IS NULL OR revoked_at >= issued_at" json:"revoked_at,omitempty"`

	RefreshTokens []RefreshToken `gorm:"foreignKey:SessionID;references:ID" json:"refresh_tokens,omitempty"`
}

func (UserSession) TableName() string { return "user_sessions" }
