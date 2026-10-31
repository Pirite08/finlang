package experiments

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
)

// digitDecimal is an addition-only experimental representation. The coefficient
// contains ASCII magnitude digits; arithmetic carries/borrows one digit at a time.
// It never uses math/big, floating point, or the production Decimal internally.
type digitDecimal struct {
	negative    bool
	coefficient string
	scale       uint32
}

func normalizeDigits(negative bool, coefficient string, scale uint32, limits config.Limits) (digitDecimal, error) {
	coefficient = strings.TrimLeft(coefficient, "0")
	if coefficient == "" {
		return digitDecimal{}, nil
	}
	for scale > 0 && coefficient[len(coefficient)-1] == '0' {
		coefficient = coefficient[:len(coefficient)-1]
		scale--
	}
	if len(coefficient) > limits.MaxCoefficientDigits || scale > limits.MaxScale {
		return digitDecimal{}, diagnostic.New("E_NUMERIC_LIMIT", "experimental decimal exceeds numeric limits")
	}
	return digitDecimal{negative, coefficient, scale}, nil
}

func parseDigitDecimal(text string, limits config.Limits) (digitDecimal, error) {
	if len(text) > limits.MaxTokenBytes {
		return digitDecimal{}, diagnostic.New("E_NUMERIC_LIMIT", "experimental number token exceeds byte limit")
	}
	invalid := func() (digitDecimal, error) {
		return digitDecimal{}, diagnostic.New("E_INVALID_NUMBER", "expected signed decimal digits")
	}
	negative := strings.HasPrefix(text, "-")
	body := strings.TrimPrefix(text, "-")
	if body == "" {
		return invalid()
	}
	point := -1
	for i := range len(body) {
		if body[i] == '.' && point < 0 {
			point = i
		} else if body[i] < '0' || body[i] > '9' {
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
	return normalizeDigits(negative, strings.ReplaceAll(body, ".", ""), scale, limits)
}

func (d digitDecimal) String() string {
	text := d.coefficient
	if text == "" {
		return "0"
	}
	if d.scale > 0 {
		scale := int(d.scale)
		text = strings.Repeat("0", max(0, scale+1-len(text))) + text
		text = text[:len(text)-scale] + "." + text[len(text)-scale:]
	}
	if d.negative {
		text = "-" + text
	}
	return text
}

func addMagnitude(left, right string) string {
	out := make([]byte, max(len(left), len(right))+1)
	i, j, carry := len(left)-1, len(right)-1, 0
	for k := len(out) - 1; k >= 0; k-- {
		sum := carry
		if i >= 0 {
			sum += int(left[i] - '0')
			i--
		}
		if j >= 0 {
			sum += int(right[j] - '0')
			j--
		}
		out[k], carry = byte(sum%10)+'0', sum/10
	}
	return string(out)
}

// subtractMagnitude requires left >= right and returns their unsigned difference.
func subtractMagnitude(left, right string) string {
	out := make([]byte, len(left))
	j, borrow := len(right)-1, 0
	for i := len(left) - 1; i >= 0; i-- {
		difference := int(left[i]-'0') - borrow
		if j >= 0 {
			difference -= int(right[j] - '0')
			j--
		}
		borrow = 0
		if difference < 0 {
			difference += 10
			borrow = 1
		}
		out[i] = byte(difference) + '0'
	}
	return string(out)
}

func addDigitDecimal(a, b digitDecimal, limits config.Limits) (digitDecimal, error) {
	scale := max(a.scale, b.scale)
	align := func(d digitDecimal) string {
		coefficient := strings.TrimLeft(d.coefficient+strings.Repeat("0", int(scale-d.scale)), "0")
		if coefficient == "" {
			return "0"
		}
		return coefficient
	}
	left, right := align(a), align(b)
	if a.negative == b.negative {
		return normalizeDigits(a.negative, addMagnitude(left, right), scale, limits)
	}
	if len(left) < len(right) || len(left) == len(right) && left < right {
		return normalizeDigits(b.negative, subtractMagnitude(right, left), scale, limits)
	}
	return normalizeDigits(a.negative, subtractMagnitude(left, right), scale, limits)
}

type additionWorkload struct {
	name   string
	inputs []string
	want   string
}

func additionWorkloads() []additionWorkload {
	small := make([]string, 100)
	for i := range small {
		small[i] = "0.01"
	}
	return []additionWorkload{
		{"short_money", []string{"19.95", "0.05"}, "20"},
		{"mixed_scales", []string{"1.2", "0.003", "-0.00004"}, "1.20296"},
		{"long_coefficient", []string{strings.Repeat("9", 100) + ".99", "0.01"}, "1" + strings.Repeat("0", 100)},
		{"small_accumulation", small, "1"},
	}
}

func additionProgram(inputs []string) string {
	var source strings.Builder
	source.WriteString("let total0 = 0;\n")
	for i, text := range inputs {
		fmt.Fprintf(&source, "let total%d = total%d + %s;\n", i+1, i, text)
	}
	fmt.Fprintf(&source, "total%d;\n", len(inputs))
	return source.String()
}

func executeAdditionProgram(source string, limits config.Limits) (eval.Value, error) {
	program, err := parser.Parse("addition-benchmark.fin", source, limits)
	if err != nil {
		return nil, err
	}
	return eval.New(eval.Options{Limits: limits}).Run(program, eval.NewEnvironment(nil))
}

var additionFloatSink float64
var additionDecimalSink decimal.Decimal
var additionDigitSink digitDecimal
var additionExecutionSink eval.Value

// An operation parses the workload's entire input list. No arithmetic is timed.
func BenchmarkNumericParsing(b *testing.B) {
	limits := config.DefaultLimits()
	for _, workload := range additionWorkloads() {
		b.Run(workload.name, func(b *testing.B) {
			b.Run("float64", func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(len(workload.inputs)), "values/op")
				for i := 0; i < b.N; i++ {
					for _, text := range workload.inputs {
						value, err := strconv.ParseFloat(text, 64)
						if err != nil {
							b.Fatal(err)
						}
						additionFloatSink = value
					}
				}
			})
			b.Run("scaled_integer", func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(len(workload.inputs)), "values/op")
				for i := 0; i < b.N; i++ {
					for _, text := range workload.inputs {
						value, err := decimal.Parse(text, limits)
						if err != nil {
							b.Fatal(err)
						}
						additionDecimalSink = value
					}
				}
			})
			b.Run("digit_string", func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(len(workload.inputs)), "values/op")
				for i := 0; i < b.N; i++ {
					for _, text := range workload.inputs {
						value, err := parseDigitDecimal(text, limits)
						if err != nil {
							b.Fatal(err)
						}
						additionDigitSink = value
					}
				}
			})
		})
	}
}

