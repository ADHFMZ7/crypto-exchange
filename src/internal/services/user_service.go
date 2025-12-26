package services

import (
	"context"
	"fmt"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type UserService struct {
	store *stores.UserStore
}

func NewUserService(userStore *stores.UserStore) *UserService {
	return &UserService{store: userStore}
}

func (s *UserService) RegisterUser(ctx context.Context, email, fullname, password string) (*models.User, error) {

	// TODO: Data validation layer perhaps
	// For now assuming data is valid
	// validate email
	// validate password

	hashed_password, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user, err := s.store.CreateUser(
		ctx,
		fullname,
		email,
		hashed_password,
	)
	if err != nil {
		return nil, err
	}

	fmt.Println(user)
	return user, nil
}
