package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/orm"

	"gorm.io/gorm"
)

func cmdAI(args []string) int {
	if len(args) == 0 {
		printAIUsage()
		return exitUsage
	}
	switch args[0] {
	case "provider":
		return cmdAIProvider(args[1:])
	case "profile":
		return cmdAIProfile(args[1:])
	case "help", "-h", "--help":
		printAIUsage()
		return exitOK
	default:
		return usageError("ai", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func printAIUsage() {
	fmt.Fprint(os.Stdout, `usage: bpmn-ai ai provider <subcommand>

SUBCOMMANDS (provider)
  provider list                          List configured AI providers
  provider add <name> --type TYPE --key KEY [--base-url URL] [--model MODEL] [--default] [--key-stdin]
  provider remove <name>
  provider show <name> [--reveal]
  provider set-default <name>

SUBCOMMANDS (profile)
  profile list
  profile get <name>
  profile set <name> --model MODEL [--max-tokens N] [--priority quality|speed|cost]
  profile reset [--name NAME]

EXAMPLES
  bpmn-ai ai provider add openrouter --type openai --key sk-or-... --base-url https://openrouter.ai/api/v1 --default
  echo sk-ant-... | bpmn-ai ai provider add anthropic --type anthropic --key-stdin
  bpmn-ai ai profile set fast --model openrouter/openai/gpt-4o-mini --max-tokens 2048 --priority speed
  bpmn-ai ai profile list
`)
}

func cmdAIProvider(args []string) int {
	if len(args) == 0 {
		printAIUsage()
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdAIProviderList(args[1:])
	case "add":
		return cmdAIProviderAdd(args[1:])
	case "remove":
		return cmdAIProviderRemove(args[1:])
	case "show":
		return cmdAIProviderShow(args[1:])
	case "set-default":
		return cmdAIProviderSetDefault(args[1:])
	case "help", "-h", "--help":
		printAIUsage()
		return exitOK
	default:
		return usageError("ai provider", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func openProviderStore() (*orm.ProviderStore, int) {
	_, backend, code := openBackend()
	if backend == nil {
		return nil, code
	}
	if backend.DB == nil {
		_ = backend.Close()
		return nil, fail("ai provider management requires a SQL database (current driver: %s)", backend.Driver)
	}
	// Keep backend open for the caller's lifetime is not possible here; instead
	// return a store bound to the DB and let the caller close via backend.
	// We store the backend in a package-level holder for cleanup by the command.
	return orm.NewProviderStore(backend.DB), 0
}

// backendHolder keeps the last opened backend so commands can close it.
var lastBackend *bootstrap.Backend

func openProviderStoreWithBackend() (*orm.ProviderStore, *bootstrap.Backend, int) {
	_, backend, code := openBackend()
	if backend == nil {
		return nil, nil, code
	}
	if backend.DB == nil {
		_ = backend.Close()
		return nil, nil, fail("ai provider management requires a SQL database (current driver: %s)", backend.Driver)
	}
	return orm.NewProviderStore(backend.DB), backend, exitOK
}

func cmdAIProviderList(args []string) int {
	fs := flag.NewFlagSet("ai provider list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}

	ps, backend, code := openProviderStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	providers, err := ps.List(context.Background())
	if err != nil {
		return fail("list providers: %v", err)
	}
	if *asJSON {
		// mask keys
		for i := range providers {
			providers[i].APIKey = maskSecret(providers[i].APIKey)
		}
		return printJSONQuiet(providers)
	}
	rows := make([][]string, 0, len(providers))
	for _, p := range providers {
		def := ""
		if p.IsDefault {
			def = "*"
		}
		rows = append(rows, []string{p.Name + def, p.Type, p.BaseURL, maskSecret(p.APIKey), p.DefaultModel})
	}
	printTable([]string{"NAME", "TYPE", "BASE URL", "API KEY", "DEFAULT MODEL"}, rows)
	if len(providers) == 0 {
		fmt.Println("\nno providers configured (use 'bpmn-ai ai provider add')")
	}
	return exitOK
}

func cmdAIProviderAdd(args []string) int {
	fs := flag.NewFlagSet("ai provider add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	providerType := fs.String("type", "", "provider type: openai or anthropic (required)")
	key := fs.String("key", "", "API key")
	keyStdin := fs.Bool("key-stdin", false, "read the API key from stdin")
	baseURL := fs.String("base-url", "", "base URL (optional)")
	model := fs.String("model", "", "default model (optional)")
	isDefault := fs.Bool("default", false, "mark as the default provider")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("ai provider add", "expected a provider name")
	}
	name := fs.Arg(0)
	if *providerType == "" {
		return usageError("ai provider add", "--type is required")
	}
	if *providerType != "openai" && *providerType != "anthropic" {
		return usageError("ai provider add", "--type must be openai or anthropic")
	}

	apiKey := *key
	if *keyStdin {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return fail("read key from stdin: %v", err)
		}
		apiKey = strings.TrimSpace(line)
	}
	if apiKey == "" {
		return usageError("ai provider add", "--key or --key-stdin is required")
	}

	ps, backend, code := openProviderStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	cfg := orm.ProviderConfig{
		Name:         name,
		Type:         *providerType,
		BaseURL:      *baseURL,
		APIKey:       apiKey,
		DefaultModel: *model,
		IsDefault:    *isDefault,
	}
	if err := ps.Save(context.Background(), cfg); err != nil {
		return fail("save provider: %v", err)
	}
	fmt.Printf("provider %q saved (type=%s)\n", name, *providerType)
	if *isDefault {
		fmt.Println("marked as default")
	}
	fmt.Println("warning: the API key is stored in plaintext in the database; restrict DB access accordingly")
	return exitOK
}

func cmdAIProviderRemove(args []string) int {
	fs := flag.NewFlagSet("ai provider remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("ai provider remove", "expected a provider name")
	}

	ps, backend, code := openProviderStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if err := ps.Delete(context.Background(), fs.Arg(0)); err != nil {
		return fail("remove provider: %v", err)
	}
	fmt.Printf("provider %s removed\n", fs.Arg(0))
	return exitOK
}

func cmdAIProviderShow(args []string) int {
	fs := flag.NewFlagSet("ai provider show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	reveal := fs.Bool("reveal", false, "show the API key in clear text")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("ai provider show", "expected a provider name")
	}

	ps, backend, code := openProviderStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	cfg, err := ps.Get(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("provider not found: %v", err)
	}
	key := maskSecret(cfg.APIKey)
	if *reveal {
		key = cfg.APIKey
		fmt.Println("warning: the API key is printed in clear text")
	}
	fmt.Printf("name:          %s\n", cfg.Name)
	fmt.Printf("type:          %s\n", cfg.Type)
	fmt.Printf("base_url:      %s\n", cfg.BaseURL)
	fmt.Printf("api_key:       %s\n", key)
	fmt.Printf("default_model: %s\n", cfg.DefaultModel)
	fmt.Printf("is_default:    %t\n", cfg.IsDefault)
	fmt.Printf("updated_at:    %s\n", cfg.UpdatedAt.Format(time.RFC3339))
	return exitOK
}

func cmdAIProviderSetDefault(args []string) int {
	fs := flag.NewFlagSet("ai provider set-default", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("ai provider set-default", "expected a provider name")
	}

	ps, backend, code := openProviderStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	cfg, err := ps.Get(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("provider not found: %v", err)
	}
	// Clear the previous default.
	all, err := ps.List(context.Background())
	if err != nil {
		return fail("list providers: %v", err)
	}
	for _, p := range all {
		if p.IsDefault && p.Name != cfg.Name {
			p.IsDefault = false
			if err := ps.Save(context.Background(), p); err != nil {
				return fail("update provider: %v", err)
			}
		}
	}
	cfg.IsDefault = true
	if err := ps.Save(context.Background(), cfg); err != nil {
		return fail("save provider: %v", err)
	}
	fmt.Printf("provider %s marked as default\n", cfg.Name)
	return exitOK
}


func cmdAIProfile(args []string) int {
	if len(args) == 0 {
		printAIUsage()
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdAIProfileList(args[1:])
	case "get":
		return cmdAIProfileGet(args[1:])
	case "set":
		return cmdAIProfileSet(args[1:])
	case "reset":
		return cmdAIProfileReset(args[1:])
	case "help", "-h", "--help":
		printAIUsage()
		return exitOK
	default:
		return usageError("ai profile", fmt.Sprintf("unknown subcommand %q", args[0]))
	}
}

func cmdAIProfileList(args []string) int {
	fs := flag.NewFlagSet("ai profile list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}

	ps, backend, code := openProfileStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	profiles, err := ps.List(context.Background())
	if err != nil {
		return fail("list profiles: %v", err)
	}
	if *asJSON {
		return printJSONQuiet(profiles)
	}
	rows := make([][]string, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, []string{p.Name, p.Model, fmt.Sprintf("%d", p.MaxTokens), p.Priority, p.UpdatedAt.Format(time.RFC3339)})
	}
	printTable([]string{"NAME", "MODEL", "MAX TOKENS", "PRIORITY", "UPDATED AT"}, rows)
	if len(profiles) == 0 {
		fmt.Println("\nno profiles configured (using built-in defaults)")
	}
	return exitOK
}

func cmdAIProfileGet(args []string) int {
	fs := flag.NewFlagSet("ai profile get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("ai profile get", "expected a profile name")
	}

	ps, backend, code := openProfileStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	p, err := ps.Get(context.Background(), fs.Arg(0))
	if err != nil {
		return fail("profile not found: %v", err)
	}
	fmt.Printf("name:       %s\n", p.Name)
	fmt.Printf("model:      %s\n", p.Model)
	fmt.Printf("max_tokens: %d\n", p.MaxTokens)
	fmt.Printf("priority:   %s\n", p.Priority)
	fmt.Printf("updated_at: %s\n", p.UpdatedAt.Format(time.RFC3339))
	return exitOK
}

func cmdAIProfileSet(args []string) int {
	fs := flag.NewFlagSet("ai profile set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	model := fs.String("model", "", "model name (required)")
	maxTokens := fs.Int("max-tokens", 0, "max tokens (0 = keep default)")
	priority := fs.String("priority", "", "priority: quality, speed or cost")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		return usageError("ai profile set", "expected a profile name")
	}
	name := fs.Arg(0)
	if !orm.IsValidProfileName(name) {
		return usageError("ai profile set", fmt.Sprintf("unknown profile %q (expected: %s)", name, strings.Join(orm.ValidProfileNames(), ", ")))
	}
	if *model == "" {
		return usageError("ai profile set", "--model is required")
	}

	p := orm.ModelProfile{
		Name:      name,
		Model:     *model,
		MaxTokens: *maxTokens,
		Priority:  *priority,
	}
	if p.MaxTokens == 0 {
		p.MaxTokens = 4096
	}

	ps, backend, code := openProfileStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	if err := ps.Save(context.Background(), p); err != nil {
		return fail("save profile: %v", err)
	}
	fmt.Printf("profile %q set to model %s\n", name, *model)
	return exitOK
}

func cmdAIProfileReset(args []string) int {
	fs := flag.NewFlagSet("ai profile reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "reset a single profile (default: all)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if *name != "" && !orm.IsValidProfileName(*name) {
		return usageError("ai profile reset", fmt.Sprintf("unknown profile %q", *name))
	}

	ps, backend, code := openProfileStoreWithBackend()
	if ps == nil {
		return code
	}
	defer func() { _ = backend.Close() }()

	targets := orm.ValidProfileNames()
	if *name != "" {
		targets = []string{*name}
	}
	for _, n := range targets {
		if err := ps.Delete(context.Background(), n); err != nil && err != gorm.ErrRecordNotFound {
			return fail("reset profile %s: %v", n, err)
		}
	}
	fmt.Printf("reset %d profile(s) to built-in defaults\n", len(targets))
	return exitOK
}

func openProfileStoreWithBackend() (*orm.ProfileStore, *bootstrap.Backend, int) {
	_, backend, code := openBackend()
	if backend == nil {
		return nil, nil, code
	}
	if backend.DB == nil {
		_ = backend.Close()
		return nil, nil, fail("ai profile management requires a SQL database (current driver: %s)", backend.Driver)
	}
	return orm.NewProfileStore(backend.DB), backend, exitOK
}
