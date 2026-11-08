package eval

import (
	"fmt"
	"strings"

	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

const KindMatrix Kind = "Matrix"

// Matrix cells are immutable and stored in row-major order.
type matrixValue struct {
	rows, cols int
	cells      []decimal.Decimal
}

func (matrixValue) Kind() Kind { return KindMatrix }

func (v matrixValue) Inspect() string {
	return inspectComposite(v, KindMatrix, len(v.cells))
}

func (v matrixValue) appendInspection(text *strings.Builder, remaining *int) bool {
	if len(v.cells) > *remaining || !writeInspection(text, "matrix([") {
		return false
	}
	for row := range v.rows {
		if (row > 0 && !writeInspection(text, ", ")) || !writeInspection(text, "[") {
			return false
		}
		for column := range v.cols {
			if (column > 0 && !writeInspection(text, ", ")) ||
				!appendValueInspection(text, remaining, DecimalValue{Value: v.cells[row*v.cols+column]}) {
				return false
			}
		}
		if !writeInspection(text, "]") {
			return false
		}
	}
	return writeInspection(text, "])")
}

func (e *Evaluator) matrixBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	var result matrixValue
	var err error
	if name == "matrix" {
		result, err = e.newMatrix(args[0], span)
	} else {
		left, ok := args[0].(matrixValue)
		if !ok {
			return nil, typeError(name, args...)
		}
		switch name {
		case "transpose":
			result, err = e.transposeMatrix(left, span)
		case "mat_add", "mat_mul":
			right, ok := args[1].(matrixValue)
			if !ok {
				return nil, typeError(name, args...)
			}
			if name == "mat_add" {
				result, err = e.addMatrices(left, right, span)
			} else {
				result, err = e.multiplyMatrices(left, right, span)
			}
		default:
			return nil, diagnostic.New("E_INTERNAL", "unknown matrix builtin")
		}
	}
	if err != nil {
		return nil, err
	}
	e.emit(trace.Event{Kind: name, Span: span}, args, result)
	return result, nil
}

func (e *Evaluator) newMatrix(value Value, span diagnostic.Span) (matrixValue, error) {
	rows, ok := value.(listValue)
	if !ok {
		return matrixValue{}, typeError("matrix", value)
	}
	if len(rows.elements) == 0 {
		return matrixValue{}, diagnostic.New("E_MATRIX_SHAPE", "matrix must have at least one row")
	}
	first, ok := rows.elements[0].(listValue)
	if !ok {
		return matrixValue{}, typeError("matrix row", rows.elements[0])
	}
	height, width := len(rows.elements), len(first.elements)
	if width == 0 {
		return matrixValue{}, diagnostic.New("E_MATRIX_SHAPE", "matrix must have at least one column")
	}
	if err := e.chargeCollectionWork(int64(height), span); err != nil {
		return matrixValue{}, err
	}
	for index, item := range rows.elements {
		row, ok := item.(listValue)
		if !ok {
			return matrixValue{}, typeError("matrix row", item)
		}
		if len(row.elements) != width {
			return matrixValue{}, diagnostic.New("E_MATRIX_SHAPE", fmt.Sprintf("matrix row %d has %d columns; expected %d", index+1, len(row.elements), width))
		}
	}
	if err := e.chargeMatrixWork(height, width, 1, span); err != nil {
		return matrixValue{}, err
	}
	result := matrixValue{rows: height, cols: width, cells: make([]decimal.Decimal, 0, height*width)}
	for _, item := range rows.elements {
		for _, cell := range item.(listValue).elements {
			number, ok := cell.(DecimalValue)
			if !ok {
				return matrixValue{}, typeError("matrix cell", cell)
			}
			result.cells = append(result.cells, number.Value)
		}
	}
	return result, nil
}

func (e *Evaluator) addMatrices(left, right matrixValue, span diagnostic.Span) (matrixValue, error) {
	if left.rows != right.rows || left.cols != right.cols {
		message := fmt.Sprintf("matrix addition requires the same dimensions; received %dx%d and %dx%d", left.rows, left.cols, right.rows, right.cols)
		return matrixValue{}, diagnostic.New("E_MATRIX_SHAPE", message)
	}
	if err := e.chargeMatrixWork(left.rows, left.cols, 1, span); err != nil {
		return matrixValue{}, err
	}
	result := matrixValue{rows: left.rows, cols: left.cols, cells: make([]decimal.Decimal, len(left.cells))}
	for i, value := range left.cells {
		sum, err := decimal.Add(value, right.cells[i], e.options.Limits)
		if err != nil {
			return matrixValue{}, err
		}
		result.cells[i] = sum
	}
	return result, nil
}

func (e *Evaluator) multiplyMatrices(left, right matrixValue, span diagnostic.Span) (matrixValue, error) {
	if left.cols != right.rows {
		message := fmt.Sprintf("matrix multiplication requires matching inner dimensions; received %dx%d and %dx%d", left.rows, left.cols, right.rows, right.cols)
		return matrixValue{}, diagnostic.New("E_MATRIX_SHAPE", message)
	}
	if err := e.chargeMatrixWork(left.rows, right.cols, left.cols, span); err != nil {
		return matrixValue{}, err
	}
	result := matrixValue{rows: left.rows, cols: right.cols, cells: make([]decimal.Decimal, left.rows*right.cols)}
	for row := range result.rows {
		for column := range result.cols {
			var sum decimal.Decimal
			for k := range left.cols {
				product, err := decimal.Mul(left.cells[row*left.cols+k], right.cells[k*right.cols+column], e.options.Limits)
				if err != nil {
					return matrixValue{}, err
				}
				sum, err = decimal.Add(sum, product, e.options.Limits)
				if err != nil {
					return matrixValue{}, err
				}
			}
			result.cells[row*result.cols+column] = sum
		}
	}
	return result, nil
}

func (e *Evaluator) transposeMatrix(matrix matrixValue, span diagnostic.Span) (matrixValue, error) {
	if err := e.chargeMatrixWork(matrix.rows, matrix.cols, 1, span); err != nil {
		return matrixValue{}, err
	}
	result := matrixValue{rows: matrix.cols, cols: matrix.rows, cells: make([]decimal.Decimal, len(matrix.cells))}
	for row := range matrix.rows {
		for column := range matrix.cols {
			result.cells[column*result.cols+row] = matrix.cells[row*matrix.cols+column]
		}
	}
	return result, nil
}

// Check the cost against the remaining budget before multiplying dimensions
// or allocating the result. Matrix dimensions and per-cell work are positive.
func (e *Evaluator) chargeMatrixWork(rows, cols, perCell int, span diagnostic.Span) error {
	remaining := e.options.Limits.MaxEvalSteps - e.steps
	if rows > remaining/cols/perCell {
		return diagnostic.WithSpan(diagnostic.New("E_EXECUTION_LIMIT", "evaluation step limit exceeded"), span)
	}
	return e.chargeCollectionWork(int64(rows*cols*perCell), span)
}
