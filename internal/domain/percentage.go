package domain

import (
	"errors"
	"math/big"
)

// ErrNegativePercentage is returned when constructing a Percentage from a nil or
// negative value. Display percentages are nonnegative by invariant; callers
// clamp negative ratios before construction.
var ErrNegativePercentage = errors.New("percentage must be nonnegative")

// Percentage is an immutable, transport-neutral whole-number display percentage.
// It preserves arbitrary precision through a canonical nonnegative base-10 digit
// string so total-consistent portfolios with offsetting components can render
// ratios beyond machine integers without narrowing through int or int64. It
// carries no JSON, OpenAPI, or metric annotations; the HTTP boundary renders it
// as an unquoted JSON integer token.
type Percentage struct {
	digits string
}

// NewPercentage builds a Percentage from a nonnegative arbitrary-precision
// integer. It copies the canonical decimal representation so the caller cannot
// mutate the stored value through the supplied big.Int afterward.
func NewPercentage(value *big.Int) (Percentage, error) {
	if value == nil || value.Sign() < 0 {
		return Percentage{}, ErrNegativePercentage
	}
	return Percentage{digits: value.String()}, nil
}

// String returns the canonical base-10 digits, "0" for the zero value.
func (p Percentage) String() string {
	if p.digits == "" {
		return "0"
	}
	return p.digits
}

// MarshalJSON renders the percentage as an unquoted base-10 JSON integer token,
// preserving every digit without narrowing through a machine integer.
func (p Percentage) MarshalJSON() ([]byte, error) {
	return []byte(p.String()), nil
}
