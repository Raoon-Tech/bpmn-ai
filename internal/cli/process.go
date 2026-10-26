package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
)

func cmdProcess(args []string) int {
	if len(args) == 0 {
		printProcessUsage()
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdProcessList(args[1:])
	case "get":
		return cmdProcessGet(args[1:])
	case "deploy":
		return cmdProcessDeploy(args[1:])
	case "delete":
		return cmdProcessDelete(args[1:])
	case "help", "-h", "--help":
		printProcessUsage()
		return exitOK
	default:
		return usageError("process", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printProcessUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai process <subcommand>

SUBCOMMANDS
  list                   List deployed processes
  get <id>               Show a process definition
  deploy <file.bpmn>     Deploy a BPMN file
  delete <id>            Delete a process (--force)

EXAMPLES
  bpmn-ai process list
  bpmn-ai process deploy examples/approval-process.bpmn
  bpmn-ai process delete 96870a21-... --force
`)
}

func cmdProcessList(args []string) int {
	fs := flag.NewFlagSet("process list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("process list", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	procs, err := backend.Store.ListProcesses(context.Background())
	if err != nil {
		return fail("list processes: %v", err)
	}
	if *asJSON {
		return printJSONQuiet(procs)
	}
	rows := make([][]string, 0, len(procs))
	for _, p := range procs {
		rows = append(rows, []string{p.ID, p.Name, fmt.Sprintf("%d", p.Version), p.CreatedAt.Format(time.RFC3339)})
	}
	fmt.Printf("database: %s\n\n", bootstrap.Describe(cfg.Database))
	printTable([]string{"ID", "NAME", "VERSION", "CREATED AT"}, rows)
	return exitOK
}

func cmdProcessGet(args []string) int {
	fs := flag.NewFlagSet("process get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("process get", "expected a process id")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	proc, err := backend.Store.GetProcess(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("get process: %v", err)
	}
	if *asJSON {
		return printJSONQuiet(proc)
	}
	fmt.Printf("id:      %s\n", proc.ID)
	fmt.Printf("name:    %s\n", proc.Name)
	fmt.Printf("version: %d\n", proc.Version)
	fmt.Printf("created: %s\n", proc.CreatedAt.Format(time.RFC3339))
	fmt.Printf("elements: %d\n", len(proc.Elements))
	fmt.Printf("flows:    %d\n", len(proc.Flows))
	return exitOK
}

func cmdProcessDeploy(args []string) int {
	fs := flag.NewFlagSet("process deploy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("process deploy", "expected the path to a .bpmn file")
	}

	proc, err := bpmn.NewParser().ParseFile(fs.Arg(0))
	if err != nil {
		return fail("parse %s: %v", fs.Arg(0), err)
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if err := backend.Store.SaveProcess(context.Background(), proc); err != nil {
		return fail("save process: %v", err)
	}
	fmt.Printf("process %q (%s) deployed\n", proc.Name, proc.ID)
	return exitOK
}

func cmdProcessDelete(args []string) int {
	fs := flag.NewFlagSet("process delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "confirm deletion")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("process delete", "expected a process id")
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}
	if !*force && !cfg.Database.AllowDestructive {
		return fail("deletion is destructive: pass --force or set DB_ALLOW_DESTRUCTIVE=true")
	}

	_, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if err := backend.Store.DeleteProcess(context.Background(), fs.Arg(0)); err != nil {
		return fail("delete process: %v", err)
	}
	fmt.Printf("process %s deleted\n", fs.Arg(0))
	return exitOK
}

func printJSONQuiet(v any) int {
	if err := printJSON(v); err != nil {
		return fail("render json: %v", err)
	}
	return exitOK
}

