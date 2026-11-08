package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Pirite08/finlang/internal/transfer"
)

func serveCommand(args []string, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runServe(ctx, args, stderr)
}

func runServe(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("fin serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	app := flags.String("app", "", "path to the JSON route manifest (required)")
	port := flags.Int("port", 8082, "loopback HTTP port")
	database := flags.String("db", "", "SQLite ledger path for transfer routes")
	seed := flags.Bool("seed-demo", false, "add missing demo accounts")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *app == "" || flags.NArg() != 0 || *port < 0 || *port > 65535 {
		fmt.Fprintln(stderr, "fin serve requires -app, a port between 0 and 65535, and no positional arguments")
		return 2
	}

	loaded, err := loadServeApp(*app)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitCode(err)
	}
	if len(loaded.planArtifacts) != 0 {
		if *database == "" {
			fmt.Fprintln(stderr, "fin serve requires -db for transfer routes")
			return 2
		}
		loaded.ledger, err = transfer.Open(ctx, *database)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		defer loaded.ledger.Close()
		if err := loaded.ledger.RegisterPlanArtifacts(ctx, loaded.planArtifacts); err != nil {
			fmt.Fprintln(stderr, "register transfer artifacts:", err)
			return 1
		}
		loaded.planArtifacts = nil
		if *seed {
			if err := loaded.ledger.SeedDemo(ctx); err != nil {
				fmt.Fprintln(stderr, err)
				return 2
			}
		}
	} else if *database != "" || *seed {
		fmt.Fprintln(stderr, "-db and -seed-demo require a transfer route")
		return 2
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer listener.Close()
	if _, err := fmt.Fprintf(stderr, "fin serve listening on http://%s\n", listener.Addr()); err != nil {
		return 2
	}

	server := &http.Server{
		Handler:           newRuleHandler(loaded),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			<-done
			fmt.Fprintln(stderr, err)
			return 2
		}
		err = <-done
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}
