package models

import (
	"net"
	"time"

	"github.com/google/uuid"
)

// Device (table: devices).
//
// (service_provider_id, id) carries a composite unique index
// (uq_devices_tenant_id) so `user_sessions` can FK against it per-tenant.
type Device struct {
	ID                uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid();uniqueIndex:uq_devices_tenant_id,priority:2" json:"id"`
	ServiceProviderID uuid.UUID        `gorm:"column:service_provider_id;type:uuid;not null;index:idx_devices_tenant;uniqueIndex:uq_devices_tenant_id,priority:1" json:"service_provider_id"`
	ServiceProvider   *ServiceProvider `gorm:"foreignKey:ServiceProviderID;references:ID;constraint:OnDelete:CASCADE" json:"service_provider,omitempty"`

	UserID uuid.UUID `gorm:"column:user_id;type:uuid;not null;index:idx_devices_user" json:"user_id"`
	User   *User     `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE" json:"user,omitempty"`

	DeviceUUID string  `gorm:"column:device_uuid;size:128;not null" json:"device_uuid"`
	DeviceName *string `gorm:"column:device_name;size:150" json:"device_name,omitempty"`
	DeviceType string  `gorm:"column:device_type;size:32;not null;check:chk_device_type,device_type IN ('MOBILE_IOS','MOBILE_ANDROID','WEB','DESKTOP','API_CLIENT')" json:"device_type"`
	OSName     *string `gorm:"column:os_name;size:64" json:"os_name,omitempty"`
	OSVersion  *string `gorm:"column:os_version;size:32" json:"os_version,omitempty"`
	AppVersion *string `gorm:"column:app_version;size:32" json:"app_version,omitempty"`
	FCMToken   *string `gorm:"column:fcm_token" json:"-"`
	IPAddress  *net.IP `gorm:"column:ip_address;type:inet" json:"ip_address,omitempty"`
	UserAgent  *string `gorm:"column:user_agent" json:"user_agent,omitempty"`
	IsTrusted  bool    `gorm:"column:is_trusted;not null;default:false" json:"is_trusted"`

	StatusID int16         `gorm:"column:status_id;not null;index:idx_devices_status" json:"status_id"`
	Status   *DeviceStatus `gorm:"foreignKey:StatusID;references:ID;constraint:OnDelete:RESTRICT" json:"status,omitempty"`

	LastSeenAt *time.Time `gorm:"column:last_seen_at;index:idx_devices_last_seen" json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
}

func (Device) TableName() string { return "devices" }
