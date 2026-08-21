package models

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken (table: refresh_tokens).
type RefreshToken struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid();uniqueIndex:uq_refresh_tokens_tenant_id,priority:2" json:"id"`
	ServiceProviderID uuid.UUID        `gorm:"column:service_provider_id;type:uuid;not null;uniqueIndex:uq_refresh_tokens_tenant_id,priority:1" json:"service_provider_id"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	SessionID uuid.UUID    `gorm:"column:session_id;type:uuid;not null;index:idx_refresh_tokens_session" json:"session_id"`
	Session   *UserSession `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE" json:"session,omitempty"`

	TokenHash string `gorm:"column:token_hash;not null;uniqueIndex" json:"-"`

	StatusID int16               `gorm:"column:status_id;not null" json:"status_id"`
	Status   *RefreshTokenStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	ReplacedByID *uuid.UUID    `gorm:"column:replaced_by_id;type:uuid" json:"replaced_by_id,omitempty"`
	ReplacedBy   *RefreshToken `gorm:"foreignKey:ReplacedByID;references:ID;constraint:OnDelete:SET NULL" json:"replaced_by,omitempty"`

	IssuedAt  time.Time  `gorm:"column:issued_at;not null;default:now()" json:"issued_at"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null;check:chk_refresh_token_expiry,expires_at > issued_at" json:"expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
