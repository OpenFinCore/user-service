package repository

import (
	c "ofc-user-service/context"
	"ofc-user-service/models"

	"gorm.io/gorm"
)

// Store owns the database connection and transaction lifecycle. Actual
// table access goes through the generic Create/Get/List/Update/Delete
// functions in query.go, called with DBFrom(ctx, store) so they
// transparently participate in an in-flight transaction when there is one.
type Store interface {
	// DB returns the base *gorm.DB (outside of any transaction).
	DB() *gorm.DB

	BeginTx(ctx c.Context) (c.Context, error)
	CommitTx(ctx c.Context) error
	Rollback(ctx c.Context) error

	TryAdvisoryLock(ctx c.Context, id int) (bool, error)
	AdvisoryUnlock(ctx c.Context, id int) error

	GetSeedRegistryByFileName(ctx c.Context, fileName string) (*models.SeedRegistry, error)
	UpdateSeedRegistry(ctx c.Context, registry *models.SeedRegistry) error
}

// DBFrom returns the transaction bound to ctx if there is one, otherwise
// the store's base connection. Every repository call in the service layer
// should go through this so it automatically joins an open transaction.
func DBFrom(ctx c.Context, store Store) *gorm.DB {
	if tx := ctx.Tx(); tx != nil {
		return tx
	}
	return store.DB()
}
