package decimal

import (
	"example.com/finlang/internal/config"
	"example.com/finlang/internal/diagnostic"
	"math/big"
	"strings"
)

// Parse accepts an optional minus sign, ASCII integer digits and optional fraction.
func Parse(text string, limits config.Limits) (Decimal, error) {
	if len(text) > limits.MaxTokenBytes {
		return Decimal{}, diagnostic.New("E_NUMERIC_LIMIT", "number token exceeds byte limit")
	}
	invalid := func() (Decimal, error) {
		return Decimal{}, diagnostic.New("E_INVALID_NUMBER", "expected decimal digits with optional minus sign and fraction")
	}
	if text == "" {
		return invalid()
	}
	body := text
	if body[0] == '-' {
		body = body[1:]
	}
	if body == "" {
		return invalid()
	}
	point := -1
	for i := 0; i < len(body); i++ {
		if body[i] == '.' && point < 0 {
			point = i
			continue
		}
		if body[i] < '0' || body[i] > '9' {
			return invalid()
		}
	}
	if point == 0 || point == len(body)-1 {
		return invalid()
	}
	var scale uint32
	if point >= 0 {
		scale = uint32(len(body) - point - 1)
	}
	c, ok := new(big.Int).SetString(strings.ReplaceAll(text, ".", ""), 10)
	if !ok {
		return invalid()
	}
	return makeDecimal(c, scale, limits)
}
