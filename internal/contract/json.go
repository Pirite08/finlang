package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/Pirite08/finlang/internal/config"
)

// readJSON bounds bytes before tokenization. UseNumber ensures that no JSON
// token, including rejected numeric amounts, ever passes through float64.
func readJSON(r io.Reader, limits config.Limits) (any, error) {
	if limits.MaxSourceBytes < 0 || limits.MaxSyntaxDepth < 1 {
		return nil, fmt.Errorf("invalid JSON resource limits")
	}
	data, err := io.ReadAll(io.LimitReader(r, int64(limits.MaxSourceBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limits.MaxSourceBytes {
		return nil, fmt.Errorf("JSON exceeds byte limit")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("JSON is not valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	value, err := scanValue(d, 0, limits.MaxSyntaxDepth)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON document")
		}
		return nil, err
	}
	return value, nil
}

func scanValue(d *json.Decoder, depth, maxDepth int) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	if depth >= maxDepth {
		return nil, fmt.Errorf("JSON exceeds nesting depth")
	}
	switch delim {
	case '{':
		result := map[string]any{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("expected object key")
			}
			if _, exists := result[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			result[key], err = scanValue(d, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
		}
		end, err := d.Token()
		if err != nil {
			return nil, err
		}
		if end != json.Delim('}') {
			return nil, fmt.Errorf("expected closing object")
		}
		return result, nil
	case '[':
		result := []any{}
		for d.More() {
			value, err := scanValue(d, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		end, err := d.Token()
		if err != nil {
			return nil, err
		}
		if end != json.Delim(']') {
			return nil, fmt.Errorf("expected closing array")
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unexpected delimiter")
	}
}

func object(value any, required, optional []string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected JSON object")
	}
	known := map[string]bool{}
	for _, key := range required {
		known[key] = true
		if _, present := object[key]; !present {
			return nil, fmt.Errorf("missing field %q", key)
		}
	}
	for _, key := range optional {
		known[key] = true
	}
	for key := range object {
		if !known[key] {
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}
	return object, nil
}

func stringField(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", key)
	}
	return value, nil
}

func scaleField(object map[string]any, key string, limits config.Limits) (uint32, error) {
	number, ok := object[key].(json.Number)
	if !ok {
		return 0, fmt.Errorf("field %q must be an integer", key)
	}
	n, err := strconv.ParseUint(string(number), 10, 32)
	if err != nil || n > uint64(limits.MaxScale) {
		return 0, fmt.Errorf("field %q exceeds allowed scale", key)
	}
	return uint32(n), nil
}
