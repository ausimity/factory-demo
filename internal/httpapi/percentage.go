package httpapi

import (
	"fmt"
	"regexp"
)

// canonicalPercentageDigits matches the canonical nonnegative integer digit
// forms domain.Percentage.String() produces: a lone zero or a leading-nonzero
// run. It rejects signs, leading zeros, decimals, and any non-digit content.
var canonicalPercentageDigits = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// percentageResponse owns the public wire representation of a display
// percentage. It is constructed only from domain.Percentage.String() and
// renders those digits as an exact unquoted base-10 JSON integer token,
// preserving arbitrary precision without narrowing through a machine integer.
// Keeping this concern in the HTTP boundary lets the domain value stay
// transport neutral.
type percentageResponse struct {
	digits string
}

// newPercentageResponse wraps canonical decimal digits produced by a domain
// percentage for HTTP serialization.
func newPercentageResponse(digits string) percentageResponse {
	return percentageResponse{digits: digits}
}

// MarshalJSON emits the canonical digits as an unquoted JSON integer. It rejects
// any non-canonical string so a malformed value fails serialization rather than
// producing invalid or quoted JSON.
func (p percentageResponse) MarshalJSON() ([]byte, error) {
	if !canonicalPercentageDigits.MatchString(p.digits) {
		return nil, fmt.Errorf("percentage response: non-canonical digits %q", p.digits)
	}
	return []byte(p.digits), nil
}
