package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

const maxResponseBytes = 1 << 20

// coreCurrencyPattern enforces the v2 contract's uppercase three-letter currency
// code. The v1 path keeps its historically lenient domain.NewMoney handling.
var coreCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type CoreClient struct {
	baseURL string
	client  *http.Client
}

func NewCoreClient(baseURL string, client *http.Client) *CoreClient {
	return &CoreClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

// coreMoney is the market-price wire shape and is intentionally lenient; it is
// shared with the Market client below and unrelated to the strict Core
// account contracts.
type coreMoney struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

// Core v1 wire DTOs. Pointers make required scalar presence distinguishable
// from an explicit zero or a null, and nil slices distinguish missing/null
// arrays from a valid non-null empty array.
type coreV1Response struct {
	CustomerID *string         `json:"customer_id"`
	Accounts   []coreV1Account `json:"accounts"`
}

type coreV1Account struct {
	AccountID   *string          `json:"account_id"`
	AccountType *string          `json:"account_type"`
	CashBalance *coreV1Money     `json:"cash_balance"`
	Positions   []coreV1Position `json:"positions"`
}

type coreV1Money struct {
	AmountMinor *int64  `json:"amount_minor"`
	Currency    *string `json:"currency"`
}

type coreV1Position struct {
	Symbol   *string `json:"symbol"`
	Quantity *string `json:"quantity"`
}

// Core v2 wire DTOs. Same presence rules as v1.
type coreV2Response struct {
	Client     *coreV2Client     `json:"client"`
	Portfolios []coreV2Portfolio `json:"portfolios"`
}

type coreV2Client struct {
	ID *string `json:"id"`
}

type coreV2Portfolio struct {
	ID       *string         `json:"id"`
	Category *string         `json:"category"`
	Balances *coreV2Balances `json:"balances"`
	Assets   []coreV2Asset   `json:"assets"`
}

type coreV2Balances struct {
	Cash *coreV2Money `json:"cash"`
}

type coreV2Money struct {
	MinorUnits   *int64  `json:"minor_units"`
	CurrencyCode *string `json:"currency_code"`
}

type coreV2Asset struct {
	Ticker *string `json:"ticker"`
	Units  *string `json:"units"`
}

type coreVersion int

const (
	coreVersionUnknown coreVersion = iota
	coreVersionV1
	coreVersionV2
)

var coreCategories = map[string]struct{}{
	"BROKERAGE":  {},
	"RETIREMENT": {},
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

	raw, err := readBoundedBody(response.Body)
	if err != nil {
		return nil, err
	}

	version, err := detectCoreVersion(raw)
	if err != nil {
		return nil, err
	}

	switch version {
	case coreVersionV1:
		var payload coreV1Response
		if err := strictDecodeCore(raw, &payload); err != nil {
			return nil, err
		}
		return mapCoreV1(payload, customerID)
	case coreVersionV2:
		var payload coreV2Response
		if err := strictDecodeCore(raw, &payload); err != nil {
			return nil, err
		}
		return mapCoreV2(payload, customerID)
	default:
		return nil, errors.New("core response does not match a supported contract signature")
	}
}

// readBoundedBody reads at most maxResponseBytes and rejects anything larger.
// Trailing whitespace counts toward the bound because it is part of the body.
func readBoundedBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read core response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("core response exceeds maximum allowed size")
	}
	return raw, nil
}

// detectCoreVersion identifies a complete v1 or v2 signature independent of
// top-level field order and rejects mixed or incomplete envelopes before any
// mapping is attempted.
func detectCoreVersion(raw []byte) (coreVersion, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return coreVersionUnknown, fmt.Errorf("decode core response envelope: %w", err)
	}
	_, hasCustomerID := top["customer_id"]
	_, hasAccounts := top["accounts"]
	_, hasClient := top["client"]
	_, hasPortfolios := top["portfolios"]

	anyV1 := hasCustomerID || hasAccounts
	anyV2 := hasClient || hasPortfolios

	switch {
	case anyV1 && anyV2:
		return coreVersionUnknown, errors.New("core response contains mixed contract signatures")
	case hasCustomerID && hasAccounts:
		return coreVersionV1, nil
	case hasClient && hasPortfolios:
		return coreVersionV2, nil
	default:
		return coreVersionUnknown, errors.New("core response does not match a supported contract signature")
	}
}

// strictDecodeCore decodes exactly one JSON value with unknown-field rejection
// at every depth and rejects any trailing content other than whitespace.
func strictDecodeCore(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode core response: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return errors.New("core response contains trailing data")
	}
	return nil
}

