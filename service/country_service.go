package service

import (
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type CountryService struct{ *CRUDService[models.Country] }

func NewCountryService(store repository.Store) *CountryService {
	return &CountryService{NewCRUDService[models.Country](store)}
}
