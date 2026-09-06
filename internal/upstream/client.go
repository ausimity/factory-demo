package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

const maxResponseBytes = 1 << 20

type CoreClient struct {
	baseURL string
	client  *http.Client
}

func NewCoreClient(baseURL string, client *http.Client) *CoreClient {
	return &CoreClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

type coreAccountsResponse struct {
	CustomerID string        `json:"customer_id"`
	Accounts   []coreAccount `json:"accounts"`
}

type coreAccount struct {
	AccountID   string         `json:"account_id"`
	AccountType string         `json:"account_type"`
	CashBalance coreMoney      `json:"cash_balance"`
	Positions   []corePosition `json:"positions"`
}

type coreMoney struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

type corePosition struct {
	Symbol   string `json:"symbol"`
	Quantity string `json:"quantity"`
}

func (c *CoreClient) Accounts(ctx context.Context, customerID string) ([]domain.Account, error) {
	endpoint := c.baseURL + "/v1/customers/" + url.PathEscape(customerID) + "/accounts"
	response, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return nil, portfolio.ErrCustomerNotFound
	}
	if response.StatusCode != http.StatusOK {
		return nil, responseError("core", response)
	}

	var payload coreAccountsResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode core response: %w", err)
	}
	if payload.CustomerID != customerID || payload.Accounts == nil {
		return nil, errors.New("core response failed contract validation")
	}

	accounts := make([]domain.Account, 0, len(payload.Accounts))
	for _, item := range payload.Accounts {
		if item.AccountID == "" || item.AccountType == "" {
			return nil, errors.New("core account failed contract validation")
		}
		cash, err := domain.NewMoney(item.CashBalance.AmountMinor, item.CashBalance.Currency)
		if err != nil {
			return nil, fmt.Errorf("parse cash for account %s: %w", item.AccountID, err)
		}
		account := domain.Account{
			ID:        item.AccountID,
			Type:      item.AccountType,
			Cash:      cash,
			Positions: make([]domain.Position, 0, len(item.Positions)),
		}
		for _, position := range item.Positions {
			if position.Symbol == "" {
				return nil, errors.New("core position failed contract validation")
			}
			quantity, err := domain.ParseQuantity(position.Quantity)
			if err != nil {
				return nil, fmt.Errorf("parse quantity for %s: %w", position.Symbol, err)
			}
			account.Positions = append(account.Positions, domain.Position{
				Symbol:   position.Symbol,
				Quantity: quantity,
			})
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func (c *CoreClient) Healthy(ctx context.Context) error {
	return c.health(ctx, c.baseURL)
}

func (c *CoreClient) get(ctx context.Context, endpoint string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create core request: %w", err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call core: %w", err)
	}
	return response, nil
}

func (c *CoreClient) health(ctx context.Context, baseURL string) error {
	response, err := c.get(ctx, baseURL+"/health")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return responseError("core health", response)
	}
	return nil
}

type MarketClient struct {
	baseURL string
	client  *http.Client
}

func NewMarketClient(baseURL string, client *http.Client) *MarketClient {
	return &MarketClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

type marketPricesResponse struct {
	Prices []marketPrice `json:"prices"`
}

type marketPrice struct {
	Symbol string    `json:"symbol"`
	Price  coreMoney `json:"price"`
	AsOf   time.Time `json:"as_of"`
}

func (c *MarketClient) Prices(ctx context.Context, symbols []string) (map[string]domain.Price, error) {
	if len(symbols) == 0 {
		return map[string]domain.Price{}, nil
	}

	query := url.Values{}
	query.Set("symbols", strings.Join(symbols, ","))
	endpoint := c.baseURL + "/v1/prices?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create market request: %w", err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call market: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, responseError("market", response)
	}

	var payload marketPricesResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode market response: %w", err)
	}

	prices := make(map[string]domain.Price, len(payload.Prices))
	for _, item := range payload.Prices {
		if item.Symbol == "" || item.AsOf.IsZero() {
			return nil, errors.New("market price failed contract validation")
		}
		value, err := domain.NewMoney(item.Price.AmountMinor, item.Price.Currency)
		if err != nil {
			return nil, fmt.Errorf("parse price for %s: %w", item.Symbol, err)
		}
		prices[item.Symbol] = domain.Price{Symbol: item.Symbol, Value: value, AsOf: item.AsOf}
	}
	return prices, nil
}

func (c *MarketClient) Healthy(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create market health request: %w", err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("call market health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return responseError("market health", response)
	}
	return nil
}

func responseError(service string, response *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	return fmt.Errorf("%s returned HTTP %d", service, response.StatusCode)
}
