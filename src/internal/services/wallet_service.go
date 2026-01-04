package services

import (
	"errors"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/repositories"
	"golang.org/x/crypto/bcrypt"
)

type WalletService struct {
	WalletRepo *
	UserRepo   *repositories.UserRepository
}