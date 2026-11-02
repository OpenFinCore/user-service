package service

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type ServiceProviderService struct {
	*CRUDService[models.ServiceProvider]
	statuses *ServiceProviderStatusService
}

func NewServiceProviderService(store repository.Store, statuses *ServiceProviderStatusService) *ServiceProviderService {
	return &ServiceProviderService{CRUDService: NewCRUDService[models.ServiceProvider](store), statuses: statuses}
}

// GetByID returns the (non-deleted) service provider with id, or nil.
func (s *ServiceProviderService) GetByID(ctx c.Context, id uuid.UUID) (*models.ServiceProvider, error) {
	return s.Get(ctx, repository.QueryOptions{Filters: []repository.Filter{repository.Eq("id", id)}})
}

// GetActiveByID returns the service provider with id only if it is
// ACTIVE; a missing, deleted or non-active one returns nil.
func (s *ServiceProviderService) GetActiveByID(ctx c.Context, id uuid.UUID) (*models.ServiceProvider, error) {
	sp, err := s.GetByID(ctx, id)
	if err != nil || sp == nil {
		return nil, err
	}
	activeID, err := s.statuses.IDByName(ctx, models.ServiceProviderStatusActive)
	if err != nil {
		return nil, fmt.Errorf("service provider: resolving status: %w", err)
	}
	if sp.StatusID != activeID {
		return nil, nil
	}
	return sp, nil
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
