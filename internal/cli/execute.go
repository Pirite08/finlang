package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"slices"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/fx"
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
	case "E_INPUT_IO", "E_OUTPUT_IO", "E_TRACE_IO", "E_AUDIT_IO", "E_OPS_LOG_IO":
		return 2
	}
	return 1
}

func (c command) runFile(limits config.Limits) (code int) {
	protectedPaths := []string{c.file, c.contract, c.input, c.fxFixed, c.fxSnapshot, c.replayAudit}
	calculationLog := c.trace
	logError := diagnostic.Code("E_TRACE_IO")
	if c.audit != "" {
		calculationLog, logError = c.audit, "E_AUDIT_IO"
	}
	if err := validateLogDestination(c.opsLog, append(protectedPaths, calculationLog), "E_OPS_LOG_IO"); err != nil {
		return c.fail(err, 2)
	}
	protectedPaths = append(protectedPaths, c.opsLog)
	if err := validateLogDestination(calculationLog, protectedPaths, logError); err != nil {
		return c.fail(err, 2)
	}
	var runID string
	if c.audit != "" || c.opsLog != "" {
		runID = rand.Text()
	}
	if c.opsLog != "" {
		operations, err := openOperations(c.opsLog)
		if err != nil {
			return c.fail(err, 2)
		}
		c.operationError = new(diagnostic.Code)
		defer func() { c.finishOperation(operations, runID, code) }()
	}
	source, err := readLimitedFile(c.file, limits)
	if err != nil {
		return c.fail(err, exitCode(err))
	}
	var spec contract.Spec
	var args []eval.Value
	var rawInput json.RawMessage
	var contractHash string
	var replay trace.Header
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
		if c.audit != "" || c.replayAudit != "" {
			contractHash = sha256Hex(data)
		}
		if c.replayAudit != "" {
			replay, err = loadAuditReplay(c.replayAudit, limits)
			if err != nil {
				return c.fail(err, exitCode(err))
			}
			if replay.ProgramSHA256 != sha256Hex(source) || replay.ContractSHA256 != contractHash || replay.Rule != spec.Rule || replay.ContractVersion != spec.SchemaVersion {
				return c.fail(diagnostic.New("E_AUDIT_REPLAY", "source or contract does not match the audit record"), 1)
			}
			c.ruleOnly = c.ruleOnly || replay.ExecutionProfile == ruleOnlyProfile
			data = replay.Input
		} else {
			data, err = readLimitedFile(c.input, limits)
			if err != nil {
				return c.fail(err, exitCode(err))
			}
		}
		args, err = contract.DecodeInput(bytes.NewReader(data), spec, limits)
		if err != nil {
			return c.fail(err, 1)
		}
		rawInput = data
	}
	program, err := parser.Parse(c.file, string(source), limits)
	if err != nil {
		return c.fail(err, 1)
	}
	if c.ruleOnly && program.Rule == nil {
		return c.fail(diagnostic.New("E_RULE_POLICY", "rule-only execution requires a rule declaration"), 1)
	}
	var recorder *trace.Recorder
	options := eval.Options{Limits: limits, Stdout: c.stdout, JSONMode: c.json, RuleOnly: c.ruleOnly}
	if program.Rule != nil {
		if !c.json {
			return c.fail(diagnostic.New("E_CLI_USAGE", "rule execution requires -json, -contract and -input"), 2)
		}
		args, err = ruleArguments(program.Rule, spec, args)
		if err != nil {
			return c.fail(err, 1)
		}
		switch {
		case c.replayAudit != "":
			var snapshot *fx.Snapshot
			snapshot, err = fx.NewSnapshot(replay.FXQuotes)
			if c.ruleOnly {
				options.FXSnapshot = snapshot
			} else if err == nil {
				options.FXProvider = snapshot
			}
		case c.ruleOnly:
			options.FXSnapshot, err = c.loadSuppliedQuotes(limits)
		default:
			options.FXProvider, err = c.quoteProvider(limits)
		}
		if err != nil {
			return c.fail(err, exitCode(err))
		}
	} else if c.hasFXSource() {
		return c.fail(diagnostic.New("E_CLI_USAGE", "FX source options require a rule declaration"), 2)
	}
	if calculationLog != "" {
		recorder = trace.NewRecorder(limits.MaxTraceEvents)
		options.Trace = recorder
	}
	evaluator := eval.New(options)
	var result eval.Value
	var runErr error
	if program.Rule != nil {
		result, runErr = evaluator.RunRule(program.Rule, args...)
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
	if c.replayAudit != "" && diagnostic.CodeOf(runErr) != "E_RULE_POLICY" {
		if runErr != nil || !slices.Equal(replay.FXQuotes, evaluator.FXQuotes()) {
			runErr = diagnostic.New("E_AUDIT_REPLAY", "execution did not use the recorded inputs and quotes successfully")
		} else {
			runErr = verifyAuditReplayResult(replay.Result, output.Bytes())
		}
	}
	if recorder != nil {
		header := trace.Header{SchemaVersion: "1", ProgramSHA256: sha256Hex(source), ContractVersion: spec.SchemaVersion, RunnerVersion: "fin " + Version, Input: rawInput, Limits: limits, FXQuotes: evaluator.FXQuotes()}
		if c.audit != "" {
			policy := c.fxPolicy.Snapshot(c.fxMode())
			header.RunID = runID
			header.Rule = spec.Rule
			header.ContractSHA256 = contractHash
			header.FXPolicy = &policy
			if c.ruleOnly {
				header.ExecutionProfile = ruleOnlyProfile
			}
			header.ErrorCode = diagnostic.CodeOf(runErr)
			if runErr == nil {
				header.Result = output.Bytes()
			}
		}
		if err := writeTrace(calculationLog, header, recorder, protectedPaths, c.audit != ""); err != nil {
			return c.fail(err, 2)
		}
	}
	if runErr != nil {
		return c.fail(runErr, exitCode(runErr))
	}
	if c.json {
		if _, err := output.WriteTo(c.stdout); err != nil {
			return (command{stderr: c.stderr, operationError: c.operationError}).fail(diagnostic.New("E_OUTPUT_IO", err.Error()), 2)
		}
	}
	return 0
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
