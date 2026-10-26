package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/api/http"
	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/activities"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/events"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/flows"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/gateways"
	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/queue"
	"github.com/Raoon-Tech/bpmn-ai/pkg/ai"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
)

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: bpmn-ai serve")
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("serve", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}

	logger, err := observability.NewFromConfig(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return fail("failed to create logger: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target := bootstrap.Describe(cfg.Database)
	backend, err := bootstrap.Open(ctx, cfg.Database)
	if err != nil {
		logger.Error("cannot open database", "driver", cfg.Database.Driver, "target", target, "error", err)
		return exitError
	}
	defer func() { _ = backend.Close() }()
	logger.Info("store ready", "driver", backend.Driver, "target", target)

	if backend.DB != nil {
		if err := ensureMigrations(ctx, cfg, backend, logger); err != nil {
			logger.Error("database migrations failed", "error", err)
			return exitError
		}
	}

	metrics := observability.DefaultMetrics()
	dispatcher := observability.NewDispatcher()
	auditWriter, err := observability.NewFileAuditWriter(cfg.Audit.Dir, cfg.Audit.Enabled, logger)
	if err != nil {
		logger.Error("failed to create audit writer", "error", err)
		return exitError
	}
	_ = observability.NewAuditor(dispatcher, auditWriter)

	st := backend.Store

	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(st, st).WithDispatcher(dispatcher)
	q := queue.NewWorkerPool(st, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  cfg.Engine.WorkerCount,
		PollInterval: cfg.Engine.QueuePollInterval,
	}).WithDispatcher(dispatcher)

	registry := engine.NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeTerminateEvent, events.NewTerminateEvent)
	registry.Register(bpmn.ElementTypeTimerEvent, events.NewTimerEvent)
	registry.Register(bpmn.ElementTypeMessageThrow, events.NewMessageThrowEvent)
	registry.Register(bpmn.ElementTypeMessageCatch, events.NewMessageCatchEvent)
	registry.Register(bpmn.ElementTypeExclusiveGateway, gateways.NewExclusiveGateway)
	registry.Register(bpmn.ElementTypeParallelGateway, gateways.NewParallelGateway)
	registry.Register(bpmn.ElementTypeInclusiveGateway, gateways.NewInclusiveGateway)
	registry.Register(bpmn.ElementTypeEventBasedGateway, gateways.NewEventBasedGateway)
	registry.Register(bpmn.ElementTypeUserTask, activities.NewUserTask)
	registry.Register(bpmn.ElementTypeScriptTask, activities.NewScriptTask)
	registry.Register(bpmn.ElementTypeServiceTask, activities.NewServiceTask)
	registry.Register(bpmn.ElementTypeSequenceFlow, flows.NewSequenceFlow)
	registry.Register(bpmn.ElementTypeSubProcess, activities.NewSubProcess)
	registry.Register(bpmn.ElementTypeErrorCatch, events.NewErrorCatchEvent)
	registry.Register(bpmn.ElementTypeErrorEnd, events.NewErrorEndEvent)
	registry.Register(bpmn.ElementTypeCallActivity, activities.NewCallActivity)
	registry.Register(bpmn.ElementTypeSignalThrow, events.NewSignalThrowEvent)
	registry.Register(bpmn.ElementTypeSignalCatch, events.NewSignalCatchEvent)

	aiRegistry := ai.NewProviderRegistry()
	aiRegistry.Register("openai", ai.NewOpenAIProvider)
	aiRegistry.Register("anthropic", ai.NewAnthropicProvider)

	providerPool := ai.NewProviderPool()
	toolRegistry := ai.NewToolRegistry()
	promptManager := ai.NewPromptManager()

	var aiGateway ai.Gateway
	var modelRouter *ai.ModelRouter
	var ragSystem *ai.RAGSystem

	// OpenRouter takes precedence — single API key for 200+ models
	openRouterKey := os.Getenv("AI_OPENROUTER_API_KEY")
	if openRouterKey != "" {
		orGW, err := aiRegistry.Create("openai", openRouterKey, "https://openrouter.ai/api/v1")
		if err != nil {
			logger.Error("failed to create OpenRouter provider", "error", err)
		} else {
			providerPool.Add("openrouter", orGW)

			orFallback, err := aiRegistry.Create("openai", openRouterKey, "https://openrouter.ai/api/v1")
			if err != nil {
				aiGateway = orGW
			} else {
				providerPool.Add("openrouter-fallback", orFallback)
				aiGateway = ai.NewFallbackGateway(orGW, orFallback)
			}

			logger.Info("OpenRouter configured as primary AI provider (200+ models)")
		}
	}

	// If no OpenRouter, fall back to classic provider configuration
	if openRouterKey == "" && cfg.AI.APIKey != "" {
		primary, err := aiRegistry.Create(cfg.AI.Provider, cfg.AI.APIKey, cfg.AI.BaseURL)
		if err != nil {
			logger.Error("failed to create primary AI provider", "error", err)
		} else {
			providerPool.Add("primary", primary)
			providerPool.Add(cfg.AI.Provider, primary)

			secondary, err := aiRegistry.Create(cfg.AI.Provider, cfg.AI.APIKey, cfg.AI.BaseURL)
			if err != nil {
				logger.Error("failed to create fallback AI provider", "error", err)
				aiGateway = primary
			} else {
				providerPool.Add("fallback", secondary)
				aiGateway = ai.NewFallbackGateway(primary, secondary)
			}
		}
	}

	// Add extra providers alongside OpenRouter or classic config
	if anthropicKey := os.Getenv("AI_ANTHROPIC_API_KEY"); anthropicKey != "" {
		if anthropicGW, err := aiRegistry.Create("anthropic", anthropicKey, ""); err == nil {
			providerPool.Add("anthropic", anthropicGW)
			logger.Info("Anthropic provider added to model router")
		}
	}
	if groqKey := os.Getenv("AI_GROQ_API_KEY"); groqKey != "" {
		if groqGW, err := aiRegistry.Create("openai", groqKey, "https://api.groq.com/openai/v1"); err == nil {
			providerPool.Add("groq", groqGW)
			logger.Info("Groq provider added to model router")
		}
	}
	if extra := os.Getenv("AI_EXTRA_PROVIDERS"); extra != "" {
		for _, entry := range strings.Split(extra, ",") {
			parts := strings.Split(strings.TrimSpace(entry), ":")
			if len(parts) >= 3 {
				if extraGW, err := aiRegistry.Create("openai", parts[1], parts[2]); err == nil {
					providerPool.Add(parts[0], extraGW)
					logger.Info("Extra provider added", "name", parts[0])
				}
			}
		}
	}

	if aiGateway != nil {
		modelRouter = ai.NewModelRouter(providerPool)

		// Use OpenRouter-prefixed profiles when OpenRouter is primary
		profileModel := cfg.AI.DefaultModel
		if openRouterKey != "" {
			profileModel = "openrouter/openai/gpt-4o"
			modelRouter.SetProfiles(map[string]ai.Profile{
				"complex": {Model: "openrouter/openai/gpt-4o", MaxTokens: 8192, Priority: "quality"},
				"fast":    {Model: "openrouter/openai/gpt-4o-mini", MaxTokens: 2048, Priority: "speed"},
				"cheap":   {Model: "openrouter/openai/gpt-4o-mini", MaxTokens: 1024, Priority: "cost"},
				"auto":    {Model: "openrouter/openai/gpt-4o", MaxTokens: 4096, Priority: "quality"},
			})
		}
		modelRouter.AddProfile("auto", ai.Profile{Model: profileModel, MaxTokens: cfg.AI.MaxTokens, Priority: "quality"})

		embedderKey := cfg.AI.APIKey
		embedderBase := cfg.AI.BaseURL
		if openRouterKey != "" {
			embedderKey = openRouterKey
			embedderBase = "https://openrouter.ai/api/v1"
		}
		embedder, err := ai.NewOpenAIEmbedder(embedderKey, embedderBase)
		if err == nil {
			ragSystem = ai.NewRAGSystem(embedder)
			ragSystem.AddCollection("default", ai.NewInMemoryVectorStore())
			logger.Info("RAG system initialized with default collection")
		} else {
			logger.Info("failed to create embedder, RAG disabled", "error", err)
		}

		registry.Register(bpmn.ElementTypeAITask, activities.NewAITaskConstructor(aiGateway, toolRegistry, modelRouter, ragSystem, promptManager))
		logger.Info("AI task element registered with model router",
			"providers", providerPool.List(),
			"default_model", cfg.AI.DefaultModel,
			"default_profile", cfg.AI.DefaultProfile,
		)
	} else {
		logger.Info("AI not configured — set AI_API_KEY or AI_OPENROUTER_API_KEY")
	}

	eng := engine.New(engine.Config{
		WorkerCount:      cfg.Engine.WorkerCount,
		MaxLoops:         cfg.Engine.MaxLoops,
		ExecutionTimeout: cfg.Engine.ExecutionTimeout,
	}, registry, st, logger, q)
	eng.WithDispatcher(dispatcher)

	q.WithHandler(eng.JobHandler())
	q.Start(ctx)

	srv := http.NewServer(http.ServerConfig{
		Host:         cfg.Server.Host,
		Port:         cfg.Server.Port,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
		TLSCertFile:  cfg.Server.TLSCertFile,
		TLSKeyFile:   cfg.Server.TLSKeyFile,
	}, st, eng, q, logger, metrics)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("server error", "error", err)
		}
	}()

	<-stop
	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	q.Stop()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		return exitError
	}

	if err := auditWriter.Close(); err != nil {
		logger.Error("audit writer close error", "error", err)
	}

	logger.Info("server stopped")
	return exitOK
}

// ensureMigrations applies pending migrations when DB_AUTO_MIGRATE is enabled
// and refuses to start otherwise if the schema is behind.
func ensureMigrations(ctx context.Context, cfg config.Config, backend *bootstrap.Backend, logger *observability.Logger) error {
	if cfg.Database.AutoMigrate {
		applied, err := migrate.Up(ctx, backend.DB, migrate.Options{})
		if err != nil {
			return err
		}
		if len(applied) > 0 {
			logger.Info("migrations applied", "versions", applied)
		}
		return nil
	}

	pending, err := migrate.Pending(ctx, backend.DB, migrate.Options{})
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%d pending migration(s): %s (run \"bpmn-ai migrate up\" or set DB_AUTO_MIGRATE=true)",
			len(pending), strings.Join(pending, ", "))
	}
	return nil
}
