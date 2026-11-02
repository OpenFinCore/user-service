package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	c "ofc-user-service/context"
	"ofc-user-service/logger"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

// The capability DevTenantService uses; ServiceProviderService satisfies it.
type devTenantProviders interface {
	ExistsByIDIncludingDeleted(ctx c.Context, id uuid.UUID) (bool, error)
	CreateActive(ctx c.Context, sp *models.ServiceProvider) error
}

// DevTenantService makes local development usable out of the box: it
// guarantees an ACTIVE tenant with a fixed, known id exists, so tenant
// scoped APIs can be called (e.g. from Postman) without manual SQL.
// It is only ever run in the development stage.
type DevTenantService struct {
	store            repository.Store
	serviceProviders devTenantProviders
	id               uuid.UUID
	name             string
}

func NewDevTenantService(
	store repository.Store,
	serviceProviders *ServiceProviderService,
	id uuid.UUID,
	name string,
) *DevTenantService {
	return &DevTenantService{
		store:            store,
		serviceProviders: serviceProviders,
		id:               id,
		name:             name,
	}
}

// EnsureDevTenant creates the dev tenant if no tenant with its id exists
// (soft-deleted included); an existing one is left untouched. It reports
// whether it created the tenant.
func (s *DevTenantService) EnsureDevTenant(ctx c.Context) (created bool, err error) {
	err = repository.RunInTx(ctx, s.store, func(ctx c.Context) error {
		exists, err := s.serviceProviders.ExistsByIDIncludingDeleted(ctx, s.id)
		if err != nil {
			return fmt.Errorf("dev tenant: checking: %w", err)
		}
		if exists {
			return nil
		}

		if err := s.serviceProviders.CreateActive(ctx, &models.ServiceProvider{ID: s.id, Name: s.name}); err != nil {
			return fmt.Errorf("dev tenant: creating: %w", err)
		}
		created = true
		return nil
	})
	if err != nil {
		return false, err
	}

	if created {
		logger.LogInfo(fmt.Sprintf("development tenant created: %s (%s)", s.id, s.name), "")
	} else {
		logger.LogInfo(fmt.Sprintf("development tenant exists, skipping: %s", s.id), "")
	}
	return created, nil
}

// ensureDevTenant is called by NewServiceContainer in the development stage.
func ensureDevTenant(s *DevTenantService) {
	if _, err := s.EnsureDevTenant(c.NewContext(context.Background())); err != nil {
		panic(err)
	}
}
