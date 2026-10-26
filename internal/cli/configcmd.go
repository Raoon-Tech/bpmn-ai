package cli

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/Raoon-Tech/bpmn-ai/config"
)

func cmdConfig(args []string) int {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "show":
			return cmdConfigShow(args[1:])
		case "help", "-h", "--help":
			printConfigUsage()
			return exitOK
		}
	}
	return cmdConfigShow(args)
}

func printConfigUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai config show [--json]

Print the effective configuration. Passwords and API keys are masked.
`)
}

func cmdConfigShow(args []string) int {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print the configuration as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("config show", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}
	safe := sanitizeConfig(cfg)

	if *asJSON {
		if err := printJSON(safe); err != nil {
			return fail("render configuration: %v", err)
		}
		return exitOK
	}

	rows := [][]string{
		{"server.host", safe.Server.Host},
		{"server.port", fmt.Sprintf("%d", safe.Server.Port)},
		{"database.driver", safe.Database.Driver},
		{"database.target", describeDatabase(safe.Database)},
		{"database.user", safe.Database.User},
		{"database.password", maskSecret(cfg.Database.Password)},
		{"database.auto_migrate", fmt.Sprintf("%t", safe.Database.AutoMigrate)},
		{"database.allow_destructive", fmt.Sprintf("%t", safe.Database.AllowDestructive)},
		{"database.pool_max", fmt.Sprintf("%d", safe.Database.MaxConns)},
		{"database.pool_min", fmt.Sprintf("%d", safe.Database.MinConns)},
		{"database.log_level", safe.Database.LogLevel},
		{"engine.workers", fmt.Sprintf("%d", safe.Engine.WorkerCount)},
		{"engine.max_loops", fmt.Sprintf("%d", safe.Engine.MaxLoops)},
		{"engine.execution_timeout", safe.Engine.ExecutionTimeout.String()},
		{"ai.provider", safe.AI.Provider},
		{"ai.default_model", safe.AI.DefaultModel},
		{"ai.api_key", maskSecret(cfg.AI.APIKey)},
		{"ai.openrouter_api_key", maskSecret(os.Getenv("AI_OPENROUTER_API_KEY"))},
		{"ai.anthropic_api_key", maskSecret(os.Getenv("AI_ANTHROPIC_API_KEY"))},
		{"ai.cache.type", safe.AI.Cache.Type},
		{"log.level", safe.Log.Level},
		{"log.format", safe.Log.Format},
		{"audit.enabled", fmt.Sprintf("%t", safe.Audit.Enabled)},
		{"audit.dir", safe.Audit.Dir},
	}
	printTable([]string{"KEY", "VALUE"}, rows)
	return exitOK
}

// sanitizeConfig returns a copy without secrets.
func sanitizeConfig(cfg config.Config) config.Config {
	safe := cfg
	safe.Database.Password = maskSecret(cfg.Database.Password)
	safe.Database.URL = maskURL(cfg.Database.URL)
	safe.AI.APIKey = maskSecret(cfg.AI.APIKey)
	safe.AI.Cache.RedisURL = maskURL(cfg.AI.Cache.RedisURL)
	return safe
}

func describeDatabase(cfg config.DatabaseConfig) string {
	switch cfg.Driver {
	case config.DBDriverMemory:
		return "memory"
	case config.DBDriverSQLite:
		if cfg.URL != "" {
			return maskURL(cfg.URL)
		}
		return "sqlite:" + cfg.File
	}
	if cfg.URL != "" {
		return maskURL(cfg.URL)
	}
	return fmt.Sprintf("%s://%s:%d/%s", cfg.Driver, cfg.Host, cfg.Port, cfg.Name)
}

// maskURL hides the password of a connection URL.
func maskURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword(u.User.Username(), "****")
	}
	return u.String()
}
