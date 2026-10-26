package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

func cmdMigrate(args []string) int {
	if len(args) == 0 {
		printMigrateUsage()
		return exitUsage
	}
	switch strings.ToLower(args[0]) {
	case "status":
		return cmdMigrateStatus(args[1:])
	case "up":
		return cmdMigrateUp(args[1:])
	case "down":
		return cmdMigrateDown(args[1:])
	case "create":
		return cmdMigrateCreate(args[1:])
	case "auto":
		return cmdMigrateAuto(args[1:])
	case "baseline":
		return cmdMigrateBaseline(args[1:])
	case "reset":
		return cmdMigrateReset(args[1:])
	case "help", "-h", "--help":
		printMigrateUsage()
		return exitOK
	default:
		return usageError("migrate", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printMigrateUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai migrate <subcommand>

SUBCOMMANDS
  status                 List registered migrations and their state
  up [--to VERSION]      Apply pending migrations
      [--dry-run]
  down --steps N         Roll back the last N migrations
      [--force] [--dry-run]
  create <name>          Scaffold a new migration file
      [--dir DIR]
  auto                   Synchronize the schema with AutoMigrate (development only)
  baseline [VERSION]     Mark migrations as applied without running them
      [--all]
  reset --force          Roll back every migration (development only)

EXAMPLES
  bpmn-ai migrate create add_priority_to_jobs
  bpmn-ai migrate up --dry-run
  bpmn-ai migrate up
  bpmn-ai migrate down --steps 1 --force
`)
}

// --- status ---

func cmdMigrateStatus(args []string) int {
	fs := flag.NewFlagSet("migrate status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("migrate status", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, backend, code := openSQLBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	statuses, err := migrate.ListStatus(context.Background(), backend.DB)
	if err != nil {
		return fail("read migration status: %v", err)
	}

	fmt.Printf("database: %s (%s)\n\n", bootstrap.Describe(cfg.Database), backend.Driver)
	rows := make([][]string, 0, len(statuses))
	for _, st := range statuses {
		status := "pending"
		appliedAt := "-"
		duration := "-"
		if st.Applied {
			status = "applied"
			if st.AppliedAt != nil {
				appliedAt = st.AppliedAt.Format(time.RFC3339)
			}
			duration = fmt.Sprintf("%dms", st.DurationMs)
		}
		rows = append(rows, []string{st.Version, status, appliedAt, duration})
	}
	printTable([]string{"VERSION", "STATUS", "APPLIED AT", "DURATION"}, rows)
	return exitOK
}

// --- up ---

func cmdMigrateUp(args []string) int {
	fs := flag.NewFlagSet("migrate up", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	to := fs.String("to", "", "stop after this migration version (inclusive)")
	dryRun := fs.Bool("dry-run", false, "print the migrations that would run without executing them")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("migrate up", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, backend, code := openSQLBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	opts := migrate.Options{To: *to, DryRun: *dryRun}
	applied, err := migrate.Up(context.Background(), backend.DB, opts)
	if err != nil {
		return fail("apply migrations: %v", err)
	}

	if len(applied) == 0 {
		fmt.Printf("database %s is up to date\n", bootstrap.Describe(cfg.Database))
		return exitOK
	}
	for _, version := range applied {
		if *dryRun {
			fmt.Printf("[dry-run] would apply %s\n", version)
		} else {
			fmt.Printf("applied %s\n", version)
		}
	}
	return exitOK
}

// --- down ---

func cmdMigrateDown(args []string) int {
	fs := flag.NewFlagSet("migrate down", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	steps := fs.Int("steps", 1, "number of migrations to roll back")
	force := fs.Bool("force", false, "confirm the destructive rollback")
	dryRun := fs.Bool("dry-run", false, "print the migrations that would roll back without executing them")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("migrate down", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}
	if !*force && !cfg.Database.AllowDestructive && !*dryRun {
		return fail("rollback is destructive: pass --force or set DB_ALLOW_DESTRUCTIVE=true")
	}

	backend, err := bootstrap.Open(context.Background(), cfg.Database)
	if err != nil {
		return fail("open database: %v", err)
	}
	defer func() { _ = backend.Close() }()
	if backend.DB == nil {
		return fail("migrations require a SQL database (current driver: %s)", backend.Driver)
	}

	rolledBack, err := migrate.Down(context.Background(), backend.DB, *steps, migrate.Options{DryRun: *dryRun})
	if err != nil {
		return fail("rollback migrations: %v", err)
	}
	if len(rolledBack) == 0 {
		fmt.Println("nothing to roll back")
		return exitOK
	}
	for _, version := range rolledBack {
		if *dryRun {
			fmt.Printf("[dry-run] would roll back %s\n", version)
		} else {
			fmt.Printf("rolled back %s\n", version)
		}
	}
	return exitOK
}

// --- reset ---

func cmdMigrateReset(args []string) int {
	fs := flag.NewFlagSet("migrate reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "confirm dropping every table")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("migrate reset", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}
	if !*force && !cfg.Database.AllowDestructive {
		return fail("reset drops every table: pass --force or set DB_ALLOW_DESTRUCTIVE=true")
	}

	backend, err := bootstrap.Open(context.Background(), cfg.Database)
	if err != nil {
		return fail("open database: %v", err)
	}
	defer func() { _ = backend.Close() }()
	if backend.DB == nil {
		return fail("migrations require a SQL database (current driver: %s)", backend.Driver)
	}

	rolledBack, err := migrate.Reset(context.Background(), backend.DB, migrate.Options{})
	if err != nil {
		return fail("reset database: %v", err)
	}
	if len(rolledBack) == 0 {
		fmt.Println("nothing to reset")
		return exitOK
	}
	for _, version := range rolledBack {
		fmt.Printf("rolled back %s\n", version)
	}
	return exitOK
}

// --- create ---

func cmdMigrateCreate(args []string) int {
	fs := flag.NewFlagSet("migrate create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "pkg/store/migrations", "directory where the migration file is written")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("migrate create", "expected exactly one migration name")
	}

	path, err := migrate.Scaffold(*dir, fs.Arg(0))
	if err != nil {
		return fail("create migration: %v", err)
	}
	fmt.Printf("created %s\n", path)
	fmt.Println("edit the Up/Down functions, then run \"bpmn-ai migrate up\" (rebuild the binary first)")
	return exitOK
}

// --- auto ---

func cmdMigrateAuto(args []string) int {
	fs := flag.NewFlagSet("migrate auto", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("migrate auto", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, backend, code := openSQLBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if err := orm.AutoMigrate(backend.DB); err != nil {
		return fail("auto migrate: %v", err)
	}
	fmt.Printf("schema synchronized with AutoMigrate on %s\n", bootstrap.Describe(cfg.Database))
	return exitOK
}

// --- baseline ---

func cmdMigrateBaseline(args []string) int {
	fs := flag.NewFlagSet("migrate baseline", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	all := fs.Bool("all", false, "mark every registered migration as applied")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 1 {
		return usageError("migrate baseline", "expected at most one migration version")
	}
	version := ""
	if fs.NArg() == 1 {
		version = fs.Arg(0)
	}
	if !*all && version == "" {
		return usageError("migrate baseline", "provide a version or use --all")
	}

	_, backend, code := openSQLBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	marked, err := migrate.Baseline(context.Background(), backend.DB, version, *all)
	if err != nil {
		return fail("baseline migrations: %v", err)
	}
	if len(marked) == 0 {
		fmt.Println("nothing to baseline")
		return exitOK
	}
	for _, v := range marked {
		fmt.Printf("baselined %s\n", v)
	}
	return exitOK
}

// openSQLBackend loads the configuration and opens a SQL backend. Commands that
// require SQL print an error and return a non-nil exit code on failure.
func openSQLBackend() (config.Config, *bootstrap.Backend, int) {
	cfg, err := loadConfig()
	if err != nil {
		return cfg, nil, fail("invalid configuration: %v", err)
	}
	backend, err := bootstrap.Open(context.Background(), cfg.Database)
	if err != nil {
		return cfg, nil, fail("open database (%s): %v", bootstrap.Describe(cfg.Database), err)
	}
	if backend.DB == nil {
		_ = backend.Close()
		return cfg, nil, fail("migrations require a SQL database (current driver: %s)", backend.Driver)
	}
	return cfg, backend, exitOK
}
