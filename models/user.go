package models

import (
	"net"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User (table: users).
//
// (service_provider_id, id) carries a composite unique index
// (uq_users_tenant_id) so tenant-scoped children (devices, sessions,
// refresh tokens, password reset tokens) can FK against it per-tenant.
type User struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid();uniqueIndex:uq_users_tenant_id,priority:2" json:"id"`
	ServiceProviderID uuid.UUID        `gorm:"column:service_provider_id;type:uuid;not null;index:idx_users_tenant;uniqueIndex:uq_users_tenant_id,priority:1" json:"service_provider_id"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	UserTypeID *uuid.UUID `gorm:"column:user_type_id;type:uuid;index:idx_users_type" json:"user_type_id,omitempty"`
	UserType   *UserType  `gorm:"foreignKey:UserTypeID;references:ID;constraint:OnDelete:SET NULL" json:"user_type,omitempty"`

	RoleID uuid.UUID `gorm:"column:role_id;type:uuid;not null;index:idx_users_role" json:"role_id"`
	Role   *Role     `gorm:"foreignKey:RoleID;references:ID;constraint:OnDelete:RESTRICT" json:"role,omitempty"`

	Username    *string `gorm:"column:username;size:50;check:chk_username_not_blank,username IS NULL OR btrim(username) <> ''" json:"username,omitempty"`
	Email       string  `gorm:"column:email;type:citext;not null" json:"email"`
	PhoneNumber *string `gorm:"column:phone_number;size:32" json:"phone_number,omitempty"`
	FirstName   *string `gorm:"column:first_name;size:100" json:"first_name,omitempty"`
	LastName    *string `gorm:"column:last_name;size:100" json:"last_name,omitempty"`
	ProfileURL  *string `gorm:"column:profile_url;size:500" json:"profile_url,omitempty"`

	CountryID *uuid.UUID `gorm:"column:country_id;type:uuid;index:idx_users_country" json:"country_id,omitempty"`
	Country   *Country   `gorm:"foreignKey:CountryID;references:ID;constraint:OnDelete:RESTRICT" json:"country,omitempty"`

	PasswordHash      string     `gorm:"column:password_hash;not null" json:"-"`
	PasswordAlgo      string     `gorm:"column:password_algo;size:32;not null;default:argon2id;check:chk_password_algo,password_algo IN ('argon2id','bcrypt')" json:"password_algo"`
	PasswordUpdatedAt *time.Time `gorm:"column:password_updated_at" json:"password_updated_at,omitempty"`
	PasswordExpiresAt *time.Time `gorm:"column:password_expires_at" json:"password_expires_at,omitempty"`

	EmailVerifiedAt *time.Time `gorm:"column:email_verified_at" json:"email_verified_at,omitempty"`
	PhoneVerifiedAt *time.Time `gorm:"column:phone_verified_at" json:"phone_verified_at,omitempty"`

	TwoFactorStatusID        int16            `gorm:"column:two_factor_status_id;not null" json:"two_factor_status_id"`
	TwoFactorStatus          *TwoFactorStatus `gorm:"foreignKey:TwoFactorStatusID;references:ID;constraint:OnDelete:RESTRICT" json:"two_factor_status,omitempty"`
	TwoFactorSecretEncrypted *string          `gorm:"column:two_factor_secret_encrypted" json:"-"`

	StatusID int16       `gorm:"column:status_id;not null;index:idx_users_status" json:"status_id"`
	Status   *UserStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	FailedLoginAttempts int16      `gorm:"column:failed_login_attempts;not null;default:0;check:chk_failed_login_attempts,failed_login_attempts >= 0" json:"failed_login_attempts"`
	LockedUntil         *time.Time `gorm:"column:locked_until" json:"locked_until,omitempty"`
	LastLoginAt         *time.Time `gorm:"column:last_login_at;index:idx_users_last_login" json:"last_login_at,omitempty"`
	LastLoginIP         *net.IP    `gorm:"column:last_login_ip;type:inet" json:"last_login_ip,omitempty"`

	CreatedBy *uuid.UUID `gorm:"column:created_by;type:uuid" json:"created_by,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	Devices             []Device             `gorm:"foreignKey:UserID;references:ID" json:"devices,omitempty"`
	Sessions            []UserSession        `gorm:"foreignKey:UserID;references:ID" json:"sessions,omitempty"`
	PasswordResetTokens []PasswordResetToken `gorm:"foreignKey:UserID;references:ID" json:"password_reset_tokens,omitempty"`
}

func (User) TableName() string { return "users" }
