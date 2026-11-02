package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type ServiceProviderService struct {
	*CRUDService[models.ServiceProvider]
}

func NewServiceProviderService(store repository.Store) *ServiceProviderService {
	return &ServiceProviderService{NewCRUDService[models.ServiceProvider](store)}
}



type UserTypeService struct{ *CRUDService[models.UserType] }

func NewUserTypeService(store repository.Store) *UserTypeService {
	return &UserTypeService{NewCRUDService[models.UserType](store)}
}

type PermissionService struct {
	*CRUDService[models.Permission]
}

func NewPermissionService(store repository.Store) *PermissionService {
	return &PermissionService{NewCRUDService[models.Permission](store)}
}

type RoleService struct{ *CRUDService[models.Role] }

func NewRoleService(store repository.Store) *RoleService {
	return &RoleService{NewCRUDService[models.Role](store)}
}

type RolePermissionService struct {
	*CRUDService[models.RolePermission]
}

func NewRolePermissionService(store repository.Store) *RolePermissionService {
	return &RolePermissionService{NewCRUDService[models.RolePermission](store)}
}

type CountryService struct{ *CRUDService[models.Country] }

func NewCountryService(store repository.Store) *CountryService {
	return &CountryService{NewCRUDService[models.Country](store)}
}

type UserService struct{ *CRUDService[models.User] }

func NewUserService(store repository.Store) *UserService {
	return &UserService{NewCRUDService[models.User](store)}
}
