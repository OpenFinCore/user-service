package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"ofc-user-service/conf"
	c "ofc-user-service/context"
	"ofc-user-service/models"

	"ofc-user-service/repository"

	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	storeOnce sync.Once
	store     repository.Store
)

type Store struct {
	db *gorm.DB
}

// SharedStore returns the process-wide PostgreSQL store. On first use it
// migrates the schema (when enabled, as the admin role) and then connects
// as the app role, which is what every request runs as.
func SharedStore() repository.Store {
	storeOnce.Do(func() {
		cfg := conf.GetConfig()
		if cfg.PostDataSource.EnableAutoMigrate {
			if err := migrate(cfg.PostDataSource); err != nil {
				panic(err)
			}
		}
		db, err := openAppDB(cfg)
		if err != nil {
			panic(err)
		}
		store = NewStore(db)
	})
	return store
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *gorm.DB {
	return s.db
}

// migratedModels is every table GORM AutoMigrate is allowed to manage.
// AuditLog is intentionally excluded - it is range-partitioned in the
// schema, which AutoMigrate cannot express, so it is created directly by
// postMigrationSQL instead (see migrations.go). Order matters: parents
// before the children that FK against them.
func migratedModels() []interface{} {
	return []interface{}{
		// Lookup/status tables (no dependencies).
		&models.ServiceProviderStatus{},
		&models.UserTypeStatus{},
		&models.TwoFactorStatus{},
		&models.UserStatus{},
		&models.DeviceStatus{},
		&models.SessionStatus{},
		&models.RefreshTokenStatus{},
		&models.PasswordResetTokenStatus{},

		// Core reference/tenant tables.
		&models.ServiceProvider{},
		&models.UserType{},
		&models.Permission{},
		&models.Role{},
		&models.Country{},
		&models.User{},
		&models.RolePermission{},

		// Auth/session tables.
		&models.Device{},
		&models.UserSession{},
		&models.RefreshToken{},
		&models.PasswordResetToken{},

		// Bookkeeping.
		&models.SeedRegistry{},
	}
}

// buildDSN assembles a libpq-style connection string, skipping empty
// values. A literal but empty "password=" field confuses lib/pq's parser
// and makes it silently ignore sslmode, so blank values must be omitted.
// Keys are emitted in sorted-by-insertion order of the keys slice.
func buildDSN(keys []string, params map[string]string) string {
	var b strings.Builder
	for _, key := range keys {
		v, ok := params[key]
		if !ok || v == "" {
			continue
		}
		escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
		fmt.Fprintf(&b, "%s='%s' ", key, escaped)
	}
	return strings.TrimSpace(b.String())
}

var dsnKeys = []string{"host", "port", "user", "password", "dbname", "sslmode", "application_name", "statement_timeout", "idle_in_transaction_session_timeout"}

func dsn(db conf.PostgreDataSourceConfig, user, password, dbname string, extra map[string]string) string {
	params := map[string]string{
		"host":     db.Addr,
		"port":     db.Port,
		"user":     user,
		"password": password,
		"dbname":   dbname,
		"sslmode":  db.SSLMode,
	}
	for k, v := range extra {
		params[k] = v
	}
	return buildDSN(dsnKeys, params)
}

// migrate runs as the admin role: it creates the database, the extensions,
// the tables, everything GORM can't express (postMigrationSQL), reconciles
// the indexes and (re)creates the app role with exactly the privileges it
// needs.
func migrate(db conf.PostgreDataSourceConfig) error {
	if err := createDatabase(db); err != nil {
		return err
	}

	adminDSN := dsn(db, db.AdminUser, db.AdminPassword, db.Database, map[string]string{"application_name": "ofc-user-service-migrate"})
	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return fmt.Errorf("connecting to %s as admin: %w", db.Database, err)
	}
	defer admin.Close()
	if err := pingWithRetry(admin, db.ConnectAttempts); err != nil {
		return fmt.Errorf("connecting to %s as admin: %w", db.Database, err)
	}

	// Extensions/functions must exist before AutoMigrate runs (citext type,
	// gen_random_uuid() defaults).
	if _, err := admin.Exec(preMigrationSQL); err != nil {
		return fmt.Errorf("pre-migration SQL: %w", err)
	}

	gdb, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
	if err != nil {
		return fmt.Errorf("opening admin gorm connection: %w", err)
	}
	if sqlDB, err := gdb.DB(); err == nil {
		defer sqlDB.Close()
	}
	for _, table := range migratedModels() {
		if err := gdb.AutoMigrate(table); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", table, err)
		}
	}

	if _, err := admin.Exec(postMigrationSQL); err != nil {
		return fmt.Errorf("post-migration SQL: %w", err)
	}
	if err := reconcileIndexes(admin); err != nil {
		return err
	}
	if err := ensureAppRole(admin, db); err != nil {
		return err
	}
	log.Printf("Database %q migrated; app role %q ready.", db.Database, db.User)
	return nil
}

