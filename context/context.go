package context

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Context wraps the standard context.Context with a request-scoped
// transaction handle, the acting user/tenant, and the current session id.
// It is passed through the repository and service layers so that a single
// BeginTx/CommitTx/Rollback can span multiple repository calls.
type Context struct {
	baseCtx           context.Context
	tx                *gorm.DB
	actorID           *uuid.UUID
	serviceProviderID *uuid.UUID
	session           string
}

// NewContext creates a new Context from a standard context.Context.
func NewContext(ctx context.Context) Context {
	return Context{baseCtx: ctx}
}

// Read-only accessors
func (c Context) Context() context.Context { return c.baseCtx }
func (c Context) Tx() *gorm.DB             { return c.tx }
func (c Context) ActorID() *uuid.UUID      { return c.actorID }
func (c Context) ServiceProviderID() *uuid.UUID {
	return c.serviceProviderID
}
func (c Context) Session() string { return c.session }

func (c Context) Deadline() (deadline time.Time, ok bool) { return c.baseCtx.Deadline() }
func (c Context) Done() <-chan struct{}                   { return c.baseCtx.Done() }
func (c Context) Err() error                              { return c.baseCtx.Err() }

// WithContext returns a Context with an updated context.Context.
func (c Context) WithContext(ctx context.Context) Context {
	c.baseCtx = ctx
	return c
}

// WithTx returns a Context with an updated transaction handle.
func (c Context) WithTx(tx *gorm.DB) Context {
	c.tx = tx
	return c
}

// WithActorID returns a Context carrying the id of the acting user.
func (c Context) WithActorID(actorID uuid.UUID) Context {
	c.actorID = &actorID
	return c
}

// WithServiceProviderID returns a Context scoped to the given tenant.
func (c Context) WithServiceProviderID(serviceProviderID uuid.UUID) Context {
	c.serviceProviderID = &serviceProviderID
	return c
}

// WithSession returns a Context carrying a session/correlation id for logging.
func (c Context) WithSession(session string) Context {
	c.session = session
	return c
}
