// Command telnet-smoke is a standalone telnet driver for a running AnotherMUD
// engine — both an interactive client for manual smoke testing and a runner for
// named, scripted scenarios. It is the manual-use twin of the Go integration
// test in smoke_test.go; both share the scenario helpers in scenario.go and the
// generic send/expect core in internal/telnettest.
//
//	# interactive (type at the engine yourself; Ctrl-C to quit)
//	go run ./cmd/telnet-smoke -addr 127.0.0.1:4000
//
//	# run a scenario, exit non-zero on failure
//	go run ./cmd/telnet-smoke -addr 127.0.0.1:4000 -scenario login-look -name Smoketest
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4000", "engine telnet address (host:port)")
	scenario := flag.String("scenario", "", "named scenario to run; empty = interactive passthrough")
	name := flag.String("name", "Smoketest", "character name used by scenarios")
	timeout := flag.Duration("timeout", 8*time.Second, "default expect timeout")
	transcript := flag.Bool("transcript", false, "tee server output to stderr (scenario mode)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	opts := []telnettest.Option{telnettest.WithTimeout(*timeout)}
	if *transcript {
		opts = append(opts, telnettest.WithTranscript(os.Stderr))
	}

	c, err := telnettest.Dial(*addr, opts...)
	if err != nil {
		log.Error("dial failed", "addr", *addr, "err", err)
		os.Exit(1)
	}
	defer c.Close()

	if *scenario == "" {
		runInteractive(c, log)
		return
	}

	fn, ok := scenarios[*scenario]
	if !ok {
		log.Error("unknown scenario", "scenario", *scenario, "available", scenarioNames())
		os.Exit(2)
	}
	if err := fn(c, *name); err != nil {
		log.Error("scenario FAILED", "scenario", *scenario, "name", *name, "err", err)
		os.Exit(1)
	}
	log.Info("scenario PASSED", "scenario", *scenario, "name", *name)
}

func runInteractive(c *telnettest.Client, log *slog.Logger) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := c.Interact(ctx, os.Stdin, os.Stdout); err != nil &&
		!errors.Is(err, context.Canceled) {
		log.Error("interactive session ended", "err", err)
		os.Exit(1)
	}
}
