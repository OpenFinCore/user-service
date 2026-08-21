package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ServiceProvider is a tenant (table: service_providers).
type ServiceProvider struct {
	ID     uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name   string    `gorm:"column:name;size:150;not null;check:chk_service_provider_name,btrim(name) <> ''" json:"name"`
	Domain *string   `gorm:"column:domain;type:citext" json:"domain,omitempty"`

	StatusID int16                  `gorm:"column:status_id;not null" json:"status_id"`
	Status   *ServiceProviderStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	Config datatypes.JSON `gorm:"column:config;type:jsonb;not null;default:'{}'" json:"config,omitempty"`

	CreatedBy *uuid.UUID `gorm:"column:created_by;type:uuid" json:"created_by,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// Reverse relations, opt-in via Preload.
	UserTypes []UserType `gorm:"foreignKey:ServiceProviderID;references:ID" json:"user_types,omitempty"`
	Roles     []Role     `gorm:"foreignKey:ServiceProviderID;references:ID" json:"roles,omitempty"`
	Users     []User     `gorm:"foreignKey:ServiceProviderID;references:ID" json:"users,omitempty"`
}

func (ServiceProvider) TableName() string { return "service_providers" }
