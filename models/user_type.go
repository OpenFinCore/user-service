package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserType (table: user_types) is either global (ServiceProviderID == nil)
// or scoped to a single tenant.
//
// (service_provider_id, id) carries a composite unique index
// (uq_user_types_tenant_id) so that tenant-scoped children (e.g. users)
// can declare a composite foreign key against it, guaranteeing the
// referenced user_type belongs to the same tenant.
type UserType struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid();uniqueIndex:uq_user_types_tenant_id,priority:2" json:"id"`
	ServiceProviderID *uuid.UUID       `gorm:"column:service_provider_id;type:uuid;index:idx_user_types_tenant;uniqueIndex:uq_user_types_tenant_id,priority:1" json:"service_provider_id,omitempty"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	Name        string  `gorm:"column:name;size:64;not null;check:chk_user_type_name,btrim(name) <> ''" json:"name"`
	Label       string  `gorm:"column:label;size:100;not null;check:chk_user_type_label,btrim(label) <> ''" json:"label"`
	Description *string `gorm:"column:description" json:"description,omitempty"`

	StatusID int16           `gorm:"column:status_id;not null" json:"status_id"`
	Status   *UserTypeStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

func (UserType) TableName() string { return "user_types" }
