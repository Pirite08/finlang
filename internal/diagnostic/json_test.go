package diagnostic

import (
	"encoding/json"
	"testing"
)

func TestSpanJSONContract(t *testing.T) {
	encoded, err := json.Marshal(Span{File: "demo.fin", Start: 0, End: 3, Line: 1, Column: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{\"file\":\"demo.fin\",\"start\":0,\"end\":3,\"line\":1,\"column\":1}" {
		t.Fatalf("source location JSON: %s", encoded)
	}
}
