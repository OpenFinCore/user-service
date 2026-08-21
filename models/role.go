package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Role (table: roles) is either a global system role (ServiceProviderID ==
// nil, IsSystem == true) or a tenant-owned role.
//
// (service_provider_id, id) carries a composite unique index
// (uq_roles_tenant_id) so `users` can FK against it per-tenant.
type Role struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid();uniqueIndex:uq_roles_tenant_id,priority:2" json:"id"`
	ServiceProviderID *uuid.UUID       `gorm:"column:service_provider_id;type:uuid;index:idx_roles_tenant;uniqueIndex:uq_roles_tenant_id,priority:1" json:"service_provider_id,omitempty"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	Name        string  `gorm:"column:name;size:64;not null;check:chk_role_name,btrim(name) <> ''" json:"name"`
	Label       string  `gorm:"column:label;size:100;not null;check:chk_role_label,btrim(label) <> ''" json:"label"`
	Description *string `gorm:"column:description" json:"description,omitempty"`
	IsSystem    bool    `gorm:"column:is_system;not null;default:false" json:"is_system"`
	IsActive    bool    `gorm:"column:is_active;not null;default:true" json:"is_active"`

	CreatedBy *uuid.UUID `gorm:"column:created_by;type:uuid" json:"created_by,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	RolePermissions []RolePermission `gorm:"foreignKey:RoleID;references:ID" json:"role_permissions,omitempty"`
}

func (Role) TableName() string { return "roles" }
