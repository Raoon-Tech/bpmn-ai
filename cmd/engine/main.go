package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Raoon-Tech/bpmn-ai/api/http"
	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/activities"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/events"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/flows"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/gateways"
	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/queue"
	"github.com/Raoon-Tech/bpmn-ai/pkg/ai"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
	pgstore "github.com/Raoon-Tech/bpmn-ai/pkg/store/sql"
)

func main() {
	// Load .env file if it exists (ignores error if missing)
	_ = godotenv.Load()

	cfg := config.Default()

	logger, err := observability.NewFromConfig(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}

	metrics := observability.DefaultMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := getStore(ctx, &cfg, logger)
	if store == nil {
		logger.Error("no store available")
		os.Exit(1)
	}

	dispatcher := observability.NewDispatcher()
	auditWriter, err := observability.NewFileAuditWriter(cfg.Audit.Dir, cfg.Audit.Enabled, logger)
	if err != nil {
		logger.Error("failed to create audit writer", "error", err)
		os.Exit(1)
	}
	_ = observability.NewAuditor(dispatcher, auditWriter)

	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(store, store).WithDispatcher(dispatcher)
	q := queue.NewWorkerPool(store, nil, retry, dlq, queue.WorkerPoolConfig{
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
	}, registry, store, logger, q)
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
	}, store, eng, q, logger, metrics)

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
		os.Exit(1)
	}

	if err := auditWriter.Close(); err != nil {
		logger.Error("audit writer close error", "error", err)
	}

	logger.Info("server stopped")
}

func getStore(ctx context.Context, cfg *config.Config, logger *observability.Logger) store.Store {
	if cfg.Database.URL != "" {
		s, err := pgstore.NewStore(ctx, cfg.Database.URL)
		if err == nil {
			logger.Info("using PostgreSQL store", "database_url", cfg.Database.URL)
			return s
		}
		logger.Info("failed to connect to PostgreSQL, falling back to in-memory", "error", err)
	}
	logger.Info("using in-memory store")
	return memory.NewStore()
}
