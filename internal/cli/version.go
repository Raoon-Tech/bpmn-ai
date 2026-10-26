package cli

import (
	"flag"
	"fmt"
	"os"
)

// Build information, overridable with -ldflags "-X ...".
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func cmdVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print version information as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("version", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	if *asJSON {
		if err := printJSON(map[string]string{
			"version": Version,
			"commit":  Commit,
			"date":    BuildDate,
		}); err != nil {
			return fail("render version: %v", err)
		}
		return exitOK
	}

	fmt.Printf("bpmn-ai %s (commit %s, built %s)\n", Version, Commit, BuildDate)
	return exitOK
}
