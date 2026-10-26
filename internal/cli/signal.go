package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
)

func cmdSignal(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printSignalUsage()
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	if args[0] != "send" {
		return usageError("signal", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
	return cmdSignalSend(args[1:])
}

func printSignalUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai signal send <signalRef> [--var k=v]...

EXAMPLES
  bpmn-ai signal send customer-updated --var tier=premium
`)
}

func cmdSignalSend(args []string) int {
	fs := flag.NewFlagSet("signal send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var vars stringSliceFlag
	fs.Var(&vars, "var", "variable in key=value form (repeatable)")
	timeout := fs.Duration("timeout", 30*time.Second, "execution timeout")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("signal send", "expected a signal ref")
	}
	signalRef := fs.Arg(0)

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}
	logger, err := observabilityNew(cfg)
	if err != nil {
		return fail("logger: %v", err)
	}

	variables, err := parseVars(vars)
	if err != nil {
		return fail("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	rt, err := bootstrap.NewRuntime(ctx, cfg, logger)
	if err != nil {
		return fail("open runtime: %v", err)
	}
	defer func() { _ = rt.Close() }()

	rt.Start(ctx)
	defer rt.Stop()

	affected, err := rt.Engine.SendSignal(ctx, signalRef, variables)
	if err != nil {
		return fail("send signal: %v", err)
	}
	fmt.Printf("signal %q delivered to %d instance(s)\n", signalRef, len(affected))
	for _, id := range affected {
		fmt.Printf("  %s\n", id)
	}
	return exitOK
}
