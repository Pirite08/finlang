// Package cli implements the fin command line.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/repl"
)

const Version = "v0.2.0"
const help = "Usage: fin [-file path [-trace path]] | [-version] | [-h]\n" +
	"  no arguments  start the single-line REPL\n" +
	"  -file path    execute a UTF-8 source file\n" +
	"  -json         run the contract entry and emit one JSON result\n" +
	"  -contract path  JSON contract (required with -json)\n" +
	"  -input path     JSON inputs (required with -json)\n" +
	"  -trace path     write bounded JSONL calculation records\n" +
	"  -version      print the version\n" +
	"  -h            show help\n"

type command struct {
	file, contract, input, trace, rule string
	json                               bool
	stdout, stderr                     io.Writer
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
	for name, path := range map[string]string{"file": c.file, "contract": c.contract, "input": c.input, "trace": c.trace} {
		if provided[name] && path == "" {
			return c.fail(diagnostic.New("E_CLI_USAGE", "-"+name+" requires a nonempty path"), 2)
		}
	}
	if c.json {
		if c.file == "" || c.contract == "" || c.input == "" {
			return c.fail(diagnostic.New("E_CLI_USAGE", "-json requires -file, -contract and -input"), 2)
		}
	} else if provided["contract"] || provided["input"] {
		return c.fail(diagnostic.New("E_CLI_USAGE", "-contract and -input require -json"), 2)
	}
	limits := config.DefaultLimits()
	if c.file == "" {
		if c.trace != "" {
			return c.fail(diagnostic.New("E_CLI_USAGE", "-trace requires a source file"), 2)
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
	if _, writeErr := fmt.Fprintln(c.stderr, err); writeErr != nil {
		code = 2
	}
	if c.json {
		if writeErr := contract.EncodeError(c.stdout, c.rule, err); writeErr != nil {
			fmt.Fprintln(c.stderr, writeErr)
			return 2
		}
	}
	return code
}
