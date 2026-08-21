package postgres

// preMigrationSQL creates the extensions and helper functions that GORM's
// AutoMigrate cannot express and that later columns/triggers depend on
// (gen_random_uuid(), the citext type, the updated_at trigger function and
// the RLS helper function). It must run before AutoMigrate.
const preMigrationSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION current_service_provider_id()
RETURNS UUID
LANGUAGE sql
STABLE
AS $$
    SELECT NULLIF(
        current_setting('app.current_service_provider_id', true),
        ''
    )::UUID;
$$;
`

// postMigrationSQL adds everything the schema in docs/user_service_schema.sql
// specifies that GORM struct tags cannot express: partial unique indexes
// (a WHERE clause), the multi-tenant "same tenant" composite foreign keys,
// updated_at triggers, the append-only partitioned audit_logs table, and
// row-level security policies. It must run after AutoMigrate has created
// the plain tables/columns.
const postMigrationSQL = `
-- Partial unique indexes (WHERE clauses aren't expressible via GORM tags).
CREATE UNIQUE INDEX IF NOT EXISTS uq_service_providers_domain
ON service_providers(domain)
WHERE domain IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_types_global_name
ON user_types(name)
WHERE service_provider_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_types_tenant_name
ON user_types(service_provider_id, name)
WHERE service_provider_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_roles_tenant_name
ON roles(service_provider_id, name)
WHERE service_provider_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_roles_global_system_name
ON roles(name)
WHERE service_provider_id IS NULL AND is_system = TRUE AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_active_email
ON users(service_provider_id, email)
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_active_username
ON users(service_provider_id, lower(username))
WHERE username IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_devices_user_device
ON devices(service_provider_id, user_id, device_uuid);

-- Cross-table "same tenant" composite foreign keys: guarantee a child row's
-- service_provider_id always matches the tenant of the row it points at.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_users_role_same_tenant' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT fk_users_role_same_tenant
            FOREIGN KEY (service_provider_id, role_id)
            REFERENCES roles(service_provider_id, id)
            ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_users_user_type_same_tenant' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT fk_users_user_type_same_tenant
            FOREIGN KEY (service_provider_id, user_type_id)
            REFERENCES user_types(service_provider_id, id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_users_created_by_same_tenant' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT fk_users_created_by_same_tenant
            FOREIGN KEY (service_provider_id, created_by)
            REFERENCES users(service_provider_id, id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_devices_user_same_tenant' AND conrelid = 'devices'::regclass
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT fk_devices_user_same_tenant
            FOREIGN KEY (service_provider_id, user_id)
            REFERENCES users(service_provider_id, id)
            ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sessions_user_same_tenant' AND conrelid = 'user_sessions'::regclass
    ) THEN
        ALTER TABLE user_sessions
            ADD CONSTRAINT fk_sessions_user_same_tenant
            FOREIGN KEY (service_provider_id, user_id)
            REFERENCES users(service_provider_id, id)
            ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sessions_device_same_tenant' AND conrelid = 'user_sessions'::regclass
    ) THEN
        ALTER TABLE user_sessions
            ADD CONSTRAINT fk_sessions_device_same_tenant
            FOREIGN KEY (service_provider_id, device_id)
            REFERENCES devices(service_provider_id, id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_refresh_tokens_session_same_tenant' AND conrelid = 'refresh_tokens'::regclass
    ) THEN
        ALTER TABLE refresh_tokens
            ADD CONSTRAINT fk_refresh_tokens_session_same_tenant
            FOREIGN KEY (service_provider_id, session_id)
            REFERENCES user_sessions(service_provider_id, id)
            ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_refresh_tokens_replaced_by' AND conrelid = 'refresh_tokens'::regclass
    ) THEN
        ALTER TABLE refresh_tokens
            ADD CONSTRAINT fk_refresh_tokens_replaced_by
            FOREIGN KEY (service_provider_id, replaced_by_id)
            REFERENCES refresh_tokens(service_provider_id, id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_password_reset_user_same_tenant' AND conrelid = 'password_reset_tokens'::regclass
    ) THEN
        ALTER TABLE password_reset_tokens
            ADD CONSTRAINT fk_password_reset_user_same_tenant
            FOREIGN KEY (service_provider_id, user_id)
            REFERENCES users(service_provider_id, id)
            ON DELETE CASCADE;
    END IF;
