package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenFinCore/permissions-package/auth/permission"

	"ofc-user-service/conf"
	c "ofc-user-service/context"
	"ofc-user-service/logger"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

// SeedsService populates every lookup/reference table so the rest of the
// schema (which FKs against them) is usable immediately after migration.
type SeedsService interface {
	SeedAll() error
}

type seedsService struct {
	store repository.Store
	conf  *conf.Config
}

func NewSeedsService(store repository.Store, config *conf.Config) SeedsService {
	return &seedsService{store: store, conf: config}
}

// seedAdvisoryLockID is an arbitrary, unique Postgres advisory lock id used
// so that multiple instances starting up concurrently don't seed twice.
const seedAdvisoryLockID = 159159

func (s *seedsService) SeedAll() (err error) {
	ctx := c.NewContext(context.Background())

	acquired, err := s.store.TryAdvisoryLock(ctx, seedAdvisoryLockID)
	if err != nil {
		return fmt.Errorf("seed: acquiring advisory lock: %w", err)
	}
	if !acquired {
		logger.LogInfo("Seeding already in progress on another instance, skipping.", "")
		return nil
	}
	defer s.store.AdvisoryUnlock(ctx, seedAdvisoryLockID)

	ctx, err = s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.store.Rollback(ctx)
		}
	}()

	dir := s.conf.SeedData.Dir

	byName := func(name string) repository.QueryOptions {
		return repository.QueryOptions{Filters: []repository.Filter{repository.Eq("name", name)}}
	}

	seeders := []func() error{
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "service_provider_statuses.json"),
				func(m models.ServiceProviderStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "user_type_statuses.json"),
				func(m models.UserTypeStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "two_factor_statuses.json"),
				func(m models.TwoFactorStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "user_statuses.json"),
				func(m models.UserStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "device_statuses.json"),
				func(m models.DeviceStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "session_statuses.json"),
				func(m models.SessionStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "refresh_token_statuses.json"),
				func(m models.RefreshTokenStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "password_reset_token_statuses.json"),
				func(m models.PasswordResetTokenStatus) repository.QueryOptions { return byName(m.Name) })
		},
		func() error {
			return seedFromFile(s.store, ctx, filepath.Join(dir, "countries.json"),
				func(m models.Country) repository.QueryOptions {
					return repository.QueryOptions{Filters: []repository.Filter{repository.Eq("iso_alpha2", m.IsoAlpha2)}}
				})
		},
		func() error { return s.seedPermissions(ctx) },
	}

	for _, seed := range seeders {
		if err = seed(); err != nil {
			return err
		}
	}

	return s.store.CommitTx(ctx)
}

// seedPermissions seeds the `permissions` table from
// github.com/OpenFinCore/permissions-package, the single
// cross-service source of truth for what permissions exist and what they
// mean. Permission codes (Definition.Code) are globally unique and
// immutable per that package's governance rules, so they are stored in
// the schema's unique `name` column; the mutable label/description/active
// fields are kept in sync on every seed run instead of being skipped once
// a row exists.
func (s *seedsService) seedPermissions(ctx c.Context) error {
	defs := permission.AllDefinitions()

	data, err := json.Marshal(defs)
	if err != nil {
		return err
	}
	sum := md5.Sum(data)
	hash := hex.EncodeToString(sum[:])

	const registryKey = "github.com/OpenFinCore/permissions-package"
	registry, err := s.store.GetSeedRegistryByFileName(ctx, registryKey)
	if err != nil {
		return err
	}
	if registry != nil && registry.FileHash == hash {
		logger.LogInfo("Permission registry unchanged, skipping...", "")
		return nil
	}
	if registry == nil {
		registry = &models.SeedRegistry{FileName: registryKey}
	}

	db := repository.DBFrom(ctx, s.store)
	for _, d := range defs {
		description := d.Description
		isActive := !d.Deprecated

		existing, err := repository.Get[models.Permission](db, repository.QueryOptions{
			Filters: []repository.Filter{repository.Eq("name", d.Code)},
		})
		if err != nil {
			return err
		}

		if existing == nil {
			if err := repository.Create(db, &models.Permission{
				Name:        d.Code,
				Label:       d.Name,
				Description: &description,
				IsSystem:    true,
				IsActive:    isActive,
			}); err != nil {
				return err
			}
			continue
		}

		if _, err := repository.Update[models.Permission](db,
			repository.QueryOptions{Filters: []repository.Filter{repository.Eq("name", d.Code)}},
			map[string]interface{}{
				"label":       d.Name,
				"description": description,
				"is_active":   isActive,
				"is_system":   true,
			}); err != nil {
			return err
		}
	}

	registry.FileHash = hash
	registry.LastAppliedAt = time.Now()
	return s.store.UpdateSeedRegistry(ctx, registry)
}

// seedFromFile loads a JSON array of T from filePath and inserts whatever
// keyFilter can't already find in the table, skipping the whole file when
// its content hash matches the last run recorded in seed_registry.
func seedFromFile[T any](store repository.Store, ctx c.Context, filePath string, keyFilter func(T) repository.QueryOptions) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("seed: reading %s: %w", filePath, err)
	}

	sum := md5.Sum(data)
	hash := hex.EncodeToString(sum[:])

	registry, err := store.GetSeedRegistryByFileName(ctx, filePath)
	if err != nil {
		return err
	}
	if registry != nil && registry.FileHash == hash {
		logger.LogInfo(fmt.Sprintf("%s unchanged, skipping...", filePath), "")
		return nil
	}
	if registry == nil {
		registry = &models.SeedRegistry{FileName: filePath}
	}

	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("seed: unmarshaling %s: %w", filePath, err)
	}

	db := repository.DBFrom(ctx, store)
	for i := range items {
		exists, err := repository.Exists[T](db, keyFilter(items[i]))
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := repository.Create(db, &items[i]); err != nil {
			return fmt.Errorf("seed: inserting from %s: %w", filePath, err)
		}
	}

	registry.FileHash = hash
	registry.LastAppliedAt = time.Now()
	return store.UpdateSeedRegistry(ctx, registry)
}
