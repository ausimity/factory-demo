package domain

import "time"

type Position struct {
	Symbol   string
	Quantity Quantity
}

type Account struct {
	ID        string
	Type      string
	Cash      Money
	Positions []Position
}

type Price struct {
	Symbol string
	Value  Money
	AsOf   time.Time
}

type Holding struct {
	Symbol      string
	Quantity    Quantity
	UnitPrice   Money
	MarketValue Money
}

type AccountPortfolio struct {
	ID          string
	Type        string
	Cash        Money
	MarketValue Money
	Holdings    []Holding
}

type Portfolio struct {
	CustomerID       string
	AsOf             time.Time
	TotalMarketValue Money
	Accounts         []AccountPortfolio
}
