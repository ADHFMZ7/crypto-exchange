package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type WalletService struct {
	WalletStore *stores.WalletStore
	UserStore   *stores.UserStore

	// Registry is what makes a deposit currency checkable rather than trusted.
	Registry *market.Registry
}

func NewWalletService(walletStore *stores.WalletStore, userStore *stores.UserStore, registry *market.Registry) *WalletService {
	return &WalletService{
		WalletStore: walletStore,
		UserStore:   userStore,
		Registry:    registry,
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

// ErrUnknownCurrency is returned for a currency the exchange does not list.
var ErrUnknownCurrency = errors.New("unknown currency")

// DepositToWallet applies a signed delta to one of the user's balances — a
// withdrawal is the same call with a negative amount.
//
// The currency is validated against the registry rather than taken on trust:
// without that, a typo creates a balance row in a currency nothing can trade,
// and the unique constraint then makes it permanent.
func (service *WalletService) DepositToWallet(ctx context.Context, userID int64, currency string, amount int64) error {

	if _, ok := service.Registry.Currency(currency); !ok {
		return fmt.Errorf("%w: %s", ErrUnknownCurrency, currency)
	}

	if amount == 0 {
		return errors.New("amount must be non-zero")
	}

	return service.WalletStore.AdjustBalance(ctx, userID, currency, amount)
}
