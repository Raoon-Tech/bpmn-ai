package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

func cmdTask(args []string) int {
	if len(args) == 0 {
		printTaskUsage()
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdTaskList(args[1:])
	case "claim":
		return cmdTaskClaim(args[1:])
	case "complete":
		return cmdTaskComplete(args[1:])
	case "help", "-h", "--help":
		printTaskUsage()
		return exitOK
	default:
		return usageError("task", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printTaskUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai task <subcommand>

SUBCOMMANDS
  list <caseID>               List active user tasks of a case
  claim <flowID> [--user ID]  Claim a task
  complete <flowID> [--var k=v]...   Complete a task and resume the case

EXAMPLES
  bpmn-ai task list 049a3bed-...
  bpmn-ai task complete 1234... --var approved=true
`)
}

func cmdTaskList(args []string) int {
	fs := flag.NewFlagSet("task list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("task list", "expected a case id")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	flows, err := backend.Store.GetFlowsByInstance(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("list tasks: %v", err)
	}
	tasks := make([]map[string]any, 0)
	rows := make([][]string, 0)
	for _, f := range flows {
		if f.ElementType == bpmn.ElementTypeUserTask && f.Status == store.FlowStatusActive {
			tasks = append(tasks, map[string]any{
				"flow_id":     f.ID,
				"element_id":  f.ElementID,
				"thread_id":   f.ThreadID,
				"started_at":  f.StartedAt,
			})
			started := "-"
			if f.StartedAt != nil {
				started = f.StartedAt.Format(time.RFC3339)
			}
			rows = append(rows, []string{f.ID, f.ElementID, fmt.Sprintf("%d", f.ThreadID), started})
		}
	}
	if *asJSON {
		return printJSONQuiet(tasks)
	}
	printTable([]string{"FLOW ID", "ELEMENT", "THREAD", "STARTED AT"}, rows)
	return exitOK
}

func cmdTaskClaim(args []string) int {
	fs := flag.NewFlagSet("task claim", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	user := fs.String("user", "", "user claiming the task (required)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("task claim", "expected a flow id")
	}
	if *user == "" {
		return usageError("task claim", "--user is required")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	flow, err := backend.Store.GetFlow(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("task not found: %v", err)
	}
	if flow.Status != store.FlowStatusActive {
		return fail("task is not active")
	}
	flow.Status = store.FlowStatusActive
	if err := backend.Store.UpdateFlow(context.Background(), flow); err != nil {
		return fail("claim task: %v", err)
	}
	fmt.Printf("task %s claimed by %s\n", fs.Arg(0), *user)
	return exitOK
}

func cmdTaskComplete(args []string) int {
	fs := flag.NewFlagSet("task complete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var vars stringSliceFlag
	fs.Var(&vars, "var", "variable in key=value form (repeatable)")
	timeout := fs.Duration("timeout", 30*time.Second, "execution timeout")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("task complete", "expected a flow id")
	}
	flowID := fs.Arg(0)

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

	flow, err := rt.Store.GetFlow(ctx, flowID)
	if err != nil {
		return fail("task not found: %v", err)
	}
	if flow.Status != store.FlowStatusActive {
		return fail("task is not active")
	}

	now := time.Now()
	flow.Status = store.FlowStatusCompleted
	flow.FinishedAt = &now
	d := int(time.Since(*flow.StartedAt).Milliseconds())
	flow.DurationMs = &d
	if err := rt.Store.UpdateFlow(ctx, flow); err != nil {
		return fail("complete task: %v", err)
	}

	rt.Start(ctx)
	defer rt.Stop()

	if err := rt.Engine.Continue(ctx, flow.InstanceID, flow.ID, variables); err != nil {
		logger.Error("engine continuation failed", "error", err, "instance_id", flow.InstanceID)
	}

	inst, _ := rt.Store.GetInstance(ctx, flow.InstanceID)
	if inst != nil {
		fmt.Printf("case_id: %s\n", inst.ID)
		fmt.Printf("status:  %s\n", inst.Status)
		fmt.Printf("variables: %s\n", formatVars(inst.Variables))
	}
	return exitOK
}
