package models

import (
	"time"

	"github.com/google/uuid"
)

// RolePermission (table: role_permissions) grants a Permission to a Role.
// ServiceProviderID mirrors the owning role's tenant (nullable for global
// system roles) and is kept denormalized to support row-level security.
type RolePermission struct {
	ID                uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ServiceProviderID *uuid.UUID `gorm:"column:service_provider_id;type:uuid;index" json:"service_provider_id,omitempty"`

	RoleID uuid.UUID `gorm:"column:role_id;type:uuid;not null;uniqueIndex:uq_role_permissions,priority:1;index" json:"role_id"`
	Role   *Role     `gorm:"foreignKey:RoleID;references:ID;constraint:OnDelete:CASCADE" json:"role,omitempty"`

	PermissionID uuid.UUID   `gorm:"column:permission_id;type:uuid;not null;uniqueIndex:uq_role_permissions,priority:2;index" json:"permission_id"`
	Permission   *Permission `gorm:"foreignKey:PermissionID;references:ID;constraint:OnDelete:RESTRICT" json:"permission,omitempty"`

	GrantedBy *uuid.UUID `gorm:"column:granted_by;type:uuid" json:"granted_by,omitempty"`
	GrantedAt time.Time  `gorm:"column:granted_at;not null;default:now()" json:"granted_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at;check:chk_role_permission_dates,revoked_at IS NULL OR revoked_at >= granted_at" json:"revoked_at,omitempty"`
}

func (RolePermission) TableName() string { return "role_permissions" }
