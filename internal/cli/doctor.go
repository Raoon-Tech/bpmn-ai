package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
)

type checkResult struct {
	name     string
	ok       bool
	warning  bool
	critical bool
	detail   string
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("doctor", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	results := runChecks()
	failed := false
	for _, r := range results {
		label := "OK  "
		if !r.ok {
			if r.warning {
				label = "WARN"
			} else {
				label = "FAIL"
			}
		}
		if !r.ok && r.critical {
			failed = true
		}
		fmt.Printf("[%s] %s", label, r.name)
		if r.detail != "" {
			fmt.Printf(" — %s", r.detail)
		}
		fmt.Println()
	}
	if failed {
		return exitError
	}
	return exitOK
}

func runChecks() []checkResult {
	var results []checkResult

	cfg, err := loadConfig()
	if err != nil {
		return append(results, checkResult{name: "configuration", ok: false, critical: true, detail: err.Error()})
	}
	results = append(results, checkResult{name: "configuration", ok: true, detail: fmt.Sprintf("driver=%s", cfg.Database.Driver)})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	backend, err := bootstrap.Open(ctx, cfg.Database)
	if err != nil {
		results = append(results, checkResult{
			name: "database", ok: false, critical: true,
			detail: fmt.Sprintf("%s: %v", bootstrap.Describe(cfg.Database), err),
		})
		results = append(results, auditDirCheck(cfg.Audit.Dir, cfg.Audit.Enabled))
		results = append(results, aiCheck(cfg.AI.APIKey))
		results = append(results, portCheck(cfg.Server.Host, cfg.Server.Port))
		return results
	}
	defer func() { _ = backend.Close() }()
	results = append(results, checkResult{
		name: "database", ok: true,
		detail: fmt.Sprintf("%s (%s)", bootstrap.Describe(cfg.Database), backend.Driver),
	})

	if backend.DB == nil {
		results = append(results, checkResult{name: "migrations", ok: true, warning: true, detail: "skipped with the memory store"})
	} else {
		statuses, err := migrate.ListStatus(ctx, backend.DB)
		if err != nil {
			results = append(results, checkResult{name: "migrations", ok: false, critical: true, detail: err.Error()})
		} else {
			var pending []string
			for _, st := range statuses {
				if !st.Applied {
					pending = append(pending, st.Version)
				}
			}
			switch {
			case len(pending) == 0:
				results = append(results, checkResult{name: "migrations", ok: true, detail: "schema up to date"})
			case cfg.Database.AutoMigrate:
				results = append(results, checkResult{
					name: "migrations", ok: true, warning: true,
					detail: fmt.Sprintf("%d pending (applied automatically on start): %s", len(pending), strings.Join(pending, ", ")),
				})
			default:
				results = append(results, checkResult{
					name: "migrations", ok: false, critical: true,
					detail: fmt.Sprintf("%d pending: %s (run \"bpmn-ai migrate up\")", len(pending), strings.Join(pending, ", ")),
				})
			}
		}
	}

	results = append(results, auditDirCheck(cfg.Audit.Dir, cfg.Audit.Enabled))
	results = append(results, aiCheck(cfg.AI.APIKey))
	results = append(results, portCheck(cfg.Server.Host, cfg.Server.Port))
	return results
}

func auditDirCheck(dir string, enabled bool) checkResult {
	if !enabled {
		return checkResult{name: "audit log", ok: true, warning: true, detail: "disabled"}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return checkResult{name: "audit log", ok: false, critical: true, detail: fmt.Sprintf("cannot create %s: %v", dir, err)}
	}
	file, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return checkResult{name: "audit log", ok: false, critical: true, detail: fmt.Sprintf("%s is not writable: %v", dir, err)}
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return checkResult{name: "audit log", ok: true, detail: dir}
}

func aiCheck(apiKey string) checkResult {
	configured := apiKey != "" ||
		os.Getenv("AI_OPENROUTER_API_KEY") != "" ||
		os.Getenv("AI_ANTHROPIC_API_KEY") != "" ||
		os.Getenv("AI_GROQ_API_KEY") != ""
	if !configured {
		return checkResult{name: "AI provider", ok: true, warning: true, detail: "not configured (AI tasks will not run)"}
	}
	return checkResult{name: "AI provider", ok: true, detail: "configured"}
}

func portCheck(host string, port int) checkResult {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return checkResult{name: "http port", ok: false, warning: true, detail: fmt.Sprintf("%s is not available: %v", address, err)}
	}
	_ = listener.Close()
	return checkResult{name: "http port", ok: true, detail: address}
}
