package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"ofc-user-service/conf"
	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	gdb       *gorm.DB
	storeOnce sync.Once
	store     repository.Store
)

type Store struct {
	db *gorm.DB
}

// SharedStore returns the process-wide PostgreSQL connection, connecting,
// migrating and seeding-registry-bootstrapping it on first use.
func SharedStore() repository.Store {
	storeOnce.Do(func() {
		err := initDb()
		if err != nil {
			panic(err)
		}
		store = NewStore(gdb)
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

// buildDSN assembles a libpq connection string, skipping empty values.
// A literal but empty "password=" field confuses lib/pq's parser and makes
// it silently ignore sslmode, so blank values must be omitted rather than
// passed through.
func buildDSN(params map[string]string) string {
	var b strings.Builder
	for _, key := range []string{"host", "port", "user", "password", "dbname", "sslmode"} {
		v, ok := params[key]
		if !ok || v == "" {
			continue
		}
		escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
		fmt.Fprintf(&b, "%s='%s' ", key, escaped)
	}
	return strings.TrimSpace(b.String())
}

func initDb() error {
	cfg := conf.GetConfig()

	// Connect to PostgreSQL server without specifying a database.
	serverURL := buildDSN(map[string]string{
		"host":     cfg.PostDataSource.Addr,
		"port":     cfg.PostDataSource.Port,
		"user":     cfg.PostDataSource.User,
		"password": cfg.PostDataSource.Password,
		"sslmode":  cfg.PostDataSource.SSLMode,
	})

	sdb, err := sql.Open("postgres", serverURL)
	if err != nil {
		return err
	}
	defer sdb.Close()

	// Create the database if it doesn't exist.
	_, err = sdb.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.PostDataSource.Database))
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create database: %w", err)
	}
	log.Printf("Database %q for user-service is ready.", cfg.PostDataSource.Database)

	url := buildDSN(map[string]string{
		"host":     cfg.PostDataSource.Addr,
		"port":     cfg.PostDataSource.Port,
		"user":     cfg.PostDataSource.User,
		"password": cfg.PostDataSource.Password,
		"dbname":   cfg.PostDataSource.Database,
		"sslmode":  cfg.PostDataSource.SSLMode,
	})

	// Extensions/functions must exist before AutoMigrate runs (citext type,
	// gen_random_uuid() defaults).
	appDB, err := sql.Open("postgres", url)
	if err != nil {
		return fmt.Errorf("failed to connect to %s database: %w", cfg.PostDataSource.Database, err)
	}
	defer appDB.Close()

	if _, err := appDB.Exec(preMigrationSQL); err != nil {
		return fmt.Errorf("failed running pre-migration SQL: %w", err)
	}

	gormOpt := &gorm.Config{}

	gdb, err = gorm.Open(postgres.Open(url), gormOpt)
	for err != nil {
		log.Println("Retrying connecting to database...")
		time.Sleep(5 * time.Second)
		gdb, err = gorm.Open(postgres.Open(url), gormOpt)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)

	if cfg.PostDataSource.EnableAutoMigrate {
		for _, table := range migratedModels() {
			log.Printf("Migrating database, table: %v", reflect.TypeOf(table))
			if err := gdb.AutoMigrate(table); err != nil {
				return fmt.Errorf("failed to migrate %T: %w", table, err)
			}
		}

		if _, err := appDB.Exec(postMigrationSQL); err != nil {
			return fmt.Errorf("failed running post-migration SQL: %w", err)
		}
	}

	return nil
}

func (s *Store) BeginTx(ctx c.Context) (c.Context, error) {
	tx := s.db.Begin()
	if tx.Error != nil {
		return c.Context{}, tx.Error
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

func (s *Store) TryAdvisoryLock(ctx c.Context, id int) (bool, error) {
	var acquired bool
	err := repository.DBFrom(ctx, s).Raw("SELECT pg_try_advisory_lock(?)", id).Scan(&acquired).Error
	return acquired, err
}

func (s *Store) AdvisoryUnlock(ctx c.Context, id int) error {
	return repository.DBFrom(ctx, s).Exec("SELECT pg_advisory_unlock(?)", id).Error
}
