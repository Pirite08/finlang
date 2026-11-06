package repl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
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

func TestREPLReleasesLongLinesWhileKeepingBindings(t *testing.T) {
	var script strings.Builder
	padding := strings.Repeat("x", 64<<10)
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&script, "let value%d = 1; // %s\n", i, padding)
		fmt.Fprintf(&script, "let add%d = fn(x) { return x + 1; }; // %s\n", i, padding)
	}
	script.WriteString("add15(value0)\n")
	gate := &pausedEOF{ready: make(chan struct{}), release: make(chan struct{})}
	input := io.MultiReader(strings.NewReader(script.String()), gate)
	var out, errOut bytes.Buffer
	var before, held runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	done := make(chan error, 1)
	go func() { done <- Run(input, &out, &errOut, config.DefaultLimits()) }()
	select {
	case <-gate.ready:
	case err := <-done:
		t.Fatalf("REPL stopped before the memory check: %v", err)
	}
	runtime.GC()
	runtime.ReadMemStats(&held)
	close(gate.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(&script)
	if out.String() != "2\n" || errOut.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", &out, &errOut)
	}
	// The 2 MiB of comments should be collectible while the REPL waits for input.
	if delta := int64(held.HeapAlloc) - int64(before.HeapAlloc); delta > 512<<10 {
		t.Fatalf("open session retained %d additional heap bytes", delta)
	}
}

type pausedEOF struct{ ready, release chan struct{} }

func (r *pausedEOF) Read([]byte) (int, error) {
	close(r.ready)
	<-r.release
	return 0, io.EOF
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