// All representations start from zero and perform one addition per input. Inputs
// are prepared outside the timer; formatting and parsing are excluded. Each exact
// addition returns a normalized, limit-checked value. Float64 is approximate.
func BenchmarkAdditionRepresentation(b *testing.B) {
	limits := config.DefaultLimits()
	for _, workload := range additionWorkloads() {
		b.Run(workload.name, func(b *testing.B) {
			b.Run("float64", func(b *testing.B) {
				inputs := make([]float64, len(workload.inputs))
				for i, text := range workload.inputs {
					var err error
					inputs[i], err = strconv.ParseFloat(text, 64)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(len(inputs)), "adds/op")
				for i := 0; i < b.N; i++ {
					var sum float64
					for _, value := range inputs {
						sum += value
					}
					additionFloatSink = sum
				}
			})
			b.Run("scaled_integer", func(b *testing.B) {
				inputs := make([]decimal.Decimal, len(workload.inputs))
				for i, text := range workload.inputs {
					var err error
					inputs[i], err = decimal.Parse(text, limits)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(len(inputs)), "adds/op")
				for i := 0; i < b.N; i++ {
					var sum decimal.Decimal
					for _, value := range inputs {
						var err error
						sum, err = decimal.Add(sum, value, limits)
						if err != nil {
							b.Fatal(err)
						}
					}
					additionDecimalSink = sum
				}
			})
			b.Run("digit_string", func(b *testing.B) {
				inputs := make([]digitDecimal, len(workload.inputs))
				for i, text := range workload.inputs {
					var err error
					inputs[i], err = parseDigitDecimal(text, limits)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(len(inputs)), "adds/op")
				for i := 0; i < b.N; i++ {
					var sum digitDecimal
					for _, value := range inputs {
						var err error
						sum, err = addDigitDecimal(sum, value, limits)
						if err != nil {
							b.Fatal(err)
						}
					}
					additionDigitSink = sum
				}
			})
		})
	}
}

// Whole execution includes lexer, parser, fresh evaluator/environment and exact
// numeric evaluation. Source construction, CLI/process startup and I/O are excluded.
func BenchmarkInterpreterExecution(b *testing.B) {
	limits := config.DefaultLimits()
	for _, workload := range additionWorkloads() {
		b.Run(workload.name, func(b *testing.B) {
			source := additionProgram(workload.inputs)
			b.ReportAllocs()
			b.ResetTimer()
			b.ReportMetric(float64(len(workload.inputs)), "adds/op")
			for i := 0; i < b.N; i++ {
				value, err := executeAdditionProgram(source, limits)
				if err != nil {
					b.Fatal(err)
				}
				additionExecutionSink = value
			}
		})
	}
}
