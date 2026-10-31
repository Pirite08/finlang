package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jsonArgs(rule, input string) []string {
	return []string{"-file", filepath.Join("..", "..", "examples", rule+"_v1.fin"), "-contract", filepath.Join("..", "..", "contracts", rule+"_v1.json"), "-input", input, "-json"}
}

func decodeRecord(t *testing.T, data string) map[string]any {
	t.Helper()
	var record map[string]any
	d := json.NewDecoder(strings.NewReader(data))
	if err := d.Decode(&record); err != nil {
		t.Fatalf("invalid JSON output %q: %v", data, err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		t.Fatalf("stdout contains trailing data: %v", err)
	}
	return record
}

func TestJSONRulesAndTrace(t *testing.T) {
	for _, tc := range []struct{ rule, currency, amount string }{{"interest", "KRW", "25000"}, {"fee", "KRW", "1500"}, {"sum", "USD", "1.00"}} {
		t.Run(tc.rule, func(t *testing.T) {
			input := filepath.Join("..", "..", "testdata", "inputs", tc.rule+".json")
			path := filepath.Join(t.TempDir(), "nested", "trace.jsonl")
			args := append(jsonArgs(tc.rule, input), "-trace", path)
			var out, errOut bytes.Buffer
			code := Run(args, strings.NewReader(""), &out, &errOut)
			if code != 0 || errOut.Len() != 0 {
				t.Fatalf("exit=%d stderr=%s", code, &errOut)
			}
			got := decodeRecord(t, out.String())
			result := got["result"].(map[string]any)
			if got["status"] != "ok" || result["currency"] != tc.currency || result["amount"] != tc.amount {
				t.Fatalf("wrong result: %v", got)
			}
			traceData, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(traceData), []byte{'\n'})
			if len(lines) < 3 {
				t.Fatalf("trace lacks real events: %s", traceData)
			}
			header := decodeRecord(t, string(lines[0]))
			if header["record"] != "header" || header["program_sha256"] == "" || header["input"] == nil {
				t.Fatalf("bad header: %v", header)
			}
			var settlement bool
			for _, line := range lines[1 : len(lines)-1] {
				event := decodeRecord(t, string(line))
				if event["kind"] == "settle" {
					settlement = true
					if event["currency"] != tc.currency || event["source"] == nil {
						t.Fatalf("bad event: %v", event)
					}
				}
			}
			if !settlement {
				t.Fatal("actual settlement absent")
			}
			summary := decodeRecord(t, string(lines[len(lines)-1]))
			if summary["record"] != "summary" || summary["trace_truncated"] != false {
				t.Fatalf("bad summary: %v", summary)
			}
			second := path + ".second"
			args[len(args)-1] = second
			out.Reset()
			errOut.Reset()
			if code = Run(args, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatal(&errOut)
			}
			again, err := os.ReadFile(second)
			if err != nil || !bytes.Equal(traceData, again) {
				t.Fatalf("trace is not reproducible: %v", err)
			}
		})
	}
}

func TestJSONErrorsAreSingleRecords(t *testing.T) {
	good := filepath.Join("..", "..", "testdata", "inputs", "interest.json")
	cases := []struct {
		name          string
		source, input string
		code          int
		errorCode     string
	}{
		{"bad input", "", "{\"schema_version\":\"1\",\"rule\":\"interest_v1\",\"inputs\":{\"principal\":{\"currency\":\"KRW\",\"amount\":1000000},\"annual_rate\":\"0.05\",\"years\":\"0.5\"}}", 1, "E_CONTRACT_INPUT"},
		{"duplicate", "", "{\"schema_version\":\"1\",\"schema_version\":\"1\",\"rule\":\"interest_v1\",\"inputs\":{}}", 1, "E_CONTRACT_INPUT"},
		{"print", "print(1);let main=fn(a,b,c){return a;};", "", 1, "E_OUTPUT_SIDE_EFFECT"},
		{"missing entry", "let x=1;", "", 1, "E_CONTRACT_ENTRY"},
		{"unsettled", "let main=fn(a,b,c){return money(\"KRW\",1.5);};", "", 1, "E_OUTPUT_SCALE"},
		{"wrong currency", "let main=fn(a,b,c){return money(\"USD\",1);};", "", 1, "E_CONTRACT_OUTPUT"},
		{"evaluation", "let main=fn(a,b,c){return div(a,0,2,\"HALF_EVEN\");};", "", 1, "E_DIVISION_BY_ZERO"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := jsonArgs("interest", good)
			dir := t.TempDir()
			if tc.source != "" {
				path := filepath.Join(dir, "source.fin")
				os.WriteFile(path, []byte(tc.source), 0600)
				args[1] = path
			}
			if tc.input != "" {
				path := filepath.Join(dir, "input.json")
				os.WriteFile(path, []byte(tc.input), 0600)
				args[5] = path
			}
			var out, errOut bytes.Buffer
			got := Run(args, strings.NewReader(""), &out, &errOut)
			if got != tc.code {
				t.Fatalf("exit=%d want %d: %s", got, tc.code, &errOut)
			}
			record := decodeRecord(t, out.String())
			if record["status"] != "error" || record["error"].(map[string]any)["code"] != tc.errorCode {
				t.Fatalf("wrong error: %v stderr=%s", record, &errOut)
			}
		})
	}
}

func TestTraceFailurePreventsJSONSuccess(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	args := append(jsonArgs("interest", filepath.Join("..", "..", "testdata", "inputs", "interest.json")), "-trace", filepath.Join(blocker, "trace.jsonl"))
	var out, errOut bytes.Buffer
	if got := Run(args, strings.NewReader(""), &out, &errOut); got != 2 {
		t.Fatalf("exit=%d %s", got, &errOut)
	}
	record := decodeRecord(t, out.String())
	if record["status"] != "error" || record["error"].(map[string]any)["code"] != "E_TRACE_IO" {
		t.Fatalf("claimed success without trace: %v", record)
	}
}

func TestTraceCannotOverwriteInputs(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.fin")
	original := []byte("print(0.1+0.2);")
	if err := os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if got := Run([]string{"-file", source, "-trace", source}, strings.NewReader(""), &out, &errOut); got != 2 {
		t.Fatalf("exit=%d %s", got, &errOut)
	}
	contents, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(contents, original) {
		t.Fatal("source was overwritten")
	}
}

func TestTraceTruncationDoesNotChangeFileResult(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "many.fin")
	if err := os.WriteFile(source, []byte(strings.Repeat("0.1+0.2;", 10001)+"print(0.1+0.2);"), 0600); err != nil {
		t.Fatal(err)
	}
	tracePath := filepath.Join(dir, "trace.jsonl")
	var out, errOut bytes.Buffer
	if got := Run([]string{"-file", source, "-trace", tracePath}, strings.NewReader(""), &out, &errOut); got != 0 || out.String() != "0.3\n" {
		t.Fatalf("exit=%d out=%q stderr=%s", got, &out, &errOut)
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != 10002 {
		t.Fatalf("unbounded trace rows: %d", len(lines))
	}
	summary := decodeRecord(t, string(lines[len(lines)-1]))
	if summary["events_recorded"] != float64(10000) || summary["trace_truncated"] != true {
		t.Fatalf("bad truncation: %v", summary)
	}
}

func TestJSONModeUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"-json"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("exit=%d", code)
	}
	if got := decodeRecord(t, out.String()); got["status"] != "error" {
		t.Fatalf("wrong error: %v", got)
	}
}
