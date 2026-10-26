package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

func cmdJob(args []string) int {
	if len(args) == 0 {
		printJobUsage()
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdJobList(args[1:])
	case "retry":
		return cmdJobRetry(args[1:])
	case "drain":
		return cmdJobDrain(args[1:])
	case "help", "-h", "--help":
		printJobUsage()
		return exitOK
	default:
		return usageError("job", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printJobUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai job <subcommand>

SUBCOMMANDS
  list [--status STATUS] [--limit N]
  retry <id>
  drain [--limit N]

EXAMPLES
  bpmn-ai job list --status FAILED
  bpmn-ai job retry abc123
  bpmn-ai job drain --limit 50
`)
}

func cmdJobList(args []string) int {
	fs := flag.NewFlagSet("job list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	status := fs.String("status", "", "filter by status")
	limit := fs.Int("limit", 50, "max jobs")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("job list", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	var jobs []*store.JobRecord
	var err error
	if *status == "" || *status == "PENDING" {
		jobs, err = backend.Store.GetPendingJobs(context.Background(), *limit)
	} else {
		// Fall back to listing all jobs is not supported; use GetJob per id is not enumerable.
		// For non-pending statuses we report that only PENDING is queryable.
		return fail("job list only supports PENDING (or empty) status filter; got %q", *status)
	}
	if err != nil {
		return fail("list jobs: %v", err)
	}
	if *asJSON {
		return printJSONQuiet(jobs)
	}
	rows := make([][]string, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, []string{j.ID, j.InstanceID, j.Type, string(j.Status), j.ScheduledAt.Format(time.RFC3339), fmt.Sprintf("%d", j.RetryCount)})
	}
	printTable([]string{"ID", "INSTANCE", "TYPE", "STATUS", "SCHEDULED", "RETRIES"}, rows)
	return exitOK
}

func cmdJobRetry(args []string) int {
	fs := flag.NewFlagSet("job retry", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("job retry", "expected a job id")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	job, err := backend.Store.GetJob(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("job not found: %v", err)
	}
	job.Status = store.JobStatusPending
	job.RetryCount = 0
	job.ErrorMessage = ""
	job.ScheduledAt = time.Now()
	job.ExecutedAt = nil
	if err := backend.Store.UpdateJob(context.Background(), job); err != nil {
		return fail("retry job: %v", err)
	}
	fmt.Printf("job %s requeued\n", fs.Arg(0))
	return exitOK
}

func cmdJobDrain(args []string) int {
	fs := flag.NewFlagSet("job drain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", 100, "max jobs to process")
	timeout := fs.Duration("timeout", 5*time.Minute, "overall timeout")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("job drain", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}
	logger, err := observabilityNew(cfg)
	if err != nil {
		return fail("logger: %v", err)
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

	processed, err := rt.Queue.Drain(ctx, *limit)
	if err != nil {
		return fail("drain: %v", err)
	}
	fmt.Printf("processed %d job(s)\n", processed)
	return exitOK
}
