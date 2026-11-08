package main

import (
	"context"
	"encoding/json"
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
	"github.com/Pirite08/finlang/internal/transferhttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("fintransfer", flag.ContinueOnError)
	flags.SetOutput(stderr)
	database := flags.String("db", "", "path to the SQLite test ledger (required)")
	port := flags.Int("port", 8081, "loopback HTTP port")
	seed := flags.Bool("seed-demo", false, "add missing demo accounts without resetting balances")
	replay := flags.String("replay", "", "replay a committed transfer by request ID, offline")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *database == "" || flags.NArg() != 0 {
		return errors.New("fintransfer requires -db and no positional arguments")
	}
	var replayMode, portSet, seedSet bool
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "replay":
			replayMode = true
		case "port":
			portSet = true
		case "seed-demo":
			seedSet = true
		}
	})
	if replayMode {
		if *replay == "" || seedSet || portSet {
			return errors.New("-replay requires a request ID and only accepts -db alongside it")
		}
		store, err := transfer.OpenReadOnly(ctx, *database)
		if err != nil {
			return fmt.Errorf("open audit database: %w", err)
		}
		defer store.Close()
		result, err := store.Replay(ctx, *replay)
		if err != nil {
			return fmt.Errorf("replay transfer: %w", err)
		}
		return json.NewEncoder(stdout).Encode(result)
	}
	if *port < 0 || *port > 65535 {
		return errors.New("-port must be between 0 and 65535")
	}
	store, err := transfer.Open(ctx, *database)
	if err != nil {
		return fmt.Errorf("open transfer database: %w", err)
	}
	defer store.Close()
	if *seed {
		if err := store.SeedDemo(ctx); err != nil {
			return fmt.Errorf("seed demo accounts: %w", err)
		}
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	if _, err := fmt.Fprintf(stderr, "fintransfer listening on http://%s/transfers\n", listener.Addr()); err != nil {
		return err
	}

	server := &http.Server{
		Handler:           localHostOnly(transferhttp.New(store)),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdown)
		if shutdownErr != nil {
			_ = server.Close()
			<-done
			return shutdownErr
		}
		err = <-done
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func localHostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		// A browser must use a local host name even if external DNS resolves to loopback.
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "local host required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
