// Package cli implements the fin command line.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/repl"
)

const Version = "v0.5.0"
const help = "Usage: fin [-file path [-trace path]] | [-version] | [-h]\n" +
	"  no arguments  start the single-line REPL\n" +
	"  -file path    execute a UTF-8 source file\n" +
	"  -json         run the contract entry and emit one JSON result\n" +
	"  -contract path  JSON contract (required with -json)\n" +
	"  -input path     JSON inputs (required with -json)\n" +
	"  -trace path     write bounded JSONL calculation records\n" +
	"  -ops-log path   append a minimal JSONL execution status\n" +
	"  -audit path     create a sensitive JSONL audit record for a JSON execution\n" +
	"  -replay-audit path  replay a successful audit offline instead of -input and FX options\n" +
	"  -rule-only     allow only preflight-checked rules and supplied FX snapshots\n" +
	"  -fx-fixed path  use a saved FX quote for a rule\n" +
	"  -fx-snapshot path  use a JSON array of saved quotes with -rule-only\n" +
	"  -fx-url URL     request a latest FX quote from a JSON endpoint\n" +
	"  -fx-provider alpha-vantage  use FINLANG_ALPHA_VANTAGE_API_KEY\n" +
	"  -fx-max-age duration  maximum age for a live quote (default 5m)\n" +
	"  -fx-min-rate decimal  minimum live rate in KRW per USD (default " + fx.DefaultMinRate + ")\n" +
	"  -fx-max-rate decimal  maximum live rate in KRW per USD (default " + fx.DefaultMaxRate + ")\n" +
	"  -version      print the version\n" +
	"  -h            show help\n"

type command struct {
	file, contract, input, trace, rule     string
	opsLog, audit, replayAudit             string
	fxFixed, fxSnapshot, fxURL, fxProvider string
	fxPolicy                               fx.LivePolicy
	json, ruleOnly                         bool
	stdout, stderr                         io.Writer
	operationError                         *diagnostic.Code
}

