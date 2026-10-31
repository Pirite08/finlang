package eval

import (
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/money"
)

const KindMoney Kind = "Money"

type MoneyValue struct{ Value money.Money }

func (MoneyValue) Kind() Kind        { return KindMoney }
func (v MoneyValue) Inspect() string { return v.Value.Currency() + " " + v.Value.Amount().String() }

func (e *Evaluator) moneyInfix(operator string, left, right Value) (Value, error) {
	a, aMoney := left.(MoneyValue)
	b, bMoney := right.(MoneyValue)
	if aMoney && bMoney {
		var result money.Money
		var err error
		switch operator {
		case "+":
			result, err = money.Add(a.Value, b.Value, e.options.Limits)
		case "-":
			result, err = money.Sub(a.Value, b.Value, e.options.Limits)
		case "==", "!=", "<", "<=", ">", ">=":
			cmp, err := money.Cmp(a.Value, b.Value)
			if err != nil {
				return nil, err
			}
			switch operator {
			case "==":
				return BoolValue(cmp == 0), nil
			case "!=":
				return BoolValue(cmp != 0), nil
			case "<":
				return BoolValue(cmp < 0), nil
			case "<=":
				return BoolValue(cmp <= 0), nil
			case ">":
				return BoolValue(cmp > 0), nil
			case ">=":
				return BoolValue(cmp >= 0), nil
			}
		default:
			return nil, typeError(operator, left, right)
		}
		if err != nil {
			return nil, err
		}
		return MoneyValue{Value: result}, nil
	}
	if operator == "*" {
		var m money.Money
		var scalar decimal.Decimal
		if aMoney {
			n, ok := right.(DecimalValue)
			if !ok {
				return nil, typeError(operator, left, right)
			}
			m, scalar = a.Value, n.Value
		} else {
			n, ok := left.(DecimalValue)
			if !ok {
				return nil, typeError(operator, left, right)
			}
			m, scalar = b.Value, n.Value
		}
		result, err := money.Mul(m, scalar, e.options.Limits)
		if err != nil {
			return nil, err
		}
		return MoneyValue{Value: result}, nil
	}
	return nil, typeError(operator, left, right)
}
