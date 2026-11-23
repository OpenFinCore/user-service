package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type ServiceProviderService struct {
	*CRUDService[models.ServiceProvider]
	statuses *ServiceProviderStatusService
	// active caches GetActiveByID results (nil = not found / not active).
	active *ttlCache[uuid.UUID, *models.ServiceProvider]
}

// Tenant lookups happen on every tenant-scoped request, so their result is
// cached briefly. A tenant suspended in the database is therefore still
// served for up to activeTenantTTL (call InvalidateTenant on status
// changes to make it immediate on this instance).
const (
	activeTenantTTL   = 30 * time.Second
	inactiveTenantTTL = 5 * time.Second
	tenantCacheSize   = 10000
)

func NewServiceProviderService(store repository.Store, statuses *ServiceProviderStatusService) *ServiceProviderService {
	return &ServiceProviderService{
		CRUDService: NewCRUDService[models.ServiceProvider](store),
		statuses:    statuses,
		active:      newTTLCache[uuid.UUID, *models.ServiceProvider](tenantCacheSize),
	}
}

// InvalidateTenant drops the cached GetActiveByID result for id.
func (s *ServiceProviderService) InvalidateTenant(id uuid.UUID) { s.active.Delete(id) }

// GetByID returns the (non-deleted) service provider with id, or nil.
func (s *ServiceProviderService) GetByID(ctx c.Context, id uuid.UUID) (*models.ServiceProvider, error) {
	return s.Get(ctx, repository.QueryOptions{Filters: []repository.Filter{repository.Eq("id", id)}})
}

// GetActiveByID returns the service provider with id only if it is
// ACTIVE; a missing, deleted or non-active one returns nil. Results
// (including "not found") are cached briefly, see activeTenantTTL. The
// returned value is a copy: callers may not change the cached one.
func (s *ServiceProviderService) GetActiveByID(ctx c.Context, id uuid.UUID) (*models.ServiceProvider, error) {
	if sp, ok := s.active.Get(id); ok {
		return copyProvider(sp), nil
	}

	sp, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sp != nil {
		activeID, err := s.statuses.IDByName(ctx, models.ServiceProviderStatusActive)
		if err != nil {
			return nil, fmt.Errorf("service provider: resolving status: %w", err)
		}
		if sp.StatusID != activeID {
			sp = nil
		}
	}

	if sp == nil {
		s.active.Set(id, nil, inactiveTenantTTL)
		return nil, nil
	}
	s.active.Set(id, sp, activeTenantTTL)
	return copyProvider(sp), nil
}

func copyProvider(sp *models.ServiceProvider) *models.ServiceProvider {
	if sp == nil {
		return nil
	}
	cp := *sp
	return &cp
}

// CreateActive inserts sp with the ACTIVE status; an empty Config is
// stored as {}.
func (s *ServiceProviderService) CreateActive(ctx c.Context, sp *models.ServiceProvider) error {
	activeID, err := s.statuses.IDByName(ctx, models.ServiceProviderStatusActive)
	if err != nil {
		return fmt.Errorf("service provider: resolving status: %w", err)
	}
	sp.StatusID = activeID
	if len(sp.Config) == 0 {
		sp.Config = datatypes.JSON("{}")
	}
	return s.Create(ctx, sp)
}

// ExistsByIDIncludingDeleted reports whether a service provider with id
// exists, soft-deleted ones included (the id can't be reused either way).
func (s *ServiceProviderService) ExistsByIDIncludingDeleted(ctx c.Context, id uuid.UUID) (bool, error) {
	return s.Exists(ctx, repository.QueryOptions{
		Filters:  []repository.Filter{repository.Eq("id", id)},
		Unscoped: true,
	})
}
