package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

func cmdCase(args []string) int {
	if len(args) == 0 {
		printCaseUsage()
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdCaseList(args[1:])
	case "get":
		return cmdCaseGet(args[1:])
	case "start":
		return cmdCaseStart(args[1:])
	case "history":
		return cmdCaseHistory(args[1:])
	case "cancel":
		return cmdCaseCancel(args[1:])
	case "help", "-h", "--help":
		printCaseUsage()
		return exitOK
	default:
		return usageError("case", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printCaseUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai case <subcommand>

SUBCOMMANDS
  list [--status STATUS] [--limit N]
  get <id>
  start <processID> [--title T] [--var k=v]... [--timeout 30s]
  history <id>
  cancel <id>

EXAMPLES
  bpmn-ai case start 96870a21-... --var amount=10
  bpmn-ai case list --status WAITING
`)
}

func cmdCaseList(args []string) int {
	fs := flag.NewFlagSet("case list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	status := fs.String("status", "", "filter by status")
	limit := fs.Int("limit", 0, "max cases (0 = all)")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("case list", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	cases, err := backend.Store.ListInstances(context.Background(), store.InstanceStatus(*status))
	if err != nil {
		return fail("list cases: %v", err)
	}
	if *limit > 0 && len(cases) > *limit {
		cases = cases[:*limit]
	}
	if *asJSON {
		return printJSONQuiet(cases)
	}
	rows := make([][]string, 0, len(cases))
	for _, c := range cases {
		rows = append(rows, []string{c.ID, c.ProcessID, c.Title, string(c.Status), c.StartedAt.Format(time.RFC3339)})
	}
	printTable([]string{"ID", "PROCESS", "TITLE", "STATUS", "STARTED AT"}, rows)
	return exitOK
}

func cmdCaseGet(args []string) int {
	fs := flag.NewFlagSet("case get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("case get", "expected a case id")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	inst, err := backend.Store.GetInstance(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("get case: %v", err)
	}
	if *asJSON {
		return printJSONQuiet(inst)
	}
	fmt.Printf("id:         %s\n", inst.ID)
	fmt.Printf("process_id: %s\n", inst.ProcessID)
	fmt.Printf("title:      %s\n", inst.Title)
	fmt.Printf("status:     %s\n", inst.Status)
	fmt.Printf("current:    %s\n", inst.CurrentUser)
	fmt.Printf("started:    %s\n", inst.StartedAt.Format(time.RFC3339))
	if inst.FinishedAt != nil {
		fmt.Printf("finished:   %s\n", inst.FinishedAt.Format(time.RFC3339))
	}
	fmt.Printf("variables:  %s\n", formatVars(inst.Variables))
	return exitOK
}

func cmdCaseStart(args []string) int {
	fs := flag.NewFlagSet("case start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	title := fs.String("title", "", "case title")
	timeout := fs.Duration("timeout", 30*time.Second, "execution timeout")
	var vars stringSliceFlag
	fs.Var(&vars, "var", "variable in key=value form (repeatable)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("case start", "expected a process id")
	}
	processID := fs.Arg(0)

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

	proc, err := rt.Store.GetProcess(ctx, processID)
	if err != nil {
		return fail("process not found: %v", err)
	}

	instance := process.NewInstance(proc, variables)
	if *title != "" {
		instance.Title = *title
	}
	if err := rt.Store.CreateInstance(ctx, instance.ToRecord()); err != nil {
		return fail("create case: %v", err)
	}

	rt.Start(ctx)
	defer rt.Stop()

	if err := rt.Engine.Run(ctx, instance); err != nil {
		logger.Error("engine execution failed", "error", err, "instance_id", instance.ID)
	}

	inst, _ := rt.Store.GetInstance(ctx, instance.ID)
	status := string(instance.State)
	if inst != nil {
		status = string(inst.Status)
	}
	fmt.Printf("case_id: %s\n", instance.ID)
	fmt.Printf("status:  %s\n", status)
	if inst != nil {
		fmt.Printf("variables: %s\n", formatVars(inst.Variables))
	}

	tasks, _ := rt.Store.GetFlowsByInstance(ctx, instance.ID)
	pending := 0
	for _, f := range tasks {
		if f.ElementType == bpmn.ElementTypeUserTask && f.Status == store.FlowStatusActive {
			pending++
		}
	}
	if pending > 0 {
		fmt.Printf("pending tasks: %d (use 'bpmn-ai task list %s')\n", pending, instance.ID)
	}
	return exitOK
}

func cmdCaseHistory(args []string) int {
	fs := flag.NewFlagSet("case history", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("case history", "expected a case id")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	logs, err := backend.Store.GetExecutionLog(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("get history: %v", err)
	}
	if *asJSON {
		return printJSONQuiet(logs)
	}
	rows := make([][]string, 0, len(logs))
	for _, e := range logs {
		rows = append(rows, []string{e.ID, e.ElementID, string(e.ElementType), e.Action, fmt.Sprintf("%d", e.DurationMs), e.CreatedAt.Format(time.RFC3339)})
	}
	printTable([]string{"ID", "ELEMENT", "TYPE", "ACTION", "DURATION MS", "AT"}, rows)
	return exitOK
}

func cmdCaseCancel(args []string) int {
	fs := flag.NewFlagSet("case cancel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("case cancel", "expected a case id")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	inst, err := backend.Store.GetInstance(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("get case: %v", err)
	}
	inst.Status = store.InstanceStatusTerminated
	if err := backend.Store.UpdateInstance(context.Background(), inst); err != nil {
		return fail("cancel case: %v", err)
	}
	fmt.Printf("case %s terminated\n", fs.Arg(0))
	return exitOK
}

func formatVars(v map[string]any) string {
	if len(v) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// observabilityNew is a small wrapper to keep imports tidy.
func observabilityNew(cfg config.Config) (*observability.Logger, error) {
	return observability.NewFromConfig(cfg.Log.Level, cfg.Log.Format)
}
