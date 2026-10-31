package decimal_test

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/decimal"
)

// Boundary seeds run as normal regression cases too. Parse's token-size check
// bounds arbitrary fuzz input before numeric construction; no larger-input skip
// hides that check. Invalid input is allowed, a panic or failed round trip is not.
func FuzzDecimalParse(f *testing.F) {
	for _, text := range []string{
		"", "0", "-0", "-0.003", "12.30", "00012.0300",
		".5", "1.", "-", "+1", "--1", "1e3", "1,000", "NaN", " 1", "1 ", "١", "\xff",
		strings.Repeat("9", 127), strings.Repeat("9", 128), strings.Repeat("9", 129),
		"-" + strings.Repeat("9", 128),
		"0." + strings.Repeat("0", 34) + "1",
		"0." + strings.Repeat("0", 35) + "1",
		"0." + strings.Repeat("0", 36) + "1",
		"1." + strings.Repeat("0", 100),
		strings.Repeat("0", 255), strings.Repeat("0", 256), strings.Repeat("0", 257),
		"-" + strings.Repeat("0", 255), "-" + strings.Repeat("0", 256),
	} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		value, err := decimal.Parse(text, limits)
		if err != nil {
			return
		}
		canonical := value.String()
		again, err := decimal.Parse(canonical, limits)
		if err != nil {
			t.Fatalf("canonical round trip %q -> %q: %v", text, canonical, err)
		}
		if decimal.Cmp(value, again) != 0 || again.String() != canonical {
			t.Fatalf("unstable canonical round trip %q -> %q -> %q", text, canonical, again.String())
		}
	})
}
