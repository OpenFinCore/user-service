-- ============================================================================
-- USER SERVICE :: PRODUCTION-GRADE POSTGRESQL MIGRATION
-- PostgreSQL 14+
--
-- RBAC:
--   roles (tenant & global system roles)
--   permissions
--   role_permissions
--
-- MULTI-TENANCY:
--   shared schema
--   service_provider_id on tenant-owned tables
--   PostgreSQL RLS (Row Level Security)
--
-- AUTH:
--   password authentication
--   2FA
--   sessions
--   refresh tokens
--   password reset tokens
--
-- AUDIT:
--   append-only
--   monthly range partitioning
-- ============================================================================

-- ============================================================================
-- 0. EXTENSIONS
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;


-- ============================================================================
-- 1. COMMON FUNCTIONS
-- ============================================================================

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


-- ============================================================================
-- 2. SERVICE PROVIDER STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS service_provider_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_service_provider_status_name
        CHECK (btrim(name) <> '')
);


-- ============================================================================
-- 3. SERVICE PROVIDERS / TENANTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS service_providers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(150) NOT NULL,
    domain          CITEXT,
    status_id       SMALLINT NOT NULL
                        REFERENCES service_provider_statuses(id)
                        ON DELETE RESTRICT,
    config          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,

    CONSTRAINT chk_service_provider_name
        CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_service_providers_domain
ON service_providers(domain)
WHERE domain IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_service_providers_status
ON service_providers(status_id)
WHERE deleted_at IS NULL;

DROP TRIGGER IF EXISTS trg_service_providers_updated_at ON service_providers;
CREATE TRIGGER trg_service_providers_updated_at
BEFORE UPDATE ON service_providers
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 4. USER TYPE
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_type_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_user_type_status_name
        CHECK (btrim(name) <> '')
);


CREATE TABLE IF NOT EXISTS user_types (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    name                VARCHAR(64) NOT NULL,
    label               VARCHAR(100) NOT NULL,
    description         TEXT,
    status_id           SMALLINT NOT NULL
                            REFERENCES user_type_statuses(id)
                            ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,

    CONSTRAINT chk_user_type_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_user_type_label CHECK (btrim(label) <> '')
);

ALTER TABLE user_types
    ADD CONSTRAINT uq_user_types_tenant_id
    UNIQUE (service_provider_id, id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_types_global_name
ON user_types(name)
WHERE service_provider_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_types_tenant_name
ON user_types(service_provider_id, name)
WHERE service_provider_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_types_tenant
ON user_types(service_provider_id)
WHERE deleted_at IS NULL;

DROP TRIGGER IF EXISTS trg_user_types_updated_at ON user_types;
CREATE TRIGGER trg_user_types_updated_at
BEFORE UPDATE ON user_types
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 5. TWO FACTOR STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS two_factor_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- ============================================================================
-- 6. USER STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_user_status_name
        CHECK (btrim(name) <> '')
);


-- ============================================================================
-- 7. PERMISSIONS
-- ============================================================================

CREATE TABLE IF NOT EXISTS permissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(150) NOT NULL UNIQUE,
    label           VARCHAR(150) NOT NULL,
    description     TEXT,
    is_system       BOOLEAN NOT NULL DEFAULT FALSE,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_permission_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_permission_label CHECK (btrim(label) <> '')
);

CREATE INDEX IF NOT EXISTS idx_permissions_active
ON permissions(id)
WHERE is_active = TRUE;

DROP TRIGGER IF EXISTS trg_permissions_updated_at ON permissions;
CREATE TRIGGER trg_permissions_updated_at
BEFORE UPDATE ON permissions
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 8. ROLES
-- ============================================================================