// coreParseMoney validates presence of both money members before constructing
// integer-minor-unit Money. strictCurrency enforces the v2 contract's
// uppercase three-letter code; the v1 path keeps domain.NewMoney's historically
// lenient handling. Explicit zero and negative minor units remain valid.
func coreParseMoney(minor *int64, currency *string, strictCurrency bool) (domain.Money, error) {
	if minor == nil || currency == nil {
		return domain.Money{}, errors.New("core account cash failed contract validation")
	}
	if strictCurrency && !coreCurrencyPattern.MatchString(*currency) {
		return domain.Money{}, errors.New("core account currency failed contract validation")
	}
	cash, err := domain.NewMoney(*minor, *currency)
	if err != nil {
		return domain.Money{}, fmt.Errorf("parse core cash: %w", err)
	}
	return cash, nil
}

// coreParsePosition validates a nonempty symbol and present quantity, then
// parses the quantity with the existing exact fixed-precision parser.
func coreParsePosition(symbol *string, quantity *string) (domain.Position, error) {
	if symbol == nil || *symbol == "" || quantity == nil {
		return domain.Position{}, errors.New("core position failed contract validation")
	}
	value, err := domain.ParseQuantity(*quantity)
	if err != nil {
		// The parse error embeds the offending quantity value; return a fixed
		// safe class instead so a payload-derived quantity cannot reach logs.
		return domain.Position{}, errors.New("core position quantity failed contract validation")
	}
	return domain.Position{Symbol: *symbol, Quantity: value}, nil
}

func mapCoreV1(payload coreV1Response, customerID string) ([]domain.Account, error) {
	if payload.CustomerID == nil || *payload.CustomerID != customerID {
		return nil, errors.New("core response failed identity validation")
	}
	if payload.Accounts == nil {
		return nil, errors.New("core response failed contract validation")
	}

	accounts := make([]domain.Account, 0, len(payload.Accounts))
	for _, item := range payload.Accounts {
		account, err := mapCoreV1Account(item)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func mapCoreV1Account(item coreV1Account) (domain.Account, error) {
	if item.AccountID == nil || *item.AccountID == "" ||
		item.AccountType == nil || *item.AccountType == "" {
		return domain.Account{}, errors.New("core account failed contract validation")
	}
	if item.CashBalance == nil {
		return domain.Account{}, errors.New("core account cash failed contract validation")
	}
	cash, err := coreParseMoney(item.CashBalance.AmountMinor, item.CashBalance.Currency, false)
	if err != nil {
		return domain.Account{}, err
	}
	if item.Positions == nil {
		return domain.Account{}, errors.New("core account positions failed contract validation")
	}
	positions := make([]domain.Position, 0, len(item.Positions))
	for _, position := range item.Positions {
		mapped, err := coreParsePosition(position.Symbol, position.Quantity)
		if err != nil {
			return domain.Account{}, err
		}
		positions = append(positions, mapped)
	}
	return domain.Account{ID: *item.AccountID, Type: *item.AccountType, Cash: cash, Positions: positions}, nil
}

func mapCoreV2(payload coreV2Response, customerID string) ([]domain.Account, error) {
	if payload.Client == nil || payload.Client.ID == nil || *payload.Client.ID != customerID {
		return nil, errors.New("core response failed identity validation")
	}
	if payload.Portfolios == nil {
		return nil, errors.New("core response failed contract validation")
	}

	accounts := make([]domain.Account, 0, len(payload.Portfolios))
	for _, item := range payload.Portfolios {
		account, err := mapCoreV2Portfolio(item)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

func mapCoreV2Portfolio(item coreV2Portfolio) (domain.Account, error) {
	if item.ID == nil || *item.ID == "" || item.Category == nil {
		return domain.Account{}, errors.New("core account failed contract validation")
	}
	if _, ok := coreCategories[*item.Category]; !ok {
		return domain.Account{}, errors.New("core account category failed contract validation")
	}
	if item.Balances == nil || item.Balances.Cash == nil {
		return domain.Account{}, errors.New("core account cash failed contract validation")
	}
	cash, err := coreParseMoney(item.Balances.Cash.MinorUnits, item.Balances.Cash.CurrencyCode, true)
	if err != nil {
		return domain.Account{}, err
	}
	if item.Assets == nil {
		return domain.Account{}, errors.New("core account assets failed contract validation")
	}
	positions := make([]domain.Position, 0, len(item.Assets))
	for _, asset := range item.Assets {
		mapped, err := coreParsePosition(asset.Ticker, asset.Units)
		if err != nil {
			return domain.Account{}, err
		}
		positions = append(positions, mapped)
	}
	return domain.Account{ID: *item.ID, Type: *item.Category, Cash: cash, Positions: positions}, nil
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
		return nil, fmt.Errorf("call core: %w", coreTransportError(err))
	}
	return response, nil
}

// coreTransportError strips the request URL, which contains the customer
// identifier, from a client error while preserving cancellation and deadline
// semantics for callers that inspect the error chain.
func coreTransportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
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
