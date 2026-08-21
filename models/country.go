package models

import (
	"time"

	"github.com/google/uuid"
)

// Country (table: countries) is a global reference table.
type Country struct {
	ID                  uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	IsoAlpha2           string    `gorm:"column:iso_alpha2;type:char(2);not null;uniqueIndex;check:chk_country_alpha2,iso_alpha2 ~ '^[A-Z]{2}$'" json:"iso_alpha2"`
	IsoAlpha3           string    `gorm:"column:iso_alpha3;type:char(3);not null;uniqueIndex;check:chk_country_alpha3,iso_alpha3 ~ '^[A-Z]{3}$'" json:"iso_alpha3"`
	IsoNumeric          string    `gorm:"column:iso_numeric;type:char(3);not null;uniqueIndex;check:chk_country_numeric,iso_numeric ~ '^[0-9]{3}$'" json:"iso_numeric"`
	Name                string    `gorm:"column:name;size:150;not null;check:chk_country_name,btrim(name) <> ''" json:"name"`
	ShortName           *string   `gorm:"column:short_name;size:100" json:"short_name,omitempty"`
	CallingName         *string   `gorm:"column:calling_name;size:8" json:"calling_name,omitempty"`
	DefaultCurrencyName *string   `gorm:"column:default_currency_name;type:char(3)" json:"default_currency_name,omitempty"`
	IsActive            bool      `gorm:"column:is_active;not null;default:true" json:"is_active"`
	DisplayOrder        int       `gorm:"column:display_order;not null;default:0;check:chk_country_display_order,display_order >= 0" json:"display_order"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (Country) TableName() string { return "countries" }
