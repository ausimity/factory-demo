package portfolio

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/factory-demo/portfolio-api/internal/domain"
)

var (
	ErrCustomerNotFound = errors.New("customer not found")
	ErrPriceUnavailable = errors.New("price unavailable")
)

type AccountSource interface {
	Accounts(context.Context, string) ([]domain.Account, error)
	Healthy(context.Context) error
}

type PriceSource interface {
	Prices(context.Context, []string) (map[string]domain.Price, error)
	Healthy(context.Context) error
}

type Service struct {
	accounts AccountSource
	prices   PriceSource
}

func NewService(accounts AccountSource, prices PriceSource) *Service {
	return &Service{accounts: accounts, prices: prices}
}

func (s *Service) Get(ctx context.Context, customerID string) (domain.Portfolio, error) {
	accounts, err := s.accounts.Accounts(ctx, customerID)
	if err != nil {
		return domain.Portfolio{}, fmt.Errorf("load accounts: %w", err)
	}

	symbols := uniqueSymbols(accounts)
	prices, err := s.prices.Prices(ctx, symbols)
	if err != nil {
		return domain.Portfolio{}, fmt.Errorf("load prices: %w", err)
	}

	result := domain.Portfolio{CustomerID: customerID}
	for _, account := range accounts {
		accountValue := account.Cash
		accountPortfolio := domain.AccountPortfolio{
			ID:          account.ID,
			Type:        account.Type,
			Cash:        account.Cash,
			MarketValue: account.Cash,
			Holdings:    make([]domain.Holding, 0, len(account.Positions)),
		}

		for _, position := range account.Positions {
			price, ok := prices[position.Symbol]
			if !ok {
				return domain.Portfolio{}, fmt.Errorf("%w: %s", ErrPriceUnavailable, position.Symbol)
			}
			value, err := position.Quantity.MarketValue(price.Value)
			if err != nil {
				return domain.Portfolio{}, fmt.Errorf("value %s: %w", position.Symbol, err)
			}
			accountValue, err = accountValue.Add(value)
			if err != nil {
				return domain.Portfolio{}, fmt.Errorf("total account %s: %w", account.ID, err)
			}
			if price.AsOf.After(result.AsOf) {
				result.AsOf = price.AsOf
			}
			accountPortfolio.Holdings = append(accountPortfolio.Holdings, domain.Holding{
				Symbol:      position.Symbol,
				Quantity:    position.Quantity,
				UnitPrice:   price.Value,
				MarketValue: value,
			})
		}

		accountPortfolio.MarketValue = accountValue
		if result.TotalMarketValue.Currency == "" {
			result.TotalMarketValue = accountValue
		} else {
			result.TotalMarketValue, err = result.TotalMarketValue.Add(accountValue)
			if err != nil {
				return domain.Portfolio{}, fmt.Errorf("total portfolio: %w", err)
			}
		}
		result.Accounts = append(result.Accounts, accountPortfolio)
	}

	result.Insights = Insights(result)
	return result, nil
}

func (s *Service) Ready(ctx context.Context) error {
	if err := s.accounts.Healthy(ctx); err != nil {
		return fmt.Errorf("accounts dependency: %w", err)
	}
	if err := s.prices.Healthy(ctx); err != nil {
		return fmt.Errorf("prices dependency: %w", err)
	}
	return nil
}

func uniqueSymbols(accounts []domain.Account) []string {
	seen := make(map[string]struct{})
	for _, account := range accounts {
		for _, position := range account.Positions {
			seen[position.Symbol] = struct{}{}
		}
	}

	symbols := make([]string, 0, len(seen))
	for symbol := range seen {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}
