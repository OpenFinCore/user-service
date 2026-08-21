package models

import "time"

// SeedRegistry tracks which seed JSON files have already been applied
// (by content hash) so re-running the seeder is a cheap no-op when nothing
// changed.
type SeedRegistry struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FileName      string    `gorm:"uniqueIndex;not null;size:255" json:"file_name"`
	FileHash      string    `gorm:"not null;size:255" json:"file_hash"`
	LastAppliedAt time.Time `json:"last_applied_at"`
}

func (SeedRegistry) TableName() string { return "seed_registry" }
