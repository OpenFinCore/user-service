package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	c "ofc-user-service/context"
	"ofc-user-service/models"
)

type fakeDevProviders struct {
	exists    bool
	created   []models.ServiceProvider
	createErr error
}

func (f *fakeDevProviders) ExistsByIDIncludingDeleted(c.Context, uuid.UUID) (bool, error) {
	return f.exists, nil
}

func (f *fakeDevProviders) CreateActive(_ c.Context, sp *models.ServiceProvider) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, *sp)
	return nil
}

func newDevTenantHarness() (*DevTenantService, *fakeDevProviders, *fakeStore, uuid.UUID) {
	providers, store, id := &fakeDevProviders{}, &fakeStore{}, uuid.New()
	return &DevTenantService{
		store:            store,
		serviceProviders: providers,
		id:               id,
		name:             "Development Tenant",
	}, providers, store, id
}

func TestEnsureDevTenant_CreatesWhenMissing(t *testing.T) {
	svc, providers, store, id := newDevTenantHarness()

	created, err := svc.EnsureDevTenant(c.NewContext(context.Background()))
	if err != nil || !created {
		t.Fatalf("want created, got created=%v err=%v", created, err)
	}
	if len(providers.created) != 1 {
		t.Fatalf("want 1 tenant created, got %d", len(providers.created))
	}
	sp := providers.created[0]
	if sp.ID != id || sp.Name != "Development Tenant" {
		t.Fatalf("unexpected tenant %+v", sp)
	}
	if store.commits != 1 {
		t.Fatal("creation must be committed")
	}
}

func TestEnsureDevTenant_SkipsWhenExists(t *testing.T) {
	svc, providers, _, _ := newDevTenantHarness()
	providers.exists = true

	created, err := svc.EnsureDevTenant(c.NewContext(context.Background()))
	if err != nil || created || len(providers.created) != 0 {
		t.Fatalf("existing tenant must be left alone, got created=%v err=%v", created, err)
	}
}

func TestEnsureDevTenant_CreateErrorRollsBack(t *testing.T) {
	svc, providers, store, _ := newDevTenantHarness()
	providers.createErr = errors.New("db down")

	if _, err := svc.EnsureDevTenant(c.NewContext(context.Background())); err == nil {
		t.Fatal("want error")
	}
	if store.rollbacks != 1 || store.commits != 0 {
		t.Fatal("failed creation must roll back")
	}
}
