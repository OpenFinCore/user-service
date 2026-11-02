package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type RolePermissionService struct {
	*CRUDService[models.RolePermission]
}

func NewRolePermissionService(store repository.Store) *RolePermissionService {
	return &RolePermissionService{NewCRUDService[models.RolePermission](store)}
}