END;
$$;

-- updated_at triggers for every table that carries an updated_at column.
DROP TRIGGER IF EXISTS trg_service_providers_updated_at ON service_providers;
CREATE TRIGGER trg_service_providers_updated_at
BEFORE UPDATE ON service_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_user_types_updated_at ON user_types;
CREATE TRIGGER trg_user_types_updated_at
BEFORE UPDATE ON user_types FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_roles_updated_at ON roles;
CREATE TRIGGER trg_roles_updated_at
BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_permissions_updated_at ON permissions;
CREATE TRIGGER trg_permissions_updated_at
BEFORE UPDATE ON permissions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_devices_updated_at ON devices;
CREATE TRIGGER trg_devices_updated_at
BEFORE UPDATE ON devices FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_countries_updated_at ON countries;
CREATE TRIGGER trg_countries_updated_at
BEFORE UPDATE ON countries FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Append-only, monthly range-partitioned audit log. GORM AutoMigrate has no
-- notion of partitioning, so the AuditLog model is excluded from the
-- migrate list and this table is created here instead, verbatim from the
-- schema.
CREATE TABLE IF NOT EXISTS audit_logs (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY,
    service_provider_id UUID NOT NULL
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    actor_id            UUID,
    action              VARCHAR(128) NOT NULL,
    resource_type       VARCHAR(64) NOT NULL,
    resource_id         UUID,
    payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address          INET,
    user_agent          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_actor ON audit_logs(service_provider_id, actor_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(service_provider_id, action, created_at);

CREATE TABLE IF NOT EXISTS audit_logs_default PARTITION OF audit_logs DEFAULT;

-- Row-level security: every tenant-owned table only ever shows rows for
-- app.current_service_provider_id (set per-transaction by the API layer
-- once it exists). FORCE means even the table owner is subject to this,
-- so local/manual testing should connect as a superuser (which always
-- bypasses RLS) until that session-variable wiring is built.
ALTER TABLE user_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_types FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_user_types_isolation ON user_types;
CREATE POLICY p_user_types_isolation ON user_types
    USING (service_provider_id IS NULL OR service_provider_id = current_service_provider_id());

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_roles_isolation ON roles;
CREATE POLICY p_roles_isolation ON roles
    USING (service_provider_id IS NULL OR service_provider_id = current_service_provider_id());

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_users_isolation ON users;
CREATE POLICY p_users_isolation ON users
    USING (service_provider_id = current_service_provider_id());

ALTER TABLE role_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_permissions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_role_permissions_isolation ON role_permissions;
CREATE POLICY p_role_permissions_isolation ON role_permissions
    USING (service_provider_id IS NULL OR service_provider_id = current_service_provider_id());

ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_devices_isolation ON devices;
CREATE POLICY p_devices_isolation ON devices
    USING (service_provider_id = current_service_provider_id());

ALTER TABLE user_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_sessions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_user_sessions_isolation ON user_sessions;
CREATE POLICY p_user_sessions_isolation ON user_sessions
    USING (service_provider_id = current_service_provider_id());

ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_refresh_tokens_isolation ON refresh_tokens;
CREATE POLICY p_refresh_tokens_isolation ON refresh_tokens
    USING (service_provider_id = current_service_provider_id());

ALTER TABLE password_reset_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_password_reset_tokens_isolation ON password_reset_tokens;
CREATE POLICY p_password_reset_tokens_isolation ON password_reset_tokens
    USING (service_provider_id = current_service_provider_id());

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_audit_logs_isolation ON audit_logs;
CREATE POLICY p_audit_logs_isolation ON audit_logs
    USING (service_provider_id = current_service_provider_id());
`
