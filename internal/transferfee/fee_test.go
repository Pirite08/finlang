package transferfee

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
)

func TestCalculateAndReplayTransferFee(t *testing.T) {
	for _, tc := range []struct {
		name, currency string
		amount, fee    int64
		amountText     string
		feeText        string
	}{
		{"won", "KRW", 10000, 500, "10000", "500"},
		{"cents", "USD", 10000, 500, "100.00", "5.00"},
		{"tie down to zero", "KRW", 10, 0, "10", "0"},
		{"tie up to even", "USD", 30, 2, "0.30", "0.02"},
		{"tie down to even", "USD", 50, 2, "0.50", "0.02"},
		{"largest amount", "KRW", math.MaxInt64, 461168601842738790, "9223372036854775807", "461168601842738790"},
		{"largest cents", "USD", math.MaxInt64, 461168601842738790, "92233720368547758.07", "4611686018427387.90"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fee, audit, err := Calculate(tc.currency, tc.amount)
			if err != nil || fee != tc.fee {
				t.Fatalf("Calculate = %d, %v, want %d", fee, err, tc.fee)
			}
			for _, text := range []string{
				`"execution_profile":"rule-only-v1"`,
				`"contract_version":"1"`,
				`"program_sha256":`, `"contract_sha256":`, `"runner_version":`,
				`"amount":"` + tc.amountText + `"`, `"amount":"` + tc.feeText + `"`,
				`"step":"calculate"`, `"step":"settle_fee"`,
			} {
				if !bytes.Contains(audit, []byte(text)) {
					t.Fatalf("audit missing %s: %s", text, audit)
				}
			}
			if replayed, err := Replay(audit, tc.currency, tc.amount); err != nil || replayed != fee {
				t.Fatalf("Replay = %d, %v, want %d", replayed, err, fee)
			}
		})
	}
}

func TestTransferFeeRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		currency string
		amount   int64
	}{{"EUR", 100}, {"usd", 100}, {"KRW", 0}, {"USD", -1}} {
		if _, _, err := Calculate(tc.currency, tc.amount); err == nil {
			t.Errorf("accepted currency=%s amount=%d", tc.currency, tc.amount)
		}
	}
}

func TestTransferFeeRejectsTamperedAudit(t *testing.T) {
	_, original, err := Calculate("USD", 10000)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		from string
		to   string
	}{
		{"duplicate", `"schema_version":"1"`, `"schema_version":"1","schema_version":"1"`},
		{"changed output", `"amount":"5.00"`, `"amount":"6.00"`},
		{"changed output scale", `"amount":"5.00"`, `"amount":"5"`},
		{"changed input", `"amount":"100.00"`, `"amount":"100.01"`},
		{"changed profile", `"rule-only-v1"`, `"rule-only-v2"`},
		{"changed hash", `"program_sha256":"`, `"program_sha256":"0`},
		{"changed step", `"step":"settle_fee"`, `"step":"other"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tampered := bytes.Replace(original, []byte(tc.from), []byte(tc.to), 1)
			if bytes.Equal(tampered, original) || !json.Valid(tampered) {
				t.Fatalf("mutation should produce different valid JSON: %s", tampered)
			}
			if _, err := Replay(tampered, "USD", 10000); err == nil {
				t.Fatal("accepted changed audit")
			}
		})
	}
	if _, err := Replay(original, "USD", 10001); err == nil {
		t.Error("accepted a different request amount")
	}
	if _, err := Replay(original, "KRW", 10000); err == nil {
		t.Error("accepted a different request currency")
	}
}