// Run returns 0 on success, 1 for language/contract errors, 2 for usage/host I/O.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	c := command{stdout: stdout, stderr: stderr}
	flags := flag.NewFlagSet("fin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&c.file, "file", "", "source file")
	flags.StringVar(&c.contract, "contract", "", "JSON contract")
	flags.StringVar(&c.input, "input", "", "JSON inputs")
	flags.StringVar(&c.trace, "trace", "", "JSONL trace output")
	flags.StringVar(&c.opsLog, "ops-log", "", "minimal JSONL operational log")
	flags.StringVar(&c.audit, "audit", "", "new JSONL audit file")
	flags.StringVar(&c.replayAudit, "replay-audit", "", "successful JSONL audit to replay offline")
	flags.BoolVar(&c.ruleOnly, "rule-only", false, "restricted rule execution without live FX")
	flags.StringVar(&c.fxFixed, "fx-fixed", "", "saved FX quote")
	flags.StringVar(&c.fxSnapshot, "fx-snapshot", "", "saved FX quote array for rule-only mode")
	flags.StringVar(&c.fxURL, "fx-url", "", "live FX JSON endpoint")
	flags.StringVar(&c.fxProvider, "fx-provider", "", "live FX provider")
	flags.DurationVar(&c.fxPolicy.MaxAge, "fx-max-age", 5*time.Minute, "maximum live FX quote age")
	minRate := flags.String("fx-min-rate", fx.DefaultMinRate, "minimum live rate in KRW per USD")
	maxRate := flags.String("fx-max-rate", fx.DefaultMaxRate, "maximum live rate in KRW per USD")
	flags.BoolVar(&c.json, "json", false, "structured result mode")
	version := flags.Bool("version", false, "print version")
	wantHelp := flags.Bool("h", false, "show help")
	flags.Usage = func() { fmt.Fprint(stderr, help) }
	if err := flags.Parse(args); err != nil {
		c.json = jsonErrorIntent(flags, args)
		return c.fail(diagnostic.New("E_CLI_USAGE", err.Error()), 2)
	}
	provided := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if flags.NArg() != 0 {
		return c.fail(diagnostic.New("E_CLI_USAGE", "positional arguments are unsupported"), 2)
	}
	if *wantHelp || *version {
		if flags.NFlag() != 1 {
			return c.fail(diagnostic.New("E_CLI_USAGE", "choose help or version without execution options"), 2)
		}
		var err error
		if *wantHelp {
			_, err = fmt.Fprint(stdout, help)
		} else {
			_, err = fmt.Fprintln(stdout, "fin "+Version)
		}
		if err != nil {
			return c.fail(diagnostic.New("E_OUTPUT_IO", err.Error()), 2)
		}
		return 0
	}
	for name, path := range map[string]string{"file": c.file, "contract": c.contract, "input": c.input, "trace": c.trace, "ops-log": c.opsLog, "audit": c.audit, "replay-audit": c.replayAudit, "fx-fixed": c.fxFixed, "fx-snapshot": c.fxSnapshot, "fx-url": c.fxURL, "fx-provider": c.fxProvider} {
		if provided[name] && path == "" {
			return c.fail(diagnostic.New("E_CLI_USAGE", "-"+name+" requires a nonempty path"), 2)
		}
	}
	if c.json {
		if c.file == "" || c.contract == "" || (c.input == "") == (c.replayAudit == "") {
			return c.fail(diagnostic.New("E_CLI_USAGE", "-json requires -file, -contract and one of -input or -replay-audit"), 2)
		}
	} else if provided["contract"] || provided["input"] || c.audit != "" || c.replayAudit != "" {
		return c.fail(diagnostic.New("E_CLI_USAGE", "-contract, -input, -audit and -replay-audit require -json"), 2)
	}
	if c.audit != "" && c.trace != "" {
		return c.fail(diagnostic.New("E_CLI_USAGE", "choose one of -audit or -trace"), 2)
	}
	if c.replayAudit != "" && c.hasFXSource() {
		return c.fail(diagnostic.New("E_CLI_USAGE", "-replay-audit uses only the recorded quotes"), 2)
	}
	if c.ruleOnly && (!c.json || c.fxURL != "" || c.fxProvider != "") {
		return c.fail(diagnostic.New("E_CLI_USAGE", "-rule-only requires -json and does not allow live FX providers"), 2)
	}
	if c.fxSnapshot != "" && !c.ruleOnly {
		return c.fail(diagnostic.New("E_CLI_USAGE", "-fx-snapshot requires -rule-only"), 2)
	}
	if err := c.validateFXFlags(provided["fx-max-age"]); err != nil {
		return c.fail(err, 2)
	}
	if provided["fx-min-rate"] || provided["fx-max-rate"] {
		if c.fxURL == "" && c.fxProvider == "" {
			return c.fail(diagnostic.New("E_CLI_USAGE", "FX rate bounds require a live FX source"), 2)
		}
		bounds, err := fx.ParseRateBounds(*minRate, *maxRate)
		if err != nil {
			return c.fail(err, 2)
		}
		c.fxPolicy.Rates = bounds
	}
	limits := config.DefaultLimits()
	if c.file == "" {
		if c.trace != "" || c.opsLog != "" {
			return c.fail(diagnostic.New("E_CLI_USAGE", "-trace and -ops-log require a source file"), 2)
		}
		if err := repl.Run(stdin, stdout, stderr, limits); err != nil {
			return c.fail(err, 2)
		}
		return 0
	}
	return c.runFile(limits)
}

// jsonErrorIntent recovers explicit mode flags beyond a parse error. It only
// selects the error output format; successful invocations use flag.Parse.
func jsonErrorIntent(flags *flag.FlagSet, args []string) bool {
	intent := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		f := flags.Lookup(name)
		if f == nil {
			continue
		}
		boolFlag, isBool := f.Value.(interface{ IsBoolFlag() bool })
		if !isBool || !boolFlag.IsBoolFlag() {
			if !hasValue {
				i++ // A string value may itself be spelled -json or --.
			}
			continue
		}
		if name == "json" {
			if !hasValue {
				intent = true
			} else if parsed, err := strconv.ParseBool(value); err == nil {
				intent = parsed
			}
		}
	}
	return intent
}

func (c command) fail(err error, code int) int {
	errorCode := diagnostic.CodeOf(err)
	if _, writeErr := fmt.Fprintln(c.stderr, err); writeErr != nil {
		code = 2
		errorCode = "E_OUTPUT_IO"
	}
	if c.json {
		if writeErr := contract.EncodeError(c.stdout, c.rule, err); writeErr != nil {
			fmt.Fprintln(c.stderr, writeErr)
			code = 2
			errorCode = "E_OUTPUT_IO"
		}
	}
	if c.operationError != nil {
		*c.operationError = errorCode
	}
	return code
}
