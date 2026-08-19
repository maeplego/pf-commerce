package money

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrInvalid  = errors.New("invalid money")
	ErrCurrency = errors.New("currency mismatch")
	ErrOverflow = errors.New("money overflow")
)

// Amount is integer minor units (yen for JPY). Floats are rejected at the HTTP boundary.
type Amount struct {
	Minor    int64
	Currency string
}

func New(minor int64, currency string) (Amount, error) {
	if minor < 0 {
		return Amount{}, fmt.Errorf("%w: negative", ErrInvalid)
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return Amount{}, fmt.Errorf("%w: currency", ErrInvalid)
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return Amount{}, fmt.Errorf("%w: currency", ErrInvalid)
		}
	}
	return Amount{Minor: minor, Currency: currency}, nil
}

func JPY(minor int64) (Amount, error) {
	return New(minor, "JPY")
}

func (a Amount) Add(b Amount) (Amount, error) {
	if a.Currency != b.Currency {
		return Amount{}, ErrCurrency
	}
	if b.Minor > 0 && a.Minor > math.MaxInt64-b.Minor {
		return Amount{}, ErrOverflow
	}
	return Amount{Minor: a.Minor + b.Minor, Currency: a.Currency}, nil
}

func (a Amount) MulQty(qty int) (Amount, error) {
	if qty <= 0 {
		return Amount{}, fmt.Errorf("%w: qty", ErrInvalid)
	}
	if a.Minor > 0 && int64(qty) > math.MaxInt64/a.Minor {
		return Amount{}, ErrOverflow
	}
	return Amount{Minor: a.Minor * int64(qty), Currency: a.Currency}, nil
}
