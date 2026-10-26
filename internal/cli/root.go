// Package cli implements the bpmn-ai command line interface.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Raoon-Tech/bpmn-ai/config"

	// Register the versioned migrations.
	_ "github.com/Raoon-Tech/bpmn-ai/pkg/store/migrations"
)

// Exit codes used by every command.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

const usageText = `bpmn-ai — BPMN 2.0 execution engine

USAGE
  bpmn-ai [command] [flags]

COMMANDS
  serve                  Start the HTTP engine (default when no command is given)
  migrate <subcommand>   Manage versioned database migrations
  db <subcommand>        Database operations (ping, stats, seed)
  config show            Print the effective configuration (secrets masked)
  doctor                 Check configuration, database and migrations
  version                Print version information
  help                   Show this help

Run "bpmn-ai <command> -h" for command specific flags.
`

func printUsage() {
	fmt.Fprint(os.Stdout, usageText)
}

// Run dispatches the command line arguments and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		return cmdServe(nil)
	}

	command := strings.ToLower(args[0])
	rest := args[1:]
	switch command {
	case "serve":
		return cmdServe(rest)
	case "migrate":
		return cmdMigrate(rest)
	case "db":
		return cmdDB(rest)
	case "config":
		return cmdConfig(rest)
	case "doctor":
		return cmdDoctor(rest)
	case "version":
		return cmdVersion(rest)
	case "help", "-h", "--help":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printUsage()
		return exitUsage
	}
}

// loadConfig loads and validates the effective configuration.
func loadConfig() (config.Config, error) {
	cfg := config.Default()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func fail(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	return exitError
}

func usageError(command, message string) int {
	fmt.Fprintf(os.Stderr, "error: %s\n\n", message)
	fmt.Fprintf(os.Stderr, "run \"bpmn-ai %s -h\" for usage\n", command)
	return exitUsage
}
