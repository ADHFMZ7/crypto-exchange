package services

import "github.com/ADHFMZ7/crypto-exchange/internal/stores"

type Services struct {
	Users *UserService
}

func NewServices(stores *stores.Stores) *Services {

	return &Services{
		Users: NewUserService(stores.Users),
	}
}
