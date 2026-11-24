package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type PermissionService struct {
	*CRUDService[models.Permission]
}

func NewPermissionService(store repository.Store) *PermissionService {
	return &PermissionService{NewCRUDService[models.Permission](store)}
}
