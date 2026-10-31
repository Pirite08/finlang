package decimal_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestRoundingOracleAgreement(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rounding_oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Metadata struct {
			Seed          int64  `json:"seed"`
			PythonVersion string `json:"python_version"`
			Context       struct {
				Precision int `json:"precision"`
			} `json:"context"`
		} `json:"metadata"`
		Cases []struct {
			ID, Op, A, B, Mode, Value, Error string
			Digits                           uint32
			Inexact                          bool
			Manual                           bool
		} `json:"cases"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) < 10000 || fixture.Metadata.Seed != 20261031 || fixture.Metadata.Context.Precision != 512 || fixture.Metadata.PythonVersion == "" {
		t.Fatalf("insufficient oracle fixture metadata/cases: %+v count=%d", fixture.Metadata, len(fixture.Cases))
	}
	// Manual cases are generated first and compared before random cases.
	counts := map[string]int{}
	manualCount := 0
	for i, tc := range fixture.Cases {
		mode, err := decimal.ParseMode(tc.Mode)
		if err != nil {
			t.Fatal(err)
		}
		a := mustParse(t, tc.A)
		var got decimal.Rounded
		switch tc.Op {
		case "round":
			got, err = decimal.Round(a, tc.Digits, mode, limits)
		case "div":
			got, err = decimal.Div(a, mustParse(t, tc.B), tc.Digits, mode, limits)
		default:
			t.Fatalf("unknown fixture operation %q", tc.Op)
		}
		if code := string(diagnostic.CodeOf(err)); code != tc.Error {
			t.Fatalf("case %d (%s): %s %s %s digits=%d mode=%s: error %q, want %q", i, tc.ID, tc.A, tc.Op, tc.B, tc.Digits, tc.Mode, code, tc.Error)
		}
		if err == nil && (got.Value.String() != tc.Value || got.Inexact != tc.Inexact) {
			t.Fatalf("case %d (%s): %s %s %s digits=%d mode=%s: value=%s inexact=%t; want %s,%t", i, tc.ID, tc.A, tc.Op, tc.B, tc.Digits, tc.Mode, got.Value.String(), got.Inexact, tc.Value, tc.Inexact)
		}
		if tc.Manual {
			manualCount++
		}
		category := tc.Op + ":" + tc.Error
		if err == nil {
			if got.Inexact {
				category += ":inexact"
			} else {
				category += ":exact"
			}
		}
		counts[category]++
	}
	if manualCount < 20 {
		t.Fatal("missing manual oracle cases")
	}
	t.Logf("Python=%s seed=%d cases=%d manual=%d outcomes=%v", fixture.Metadata.PythonVersion, fixture.Metadata.Seed, len(fixture.Cases), manualCount, counts)
}
