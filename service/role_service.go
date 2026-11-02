package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type RoleService struct{ *CRUDService[models.Role] }

func NewRoleService(store repository.Store) *RoleService {
	return &RoleService{NewCRUDService[models.Role](store)}
}
