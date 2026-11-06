package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/fxfeed"
	"github.com/Pirite08/finlang/internal/fxstream"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("fxfeed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listen := flags.String("listen", "127.0.0.1:8080", "loopback IP address and port for /latest")
	maxAge := flags.Duration("max-age", 5*time.Minute, "maximum age of a quote served to finlang")
	minRate := flags.String("min-rate", fx.DefaultMinRate, "minimum rate in KRW per USD")
	maxRate := flags.String("max-rate", fx.DefaultMaxRate, "maximum rate in KRW per USD")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: fxfeed [-listen IP:port] [-max-age duration] [-min-rate decimal] [-max-rate decimal]")
		fmt.Fprintln(stderr, "Set FINLANG_TWELVE_DATA_API_KEY with access to USD/KRW WebSocket quotes.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *maxAge <= 0 {
		return errors.New("fxfeed requires a positive -max-age and no positional arguments")
	}
	bounds, err := fx.ParseRateBounds(*minRate, *maxRate)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return errors.New("-listen must contain a loopback IP address and port")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return errors.New("-listen must use a loopback IP address, such as 127.0.0.1")
	}
	stream, err := fxstream.New(os.Getenv("FINLANG_TWELVE_DATA_API_KEY"))
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	receiver := fxfeed.New(fx.LivePolicy{MaxAge: *maxAge, Rates: bounds})
	server := &http.Server{
		Handler:           receiver,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	logger := log.New(stderr, "fxfeed: ", log.LstdFlags)
	logger.Printf("USD/KRW endpoint http://%s/latest (waiting for first quote)", listener.Addr())
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		stream.Run(streamCtx, func(q fx.Quote) { receiver.Publish(q) }, func(err error) { logger.Print(err) })
	}()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err = <-serveDone:
	}
	cancel()
	shutdown, finish := context.WithTimeout(context.Background(), 3*time.Second)
	defer finish()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			err = shutdownErr
		}
	}
	<-streamDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
