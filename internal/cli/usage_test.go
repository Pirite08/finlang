package cli

import (
	"bytes"
	"strings"
	"testing"
)

// These exercise Run's actual record emission, including errors before -json.
func TestUsageErrorJSONIntent(t *testing.T) {
	type usageCase struct {
		name string
		args []string
		json bool
	}
	cases := []usageCase{
		{"unknown first", []string{"-unknown", "-json"}, true},
		{"json first", []string{"-json", "-unknown"}, true},
		{"double dash spelling", []string{"-unknown", "--json"}, true},
		{"explicit true", []string{"-unknown", "-json=true"}, true},
		{"explicit false", []string{"-unknown", "-json=false"}, false},
		{"last false after error", []string{"-json", "-unknown", "--json=false"}, false},
		{"last true after error", []string{"-json=false", "-unknown", "--json=true"}, true},
		{"terminator", []string{"-unknown", "--", "-json"}, false},
		{"parsed terminator", []string{"--", "-json"}, false},
		{"positional stops flags", []string{"-unknown", "source.fin", "-json"}, false},
		{"bool does not consume next", []string{"-unknown", "-version", "-json"}, true},
		{"invalid bool is not true", []string{"-unknown", "-json=invalid"}, false},
		{"filename before error", []string{"-file", "-json", "-unknown"}, false},
		{"equals filename before error", []string{"-file=-json", "-unknown"}, false},
		{"string consumes terminator spelling", []string{"-unknown", "-file", "--", "-json"}, true},
	}
	for _, name := range []string{"file", "contract", "input", "trace"} {
		cases = append(cases,
			usageCase{name + " value", []string{"-unknown", "-" + name, "-json"}, false},
			usageCase{name + " equals value", []string{"-unknown", "--" + name + "=-json"}, false},
			usageCase{name + " value then mode", []string{"-unknown", "-" + name, "-json", "-json"}, true},
		)
	}
	for _, value := range []string{"1", "t", "T", "TRUE", "true", "True"} {
		cases = append(cases, usageCase{"true " + value, []string{"-unknown", "--json=" + value}, true})
	}
	for _, value := range []string{"0", "f", "F", "FALSE", "false", "False"} {
		cases = append(cases, usageCase{"false " + value, []string{"-json", "-unknown", "--json=" + value}, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := Run(tc.args, strings.NewReader(""), &out, &errOut); code != 2 || errOut.Len() == 0 {
				t.Fatalf("exit=%d stderr=%q", code, &errOut)
			}
			if !tc.json {
				if out.Len() != 0 {
					t.Fatalf("unexpected JSON stdout: %q", &out)
				}
				return
			}
			record := decodeRecord(t, out.String())
			if record["status"] != "error" || record["error"].(map[string]any)["code"] != "E_CLI_USAGE" {
				t.Fatalf("wrong error record: %v", record)
			}
		})
	}
}
