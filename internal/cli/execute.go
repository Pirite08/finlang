package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
)

func readLimitedFile(path string, limits config.Limits) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, diagnostic.New("E_INPUT_IO", err.Error())
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limits.MaxSourceBytes)+1))
	if err != nil {
		return nil, diagnostic.New("E_INPUT_IO", err.Error())
	}
	if len(data) > limits.MaxSourceBytes {
		return nil, diagnostic.New("E_SOURCE_LIMIT", "input exceeds maximum byte length")
	}
	return data, nil
}

func exitCode(err error) int {
	switch diagnostic.CodeOf(err) {
	case "E_INPUT_IO", "E_OUTPUT_IO", "E_TRACE_IO":
		return 2
	}
	return 1
}

func (c command) runFile(limits config.Limits) int {
	protectedPaths := []string{c.file, c.contract, c.input, c.fxFixed}
	if err := validateTraceDestination(c.trace, protectedPaths); err != nil {
		return c.fail(err, 2)
	}
	source, err := readLimitedFile(c.file, limits)
	if err != nil {
		return c.fail(err, exitCode(err))
	}
	var spec contract.Spec
	var args []eval.Value
	var rawInput json.RawMessage
	if c.json {
		data, err := readLimitedFile(c.contract, limits)
		if err != nil {
			return c.fail(err, exitCode(err))
		}
		spec, err = contract.Load(bytes.NewReader(data), limits)
		if err != nil {
			return c.fail(err, 1)
		}
		c.rule = spec.Rule
		data, err = readLimitedFile(c.input, limits)
		if err != nil {
			return c.fail(err, exitCode(err))
		}
		args, err = contract.DecodeInput(bytes.NewReader(data), spec, limits)
		if err != nil {
			return c.fail(err, 1)
		}
		rawInput = append(json.RawMessage(nil), data...)
	}
	program, err := parser.Parse(c.file, string(source), limits)
	if err != nil {
		return c.fail(err, 1)
	}
	var recorder *trace.Recorder
	options := eval.Options{Limits: limits, Stdout: c.stdout, JSONMode: c.json}
	if program.Rule != nil {
		if !c.json {
			return c.fail(diagnostic.New("E_CLI_USAGE", "rule execution requires -json, -contract and -input"), 2)
		}
		if err := validateRuleContract(program.Rule, spec); err != nil {
			return c.fail(err, 1)
		}
		options.FXProvider, err = c.quoteProvider(limits)
		if err != nil {
			return c.fail(err, exitCode(err))
		}
	} else if c.hasFXSource() {
		return c.fail(diagnostic.New("E_CLI_USAGE", "FX source options require a rule declaration"), 2)
	}
	if c.trace != "" {
		recorder = trace.NewRecorder(limits.MaxTraceEvents)
		options.Trace = recorder
	}
	evaluator := eval.New(options)
	var result eval.Value
	var runErr error
	if program.Rule != nil {
		result, runErr = evaluator.RunRule(program.Rule, args[0])
	} else {
		env := eval.NewEnvironment(nil)
		result, runErr = evaluator.Run(program, env)
		if runErr == nil && c.json {
			entry, found := env.Lookup(spec.Entry)
			if !found || entry.Kind() != eval.KindFunction {
				runErr = diagnostic.New("E_CONTRACT_ENTRY", "contract entry must name a user-defined function")
			} else {
				result, runErr = evaluator.Call(entry, args, diagnostic.Span{File: c.file, Line: 1, Column: 1})
			}
		}
	}
	var output bytes.Buffer
	if runErr == nil && c.json {
		runErr = contract.EncodeResult(&output, spec, result)
	}
	if recorder != nil {
		hash := sha256.Sum256(source)
		header := trace.Header{SchemaVersion: "1", ProgramSHA256: hex.EncodeToString(hash[:]), ContractVersion: spec.SchemaVersion, RunnerVersion: "fin " + Version, Input: rawInput, Limits: limits, FXQuotes: evaluator.FXQuotes()}
		if err := writeTrace(c.trace, header, recorder, protectedPaths); err != nil {
			return c.fail(err, 2)
		}
	}
	if runErr != nil {
		return c.fail(runErr, exitCode(runErr))
	}
	if c.json {
		if _, err := output.WriteTo(c.stdout); err != nil {
			return (command{stderr: c.stderr}).fail(diagnostic.New("E_OUTPUT_IO", err.Error()), 2)
		}
	}
	return 0
}
