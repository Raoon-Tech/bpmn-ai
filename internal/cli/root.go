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

// reorderArgs moves flags (and their values) before positional arguments so
// that callers can write "cmd <id> --flag value" as well as
// "cmd --flag value <id>". Go's flag package stops parsing at the first
// positional argument.
func reorderArgs(args []string) []string {
	var flags, positionals []string
	i := 0
	for i < len(args) {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// If the flag has no "=", the next token is its value.
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i += 2
				continue
			}
			i++
			continue
		}
		positionals = append(positionals, a)
		i++
	}
	return append(flags, positionals...)
}

const usageText = `bpmn-ai — BPMN 2.0 execution engine

USAGE
  bpmn-ai [command] [flags]

COMMANDS
  serve                  Start the HTTP engine (default when no command is given)
  migrate <subcommand>   Manage versioned database migrations
  db <subcommand>        Database operations (ping, stats, seed)
  process <subcommand>   Manage process definitions
  case <subcommand>      Manage process instances (cases)
  task <subcommand>      Manage user tasks
  job <subcommand>       Manage the job queue
  message <subcommand>   Send a message to a waiting instance
  signal <subcommand>    Broadcast a signal
  ai <subcommand>        Manage AI providers (multi-provider)
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
	case "process":
		return cmdProcess(rest)
	case "case":
		return cmdCase(rest)
	case "task":
		return cmdTask(rest)
	case "job":
		return cmdJob(rest)
	case "message":
		return cmdMessage(rest)
	case "signal":
		return cmdSignal(rest)
	case "ai":
		return cmdAI(rest)
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
