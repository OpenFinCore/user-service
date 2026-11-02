package repository

import (
	c "ofc-user-service/context"
)

// RunInTx runs fn inside a transaction bound to the ctx it receives. If ctx
// already carries a transaction, fn simply joins it, so services can call
// each other and still end up in ONE transaction. The transaction is
// rolled back when fn returns an error or panics, and committed otherwise.
func RunInTx(ctx c.Context, store Store, fn func(ctx c.Context) error) (err error) {
	if ctx.Tx() != nil {
		return fn(ctx)
	}

	txCtx, err := store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = store.Rollback(txCtx)
			panic(p)
		}
	}()

	if err = fn(txCtx); err != nil {
		_ = store.Rollback(txCtx)
		return err
	}
	return store.CommitTx(txCtx)
}
