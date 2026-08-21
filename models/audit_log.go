package models

import (
	"net"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// AuditLog (table: audit_logs) is an append-only log, range-partitioned by
// created_at in the schema (docs/user_service_schema.sql). The partitioned
// parent + default partition are created by the supplemental raw-SQL
// migration (repository/postgres/migrations.go) rather than by GORM
// AutoMigrate, which has no notion of partitioning - this model is marked
// to skip migration and is used purely for querying/inserting via the
// generic repository.
type AuditLog struct {
	ID                int64            `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ServiceProviderID uuid.UUID        `gorm:"column:service_provider_id;type:uuid;not null" json:"service_provider_id"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	ActorID *uuid.UUID `gorm:"column:actor_id;type:uuid" json:"actor_id,omitempty"`
	Actor   *User      `gorm:"foreignKey:ActorID;references:ID" json:"actor,omitempty"`

	Action       string         `gorm:"column:action;size:128;not null" json:"action"`
	ResourceType string         `gorm:"column:resource_type;size:64;not null" json:"resource_type"`
	ResourceID   *uuid.UUID     `gorm:"column:resource_id;type:uuid" json:"resource_id,omitempty"`
	Payload      datatypes.JSON `gorm:"column:payload;type:jsonb;not null;default:'{}'" json:"payload,omitempty"`
	IPAddress    *net.IP        `gorm:"column:ip_address;type:inet" json:"ip_address,omitempty"`
	UserAgent    *string        `gorm:"column:user_agent" json:"user_agent,omitempty"`

	CreatedAt time.Time `gorm:"column:created_at;primaryKey;not null;default:now()" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }
