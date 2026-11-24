package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type UserTypeService struct{ *CRUDService[models.UserType] }

func NewUserTypeService(store repository.Store) *UserTypeService {
	return &UserTypeService{NewCRUDService[models.UserType](store)}
}
