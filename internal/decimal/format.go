package decimal

import (
	"strings"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func (d Decimal) String() string {
	text := d.readCoefficient().String()
	if d.scale == 0 {
		return text
	}
	sign := ""
	if text[0] == '-' {
		sign, text = text[:1], text[1:]
	}
	n := int(d.scale)
	if len(text) <= n {
		text = strings.Repeat("0", n+1-len(text)) + text
	}
	point := len(text) - n
	return sign + text[:point] + "." + text[point:]
}

// FormatAt adds trailing zeros but never rounds a value.
func FormatAt(d Decimal, digits uint32, limits config.Limits) (string, error) {
	if digits > limits.MaxScale {
		return "", diagnostic.New("E_NUMERIC_LIMIT", "target scale exceeds numeric limit")
	}
	if d.scale > digits {
		return "", diagnostic.New("E_INEXACT", "format would discard significant digits")
	}
	text := d.String()
	if digits > d.scale {
		if d.scale == 0 {
			text += "."
		}
		text += strings.Repeat("0", int(digits-d.scale))
	}
	return text, nil
}
