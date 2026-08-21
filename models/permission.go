package models

import (
	"time"

	"github.com/google/uuid"
)

// Permission (table: permissions) is global - not tenant scoped.
type Permission struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"column:name;size:150;not null;uniqueIndex;check:chk_permission_name,btrim(name) <> ''" json:"name"`
	Label       string    `gorm:"column:label;size:150;not null;check:chk_permission_label,btrim(label) <> ''" json:"label"`
	Description *string   `gorm:"column:description" json:"description,omitempty"`
	IsSystem    bool      `gorm:"column:is_system;not null;default:false" json:"is_system"`
	IsActive    bool      `gorm:"column:is_active;not null;default:true" json:"is_active"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (Permission) TableName() string { return "permissions" }
