package services

import (
	"errors"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type WalletService struct {
	WalletRepo *stores.WalletStore
	UserRepo   *stores.UserStore
}

func NewWalletService(walletRepo *stores.WalletStore, userRepo *stores.UserStore) *WalletService {
	return &WalletService{
		WalletRepo: walletRepo,
		UserRepo:   userRepo,
	}
}

func (service *WalletService) GetWalletByUserID(userID int64) (*models.Wallet, error) {
	// Check if user exists
	_, err := service.UserRepo.GetByID(nil, userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Retrieve wallet
	wallet, err := service.WalletRepo.GetByUserID(nil, userID)
	if err != nil {
		return nil, err
	}

	return wallet, nil
}
