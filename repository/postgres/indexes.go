package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
)

// indexSpec is one secondary index the schema requires.
type indexSpec struct {
	Name string
	// SQL creates the index; it must define exactly what the schema
	// (docs/user_service_schema.sql) specifies.
	SQL string
	// Before runs ahead of creating a missing/changed index, e.g. to remove
	// rows that would violate a new unique index.
	Before string
}

// secondaryIndexes is the single source of truth for every non-primary,
// non-FK-constraint index (audit_logs' partitioned indexes excepted, they
// live in postMigrationSQL). Model structs deliberately carry no `index:`
// tags for these: GORM would create single-column indexes under the same
// names first, and CREATE INDEX IF NOT EXISTS would then silently keep the
// wrong definition. reconcileIndexes compares each one with what the
// database has and rebuilds any that differ.
var secondaryIndexes = []indexSpec{
	{Name: "idx_service_providers_status", SQL: `CREATE INDEX idx_service_providers_status ON service_providers (status_id) WHERE deleted_at IS NULL`},
	{Name: "idx_user_types_tenant", SQL: `CREATE INDEX idx_user_types_tenant ON user_types (service_provider_id) WHERE deleted_at IS NULL`},
	{Name: "idx_permissions_active", SQL: `CREATE INDEX idx_permissions_active ON permissions (id) WHERE is_active = TRUE`},
	{Name: "idx_roles_tenant", SQL: `CREATE INDEX idx_roles_tenant ON roles (service_provider_id) WHERE deleted_at IS NULL`},
	{Name: "idx_roles_active", SQL: `CREATE INDEX idx_roles_active ON roles (service_provider_id, is_active) WHERE deleted_at IS NULL`},
	{Name: "idx_countries_active_order", SQL: `CREATE INDEX idx_countries_active_order ON countries (display_order, name) WHERE is_active = TRUE`},

	{Name: "idx_users_tenant", SQL: `CREATE INDEX idx_users_tenant ON users (service_provider_id) WHERE deleted_at IS NULL`},
	{Name: "idx_users_role", SQL: `CREATE INDEX idx_users_role ON users (service_provider_id, role_id) WHERE deleted_at IS NULL`},
	{Name: "idx_users_status", SQL: `CREATE INDEX idx_users_status ON users (service_provider_id, status_id) WHERE deleted_at IS NULL`},
	{Name: "idx_users_type", SQL: `CREATE INDEX idx_users_type ON users (service_provider_id, user_type_id) WHERE deleted_at IS NULL`},
	{Name: "idx_users_country", SQL: `CREATE INDEX idx_users_country ON users (country_id)`},
	{Name: "idx_users_last_login", SQL: `CREATE INDEX idx_users_last_login ON users (service_provider_id, last_login_at)`},
	// One active account per phone number per tenant; also serves the
	// registration "is this phone taken" lookup.
	{Name: "uq_users_active_phone", SQL: `CREATE UNIQUE INDEX uq_users_active_phone ON users (service_provider_id, phone_number) WHERE phone_number IS NOT NULL AND deleted_at IS NULL`},

	{Name: "idx_role_permissions_role", SQL: `CREATE INDEX idx_role_permissions_role ON role_permissions (role_id)`},
	{Name: "idx_role_permissions_permission", SQL: `CREATE INDEX idx_role_permissions_permission ON role_permissions (permission_id)`},
	{Name: "idx_role_permissions_active", SQL: `CREATE INDEX idx_role_permissions_active ON role_permissions (role_id, permission_id) WHERE revoked_at IS NULL`},

	{Name: "idx_devices_user", SQL: `CREATE INDEX idx_devices_user ON devices (service_provider_id, user_id)`},
	{Name: "idx_devices_status", SQL: `CREATE INDEX idx_devices_status ON devices (service_provider_id, status_id)`},
	{Name: "idx_devices_last_seen", SQL: `CREATE INDEX idx_devices_last_seen ON devices (service_provider_id, last_seen_at)`},
	// At most one pending (not yet linked to a user) row per tenant +
	// device: the ON CONFLICT target of DeviceService.RegisterPending.
	{
		Name: "uq_devices_pending_device",
		SQL:  `CREATE UNIQUE INDEX uq_devices_pending_device ON devices (service_provider_id, device_uuid) WHERE user_id IS NULL`,
		Before: `DELETE FROM devices d USING devices k
WHERE d.user_id IS NULL AND k.user_id IS NULL
  AND d.service_provider_id = k.service_provider_id AND d.device_uuid = k.device_uuid
  AND (COALESCE(d.last_seen_at, d.created_at), d.id) < (COALESCE(k.last_seen_at, k.created_at), k.id)`,
	},

	{Name: "idx_sessions_user", SQL: `CREATE INDEX idx_sessions_user ON user_sessions (service_provider_id, user_id)`},
	{Name: "idx_sessions_device", SQL: `CREATE INDEX idx_sessions_device ON user_sessions (service_provider_id, device_id)`},
	{Name: "idx_sessions_status", SQL: `CREATE INDEX idx_sessions_status ON user_sessions (service_provider_id, status_id)`},
	{Name: "idx_sessions_active", SQL: `CREATE INDEX idx_sessions_active ON user_sessions (service_provider_id, user_id, expires_at) WHERE revoked_at IS NULL`},

	{Name: "idx_refresh_tokens_session", SQL: `CREATE INDEX idx_refresh_tokens_session ON refresh_tokens (service_provider_id, session_id)`},
	{Name: "idx_refresh_tokens_active", SQL: `CREATE INDEX idx_refresh_tokens_active ON refresh_tokens (service_provider_id, session_id, expires_at) WHERE revoked_at IS NULL`},

	{Name: "idx_password_reset_tokens_user", SQL: `CREATE INDEX idx_password_reset_tokens_user ON password_reset_tokens (service_provider_id, user_id)`},
	{Name: "idx_password_reset_tokens_active", SQL: `CREATE INDEX idx_password_reset_tokens_active ON password_reset_tokens (service_provider_id, user_id, expires_at) WHERE used_at IS NULL`},
}

