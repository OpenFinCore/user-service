package service

import (
	"fmt"
	"sync"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

// StatusLookup is the generic engine behind every status/lookup table. On
// top of the generic CRUD it resolves a seeded status name to its id.
// Seeded rows never change at runtime, so resolved ids are cached for the
// life of the process.
type StatusLookup[T any] struct {
	*CRUDService[T]

	mu    sync.RWMutex
	cache map[string]int16
}

func NewStatusLookup[T any](store repository.Store) *StatusLookup[T] {
	return &StatusLookup[T]{CRUDService: NewCRUDService[T](store), cache: map[string]int16{}}
}

// IDByName returns the id of the status called name (one of the models.*
// status name constants). A missing name is a seeding/configuration bug,
// so it is reported as an error rather than (0, nil).
func (s *StatusLookup[T]) IDByName(ctx c.Context, name string) (int16, error) {
	s.mu.RLock()
	id, ok := s.cache[name]
	s.mu.RUnlock()
	if ok {
		return id, nil
	}

	row, err := s.Get(ctx, repository.QueryOptions{Filters: []repository.Filter{repository.Eq("name", name)}})
	if err != nil {
		return 0, err
	}
	if row == nil {
		var zero T
		return 0, fmt.Errorf("status %q not found for %T (not seeded?)", name, zero)
	}
	statusRow, ok := any(*row).(interface{ StatusID() int16 })
	if !ok {
		return 0, fmt.Errorf("%T does not embed models.StatusBase", *row)
	}

	id = statusRow.StatusID()
	s.mu.Lock()
	s.cache[name] = id
	s.mu.Unlock()
	return id, nil
}

// Named wrappers around the status engine for the lookup tables. Each is
// discoverable by name (ServiceProviderStatusService, ...) and free to grow
// entity-specific methods later without disturbing the generic engine.

type ServiceProviderStatusService struct {
	*StatusLookup[models.ServiceProviderStatus]
}

func NewServiceProviderStatusService(store repository.Store) *ServiceProviderStatusService {
	return &ServiceProviderStatusService{NewStatusLookup[models.ServiceProviderStatus](store)}
}

type UserTypeStatusService struct {
	*StatusLookup[models.UserTypeStatus]
}

func NewUserTypeStatusService(store repository.Store) *UserTypeStatusService {
	return &UserTypeStatusService{NewStatusLookup[models.UserTypeStatus](store)}
}

type TwoFactorStatusService struct {
	*StatusLookup[models.TwoFactorStatus]
}

func NewTwoFactorStatusService(store repository.Store) *TwoFactorStatusService {
	return &TwoFactorStatusService{NewStatusLookup[models.TwoFactorStatus](store)}
}

type UserStatusService struct {
	*StatusLookup[models.UserStatus]
}

func NewUserStatusService(store repository.Store) *UserStatusService {
	return &UserStatusService{NewStatusLookup[models.UserStatus](store)}
}

type DeviceStatusService struct {
	*StatusLookup[models.DeviceStatus]
}

func NewDeviceStatusService(store repository.Store) *DeviceStatusService {
	return &DeviceStatusService{NewStatusLookup[models.DeviceStatus](store)}
}

type SessionStatusService struct {
	*StatusLookup[models.SessionStatus]
}

func NewSessionStatusService(store repository.Store) *SessionStatusService {
	return &SessionStatusService{NewStatusLookup[models.SessionStatus](store)}
}

type RefreshTokenStatusService struct {
	*StatusLookup[models.RefreshTokenStatus]
}

func NewRefreshTokenStatusService(store repository.Store) *RefreshTokenStatusService {
	return &RefreshTokenStatusService{NewStatusLookup[models.RefreshTokenStatus](store)}
}

type PasswordResetTokenStatusService struct {
	*StatusLookup[models.PasswordResetTokenStatus]
}

func NewPasswordResetTokenStatusService(store repository.Store) *PasswordResetTokenStatusService {
	return &PasswordResetTokenStatusService{NewStatusLookup[models.PasswordResetTokenStatus](store)}
}
