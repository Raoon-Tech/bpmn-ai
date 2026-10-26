package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
)

func cmdMessage(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printMessageUsage()
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	if args[0] != "send" {
		return usageError("message", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
	return cmdMessageSend(args[1:])
}

func printMessageUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai message send <instanceID> <messageRef> [--var k=v]...

EXAMPLES
  bpmn-ai message send 049a3bed... order-confirmed --var order_id=123
`)
}

func cmdMessageSend(args []string) int {
	fs := flag.NewFlagSet("message send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var vars stringSliceFlag
	fs.Var(&vars, "var", "variable in key=value form (repeatable)")
	timeout := fs.Duration("timeout", 30*time.Second, "execution timeout")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 2 {
		return usageError("message send", "expected <instanceID> <messageRef>")
	}
	instanceID := fs.Arg(0)
	messageRef := fs.Arg(1)

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

	if err := rt.Engine.SendMessage(ctx, instanceID, messageRef, variables); err != nil {
		return fail("send message: %v", err)
	}
	fmt.Printf("message %q delivered to instance %s\n", messageRef, instanceID)
	return exitOK
}
