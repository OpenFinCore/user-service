package models

import (
	"net"
	"time"

	"github.com/google/uuid"
)

// PasswordResetToken (table: password_reset_tokens).
type PasswordResetToken struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ServiceProviderID uuid.UUID        `gorm:"column:service_provider_id;type:uuid;not null" json:"service_provider_id"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	UserID uuid.UUID `gorm:"column:user_id;type:uuid;not null;index:idx_password_reset_tokens_user" json:"user_id"`
	User   *User     `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE" json:"user,omitempty"`

	TokenHash string `gorm:"column:token_hash;not null;uniqueIndex" json:"-"`

	StatusID int16                     `gorm:"column:status_id;not null" json:"status_id"`
	Status   *PasswordResetTokenStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	RequestedIP *net.IP    `gorm:"column:requested_ip;type:inet" json:"requested_ip,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	ExpiresAt   time.Time  `gorm:"column:expires_at;not null;check:chk_password_reset_expiry,expires_at > created_at" json:"expires_at"`
	UsedAt      *time.Time `gorm:"column:used_at" json:"used_at,omitempty"`
}

func (PasswordResetToken) TableName() string { return "password_reset_tokens" }
