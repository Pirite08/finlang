package eval

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const (
	KindRecord Kind = "Record"
	KindMap    Kind = "Map"
)

type namedValue struct {
	name  string
	value Value
}

// RecordField is a field supplied by the trusted contract decoder.
type RecordField struct {
	Name  string
	Value Value
}

type recordValue struct{ fields []namedValue }

// Map values own the lookup table and retain the first insertion order.
type mapValue struct {
	keys   []string
	values map[string]Value
}

// NewRecordValue copies the validated fields of a record in declaration order.
func NewRecordValue(fields []RecordField) Value {
	owned := make([]namedValue, len(fields))
	for i, field := range fields {
		owned[i] = namedValue{name: field.Name, value: field.Value}
	}
	return recordValue{fields: owned}
}

// RecordFields returns a copy of a Record's fields in declaration order.
func RecordFields(value Value) ([]RecordField, bool) {
	record, ok := value.(recordValue)
	if !ok {
		return nil, false
	}
	fields := make([]RecordField, len(record.fields))
	for i, field := range record.fields {
		fields[i] = RecordField{Name: field.name, Value: field.value}
	}
	return fields, true
}

// MapEntry is a read-only snapshot of an entry in an immutable Map.
type MapEntry struct {
	Key   string
	Value Value
}

// MapEntries returns a copy of a Map's entries in insertion order.
func MapEntries(value Value) ([]MapEntry, bool) {
	mapping, ok := value.(mapValue)
	if !ok {
		return nil, false
	}
	entries := make([]MapEntry, len(mapping.keys))
	for i, key := range mapping.keys {
		entries[i] = MapEntry{Key: key, Value: mapping.values[key]}
	}
	return entries, true
}

func (recordValue) Kind() Kind { return KindRecord }
func (mapValue) Kind() Kind    { return KindMap }

func (v recordValue) Inspect() string {
	return inspectComposite(v, KindRecord, len(v.fields))
}

func (v mapValue) Inspect() string {
	return inspectComposite(v, KindMap, len(v.keys))
}

func (v recordValue) appendInspection(text *strings.Builder, remaining *int) bool {
	if !writeInspection(text, "record {") {
		return false
	}
	for i, field := range v.fields {
		if !appendFieldInspection(text, remaining, i, field.name, field.value) {
			return false
		}
	}
	return writeInspection(text, "}")
}

func (v mapValue) appendInspection(text *strings.Builder, remaining *int) bool {
	if !writeInspection(text, "map {") {
		return false
	}
	for i, key := range v.keys {
		if len(key) > maxInspectionBytes || !appendFieldInspection(text, remaining, i, strconv.Quote(key), v.values[key]) {
			return false
		}
	}
	return writeInspection(text, "}")
}

func appendFieldInspection(text *strings.Builder, remaining *int, index int, name string, value Value) bool {
	return (index == 0 || writeInspection(text, ", ")) &&
		writeInspection(text, name) && writeInspection(text, ": ") &&
		appendValueInspection(text, remaining, value)
}

const maxInspectionBytes = 4096

type compositeInspection interface {
	appendInspection(*strings.Builder, *int) bool
}

func inspectComposite(value compositeInspection, kind Kind, count int) string {
	var text strings.Builder
	remaining := 64
	if value.appendInspection(&text, &remaining) {
		return text.String()
	}
	return fmt.Sprintf("%s(%d items, ...)", kind, count)
}

// Nested composites share one display budget, even when several fields
// refer to the same value.
func appendValueInspection(text *strings.Builder, remaining *int, value Value) bool {
	if *remaining == 0 {
		return false
	}
	*remaining = *remaining - 1
	if composite, ok := value.(compositeInspection); ok {
		return composite.appendInspection(text, remaining)
	}
	return writeInspection(text, value.Inspect())
}

func writeInspection(text *strings.Builder, value string) bool {
	if len(value) > maxInspectionBytes-text.Len() {
		return false
	}
	text.WriteString(value)
	return true
}

func (e *Evaluator) record(node *ast.Record, env *Environment) (Value, error) {
	capacity := min(len(node.Fields), e.options.Limits.MaxEvalSteps-e.steps)
	fields := make([]namedValue, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	for _, field := range node.Fields {
		if _, duplicate := seen[field.Name]; duplicate {
			err := diagnostic.New("E_DUPLICATE_FIELD", fmt.Sprintf("record field %q is repeated", field.Name))
			return nil, diagnostic.WithSpan(err, field.Span)
		}
		value, err := e.expression(field.Value, env)
		if err != nil {
			return nil, err
		}
		seen[field.Name] = struct{}{}
		fields = append(fields, namedValue{name: field.Name, value: value})
	}
	return recordValue{fields: fields}, nil
}

func (e *Evaluator) mapLiteral(node *ast.Map, env *Environment) (Value, error) {
	capacity := min(len(node.Entries), e.options.Limits.MaxEvalSteps-e.steps)
	result := mapValue{keys: make([]string, 0, capacity), values: make(map[string]Value, capacity)}
	for _, entry := range node.Entries {
		if _, duplicate := result.values[entry.Name]; duplicate {
			err := diagnostic.New("E_DUPLICATE_KEY", fmt.Sprintf("map key %q is repeated", entry.Name))
			return nil, diagnostic.WithSpan(err, entry.Span)
		}
		value, err := e.expression(entry.Value, env)
		if err != nil {
			return nil, err
		}
		result.keys = append(result.keys, entry.Name)
		result.values[entry.Name] = value
	}
	return result, nil
}

func (e *Evaluator) member(node *ast.Member, env *Environment) (Value, error) {
	receiver, err := e.expression(node.Receiver, env)
	if err != nil {
		return nil, err
	}
	record, ok := receiver.(recordValue)
	if !ok {
		return nil, typeError("field access", receiver)
	}
	for _, field := range record.fields {
		if err := e.tick(node.GetSpan()); err != nil {
			return nil, err
		}
		if field.name == node.Name {
			return field.value, nil
		}
	}
	return nil, diagnostic.New("E_MISSING_FIELD", fmt.Sprintf("record field %q does not exist", node.Name))
}

func (v mapValue) lookup(key Value) (Value, error) {
	text, ok := key.(StringValue)
	if !ok {
		return nil, typeError("map key lookup", key)
	}
	value, found := v.values[string(text)]
	if !found {
		return nil, diagnostic.New("E_MISSING_KEY", fmt.Sprintf("map key %q does not exist", string(text)))
	}
	return value, nil
}