// obsoleteIndexes were created by older model tags and are fully covered by
// a composite index above (every composite starts with service_provider_id).
var obsoleteIndexes = []string{"idx_devices_tenant", "idx_sessions_tenant"}

// reconcileIndexes makes the database's secondary indexes match
// secondaryIndexes: missing ones are created, ones whose definition differs
// are rebuilt, obsolete ones dropped. Matching indexes are left untouched,
// so this is cheap on every startup.
func reconcileIndexes(db *sql.DB) error {
	for _, spec := range secondaryIndexes {
		var current string
		err := db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`, spec.Name).Scan(&current)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			current = ""
		case err != nil:
			return fmt.Errorf("reading index %s: %w", spec.Name, err)
		}
		if current != "" && normalizeIndexDef(current) == normalizeIndexDef(spec.SQL) {
			continue
		}

		if spec.Before != "" {
			if _, err := db.Exec(spec.Before); err != nil {
				return fmt.Errorf("preparing index %s: %w", spec.Name, err)
			}
		}
		if current != "" {
			log.Printf("index %s differs from the schema, rebuilding (had: %s)", spec.Name, current)
			if _, err := db.Exec(`DROP INDEX IF EXISTS ` + spec.Name); err != nil {
				return fmt.Errorf("dropping index %s: %w", spec.Name, err)
			}
		} else {
			log.Printf("creating index %s", spec.Name)
		}
		if _, err := db.Exec(spec.SQL); err != nil {
			return fmt.Errorf("creating index %s: %w", spec.Name, err)
		}
	}
	for _, name := range obsoleteIndexes {
		if _, err := db.Exec(`DROP INDEX IF EXISTS ` + name); err != nil {
			return fmt.Errorf("dropping obsolete index %s: %w", name, err)
		}
	}
	return nil
}

// normalizeIndexDef reduces an index definition to a comparable form:
// Postgres reports "CREATE INDEX x ON public.t USING btree (a, b) WHERE
// (c IS NULL)" for what we write as "CREATE INDEX x ON t (a, b) WHERE c IS
// NULL".
func normalizeIndexDef(def string) string {
	def = strings.ToLower(def)
	def = strings.ReplaceAll(def, "public.", "")
	def = strings.ReplaceAll(def, " using btree", "")
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '(', ')':
			return -1
		}
		return r
	}, def)
}
