// Package repl provides a bounded, single-line read-eval-print loop.
package repl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
)

// Run preserves bindings across lines and resets execution budgets for each line.
// Source and evaluation errors are reported and reading continues; I/O errors return.
func Run(in io.Reader, out, errOut io.Writer, limits config.Limits) error {
	if limits.MaxSourceBytes <= 0 {
		limits.MaxSourceBytes = config.DefaultLimits().MaxSourceBytes
	}
	r := bufio.NewReader(in)
	env := eval.NewEnvironment(nil)
	for lineNumber := 1; ; lineNumber++ {
		source, oversized, readErr := readLine(r, limits.MaxSourceBytes)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if len(source) == 0 && !oversized && errors.Is(readErr, io.EOF) {
			return nil
		}
		file := fmt.Sprintf("<repl:%d>", lineNumber)
		var runErr error
		if oversized {
			runErr = diagnostic.WithSpan(diagnostic.New("E_SOURCE_LIMIT", "line exceeds maximum source byte length"), diagnostic.Span{File: file, Line: 1, Column: 1})
		} else {
			program, err := parser.ParseDetached(file, string(source), limits)
			runErr = err
			if err == nil {
				value, err := eval.New(eval.Options{Limits: limits, Stdout: out}).Run(program, env)
				runErr = err
				if err == nil && value.Kind() != eval.KindUnit {
					if _, err := fmt.Fprintln(out, value.Inspect()); err != nil {
						return err
					}
				}
			}
		}
		if runErr != nil {
			if diagnostic.CodeOf(runErr) == "E_OUTPUT_IO" {
				return runErr
			}
			if _, err := fmt.Fprintln(errOut, runErr); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

// readLine drains an oversized line without ever retaining more than max bytes.
func readLine(r *bufio.Reader, max int) ([]byte, bool, error) {
	var source []byte
	oversized := false
	for {
		part, err := r.ReadSlice('\n')
		part = bytes.TrimSuffix(part, []byte{'\n'})
		if !oversized {
			if len(part) > max-len(source) {
				oversized = true
				source = nil
			} else {
				source = append(source, part...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return bytes.TrimSuffix(source, []byte{'\r'}), oversized, err
	}
}
