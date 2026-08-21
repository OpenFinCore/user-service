package service

import (
	c "ofc-user-service/context"
	"ofc-user-service/repository"
)

// CRUDService is the single generic engine behind every entity-specific
// service in this package (UserService, RoleService, ...). It just forwards
// to the generic repository functions, resolving whichever *gorm.DB is
// active on ctx (a transaction, if one is open, otherwise the base
// connection) so callers never have to think about that plumbing.
type CRUDService[T any] struct {
	store repository.Store
}

func NewCRUDService[T any](store repository.Store) *CRUDService[T] {
	return &CRUDService[T]{store: store}
}

func (s *CRUDService[T]) Create(ctx c.Context, entity *T) error {
	return repository.Create(repository.DBFrom(ctx, s.store), entity)
}

func (s *CRUDService[T]) CreateMany(ctx c.Context, entities []*T) error {
	return repository.CreateMany(repository.DBFrom(ctx, s.store), entities)
}

func (s *CRUDService[T]) Get(ctx c.Context, opts repository.QueryOptions) (*T, error) {
	return repository.Get[T](repository.DBFrom(ctx, s.store), opts)
}

func (s *CRUDService[T]) List(ctx c.Context, opts repository.QueryOptions) ([]T, int64, error) {
	return repository.List[T](repository.DBFrom(ctx, s.store), opts)
}

func (s *CRUDService[T]) Update(ctx c.Context, opts repository.QueryOptions, updates map[string]interface{}) (int64, error) {
	return repository.Update[T](repository.DBFrom(ctx, s.store), opts, updates)
}

func (s *CRUDService[T]) Save(ctx c.Context, entity *T) error {
	return repository.Save(repository.DBFrom(ctx, s.store), entity)
}

func (s *CRUDService[T]) Delete(ctx c.Context, opts repository.QueryOptions) (int64, error) {
	return repository.Delete[T](repository.DBFrom(ctx, s.store), opts)
}

func (s *CRUDService[T]) HardDelete(ctx c.Context, opts repository.QueryOptions) (int64, error) {
	return repository.HardDelete[T](repository.DBFrom(ctx, s.store), opts)
}

func (s *CRUDService[T]) Count(ctx c.Context, opts repository.QueryOptions) (int64, error) {
	return repository.Count[T](repository.DBFrom(ctx, s.store), opts)
}

func (s *CRUDService[T]) Exists(ctx c.Context, opts repository.QueryOptions) (bool, error) {
	return repository.Exists[T](repository.DBFrom(ctx, s.store), opts)
}