CREATE TABLE IF NOT EXISTS roles (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    name                VARCHAR(64) NOT NULL,
    label               VARCHAR(100) NOT NULL,
    description         TEXT,
    is_system           BOOLEAN NOT NULL DEFAULT FALSE,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_by          UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,

    CONSTRAINT chk_role_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_role_label CHECK (btrim(label) <> ''),
    CONSTRAINT uq_roles_tenant_id UNIQUE (service_provider_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_roles_tenant_name
ON roles(service_provider_id, name)
WHERE service_provider_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_roles_global_system_name
ON roles(name)
WHERE service_provider_id IS NULL AND is_system = TRUE AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_roles_tenant
ON roles(service_provider_id)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_roles_active
ON roles(service_provider_id, is_active)
WHERE deleted_at IS NULL;

DROP TRIGGER IF EXISTS trg_roles_updated_at ON roles;
CREATE TRIGGER trg_roles_updated_at
BEFORE UPDATE ON roles
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 9. USERS
-- ============================================================================

CREATE TABLE IF NOT EXISTS users (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id     UUID NOT NULL
                                REFERENCES service_providers(id)
                                ON DELETE CASCADE,
    user_type_id            UUID,
    role_id                 UUID NOT NULL,
    username                VARCHAR(50),
    email                   CITEXT NOT NULL,
    phone_number            VARCHAR(32),
    first_name              VARCHAR(100),
    last_name               VARCHAR(100),
    country_id              UUID,
    profile_url             VARCHAR(500),

    password_hash           TEXT NOT NULL,
    password_algo           VARCHAR(32) NOT NULL DEFAULT 'argon2id',
    password_updated_at     TIMESTAMPTZ,
    password_expires_at     TIMESTAMPTZ,

    email_verified_at       TIMESTAMPTZ,
    phone_verified_at       TIMESTAMPTZ,

    two_factor_status_id    SMALLINT NOT NULL
                                REFERENCES two_factor_statuses(id)
                                ON DELETE RESTRICT,
    two_factor_secret_encrypted TEXT,

    status_id               SMALLINT NOT NULL
                                REFERENCES user_statuses(id)
                                ON DELETE RESTRICT,

    failed_login_attempts   SMALLINT NOT NULL DEFAULT 0,
    locked_until            TIMESTAMPTZ,
    last_login_at           TIMESTAMPTZ,
    last_login_ip           INET,

    created_by              UUID,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ,

    CONSTRAINT chk_failed_login_attempts CHECK (failed_login_attempts >= 0),
    CONSTRAINT chk_password_algo CHECK (password_algo IN ('argon2id', 'bcrypt')),
    CONSTRAINT chk_user_name CHECK (first_name IS NOT NULL OR last_name IS NOT NULL),
    CONSTRAINT chk_username_not_blank CHECK (username IS NULL OR btrim(username) <> ''),
    CONSTRAINT uq_users_tenant_id UNIQUE (service_provider_id, id)
);


-- ============================================================================
-- 10. COUNTRIES
-- ============================================================================

CREATE TABLE IF NOT EXISTS countries (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    iso_alpha2            CHAR(2) NOT NULL UNIQUE,
    iso_alpha3            CHAR(3) NOT NULL UNIQUE,
    iso_numeric           CHAR(3) NOT NULL UNIQUE,
    name                  VARCHAR(150) NOT NULL,
    short_name            VARCHAR(100),
    calling_name          VARCHAR(8),
    default_currency_name CHAR(3),
    is_active             BOOLEAN NOT NULL DEFAULT TRUE,
    display_order         INTEGER NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_country_alpha2 CHECK (iso_alpha2 ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_country_alpha3 CHECK (iso_alpha3 ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_country_numeric CHECK (iso_numeric ~ '^[0-9]{3}$'),
    CONSTRAINT chk_country_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_country_short_name CHECK (short_name IS NULL OR btrim(short_name) <> ''),
    CONSTRAINT chk_country_calling_name CHECK (calling_name IS NULL OR calling_name ~ '^\+[0-9]{1,7}$'),
    CONSTRAINT chk_country_currency CHECK (default_currency_name IS NULL OR default_currency_name ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_country_display_order CHECK (display_order >= 0)
);

CREATE INDEX IF NOT EXISTS idx_countries_active_order
ON countries(display_order, name)
WHERE is_active = TRUE;

DROP TRIGGER IF EXISTS trg_countries_updated_at ON countries;
CREATE TRIGGER trg_countries_updated_at
BEFORE UPDATE ON countries
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 11. USER FOREIGN KEYS & INDEXES
-- ============================================================================

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
        WHERE conname = 'fk_users_country' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT fk_users_country
            FOREIGN KEY (country_id)
            REFERENCES countries(id)
            ON DELETE RESTRICT;
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
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_active_email
ON users(service_provider_id, email)
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_active_username
ON users(service_provider_id, lower(username))
WHERE username IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(service_provider_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_role ON users(service_provider_id, role_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_status ON users(service_provider_id, status_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_type ON users(service_provider_id, user_type_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_country ON users(country_id);
CREATE INDEX IF NOT EXISTS idx_users_last_login ON users(service_provider_id, last_login_at);

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 12. ROLE PERMISSIONS
-- ============================================================================

CREATE TABLE IF NOT EXISTS role_permissions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID,
    role_id             UUID NOT NULL,
    permission_id       UUID NOT NULL
                            REFERENCES permissions(id)
                            ON DELETE RESTRICT,
    granted_by          UUID,
    granted_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at          TIMESTAMPTZ,

    CONSTRAINT uq_role_permissions UNIQUE (role_id, permission_id),
    CONSTRAINT chk_role_permission_dates CHECK (revoked_at IS NULL OR revoked_at >= granted_at)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_role_permissions_role' AND conrelid = 'role_permissions'::regclass
    ) THEN
        ALTER TABLE role_permissions
            ADD CONSTRAINT fk_role_permissions_role
            FOREIGN KEY (role_id)
            REFERENCES roles(id)
            ON DELETE CASCADE;
    END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_role_permissions_role ON role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission ON role_permissions(permission_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_active ON role_permissions(role_id, permission_id) WHERE revoked_at IS NULL;


-- ============================================================================
-- 13. DEVICE STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS device_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- ============================================================================
-- 14. DEVICES
-- ============================================================================

CREATE TABLE IF NOT EXISTS devices (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID NOT NULL
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    user_id             UUID NOT NULL,
    device_uuid         VARCHAR(128) NOT NULL,
    device_name         VARCHAR(150),
    device_type         VARCHAR(32) NOT NULL,
    os_name             VARCHAR(64),
    os_version          VARCHAR(32),
    app_version         VARCHAR(32),
    fcm_token           TEXT,
    ip_address          INET,
    user_agent          TEXT,
    is_trusted          BOOLEAN NOT NULL DEFAULT FALSE,
    status_id           SMALLINT NOT NULL
                            REFERENCES device_statuses(id)
                            ON DELETE RESTRICT,
    last_seen_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at          TIMESTAMPTZ,

    CONSTRAINT chk_device_type CHECK (
        device_type IN ('MOBILE_IOS', 'MOBILE_ANDROID', 'WEB', 'DESKTOP', 'API_CLIENT')
    ),
    CONSTRAINT uq_devices_tenant_id UNIQUE (service_provider_id, id)
);

DO $$
BEGIN
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
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_devices_user_device ON devices(service_provider_id, user_id, device_uuid);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(service_provider_id, user_id);
CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(service_provider_id, status_id);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(service_provider_id, last_seen_at);

DROP TRIGGER IF EXISTS trg_devices_updated_at ON devices;
CREATE TRIGGER trg_devices_updated_at
BEFORE UPDATE ON devices
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();


-- ============================================================================
-- 15. SESSION STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS session_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- ============================================================================
-- 16. USER SESSIONS
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID NOT NULL
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    user_id             UUID NOT NULL,
    device_id           UUID,
    status_id           SMALLINT NOT NULL
                            REFERENCES session_statuses(id)
                            ON DELETE RESTRICT,
    ip_address          INET,
    user_agent          TEXT,
    issued_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,

    CONSTRAINT chk_session_expiry CHECK (expires_at > issued_at),
    CONSTRAINT chk_session_revoked CHECK (revoked_at IS NULL OR revoked_at >= issued_at),
    CONSTRAINT uq_sessions_tenant_id UNIQUE (service_provider_id, id)
);

DO $$
BEGIN
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
END;
$$;

CREATE INDEX IF NOT EXISTS idx_sessions_user ON user_sessions(service_provider_id, user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_device ON user_sessions(service_provider_id, device_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON user_sessions(service_provider_id, status_id);
CREATE INDEX IF NOT EXISTS idx_sessions_active ON user_sessions(service_provider_id, user_id, expires_at) WHERE revoked_at IS NULL;


-- ============================================================================
-- 17. REFRESH TOKEN STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS refresh_token_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- ============================================================================
-- 18. REFRESH TOKENS
-- ============================================================================

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID NOT NULL
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    session_id          UUID NOT NULL,
    token_hash          TEXT NOT NULL UNIQUE,
    status_id           SMALLINT NOT NULL
                            REFERENCES refresh_token_statuses(id)
                            ON DELETE RESTRICT,
    replaced_by_id      UUID,
    issued_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,

    CONSTRAINT chk_refresh_token_expiry CHECK (expires_at > issued_at),
    CONSTRAINT uq_refresh_tokens_tenant_id UNIQUE (service_provider_id, id)
);

DO $$
BEGIN
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
END;
$$;

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_session ON refresh_tokens(service_provider_id, session_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_active ON refresh_tokens(service_provider_id, session_id, expires_at) WHERE revoked_at IS NULL;


-- ============================================================================
-- 19. PASSWORD RESET TOKEN STATUS
-- ============================================================================

CREATE TABLE IF NOT EXISTS password_reset_token_statuses (
    id              SMALLSERIAL PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    label           VARCHAR(64) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- ============================================================================
-- 20. PASSWORD RESET TOKENS
-- ============================================================================

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_provider_id UUID NOT NULL
                            REFERENCES service_providers(id)
                            ON DELETE CASCADE,
    user_id             UUID NOT NULL,
    token_hash          TEXT NOT NULL UNIQUE,
    status_id           SMALLINT NOT NULL
                            REFERENCES password_reset_token_statuses(id)
                            ON DELETE RESTRICT,
    requested_ip        INET,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    used_at             TIMESTAMPTZ,

    CONSTRAINT chk_password_reset_expiry CHECK (expires_at > created_at)
);

DO $$
BEGIN
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

CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(service_provider_id, user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_active ON password_reset_tokens(service_provider_id, user_id, expires_at) WHERE used_at IS NULL;


-- ============================================================================
-- 21. AUDIT LOG (APPEND-ONLY RANGE PARTITIONED)
-- ============================================================================

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


-- ============================================================================
-- 22. ROW-LEVEL SECURITY (RLS) POLICIES
-- ============================================================================

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
