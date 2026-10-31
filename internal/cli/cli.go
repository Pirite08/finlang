// Package cli implements the fin v0.1 command line.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/finlang/internal/config"
	"example.com/finlang/internal/diagnostic"
	"example.com/finlang/internal/eval"
	"example.com/finlang/internal/parser"
	"example.com/finlang/internal/repl"
)

const help = `Usage: fin [-file path | -version | -h]
  no arguments  start the single-line REPL
  -file path    execute a UTF-8 source file (multiline functions supported)
  -version      print the version
  -h            show help
`

// Run returns 0 on success, 1 for source/evaluation errors and 2 for usage/I/O errors.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "source file")
	version := flags.Bool("version", false, "print version")
	helpRequested := flags.Bool("h", false, "show help")
	flags.Usage = func() { fmt.Fprint(stderr, help) }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || flags.NFlag() > 1 {
		fmt.Fprintln(stderr, "fin: choose one of -file, -version, -h; positional arguments are unsupported")
		return 2
	}
	if *helpRequested {
		if _, err := fmt.Fprint(stdout, help); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	if *version {
		if _, err := fmt.Fprintln(stdout, "fin v0.1.0"); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	fileSpecified := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "file" {
			fileSpecified = true
		}
	})
	limits := config.DefaultLimits()
	if !fileSpecified {
		if err := repl.Run(stdin, stdout, stderr, limits); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	if *file == "" {
		fmt.Fprintln(stderr, "fin: -file requires a nonempty path")
		return 2
	}
	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer f.Close()
	source, err := io.ReadAll(io.LimitReader(f, int64(limits.MaxSourceBytes)+1))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(source) > limits.MaxSourceBytes {
		err := diagnostic.WithSpan(diagnostic.New("E_SOURCE_LIMIT", "source exceeds maximum byte length"), diagnostic.Span{File: *file, Line: 1, Column: 1})
		fmt.Fprintln(stderr, err)
		return 1
	}
	program, err := parser.Parse(*file, string(source), limits)
	if err == nil {
		_, err = eval.New(eval.Options{Limits: limits, Stdout: stdout}).Run(program, eval.NewEnvironment(nil))
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		if diagnostic.CodeOf(err) == "E_OUTPUT_IO" {
			return 2
		}
		return 1
	}
	return 0
}
