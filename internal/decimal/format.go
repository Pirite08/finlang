package decimal

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"strings"
)

func (d Decimal) String() string {
	c := d.coeff()
	negative := c.Sign() < 0
	text := c.Abs(c).String()
	if d.scale > 0 {
		n := int(d.scale)
		if len(text) <= n {
			text = strings.Repeat("0", n+1-len(text)) + text
		}
		text = text[:len(text)-n] + "." + text[len(text)-n:]
	}
	if negative {
		text = "-" + text
	}
	return text
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
