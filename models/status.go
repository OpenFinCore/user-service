package models

import "time"

// StatusBase is embedded by every lookup/status table in the schema
// (service_provider_statuses, user_statuses, device_statuses, ...).
// They all share the same shape: a small serial id, a machine-readable
// name, a human label, an optional description and an active flag.
type StatusBase struct {
	ID          int16     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"column:name;size:32;not null;uniqueIndex;check:chk_name_not_blank,btrim(name) <> ''" json:"name"`
	Label       string    `gorm:"column:label;size:64;not null" json:"label"`
	Description *string   `gorm:"column:description" json:"description,omitempty"`
	IsActive    bool      `gorm:"column:is_active;not null;default:true" json:"is_active"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

// ServiceProviderStatus -> table: service_provider_statuses
type ServiceProviderStatus struct {
	StatusBase `gorm:"embedded"`
}

// UserTypeStatus -> table: user_type_statuses
type UserTypeStatus struct {
	StatusBase `gorm:"embedded"`
}

// TwoFactorStatus -> table: two_factor_statuses
type TwoFactorStatus struct {
	StatusBase `gorm:"embedded"`
}

// UserStatus -> table: user_statuses
type UserStatus struct {
	StatusBase `gorm:"embedded"`
}

// DeviceStatus -> table: device_statuses
type DeviceStatus struct {
	StatusBase `gorm:"embedded"`
}

// SessionStatus -> table: session_statuses
type SessionStatus struct {
	StatusBase `gorm:"embedded"`
}

// RefreshTokenStatus -> table: refresh_token_statuses
type RefreshTokenStatus struct {
	StatusBase `gorm:"embedded"`
}

// PasswordResetTokenStatus -> table: password_reset_token_statuses
type PasswordResetTokenStatus struct {
	StatusBase `gorm:"embedded"`
}
