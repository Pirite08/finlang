package repl

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestREPLState(t *testing.T) {
	input := "let amount = 0.1;\namount + 0.2\nlet = 4;\nmissing_name\nprint(amount);\nlet double = fn(x) { return x * 2; };\ndouble(amount)\n"
	var out, errOut bytes.Buffer
	if err := Run(strings.NewReader(input), &out, &errOut, config.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if out.String() != "0.3\n0.1\n0.2\n" {
		t.Fatalf("stdout=%q", &out)
	}
	if len(strings.Split(strings.TrimSpace(errOut.String()), "\n")) != 2 {
		t.Fatalf("stderr=%q", &errOut)
	}
}

func TestREPLBoundedLineContinues(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxSourceBytes = 32
	var out, errOut bytes.Buffer
	if err := Run(strings.NewReader(strings.Repeat(" ", 10000)+"\n0.1 + 0.2"), &out, &errOut, limits); err != nil {
		t.Fatal(err)
	}
	if out.String() != "0.3\n" || !strings.Contains(errOut.String(), "E_SOURCE_LIMIT") {
		t.Fatalf("stdout=%q stderr=%q", &out, &errOut)
	}
}

func TestREPLSourceLimitBoundary(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxSourceBytes = 32
	var out, errOut bytes.Buffer
	input := strings.Repeat(" ", 29) + "0.3\n0.4\r\n\n"
	if err := Run(strings.NewReader(input), &out, &errOut, limits); err != nil {
		t.Fatal(err)
	}
	if out.String() != "0.3\n0.4\n" || errOut.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", &out, &errOut)
	}
}

func TestREPLBudgetResetsAfterError(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxEvalSteps = 50
	var out, errOut bytes.Buffer
	input := "let recurse = fn(x) { return recurse(x); };\nrecurse(1)\n0.1 + 0.2\n"
	if err := Run(strings.NewReader(input), &out, &errOut, limits); err != nil {
		t.Fatal(err)
	}
	if out.String() != "0.3\n" || !strings.Contains(errOut.String(), "E_EXECUTION_LIMIT") {
		t.Fatalf("stdout=%q stderr=%q", &out, &errOut)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("input unavailable") }

func TestREPLReadError(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := Run(brokenReader{}, &out, &errOut, config.DefaultLimits()); err == nil {
		t.Fatal("expected input error")
	}
}

func TestREPLPrintOutputFailureStopsSession(t *testing.T) {
	reader, writer := io.Pipe()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	var errOut bytes.Buffer
	err := Run(strings.NewReader("print(1)\nmissing_after_failed_output\n"), writer, &errOut, config.DefaultLimits())
	if diagnostic.CodeOf(err) != "E_OUTPUT_IO" {
		t.Fatalf("error=%v, want fatal E_OUTPUT_IO", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("output failure was treated as recoverable or next statement executed: %q", &errOut)
	}
}