// createDatabase creates the database if it is missing (connecting to the
// maintenance database "postgres").
func createDatabase(db conf.PostgreDataSourceConfig) error {
	server, err := sql.Open("postgres", dsn(db, db.AdminUser, db.AdminPassword, "postgres", nil))
	if err != nil {
		return err
	}
	defer server.Close()
	if err := pingWithRetry(server, db.ConnectAttempts); err != nil {
		return fmt.Errorf("connecting to postgres server as admin: %w", err)
	}

	var exists bool
	if err := server.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", db.Database).Scan(&exists); err != nil {
		return fmt.Errorf("checking database: %w", err)
	}
	if !exists {
		if _, err := server.Exec("CREATE DATABASE " + pq.QuoteIdentifier(db.Database)); err != nil {
			return fmt.Errorf("creating database: %w", err)
		}
		log.Printf("Database %q created.", db.Database)
	}
	return nil
}

// ensureAppRole creates or updates the role the service runs as. It can log
// in and read/write rows, but it is not a superuser, cannot bypass RLS and
// does not own the tables - so row-level security always applies to it.
// Privileges are granted table by table: never on audit_logs partitions
// (reading a partition directly would skip the parent's RLS policy), and
// UPDATE/DELETE are withheld on audit_logs, which is append-only.
func ensureAppRole(admin *sql.DB, db conf.PostgreDataSourceConfig) error {
	role := pq.QuoteIdentifier(db.User)
	attrs := "LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION"
	if db.Password != "" {
		attrs += " PASSWORD " + pq.QuoteLiteral(db.Password)
	}

	var exists bool
	if err := admin.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", db.User).Scan(&exists); err != nil {
		return fmt.Errorf("checking app role: %w", err)
	}
	verb := "CREATE ROLE "
	if exists {
		verb = "ALTER ROLE "
	}
	if _, err := admin.Exec(verb + role + " " + attrs); err != nil {
		return fmt.Errorf("creating/updating app role %q: %w", db.User, err)
	}

	grants := fmt.Sprintf(`
GRANT CONNECT ON DATABASE %[1]s TO %[2]s;
GRANT USAGE ON SCHEMA public TO %[2]s;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %[2]s;
DO $$
DECLARE t record;
BEGIN
    FOR t IN
        SELECT c.relname FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition
    LOOP
        IF t.relname = 'audit_logs' THEN
            EXECUTE format('GRANT SELECT, INSERT ON %%I TO %[3]s', t.relname);
            EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON %%I FROM %[3]s', t.relname);
        ELSE
            EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %%I TO %[3]s', t.relname);
        END IF;
    END LOOP;
    FOR t IN
        SELECT c.relname FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relispartition
    LOOP
        EXECUTE format('REVOKE ALL ON %%I FROM %[3]s', t.relname);
    END LOOP;
END;
$$;`, pq.QuoteIdentifier(db.Database), role, strings.ReplaceAll(role, "'", "''"))
	if _, err := admin.Exec(grants); err != nil {
		return fmt.Errorf("granting app role privileges: %w", err)
	}
	return nil
}

