package service

import (
	"github.com/google/uuid"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type UserService struct{ *CRUDService[models.User] }

func NewUserService(store repository.Store) *UserService {
	return &UserService{NewCRUDService[models.User](store)}
}

func (s *UserService) PhoneExists(ctx c.Context, spID uuid.UUID, phone string) (bool, error) {
	return s.Exists(ctx, repository.QueryOptions{Filters: []repository.Filter{
		repository.Eq("service_provider_id", spID),
		repository.Eq("phone_number", phone),
	}})
}
