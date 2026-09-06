package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrInvalidQuantity  = errors.New("invalid quantity")
	ErrFractionalMinor  = errors.New("value cannot be represented in minor units")
)

// Money stores currency amounts in minor units to avoid floating-point errors.
type Money struct {
	Minor    int64
	Currency string
}

func NewMoney(minor int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return Money{}, fmt.Errorf("currency must be a three-letter code")
	}
	return Money{Minor: minor, Currency: currency}, nil
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{Minor: m.Minor + other.Minor, Currency: m.Currency}, nil
}

func (m Money) Amount() string {
	sign := ""
	minor := m.Minor
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/100, minor%100)
}

// Quantity stores up to six decimal places as millionths of a unit.
type Quantity struct {
	millionths int64
}

func ParseQuantity(value string) (Quantity, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") {
		return Quantity{}, fmt.Errorf("%w: %q", ErrInvalidQuantity, value)
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 {
		return Quantity{}, fmt.Errorf("%w: %q", ErrInvalidQuantity, value)
	}

	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Quantity{}, fmt.Errorf("%w: %q", ErrInvalidQuantity, value)
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 6 {
		return Quantity{}, fmt.Errorf("%w: precision exceeds six places", ErrInvalidQuantity)
	}
	for _, character := range fraction {
		if character < '0' || character > '9' {
			return Quantity{}, fmt.Errorf("%w: %q", ErrInvalidQuantity, value)
		}
	}
	fraction += strings.Repeat("0", 6-len(fraction))

	fractional, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil && fraction != "" {
		return Quantity{}, fmt.Errorf("%w: %q", ErrInvalidQuantity, value)
	}

	return Quantity{millionths: whole*1_000_000 + fractional}, nil
}

func (q Quantity) String() string {
	whole := q.millionths / 1_000_000
	fraction := q.millionths % 1_000_000
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return fmt.Sprintf("%d.%s", whole, strings.TrimRight(fmt.Sprintf("%06d", fraction), "0"))
}

func (q Quantity) MarketValue(price Money) (Money, error) {
	product := q.millionths * price.Minor
	if product%1_000_000 != 0 {
		return Money{}, ErrFractionalMinor
	}
	return Money{Minor: product / 1_000_000, Currency: price.Currency}, nil
}