// openAppDB connects as the app role with a bounded pool, per-query
// timeouts and a check that the role really is subject to RLS.
func openAppDB(cfg *conf.Config) (*gorm.DB, error) {
	db := cfg.PostDataSource
	appDSN := dsn(db, db.User, db.Password, db.Database, map[string]string{
		"application_name":                    "ofc-user-service",
		"statement_timeout":                   strconv.Itoa(db.StatementTimeoutMs),
		"idle_in_transaction_session_timeout": strconv.Itoa(max(db.StatementTimeoutMs*6, 30000)),
	})

	gormCfg := &gorm.Config{
		// Writes outside RunInTx are single statements; GORM's implicit
		// per-write transaction would only add two round trips each.
		SkipDefaultTransaction: true,
		Logger: gormlogger.New(log.New(os.Stdout, "", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
	}

	var gdb *gorm.DB
	var err error
	for attempt := 1; attempt <= db.ConnectAttempts; attempt++ {
		gdb, err = gorm.Open(postgres.Open(appDSN), gormCfg)
		if err == nil {
			var sqlDB *sql.DB
			if sqlDB, err = gdb.DB(); err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err = sqlDB.PingContext(ctx)
				cancel()
			}
		}
		if err == nil {
			break
		}
		log.Printf("connecting to database as %q (attempt %d/%d): %v", db.User, attempt, db.ConnectAttempts, err)
		time.Sleep(backoff(attempt))
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to database as %q: %w", db.User, err)
	}

	sqlDB, _ := gdb.DB()
	sqlDB.SetMaxOpenConns(db.MaxOpenConns)
	sqlDB.SetMaxIdleConns(db.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(db.ConnMaxLifetimeMinutes) * time.Minute)
	sqlDB.SetConnMaxIdleTime(time.Duration(db.ConnMaxIdleMinutes) * time.Minute)

	if err := checkAppRole(gdb, cfg); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return gdb, nil
}

// checkAppRole refuses a role that would bypass row-level security (a
// superuser or BYPASSRLS role): tenant isolation would silently be off.
func checkAppRole(gdb *gorm.DB, cfg *conf.Config) error {
	var role struct {
		Super     bool
		BypassRLS bool
	}
	if err := gdb.Raw("SELECT rolsuper AS super, rolbypassrls AS bypass_rls FROM pg_roles WHERE rolname = current_user").Scan(&role).Error; err != nil {
		return fmt.Errorf("checking database role: %w", err)
	}
	if role.Super || role.BypassRLS {
		msg := fmt.Sprintf("database role %q bypasses row-level security (superuser=%v, bypassrls=%v): tenant isolation is OFF", cfg.PostDataSource.User, role.Super, role.BypassRLS)
		if !cfg.IsDevelopment() {
			return errors.New(msg)
		}
		log.Printf("WARNING: %s", msg)
	}
	return nil
}

func pingWithRetry(db *sql.DB, attempts int) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}
		log.Printf("waiting for database (attempt %d/%d): %v", attempt, attempts, err)
		time.Sleep(backoff(attempt))
	}
	return err
}

// backoff grows 1s, 2s, 4s ... capped at 10s.
func backoff(attempt int) time.Duration {
	return min(time.Duration(1<<min(attempt-1, 4))*time.Second, 10*time.Second)
}

func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) BeginTx(ctx c.Context) (c.Context, error) {
	db := s.db
	if base := ctx.Context(); base != nil {
		// Bind the request's cancellation/deadline to every query in the tx.
		db = db.WithContext(base)
	}
	tx := db.Begin()
	if tx.Error != nil {
		return c.Context{}, tx.Error
	}

	// Scope row-level security to the request's tenant for this
	// transaction only (is_local = true), so a pooled connection never
	// carries one tenant's id into another request.
	if sp := ctx.ServiceProviderID(); sp != nil {
		if err := tx.Exec("SELECT set_config('app.current_service_provider_id', ?, true)", sp.String()).Error; err != nil {
			tx.Rollback()
			return c.Context{}, fmt.Errorf("setting tenant for transaction: %w", err)
		}
	}

	return ctx.WithTx(tx), nil
}

func (s *Store) Rollback(ctx c.Context) error {
	return ctx.Tx().Rollback().Error
}

func (s *Store) CommitTx(ctx c.Context) error {
	return ctx.Tx().Commit().Error
}

func (s *Store) GetSeedRegistryByFileName(ctx c.Context, fileName string) (*models.SeedRegistry, error) {
	var registry models.SeedRegistry
	err := repository.DBFrom(ctx, s).Where("file_name = ?", fileName).First(&registry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &registry, nil
}

func (s *Store) UpdateSeedRegistry(ctx c.Context, registry *models.SeedRegistry) error {
	return repository.DBFrom(ctx, s).Save(registry).Error
}

func (s *Store) AdvisoryXactLock(ctx c.Context, id int64) error {
	tx := ctx.Tx()
	if tx == nil {
		return errors.New("advisory lock requires a transaction")
	}
	// Startup work may legitimately wait for another instance; lift the
	// per-query timeout for this transaction only.
	if err := tx.Exec("SET LOCAL statement_timeout = '120s'").Error; err != nil {
		return err
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", id).Error
}
