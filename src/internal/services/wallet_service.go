package services

import (
	"context"
	"errors"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type WalletService struct {
	WalletStore *stores.WalletStore
	UserStore   *stores.UserStore
}

func NewWalletService(walletStore *stores.WalletStore, userStore *stores.UserStore) *WalletService {
	return &WalletService{
		WalletStore: walletStore,
		UserStore:   userStore,
	}
}

func (service *WalletService) GetWalletByUserID(ctx context.Context, userID int64) (*models.Wallet, error) {
	// Check if user exists
	_, err := service.UserStore.GetByID(ctx, userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Retrieve wallet
	wallet, err := service.WalletStore.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return wallet, nil
}
