package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsage(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		code   int
		output string
	}{
		{[]string{"-version"}, 0, "fin v0.3.0"},
		{[]string{"-h"}, 0, "Usage:"},
		{[]string{"-unknown"}, 2, ""},
		{[]string{"unexpected"}, 2, ""},
		{[]string{"-file"}, 2, ""},
		{[]string{"-file", ""}, 2, ""},
		{[]string{"-version", "-file", "x.fin"}, 2, ""},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := Run(tc.args, strings.NewReader(""), &out, &errOut); got != tc.code {
				t.Fatalf("exit=%d, want %d: %s", got, tc.code, &errOut)
			}
			if !strings.Contains(out.String(), tc.output) {
				t.Fatalf("stdout=%q", &out)
			}
			if tc.code == 2 && errOut.Len() == 0 {
				t.Fatal("usage error lacks diagnostic")
			}
		})
	}
}

func TestFileRun(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"decimal.fin", "0.3\ntrue\n0.30\n"},
		{"simple_interest_decimal.fin", "25000\n0.333333\n"},
		{"rounding.fin", "-2\n-3\n-2\n-2\n10.00\n1.24\n1.25\n"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run([]string{"-file", filepath.Join("..", "..", "examples", tc.file)}, strings.NewReader(""), &out, &errOut)
			if code != 0 || out.String() != tc.want || errOut.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &errOut)
			}
		})
	}
}

func TestDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, source, contains string
		code                   int
	}{
		{"syntax", "let = 1;", ":1:", 1},
		{"division", "print(1 / 3);", "E_DIVISION_POLICY_REQUIRED", 1},
		{"eval", "let recurse = fn(x) { return recurse(x); }; recurse(1);", "E_EXECUTION_LIMIT", 1},
		{"source_limit", strings.Repeat(" ", (1<<20)+1), "E_SOURCE_LIMIT", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.fin")
			if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			if code := Run([]string{"-file", path}, strings.NewReader(""), &out, &errOut); code != tc.code {
				t.Fatalf("exit=%d: %s", code, &errOut)
			}
			if !strings.Contains(errOut.String(), tc.contains) {
				t.Fatalf("stderr=%q", &errOut)
			}
		})
	}
	t.Run("read failure", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if code := Run([]string{"-file", filepath.Join(t.TempDir(), "missing.fin")}, strings.NewReader(""), &out, &errOut); code != 2 {
			t.Fatalf("exit=%d", code)
		}
		if errOut.Len() == 0 {
			t.Fatal("missing read diagnostic")
		}
	})
}

func TestNoArgumentsStartsREPL(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(nil, strings.NewReader("0.1 + 0.2\n"), &out, &errOut); code != 0 || !strings.Contains(out.String(), "0.3") || errOut.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", &out, &errOut)
	}
}

func TestOutputFailureIsHostIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "print.fin")
	if err := os.WriteFile(path, []byte("print(1);"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		args  []string
		input string
	}{
		{"help", []string{"-h"}, ""},
		{"version", []string{"-version"}, ""},
		{"file print", []string{"-file", path}, ""},
		{"REPL print", nil, "print(1)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			var errOut bytes.Buffer
			if code := Run(tc.args, strings.NewReader(tc.input), writer, &errOut); code != 2 {
				t.Fatalf("exit=%d, want host I/O exit 2; stderr=%q", code, &errOut)
			}
			if errOut.Len() == 0 {
				t.Fatal("output failure lacks diagnostic")
			}
		})
	}
}
