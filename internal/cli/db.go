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
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"
)

func cmdDB(args []string) int {
	if len(args) == 0 {
		printDBUsage()
		return exitUsage
	}
	switch strings.ToLower(args[0]) {
	case "ping":
		return cmdDBPing(args[1:])
	case "stats":
		return cmdDBStats(args[1:])
	case "seed":
		return cmdDBSeed(args[1:])
	case "help", "-h", "--help":
		printDBUsage()
		return exitOK
	default:
		return usageError("db", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printDBUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai db <subcommand>

SUBCOMMANDS
  ping                   Check the database connection
  stats                  Show row counts per table
  seed <file.bpmn>       Deploy a BPMN file into the database

EXAMPLES
  bpmn-ai db ping
  bpmn-ai db seed examples/approval-process.bpmn
`)
}

func cmdDBPing(args []string) int {
	fs := flag.NewFlagSet("db ping", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("db ping", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if backend.DB == nil {
		fmt.Println("ok: memory store (no external database configured)")
		return exitOK
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sqlDB, err := backend.DB.DB()
	if err != nil {
		return fail("access sql pool: %v", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fail("ping %s: %v", bootstrap.Describe(cfg.Database), err)
	}

	version := "unknown"
	switch backend.Driver {
	case "postgres":
		backend.DB.WithContext(ctx).Raw("SELECT version()").Scan(&version)
	case "mysql":
		backend.DB.WithContext(ctx).Raw("SELECT VERSION()").Scan(&version)
	case "sqlite":
		backend.DB.WithContext(ctx).Raw("SELECT sqlite_version()").Scan(&version)
	}
	fmt.Printf("ok: %s (%s) — %s\n", bootstrap.Describe(cfg.Database), backend.Driver, firstLine(version))
	return exitOK
}

func cmdDBStats(args []string) int {
	fs := flag.NewFlagSet("db stats", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("db stats", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()
	if backend.DB == nil {
		return fail("db stats requires a SQL database (current driver: %s)", backend.Driver)
	}

	tables := []string{"processes", "instances", "flows", "threads", "jobs", "dead_letters", "execution_log", "ai_audit_log", "schema_migrations"}
	rows := make([][]string, 0, len(tables))
	for _, table := range tables {
		if !backend.DB.Migrator().HasTable(table) {
			rows = append(rows, []string{table, "-", "missing"})
			continue
		}
		var count int64
		if err := backend.DB.Table(table).Count(&count).Error; err != nil {
			rows = append(rows, []string{table, "-", err.Error()})
			continue
		}
		rows = append(rows, []string{table, fmt.Sprintf("%d", count), "ok"})
	}

	fmt.Printf("database: %s (%s)\n\n", bootstrap.Describe(cfg.Database), backend.Driver)
	printTable([]string{"TABLE", "ROWS", "STATE"}, rows)
	return exitOK
}

func cmdDBSeed(args []string) int {
	fs := flag.NewFlagSet("db seed", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("db seed", "expected the path to a .bpmn file")
	}

	path := fs.Arg(0)
	proc, err := bpmn.NewParser().ParseFile(path)
	if err != nil {
		return fail("parse %s: %v", path, err)
	}

	cfg, backend, code := openBackend()
	if backend == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if err := backend.Store.SaveProcess(context.Background(), proc); err != nil {
		return fail("save process: %v", err)
	}
	fmt.Printf("process %q (%s) stored in %s\n", proc.Name, proc.ID, bootstrap.Describe(cfg.Database))
	if backend.DB == nil {
		fmt.Println("warning: the memory store is not persistent; data is lost on exit")
	}
	return exitOK
}

// openBackend loads the configuration and opens the configured backend.
func openBackend() (config.Config, *bootstrap.Backend, int) {
	cfg, err := loadConfig()
	if err != nil {
		return cfg, nil, fail("invalid configuration: %v", err)
	}
	backend, err := bootstrap.Open(context.Background(), cfg.Database)
	if err != nil {
		return cfg, nil, fail("open database (%s): %v", bootstrap.Describe(cfg.Database), err)
	}
	return cfg, backend, exitOK
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// modelsForStats keeps orm imported for future table discovery.
var _ = orm.Models
